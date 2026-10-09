//go:build darwin || linux

package runneridentity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoleSocketsRemainIndependentAndPrivate(t *testing.T) {
	root := t.TempDir()
	a, err := OpenState(filepath.Join(root, "architect"), "https://example.com", Architect)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	e, err := OpenState(filepath.Join(root, "executor"), "https://example.com", Executor)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	first, err := a.ListenLocal()
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.ListenLocal()
	if err != nil {
		t.Fatal(err)
	}
	if first.Addr().String() == second.Addr().String() {
		t.Fatal("roles share socket")
	}
	for _, l := range []string{first.Addr().String(), second.Addr().String()} {
		info, err := os.Lstat(l)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("socket not private", err)
		}
	}
	path := first.Addr().String()
	a.Close()
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("shutdown retained socket")
	}
	if _, err = os.Stat(second.Addr().String()); err != nil {
		t.Fatal("architect shutdown removed executor socket")
	}
}
