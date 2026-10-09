package runner

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestProviderModelAndEffortRouting(t *testing.T) {
	for _, provider := range []string{"codex", "claude", "copilot"} {
		t.Run(provider, func(t *testing.T) {
			request := ProviderRequest{Agent: AgentConfig{Provider: provider, Executable: provider, Model: "requested-model", Effort: "high", Policy: "analysis", Project: t.TempDir()}, JobID: "11111111-1111-4111-8111-111111111111", StateDir: filepath.Join(t.TempDir(), "state"), Instruction: "test", OnSession: func(string) error { return nil }}
			cmd, err := providerCommand(request)
			if err != nil {
				t.Fatal(err)
			}
			modelFlag, effortFlag, effortValue := "--model", "--effort", "high"
			if provider == "codex" {
				modelFlag = "-m"
				effortFlag = "-c"
				effortValue = `model_reasoning_effort="high"`
			}
			if provider == "copilot" {
				effortFlag = "--reasoning-effort"
			}
			containsPair := func(flag, value string) bool {
				for i := 0; i+1 < len(cmd.Args); i++ {
					if cmd.Args[i] == flag && cmd.Args[i+1] == value {
						return true
					}
				}
				return false
			}
			if !containsPair(modelFlag, "requested-model") || !containsPair(effortFlag, effortValue) {
				t.Fatalf("settings lost: %v", cmd.Args)
			}
			request.Agent.Effort = ""
			cmd, err = providerCommand(request)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(cmd.Args, effortValue) {
				t.Fatal("omitted effort forced an override")
			}
		})
	}
}

func TestProviderEffortRejectsUnsupportedValues(t *testing.T) {
	for _, provider := range []string{"codex", "claude", "copilot"} {
		for _, effort := range []string{"", "bogus", "high|low", " HIGH", "high\n"} {
			if validEffort(provider, effort) {
				t.Fatalf("accepted invalid %s effort %q", provider, effort)
			}
		}
		if !validEffort(provider, "high") {
			t.Fatalf("rejected supported %s effort", provider)
		}
	}
	if validEffort("claude", "none") || validEffort("copilot", "ultra") {
		t.Fatal("provider-specific unsupported effort accepted")
	}
}
