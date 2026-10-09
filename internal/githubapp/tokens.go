// Package githubapp issues narrowly scoped installation tokens server-side.
package githubapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrConfiguration = errors.New("invalid GitHub App configuration")
var ErrUnavailable = errors.New("GitHub App verification token unavailable")

type Tokens struct {
	mu           sync.Mutex
	issuer       string
	installation int64
	key          *rsa.PrivateKey
	client       *http.Client
	now          func() time.Time
	token        string
	expires      time.Time
}

func New(issuer string, installation int64, privatePEM []byte) (*Tokens, error) {
	if issuer == "" || len(issuer) > 128 || strings.ContainsAny(issuer, " \t\r\n") || installation < 1 || len(privatePEM) > 65536 {
		return nil, ErrConfiguration
	}
	block, rest := pem.Decode(privatePEM)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, ErrConfiguration
	}
	var key *rsa.PrivateKey
	if block.Type == "RSA PRIVATE KEY" {
		key, _ = x509.ParsePKCS1PrivateKey(block.Bytes)
	} else if block.Type == "PRIVATE KEY" {
		parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
		if e == nil {
			key, _ = parsed.(*rsa.PrivateKey)
		}
	}
	if key == nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, ErrConfiguration
	}
	return &Tokens{issuer: issuer, installation: installation, key: key, client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, now: time.Now}, nil
}
func (t *Tokens) jwt() (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	now := t.now()
	claims, _ := json.Marshal(struct {
		Issuer  string `json:"iss"`
		Issued  int64  `json:"iat"`
		Expires int64  `json:"exp"`
	}{t.issuer, now.Add(-time.Minute).Unix(), now.Add(9 * time.Minute).Unix()})
	input := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, t.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", ErrUnavailable
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// Token is a repositoryaccess.TokenSource. Refreshes are serialized and occur
// before expiry. An unsuccessful refresh never returns stale credentials.
func (t *Tokens) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ctx.Err() != nil {
		return "", ErrUnavailable
	}
	if t.token != "" && t.expires.After(t.now().Add(2*time.Minute)) {
		return t.token, nil
	}
	t.token = ""
	t.expires = time.Time{}
	jwt, err := t.jwt()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", t.installation), strings.NewReader(`{"permissions":{"metadata":"read"}}`))
	if err != nil {
		return "", ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+jwt)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	response, err := t.client.Do(request)
	if err != nil {
		return "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		return "", ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if err != nil || len(body) > 256<<10 {
		return "", ErrUnavailable
	}
	var result struct {
		Token       string            `json:"token"`
		Expires     time.Time         `json:"expires_at"`
		Permissions map[string]string `json:"permissions"`
	}
	if json.Unmarshal(body, &result) != nil || result.Token == "" || len(result.Token) > 4096 || strings.ContainsAny(result.Token, " \t\r\n") || !result.Expires.After(t.now().Add(2*time.Minute)) || result.Expires.After(t.now().Add(65*time.Minute)) || result.Permissions["metadata"] != "read" {
		return "", ErrUnavailable
	}
	for name, level := range result.Permissions {
		if name != "metadata" && level != "none" {
			return "", ErrUnavailable
		}
	}
	t.token = result.Token
	t.expires = result.Expires
	return t.token, nil
}
