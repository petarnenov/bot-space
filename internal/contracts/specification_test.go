package contracts

import (
	"context"
	"github.com/petarnenov/bot-space/internal/security"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestActualOpenSpecSemanticValidationFromCommittedSnapshot(t *testing.T) {
	cli, err := exec.LookPath("openspec")
	if err != nil {
		t.Skip("OpenSpec CLI unavailable")
	}
	cli, err = filepath.Abs(cli)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatal("Git fixture failed")
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.invalid")
	artifacts := map[string]string{
		"proposal.md":        "# Feature\n\n## Why\nProvide a verified feature.\n\n## What Changes\n- Add feature.\n\n## Capabilities\n\n### New Capabilities\n- work: Feature behavior.\n\n### Modified Capabilities\n- None.\n\n## Impact\nFeature implementation.\n",
		"design.md":          "# Design\n\nImplement the specified feature.\n",
		"tasks.md":           "# Tasks\n\n## 1. Feature\n\n- [ ] 1.1 Implement feature and verify scenario.\n",
		"specs/work/spec.md": "# Work\n\n## ADDED Requirements\n\n### Requirement: Verified feature\nThe implementation SHALL return verified results.\n\n#### Scenario: Success\n- **GIVEN** valid input\n- **WHEN** feature runs\n- **THEN** verified output is returned.\n",
	}
	c := fixture()
	c.Tasks = []string{"1.1"}
	c.Scenarios = []string{"work::Verified feature::Success"}
	c.Artifacts = map[string]string{}
	for name, body := range artifacts {
		relative := "openspec/changes/feature/" + name
		p := filepath.Join(root, filepath.FromSlash(relative))
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte(body), 0600)
		c.Artifacts[relative] = security.Hash(body)
	}
	git("add", ".")
	git("commit", "-qm", "Add valid OpenSpec fixture")
	c.BaseCommit = git("rev-parse", "HEAD")
	if err = c.VerifySpecification(context.Background(), root, cli); err != nil {
		t.Fatal("actual CLI rejected snapshot", err)
	}
	c.Tasks = []string{"99.9"}
	if c.VerifySpecification(context.Background(), root, cli) != ErrArtifacts {
		t.Fatal("missing task reference accepted")
	}
	c.Tasks = []string{"1.1"}
	c.Scenarios = []string{"missing"}
	if c.VerifySpecification(context.Background(), root, cli) != ErrArtifacts {
		t.Fatal("missing scenario reference accepted")
	}
	c.Scenarios = []string{"work::Verified feature::Success"}
	invalid := strings.Replace(artifacts["specs/work/spec.md"], "SHALL", "SHOULD", 1)
	name := "openspec/changes/feature/specs/work/spec.md"
	if err = os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(invalid), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "Introduce invalid requirement")
	c.BaseCommit = git("rev-parse", "HEAD")
	c.Artifacts[name] = security.Hash(invalid)
	if c.VerifySpecification(context.Background(), root, cli) != ErrArtifacts {
		t.Fatal("semantic validation bypassed invalid requirement")
	}

}
