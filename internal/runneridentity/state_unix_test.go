//go:build darwin || linux

package runneridentity

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/petarnenov/bot-space/internal/security"
)

func TestPrivateStateLockBindingsAndStableKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "executor")
	state, err := OpenState(dir, "https://example.com", project, Executor)
	if err != nil {
		t.Fatal(err)
	}
	key := state.SigningKey()
	if duplicate, err := OpenState(dir, "https://example.com", project, Executor); !errors.Is(err, ErrStateLocked) {
		if duplicate != nil {
			duplicate.Close()
		}
		t.Fatal("duplicate state opened", err)
	}
	architect, err := OpenState(filepath.Join(filepath.Dir(dir), "architect"), "https://example.com", project, Architect)
	if err != nil {
		t.Fatal("second role on host rejected", err)
	}
	defer architect.Close()
	if bytes.Equal(key, architect.SigningKey()) {
		t.Fatal("roles share key")
	}
	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenState(dir, "https://example.com", project, Executor)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, restored.SigningKey()) {
		t.Fatal("restart replaced identity")
	}
	restored.Close()
	if wrong, err := OpenState(dir, "https://other.example", project, Executor); err != ErrInvalid {
		if wrong != nil {
			wrong.Close()
		}
		t.Fatal("server binding ignored", err)
	}
	if wrong, err := OpenState(dir, "https://example.com", project, Architect); err != ErrInvalid {
		if wrong != nil {
			wrong.Close()
		}
		t.Fatal("role binding ignored", err)
	}
	for _, name := range []string{"machine.seed", "profile.json", "runner.lock"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("non-private state file", name, err)
		}
	}
}
func TestPrivateCredentialJournalAndExpiredRefreshMetadata(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state, err := OpenState(dir, "https://example.com", project, Executor)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cert := httptest.NewTLSServer(http.NotFoundHandler())
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.TLS.Certificates[0].Certificate[0]}))
	cert.Close()
	nonce, _ := security.Secret()
	lease := Lease{Credential: Credential{RunnerID: "22222222-2222-4222-8222-222222222222", ProjectID: project, Role: Executor, Epoch: 1, Token: TokenPrefix + nonce, ExpiresAt: time.Now().Add(time.Minute)}, Endpoint: "example.com:9090", CA: ca}
	if err = state.SaveLease(lease); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.LoadLease()
	if err != nil || loaded.Token != lease.Token || loaded.Epoch != 1 {
		t.Fatal("journal lost credential", err)
	}
	loaded.ExpiresAt = time.Now().Add(-time.Minute)
	raw, _ := json.Marshal(loaded)
	if err = os.WriteFile(filepath.Join(dir, "credential.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	expired, err := state.LoadLease()
	if err != nil || expired.Validate() != ErrInvalid {
		t.Fatal("expired key-refresh metadata unavailable", err)
	}
	wrong := lease
	wrong.Role = Architect
	if state.SaveLease(wrong) != ErrInvalid {
		t.Fatal("wrong role credential stored")
	}
	raw, _ = json.Marshal(state)
	if bytes.Contains(raw, state.SigningKey()) || bytes.Contains(raw, []byte(lease.Token)) {
		t.Fatal("state marshaling exposes secrets")
	}
}
func TestStateRejectsSymlinksAndPublicPermissions(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if state, err := OpenState(link, "https://example.com", project, Executor); err == nil {
		state.Close()
		t.Fatal("symlinked state accepted")
	}
	public := filepath.Join(root, "public")
	if err := os.Mkdir(public, 0755); err != nil {
		t.Fatal(err)
	}
	if state, err := OpenState(public, "https://example.com", project, Executor); err == nil {
		state.Close()
		t.Fatal("public state accepted")
	}
	state, err := OpenState(target, "https://example.com", project, Executor)
	if err != nil {
		t.Fatal(err)
	}
	state.Close()
	seed := filepath.Join(target, "machine.seed")
	if err := os.Chmod(seed, 0644); err != nil {
		t.Fatal(err)
	}
	if state, err := OpenState(target, "https://example.com", project, Executor); err == nil {
		state.Close()
		t.Fatal("public key seed accepted")
	}
	if err := os.Remove(seed); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), seed); err != nil {
		t.Fatal(err)
	}
	if state, err := OpenState(target, "https://example.com", project, Executor); err == nil {
		state.Close()
		t.Fatal("symlinked seed accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "outside")); !os.IsNotExist(err) {
		t.Fatal("wrote through seed link")
	}
}
