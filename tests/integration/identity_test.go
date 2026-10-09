package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/httpserver"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpserver"
	"github.com/petarnenov/bot-space/internal/security"
	management "github.com/petarnenov/bot-space/internal/web"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

type fakeGitHub struct {
	mu                  sync.Mutex
	id                  int64
	username            string
	challenge, callback string
	exchanges           int
	mode                string
	verified            bool
}

func (f *fakeGitHub) handler(rw http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/authorize":
		q := r.URL.Query()
		f.challenge = q.Get("code_challenge")
		f.callback = q.Get("redirect_uri")
		if q.Get("scope") != "" || q.Get("code_challenge_method") != "S256" {
			http.Error(rw, "bad authorization", 400)
			return
		}
		u, _ := url.Parse(f.callback)
		query := u.Query()
		query.Set("state", q.Get("state"))
		query.Set("code", "mock-code")
		u.RawQuery = query.Encode()
		http.Redirect(rw, r, u.String(), 302)
	case "/token":
		f.exchanges++
		_ = r.ParseForm()
		if f.mode == "timeout" {
			select {
			case <-r.Context().Done():
			case <-time.After(11 * time.Second):
			}
			return
		}
		f.verified = security.PKCE(r.PostForm.Get("code_verifier")) == f.challenge && r.PostForm.Get("redirect_uri") == f.callback && r.PostForm.Get("client_secret") == "mock-client-secret"
		if f.mode == "token-error" {
			http.Error(rw, "provider-secret-payload", 400)
			return
		}
		if f.mode == "oversized" {
			_, _ = io.WriteString(rw, strings.Repeat("x", identity.ProviderBodyLimit+1))
			return
		}
		if !f.verified {
			http.Error(rw, "PKCE mismatch", 400)
			return
		}
		if f.mode == "missing-token" {
			_ = json.NewEncoder(rw).Encode(map[string]string{"token_type": "bearer"})
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]string{"access_token": "mock-provider-access-token", "token_type": "bearer"})
	case "/user":
		if r.Header.Get("Authorization") != "Bearer mock-provider-access-token" || r.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
			http.Error(rw, "invalid API request", 401)
			return
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{"id": f.id, "login": f.username})
	default:
		http.NotFound(rw, r)
	}
}

func browserWeb(t *testing.T) (context.Context, *pgxpool.Pool, *identity.Web, *httptest.Server, *http.Client, *fakeGitHub) {
	t.Helper()
	ctx, dsn, pool := isolated(t)
	if err := database.Migrate(ctx, dsn, bundle(t)); err != nil {
		t.Fatal(err)
	}
	fake := &fakeGitHub{id: 101, username: "synthetic-login"}
	providerServer := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(providerServer.Close)
	sessions := &identity.Sessions{Pool: pool}
	store := &workspaces.Store{Pool: pool}
	server := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, pool, bundle(t)) }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	webServer := httptest.NewUnstartedServer(server.HTTP.Handler)
	web := &identity.Web{Config: config.Identity{Enabled: true, ClientID: "mock-client-id", ClientSecret: "mock-client-secret", BaseURL: "http://" + webServer.Listener.Addr().String()}, Sessions: sessions, Workspaces: store, Provider: identity.Provider{AuthorizeURL: providerServer.URL + "/authorize", TokenURL: providerServer.URL + "/token", UserURL: providerServer.URL + "/user", Client: identity.GitHubProvider().Client}}
	web.Register(server)
	(&agents.Web{Store: &agents.Store{Pool: pool}}).Register(server, web)
	(&management.Management{Browser: web, Teams: store, Agents: &agents.Store{Pool: pool}, MailboxEnabled: true}).Register(server)
	mailboxStore, err := mailbox.New(pool, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	server.Handle("/mcp", mcpserver.New(mailboxStore, []string{web.Config.BaseURL}))
	webServer.Start()
	t.Cleanup(webServer.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	return ctx, pool, web, webServer, client, fake
}

func loginBrowser(t *testing.T, client *http.Client, base string) string {
	t.Helper()
	resp, err := client.Get(base + "/auth/github/login")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("login ended with status %d", resp.StatusCode)
	}
	return string(body)
}

