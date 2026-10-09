package integration

import (
	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/identity"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHumanIntakeHTTPRejectsAgentsAndCSRFAndEscapesContent(t *testing.T) {
	ctx, pool, _, w, owner := teams(t)
	var project string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, w.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	sessions := &identity.Sessions{Pool: pool}
	session, secret, err := sessions.Login(ctx, owner.GitHubID, owner.Username, "")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	defer server.Close()
	browser := &identity.Web{Config: config.Identity{BaseURL: "http://" + server.Listener.Addr().String()}, Sessions: sessions}
	(&backlog.Web{Store: &backlog.Store{Pool: pool, Sessions: sessions, Authority: &backlogAuthority{true}}, Browser: browser}).Register(mux)
	server.Start()
	path := "/projects/" + project + "/intentions"
	call := func(method, path string, form url.Values, cookie bool) *http.Response {
		t.Helper()
		request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Origin", server.URL)
		if cookie {
			request.AddCookie(&http.Cookie{Name: browser.CookieName(), Value: secret})
		} else {
			request.Header.Set("Authorization", "Bearer bsr_machine-token")
		}
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	form := url.Values{"csrf_token": {session.CSRF}, "idempotency_key": {"http-story"}, "title": {"<script>attack()</script>"}, "description": {"Human objective"}, "priority": {"2"}}
	response := call("POST", path, form, false)
	if response.StatusCode != 401 {
		t.Fatal("agent bearer authenticated intake")
	}
	response.Body.Close()
	form.Set("csrf_token", "wrong")
	response = call("POST", path, form, true)
	if response.StatusCode != 403 {
		t.Fatal("invalid CSRF admitted")
	}
	response.Body.Close()
	form.Set("csrf_token", session.CSRF)
	response = call("POST", path, form, true)
	if response.StatusCode != 303 {
		t.Fatal("human input rejected", response.StatusCode)
	}
	location := response.Header.Get("Location")
	response.Body.Close()
	response = call("GET", location, nil, true)
	raw, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || strings.Contains(string(raw), "<script>") || !strings.Contains(string(raw), "&lt;script&gt;") {
		t.Fatal("unsafe rendering")
	}
	response = call("GET", path, nil, true)
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(raw), "Submit objective") || !strings.Contains(string(raw), "&lt;script&gt;") {
		t.Fatal("backlog view missing")
	}
}
