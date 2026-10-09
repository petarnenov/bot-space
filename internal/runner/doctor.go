package runner

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type ProviderCheck struct {
	Provider       string `json:"provider"`
	Version        string `json:"version"`
	Authentication string `json:"authentication"`
}

var versionPattern = regexp.MustCompile("[0-9]+\\.[0-9]+\\.[0-9]+")

func ProviderChecks(ctx context.Context, config Config) ([]ProviderCheck, error) {
	var out []ProviderCheck
	for _, a := range config.Agents {
		child, cancel := context.WithTimeout(ctx, 5*time.Second)
		cmd := exec.CommandContext(child, a.Executable, "--version")
		cmd.Dir = a.Project
		data, err := cmd.Output()
		cancel()
		version := versionPattern.FindString(string(data))
		if err != nil || len(data) > 8192 || version == "" {
			return nil, errors.New("configured provider CLI is unavailable or unsupported")
		}
		check := ProviderCheck{Provider: a.Provider, Version: version, Authentication: "requires configured machine-local provider login"}
		switch a.Provider {
		case "codex":
			child, cancel = context.WithTimeout(ctx, 5*time.Second)
			data, err = exec.CommandContext(child, a.Executable, "login", "status").CombinedOutput()
			cancel()
			if err != nil || !authenticatedOutput(string(data)) {
				return nil, errors.New("Codex requires machine-local login or API authentication")
			}
			check.Authentication = "authenticated"
		case "claude":
			child, cancel = context.WithTimeout(ctx, 5*time.Second)
			data, err = exec.CommandContext(child, a.Executable, "auth", "status").CombinedOutput()
			cancel()
			if err != nil || !authenticatedOutput(string(data)) {
				return nil, errors.New("Claude requires machine-local authentication")
			}
			check.Authentication = "authenticated"
		}
		out = append(out, check)
	}
	return out, nil
}

func authenticatedOutput(output string) bool {
	text := strings.ToLower(output)
	return strings.Contains(text, "logged in") || strings.Contains(text, "authenticated")
}