func browserSecret(t *testing.T, client *http.Client, base, name string) string {
	t.Helper()
	u, _ := url.Parse(base)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	t.Fatal("missing browser cookie")
	return ""
}

func TestOAuthHTTPIdentityAndSessionRotation(t *testing.T) {
	ctx, pool, web, srv, client, fake := browserWeb(t)
	before, old, err := web.Sessions.Login(ctx, 101, "old-name", "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{{Name: web.CookieName(), Value: old, Path: "/"}})
	body := loginBrowser(t, client, srv.URL)
	if !strings.Contains(body, "synthetic-login") || !strings.Contains(body, "No workspace memberships") {
		t.Fatal("unexpected identity or automatic team access")
	}
	secret := browserSecret(t, client, srv.URL, web.CookieName())
	if secret == old {
		t.Fatal("session not rotated")
	}
	if _, err = web.Sessions.Authenticate(ctx, old); err == nil {
		t.Fatal("old cookie still valid")
	}
	after, err := web.Sessions.Authenticate(ctx, secret)
	if err != nil || after.User.ID != before.User.ID || after.User.GitHubID != 101 {
		t.Fatal("immutable ID changed")
	}
	fake.mu.Lock()
	verified := fake.verified
	fake.mu.Unlock()
	if !verified {
		t.Fatal("PKCE or callback not verified")
	}
	var sessionsContainToken bool
	if pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mailbox.sessions WHERE row_to_json(sessions)::text LIKE $1)", "%mock-provider-access-token%").Scan(&sessionsContainToken) != nil || sessionsContainToken {
		t.Fatal("GitHub token persisted")
	}
	var hash string
	if pool.QueryRow(ctx, "SELECT id_hash FROM mailbox.sessions WHERE id_hash=$1", security.Hash(secret)).Scan(&hash) != nil || hash == secret {
		t.Fatal("session stored in plaintext")
	}
	if _, err = web.Workspaces.Bootstrap(ctx, 101, "own-team"); err != nil {
		t.Fatal(err)
	}
	if _, err = web.Workspaces.Bootstrap(ctx, 201, "foreign-team"); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(content), "own-team") || strings.Contains(string(content), "foreign-team") {
		t.Fatal("workspace page leaks another team")
	}
	if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Referrer-Policy") != "no-referrer" || resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("missing browser protections")
	}
}

