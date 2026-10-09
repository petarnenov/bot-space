package runneridentity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientRejectsUnsafeOriginsAndLoginRedirect(t *testing.T) {
	for _, origin := range []string{"http://public.example", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=x", "https://example.com:99999"} {
		if _, err := NewClient(origin); err != ErrInvalid {
			t.Fatal("unsafe origin admitted", origin)
		}
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input StartProof
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("invalid request")
			return
		}
		if err := VerifyStart(input); err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
			t.Error("browser authority leaked")
		}
		json.NewEncoder(w).Encode(map[string]any{"id": project, "project_id": project, "role": "executor", "expires_at": time.Now().Add(time.Minute), "login_url": "https://attacker.example/auth"})
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	opened := false
	_, err = client.Enroll(context.Background(), project, Executor, key, func(string) error { opened = true; return nil })
	if err != ErrUnavailable || opened {
		t.Fatal("untrusted login URL opened", err)
	}
}
func TestClientDoesNotFollowMachineAPIRedirects(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL)
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	_, err := client.Enroll(context.Background(), project, Executor, key, func(string) error { return nil })
	if err != ErrUnavailable || forwarded {
		t.Fatal("machine key proof forwarded outside server", err)
	}
}
