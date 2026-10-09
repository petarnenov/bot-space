package runneridentity

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/petarnenov/bot-space/internal/ratelimit"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

const EnrollmentPath = "/runners/enrollment/"

type Web struct {
	Store                      *Store
	Browser                    *identity.Web
	ControlEndpoint, ControlCA string
}

// Register attaches admission only to a validated GitHub callback. The success
// page is informational; an ordinary GET never grants an enrollment.
func (w *Web) Register(routes identity.Routes) {
	previous := w.Browser.AfterGitHubLogin
	w.Browser.AfterGitHubLogin = func(ctx context.Context, user workspaces.User, path string) error {
		if previous != nil {
			if err := previous(ctx, user, path); err != nil {
				return err
			}
		}
		if !strings.HasPrefix(path, EnrollmentPath) {
			return nil
		}
		id := strings.TrimPrefix(path, EnrollmentPath)
		if !security.ValidUUID(id) {
			return ErrInvalid
		}
		return w.Store.AuthorizeGitHub(ctx, id, user.GitHubID)
	}
	peerLimit := ratelimit.New(30, 10, 10000, nil)
	globalLimit := ratelimit.New(200, 40, 1, nil)
	admission := func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			peer, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				peer = r.RemoteAddr
			}
			if ok, wait := peerLimit.Allow(peer); !ok {
				ratelimit.Reject(rw, wait)
				return
			}
			if ok, wait := globalLimit.Allow("enrollment"); !ok {
				ratelimit.Reject(rw, wait)
				return
			}
			next.ServeHTTP(rw, r)
		})
	}
	routes.Handle("POST /runners/enroll", admission(w.begin))
	routes.Handle("POST /runners/challenge", admission(w.challenge))
	routes.Handle("POST /runners/credential", admission(w.issue))
	routes.Handle("GET /runners/enrollment/{id}", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		headers(rw)
		if !security.ValidUUID(r.PathValue("id")) {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(rw, "<!doctype html><html lang=\"en\"><title>The Firm</title><h1>Runner enrollment</h1><p>Return to your runner to view enrollment status.</p></html>")
	}))
}
func headers(rw http.ResponseWriter) {
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("Referrer-Policy", "no-referrer")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
}
func read(rw http.ResponseWriter, r *http.Request, out any) bool {
	headers(rw)
	// Browser cross-origin requests cannot use these signed machine APIs.
	if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" {
		http.Error(rw, "Machine request required", 403)
		return false
	}
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(rw, "JSON required", 415)
		return false
	}
	r.Body = http.MaxBytesReader(rw, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(&struct{}{}) != io.EOF {
		http.Error(rw, "Invalid runner request", 400)
		return false
	}
	return true
}
func write(rw http.ResponseWriter, value any) {
	headers(rw)
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(value)
}
func reject(rw http.ResponseWriter, err error) {
	code := http.StatusServiceUnavailable
	if errors.Is(err, ErrInvalid) {
		code = 400
	} else if errors.Is(err, ErrUnauthenticated) {
		code = 401
	}
	http.Error(rw, "Runner identity request rejected", code)
}
func (w *Web) begin(rw http.ResponseWriter, r *http.Request) {
	var input StartProof
	if !read(rw, r, &input) {
		return
	}
	enrollment, err := w.Store.Begin(r.Context(), input)
	if err != nil {
		reject(rw, err)
		return
	}
	write(rw, struct {
		Enrollment
		LoginURL string `json:"login_url"`
	}{enrollment, w.Browser.Config.BaseURL + "/auth/github/login?return_to=" + EnrollmentPath + enrollment.ID})
}
func (w *Web) challenge(rw http.ResponseWriter, r *http.Request) {
	var input struct {
		ID      string `json:"id"`
		Purpose string `json:"purpose"`
	}
	if !read(rw, r, &input) {
		return
	}
	out, err := w.Store.Challenge(r.Context(), input.ID, input.Purpose)
	if err != nil {
		reject(rw, err)
		return
	}
	write(rw, out)
}
func (w *Web) issue(rw http.ResponseWriter, r *http.Request) {
	var input struct {
		ID        string `json:"id"`
		Purpose   string `json:"purpose"`
		Nonce     string `json:"nonce"`
		Signature []byte `json:"signature"`
	}
	if !read(rw, r, &input) {
		return
	}
	out, err := w.Store.Issue(r.Context(), input.ID, input.Purpose, input.Nonce, input.Signature)
	if err != nil {
		reject(rw, err)
		return
	}
	write(rw, struct {
		Credential
		Endpoint string `json:"control_endpoint"`
		CA       string `json:"control_ca"`
	}{out, w.ControlEndpoint, w.ControlCA})
}
