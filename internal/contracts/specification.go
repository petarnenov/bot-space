package contracts

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// VerifySpecification validates a private snapshot of the bound standard
// spec-driven artifacts. It never executes code/hooks from the checkout.
func (c Content) VerifySpecification(ctx context.Context, checkout, openspec string) error {
	canonical, _, err := c.Canonical()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(openspec) {
		return ErrInvalid
	}
	if err = canonical.VerifyGit(ctx, checkout); err != nil {
		return err
	}
	snapshot, err := os.MkdirTemp("", "the-firm-spec-")
	if err != nil {
		return ErrArtifacts
	}
	defer os.RemoveAll(snapshot)
	if err = os.MkdirAll(filepath.Join(snapshot, "openspec"), 0700); err != nil {
		return ErrArtifacts
	}
	if err = os.WriteFile(filepath.Join(snapshot, "openspec/config.yaml"), []byte("schema: spec-driven\n"), 0600); err != nil {
		return ErrArtifacts
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + snapshot, "OPENSPEC_TELEMETRY=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_NO_REPLACE_OBJECTS=1"}
	for name := range canonical.Artifacts {
		cmd := exec.CommandContext(ctx, "git", "-C", checkout, "--no-pager", "show", canonical.BaseCommit+":"+name)
		cmd.Env = env
		body, err := cmd.Output()
		if err != nil || len(body) > 256<<10 {
			return ErrArtifacts
		}
		target := filepath.Join(snapshot, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return ErrArtifacts
		}
		if err = os.WriteFile(target, body, 0600); err != nil {
			return ErrArtifacts
		}
	}
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, openspec, args...)
		cmd.Dir = snapshot
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil || len(out) > 4<<20 {
			return nil, ErrArtifacts
		}
		return out, nil
	}
	if _, err = run("validate", canonical.Change, "--strict", "--json", "--no-interactive"); err != nil {
		return err
	}
	tasks, err := run("instructions", "apply", "--change", canonical.Change, "--json")
	if err != nil {
		return err
	}
	var taskInfo struct {
		State string `json:"state"`
		Tasks []struct {
			Description string `json:"description"`
		} `json:"tasks"`
	}
	if json.Unmarshal(tasks, &taskInfo) != nil || taskInfo.State == "blocked" {
		return ErrArtifacts
	}
	available := map[string]bool{}
	for _, task := range taskInfo.Tasks {
		parts := strings.Fields(task.Description)
		if len(parts) > 0 {
			available[parts[0]] = true
		}
	}
	for _, task := range canonical.Tasks {
		if !available[task] {
			return ErrArtifacts
		}
	}
	raw, err := run("show", canonical.Change, "--json", "--deltas-only")
	if err != nil {
		return err
	}
	var specs struct {
		Deltas []struct {
			Spec        string `json:"spec"`
			Requirement struct {
				Name      string `json:"name"`
				Scenarios []struct {
					Name string `json:"name"`
				} `json:"scenarios"`
			} `json:"requirement"`
		} `json:"deltas"`
	}
	if json.Unmarshal(raw, &specs) != nil {
		return ErrArtifacts
	}
	scenarioRefs := map[string]int{}
	for _, delta := range specs.Deltas {
		for _, scenario := range delta.Requirement.Scenarios {
			scenarioRefs[scenario.Name]++
			scenarioRefs[delta.Spec+"::"+delta.Requirement.Name+"::"+scenario.Name]++
		}
	}
	for _, ref := range canonical.Scenarios {
		if scenarioRefs[ref] != 1 {
			return ErrArtifacts
		}
	}
	return nil
}
