//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidStartupFlagsCreateNoIdentityAndOpenNoBrowser(t *testing.T) {
	for _, args := range [][]string{
		{"enroll", "--server", "https://example.com", "--role", "admin", "--project", "11111111-1111-4111-8111-111111111111"},
		{"enroll", "--server", "http://external.example", "--role", "executor", "--project", "11111111-1111-4111-8111-111111111111"},
		{"enroll", "--server", "https://example.com", "--role", "executor", "--project", "invalid"},
	} {
		state := filepath.Join(t.TempDir(), "state")
		args = append(args, "--state", state)
		opened := false
		err := run(context.Background(), args, &bytes.Buffer{}, func(string) error { opened = true; return nil })
		if err == nil || opened {
			t.Fatal("invalid startup accepted")
		}
		if _, err = os.Stat(state); !os.IsNotExist(err) {
			t.Fatal("invalid startup wrote state")
		}
	}
}