func TestOAuthStateMismatchExpiryReplayAndSafeRedirect(t *testing.T) {
	ctx, pool, web, srv, client, fake := browserWeb(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	start, err := client.Get(srv.URL + "/auth/github/login?return_to=//evil.example")
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()
	authorize, _ := url.Parse(start.Header.Get("Location"))
	state := authorize.Query().Get("state")
	bad, err := client.Get(srv.URL + "/auth/github/callback?state=wrong&code=mock-code")
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != 401 {
		t.Fatal("mismatch accepted")
	}
	missing, err := client.Get(srv.URL + "/auth/github/callback?code=mock-code")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != 401 {
		t.Fatal("missing state accepted")
	}
	uncoupled := &http.Client{Timeout: 5 * time.Second}
	withoutCookie, err := uncoupled.Get(srv.URL + "/auth/github/callback?state=" + state + "&code=mock-code")
	if err != nil {
		t.Fatal(err)
	}
	withoutCookie.Body.Close()
	if withoutCookie.StatusCode != 401 {
		t.Fatal("state accepted without initiating browser cookie")
	}
	if _, err = pool.Exec(ctx, "UPDATE mailbox.oauth_attempts SET expires_at=now()-interval '1 second' WHERE state_hash=$1", security.Hash(state)); err != nil {
		t.Fatal(err)
	}
	expired, err := client.Get(srv.URL + "/auth/github/callback?state=" + state + "&code=mock-code")
	if err != nil {
		t.Fatal(err)
	}
	expired.Body.Close()
	if expired.StatusCode != 401 {
		t.Fatal("expired state accepted")
	}
	fake.mu.Lock()
	exchanges := fake.exchanges
	fake.mu.Unlock()
	if exchanges != 0 {
		t.Fatal("invalid state reached exchange")
	}
	start, err = client.Get(srv.URL + "/auth/github/login?return_to=//evil.example")
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()
	provider, err := client.Get(start.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	provider.Body.Close()
	callback := provider.Header.Get("Location")
	completed, err := client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	completed.Body.Close()
	if completed.StatusCode != 303 || completed.Header.Get("Location") != "/" {
		t.Fatal("unsafe callback redirect")
	}
	replay, err := client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if replay.StatusCode != 401 {
		t.Fatal("callback replay accepted")
	}
	fake.mu.Lock()
	exchanges = fake.exchanges
	fake.mu.Unlock()
	if exchanges != 1 {
		t.Fatal("replay reached exchange")
	}
	if _, err := web.Sessions.Authenticate(ctx, browserSecret(t, client, srv.URL, web.CookieName())); err != nil {
		t.Fatal(err)
	}
}

func TestSessionExpiryCSRFOriginLogoutAndCookieFlags(t *testing.T) {
	ctx, pool, web, srv, client, _ := browserWeb(t)
	loginBrowser(t, client, srv.URL)
	secret := browserSecret(t, client, srv.URL, web.CookieName())
	session, err := web.Sessions.Authenticate(ctx, secret)
	if err != nil {
		t.Fatal(err)
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	post := func(token, origin string) int {
		t.Helper()
		req, _ := http.NewRequest("POST", srv.URL+"/auth/logout", strings.NewReader(url.Values{"csrf_token": {token}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if post("bad", srv.URL) != 403 || post(session.CSRF, "https://evil.example") != 403 {
		t.Fatal("forged logout accepted")
	}
	if _, err = web.Sessions.Authenticate(ctx, secret); err != nil {
		t.Fatal("rejected logout changed state")
	}
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{{Name: "bot_space_project", Value: "11111111-1111-4111-8111-111111111111", Path: "/"}})
	if post(session.CSRF, srv.URL) != 303 {
		t.Fatal("valid logout failed")
	}
	for _, cookie := range client.Jar.Cookies(u) {
		if cookie.Name == "bot_space_project" {
			t.Fatal("logout retained selected project")
		}
	}
	if _, err = web.Sessions.Authenticate(ctx, secret); err == nil {
		t.Fatal("revoked cookie accepted")
	}
	for _, kind := range []string{"absolute", "idle"} {
		_, value, err := web.Sessions.Login(ctx, 101, "synthetic-login", "")
		if err != nil {
			t.Fatal(err)
		}
		query := "UPDATE mailbox.sessions SET expires_at=now()-interval '1 second' WHERE id_hash=$1"
		if kind == "idle" {
			query = "UPDATE mailbox.sessions SET last_seen_at=now()-interval '31 minutes' WHERE id_hash=$1"
		}
		if _, err = pool.Exec(ctx, query, security.Hash(value)); err != nil {
			t.Fatal(err)
		}
		if _, err = web.Sessions.Authenticate(ctx, value); err == nil {
			t.Fatal("expired session accepted")
		}
	}
	secureWeb := *web
	secureWeb.Config.SecureCookies = true
	secureServer := httpserver.New(func(context.Context) error { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	secureWeb.Register(secureServer)
	rw := httptest.NewRecorder()
	request := httptest.NewRequest("GET", srv.URL+"/auth/github/login", nil)
	// Read the actual login handler cookie through registered routes.
	secureServer.HTTP.Handler.ServeHTTP(rw, request)
	if len(rw.Result().Cookies()) != 1 {
		t.Fatal("missing production attempt cookie")
	}
	for _, cookie := range rw.Result().Cookies() {
		if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
			t.Fatal("insecure production cookie")
		}
	}
}

func TestOAuthProviderFailuresAreSanitized(t *testing.T) {
	for _, mode := range []string{"token-error", "missing-token", "oversized", "invalid-user", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx, pool, _, srv, client, fake := browserWeb(t)
			fake.mu.Lock()
			fake.mode = mode
			if mode == "invalid-user" {
				fake.id = 0
			}
			fake.mu.Unlock()
			response, err := client.Get(srv.URL + "/auth/github/login")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 401 || strings.Contains(string(body), "mock-provider-access-token") || strings.Contains(string(body), "provider-secret-payload") {
				t.Fatal("unsafe provider failure")
			}
			var count int
			if pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.sessions").Scan(&count) != nil || count != 0 {
				t.Fatal("failed login made session")
			}
		})
	}
}
