package githubapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestInstallationTokensSignatureScopeCacheAndRefresh(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	source, err := New("app-client-id", 42, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	source.now = func() time.Time { return now }
	calls := 0
	fail := false
	source.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.github.com/app/installations/42/access_tokens" || r.Method != "POST" {
			t.Fatal("unexpected token origin")
		}
		jwt := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		parts := strings.Split(jwt, ".")
		if len(parts) != 3 {
			t.Fatal("missing JWT")
		}
		signature, e := base64.RawURLEncoding.DecodeString(parts[2])
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], signature) != nil {
			t.Fatal("JWT signature invalid")
		}
		claimsRaw, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claims struct {
			Issuer  string `json:"iss"`
			Issued  int64  `json:"iat"`
			Expires int64  `json:"exp"`
		}
		json.Unmarshal(claimsRaw, &claims)
		if claims.Issuer != "app-client-id" || claims.Issued != now.Add(-time.Minute).Unix() || claims.Expires > now.Add(10*time.Minute).Unix() {
			t.Fatal("JWT claims invalid")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"permissions":{"metadata":"read"}}` {
			t.Fatal("token requests broader permissions")
		}
		if fail {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("secret-provider-body"))}, nil
		}
		result, _ := json.Marshal(map[string]any{"token": "synthetic-installation-token", "expires_at": now.Add(time.Hour), "permissions": map[string]string{"metadata": "read"}})
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(string(result)))}, nil
	})
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			token, err := source.Token(context.Background())
			if err != nil || token != "synthetic-installation-token" {
				t.Error("token acquisition failed", err)
			}
		}()
	}
	workers.Wait()
	if calls != 1 {
		t.Fatal("concurrent requests did not share refresh")
	}
	now = now.Add(59 * time.Minute)
	fail = true
	token, err := source.Token(context.Background())
	if err != ErrUnavailable || token != "" || strings.Contains(err.Error(), "secret-provider-body") {
		t.Fatal("failed refresh returned stale token", err)
	}
	fail = false
	if _, err = source.Token(context.Background()); err != nil || calls != 3 {
		t.Fatal("refresh did not recover", err)
	}
}
func TestGitHubAppKeyConfiguration(t *testing.T) {
	if _, err := New("app", 1, []byte("not-a-key")); err != ErrConfiguration {
		t.Fatal("invalid key accepted")
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	raw, _ := x509.MarshalPKCS8PrivateKey(key)
	if _, err := New("app", 1, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})); err != nil {
		t.Fatal("PKCS8 key rejected", err)
	}
}
