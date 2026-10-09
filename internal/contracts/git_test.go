package contracts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petarnenov/bot-space/internal/security"
)

func TestCommittedArtifactVerificationIgnoresDirtyFilesAndRejectsMismatches(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal("Git fixture failed")
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.invalid")
	c := fixture()
	for name := range c.Artifacts {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0700)
		body := []byte("committed OpenSpec input\n")
		if err := os.WriteFile(p, body, 0600); err != nil {
			t.Fatal(err)
		}
		c.Artifacts[name] = security.Hash(string(body))
	}
	git("add", ".")
	git("commit", "-qm", "Add specification fixture")
	c.BaseCommit = git("rev-parse", "HEAD")
	if err := c.VerifyGit(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "openspec/changes/feature/design.md"), []byte("dirty changes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyGit(context.Background(), root); err != nil {
		t.Fatal("dirty checkout replaced committed evidence", err)
	}
	bad := c
	bad.BaseCommit = strings.Repeat("c", 40)
	if bad.VerifyGit(context.Background(), root) != ErrArtifacts {
		t.Fatal("missing commit verified")
	}
	c.Artifacts["openspec/changes/feature/design.md"] = strings.Repeat("a", 64)
	if c.VerifyGit(context.Background(), root) != ErrArtifacts {
		t.Fatal("false digest verified")
	}
}
