package runneridentity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/petarnenov/bot-space/internal/security"
)

type Lease struct {
	Credential
	Endpoint string `json:"control_endpoint"`
	CA       string `json:"control_ca"`
}
type Client struct {
	origin string
	http   *http.Client
}

func NewClient(origin string) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrInvalid
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
		local = true
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, ErrInvalid
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return nil, ErrInvalid
		}
	}
	return &Client{origin: strings.TrimRight(origin, "/"), http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) post(ctx context.Context, path string, input, out any) error {
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > 8192 {
		return ErrInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, bytes.NewReader(raw))
	if err != nil {
		return ErrInvalid
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 {
		return ErrUnauthenticated
	}
	if response.StatusCode != 200 {
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if err != nil || len(body) > 256<<10 {
		return ErrUnavailable
	}
	if json.Unmarshal(body, out) != nil {
		return ErrUnavailable
	}
	return nil
}

// Enroll opens only this server's verified login URL. Browser cookies never
// enter the machine HTTP client and credentials never enter the browser URL.
func (c *Client) Enroll(ctx context.Context, project string, role Role, key ed25519.PrivateKey, open func(string) error) (Lease, error) {
	if len(key) != ed25519.PrivateKeySize || open == nil {
		return Lease{}, ErrInvalid
	}
	nonce, err := security.Secret()
	if err != nil {
		return Lease{}, ErrUnavailable
	}
	message, err := StartMessage(project, role, nonce)
	if err != nil {
		return Lease{}, err
	}
	var started struct {
		Enrollment
		LoginURL string `json:"login_url"`
	}
	proof := StartProof{ProjectID: project, Role: role, Nonce: nonce, PublicKey: key.Public().(ed25519.PublicKey), Signature: ed25519.Sign(key, message)}
	if err = c.post(ctx, "/runners/enroll", proof, &started); err != nil {
		return Lease{}, err
	}
	expected := c.origin + "/auth/github/login?return_to=" + EnrollmentPath + started.ID
	if !security.ValidUUID(started.ID) || !strings.EqualFold(started.ProjectID, project) || started.Role != role || started.LoginURL != expected || !started.ExpiresAt.After(time.Now()) {
		return Lease{}, ErrUnavailable
	}
	if err = open(started.LoginURL); err != nil {
		return Lease{}, ErrUnavailable
	}
	for {
		if !started.ExpiresAt.After(time.Now()) {
			return Lease{}, ErrUnauthenticated
		}
		lease, err := c.issue(ctx, started.ID, "claim", key)
		if err == nil {
			if !strings.EqualFold(lease.ProjectID, project) || lease.Role != role {
				return Lease{}, ErrUnavailable
			}
			return lease, nil
		}
		if ctx.Err() != nil {
			return Lease{}, ctx.Err()
		}
		if err != ErrUnauthenticated && err != ErrUnavailable {
			return Lease{}, err
		}
		timer := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Lease{}, ctx.Err()
		case <-timer.C:
		}
	}
}
func (c *Client) issue(ctx context.Context, id, purpose string, key ed25519.PrivateKey) (Lease, error) {
	if len(key) != ed25519.PrivateKeySize {
		return Lease{}, ErrInvalid
	}
	var challenge Challenge
	if err := c.post(ctx, "/runners/challenge", map[string]string{"id": id, "purpose": purpose}, &challenge); err != nil {
		return Lease{}, err
	}
	message, err := ChallengeMessage(id, purpose, challenge.Nonce)
	if err != nil || !challenge.ExpiresAt.After(time.Now()) {
		return Lease{}, ErrUnavailable
	}
	input := struct {
		ID        string `json:"id"`
		Purpose   string `json:"purpose"`
		Nonce     string `json:"nonce"`
		Signature []byte `json:"signature"`
	}{id, purpose, challenge.Nonce, ed25519.Sign(key, message)}
	var out Lease
	if err = c.post(ctx, "/runners/credential", input, &out); err != nil {
		return Lease{}, err
	}
	if err = out.Validate(); err != nil {
		return Lease{}, ErrInvalid
	}
	return out, nil
}
func (c *Client) Refresh(ctx context.Context, current Lease, key ed25519.PrivateKey) (Lease, error) {
	// Expiry does not prevent key-proved refresh; deactivated authority does.
	if !security.ValidUUID(current.RunnerID) {
		return Lease{}, ErrInvalid
	}
	next, err := c.issue(ctx, current.RunnerID, "refresh", key)
	if err != nil {
		return Lease{}, err
	}
	if next.RunnerID != current.RunnerID || next.ProjectID != current.ProjectID || next.Role != current.Role || next.Epoch <= current.Epoch {
		return Lease{}, ErrUnavailable
	}
	return next, nil
}
func (l Lease) Validate() error {
	if !security.ValidUUID(l.RunnerID) || !security.ValidUUID(l.ProjectID) || !validRole(l.Role) || l.Epoch == 0 || !strings.HasPrefix(l.Token, TokenPrefix) || !validNonce(strings.TrimPrefix(l.Token, TokenPrefix)) || !l.ExpiresAt.After(time.Now()) {
		return ErrInvalid
	}
	host, port, err := net.SplitHostPort(l.Endpoint)
	n, e := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/\\ \t\r\n") || e != nil || n < 1 || n > 65535 {
		return ErrInvalid
	}
	roots := x509.NewCertPool()
	if l.CA == "" || len(l.CA) > 65536 || !roots.AppendCertsFromPEM([]byte(l.CA)) {
		return ErrInvalid
	}
	return nil
}
