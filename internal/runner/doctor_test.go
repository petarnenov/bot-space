package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeProviderProbe(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider-probe.sh")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProviderChecksClaudeAuthentication(t *testing.T) {
	exe := writeProviderProbe(t, `if [ "$1" = "--version" ]; then echo "claude 2.1.292"; exit 0; fi
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then echo "Authenticated"; exit 0; fi
exit 1`)
	checks, err := ProviderChecks(context.Background(), Config{
		Agents: []AgentConfig{{
			Provider:   "claude",
			Executable: exe,
			Project:    t.TempDir(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Authentication != "authenticated" {
		t.Fatalf("unexpected checks: %#v", checks)
	}
}

func TestProviderChecksClaudeAuthenticationRequired(t *testing.T) {
	exe := writeProviderProbe(t, `if [ "$1" = "--version" ]; then echo "claude 2.1.292"; exit 0; fi
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then echo "Not signed in"; exit 0; fi
exit 1`)
	_, err := ProviderChecks(context.Background(), Config{
		Agents: []AgentConfig{{
			Provider:   "claude",
			Executable: exe,
			Project:    t.TempDir(),
		}},
	})
	if err == nil || err.Error() != "Claude requires machine-local authentication" {
		t.Fatalf("expected Claude auth error, got %v", err)
	}
}
