package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCLIHelper(t *testing.T) {
	if os.Getenv("BOT_SPACE_CLI_HELPER") != "1" {
		return
	}
	os.Args = []string{"mailbox", "serve"}
	main()
	os.Exit(0)
}

func TestInvalidConfigurationFailsSafely(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLIHelper$")
	cmd.Env = append(os.Environ(), "BOT_SPACE_CLI_HELPER=1", "DATABASE_URL=postgres://user:secret-sentinel@localhost/db", "PORT=port-sentinel")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("invalid configuration did not fail")
	}
	for _, value := range []string{"secret-sentinel", "port-sentinel", "server_started"} {
		if strings.Contains(string(output), value) {
			t.Fatal("unsafe startup output")
		}
	}
	if !strings.Contains(string(output), "PORT must be") {
		t.Fatal("missing sanitized validation error")
	}
}
