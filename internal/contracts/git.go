package contracts

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/petarnenov/bot-space/internal/security"
)

var ErrArtifacts = errors.New("contract Git artifacts could not be verified")

// VerifyGit reads only committed ordinary files. Dirty worktree contents and
// symlink targets cannot masquerade as the approved artifact revision.
func (c Content) VerifyGit(ctx context.Context, checkout string) error {
	canonical, _, err := c.Canonical()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(checkout) {
		return ErrInvalid
	}
	info, err := os.Stat(checkout)
	if err != nil || !info.IsDir() {
		return ErrArtifacts
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", checkout, "--no-pager"}, args...)...)
		// Caller environment must not redirect Git to another repository/object DB.
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_NO_REPLACE_OBJECTS=1"}
		out, err := cmd.Output()
		if err != nil {
			return nil, ErrArtifacts
		}
		return out, nil
	}
	resolved, err := run("rev-parse", "--verify", canonical.BaseCommit+"^{commit}")
	if err != nil || strings.TrimSpace(string(resolved)) != canonical.BaseCommit {
		return ErrArtifacts
	}
	for name, digest := range canonical.Artifacts {
		entry, err := run("ls-tree", canonical.BaseCommit, "--", name)
		if err != nil {
			return err
		}
		fields := strings.Fields(string(entry))
		if len(fields) != 4 || fields[0] != "100644" && fields[0] != "100755" || fields[1] != "blob" || fields[3] != name {
			return ErrArtifacts
		}
		size, err := run("cat-file", "-s", fields[2])
		if err != nil {
			return err
		}
		// Avoid buffering an unbounded object. The manifest itself is also bounded.
		n, err := parseBlobSize(string(size))
		if err != nil || n > 256<<10 {
			return ErrArtifacts
		}
		body, err := run("cat-file", "blob", fields[2])
		if err != nil || len(body) != n || security.Hash(string(body)) != digest {
			return ErrArtifacts
		}
	}
	return nil
}
func parseBlobSize(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 8 {
		return 0, ErrArtifacts
	}
	size := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, ErrArtifacts
		}
		size = size*10 + int(r-'0')
	}
	return size, nil
}
