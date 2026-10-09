package main

import (
	"errors"
	"strings"
	"testing"
)

func TestParseObjectiveOptions(t *testing.T) {
	env := map[string]string{"OBJECTIVE_PROJECT": "11111111-1111-4111-8111-111111111111", "OBJECTIVE_GITHUB_USER_ID": "42", "OBJECTIVE_TITLE": "Ship it", "OBJECTIVE_DESCRIPTION": "Detailed objective"}
	getenv := func(key string) string { return env[key] }
	got, err := parseObjectiveOptions(nil, getenv, func(string) (string, error) { return "", errors.New("unused") })
	if err != nil || got.GitHubID != 42 || got.Input.Priority != 2 || got.Input.Key == "" || got.Input.Description != "Detailed objective" {
		t.Fatal("valid objective options rejected", got, err)
	}

	delete(env, "OBJECTIVE_DESCRIPTION")
	env["OBJECTIVE_DESCRIPTION_FILE"] = "objective.md"
	got, err = parseObjectiveOptions(nil, getenv, func(path string) (string, error) {
		if path != "objective.md" {
			t.Fatal("wrong description path", path)
		}
		return "Multiline\nobjective", nil
	})
	if err != nil || got.Input.Description != "Multiline\nobjective" {
		t.Fatal("description file rejected", got, err)
	}
}

func TestParseObjectiveOptionsRejectsInvalidInputSafely(t *testing.T) {
	base := map[string]string{"OBJECTIVE_PROJECT": "11111111-1111-4111-8111-111111111111", "OBJECTIVE_GITHUB_USER_ID": "42", "OBJECTIVE_TITLE": "Ship it", "OBJECTIVE_DESCRIPTION": "Detailed objective"}
	for _, test := range []struct {
		name   string
		change func(map[string]string)
	}{
		{"missing title", func(env map[string]string) { delete(env, "OBJECTIVE_TITLE") }},
		{"two descriptions", func(env map[string]string) { env["OBJECTIVE_DESCRIPTION_FILE"] = "objective.md" }},
		{"invalid priority", func(env map[string]string) { env["OBJECTIVE_PRIORITY"] = "urgent" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := map[string]string{}
			for key, value := range base {
				env[key] = value
			}
			test.change(env)
			_, err := parseObjectiveOptions(nil, func(key string) string { return env[key] }, func(string) (string, error) { return "secret file body", nil })
			if err == nil || strings.Contains(err.Error(), "secret file body") {
				t.Fatal("invalid input was accepted or leaked", err)
			}
		})
	}
}
