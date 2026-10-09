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
	var secondProject string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,43,101,'owner','second') RETURNING id::text`, w.ID).Scan(&secondProject); err != nil {
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
	dashboardPath := "/projects/" + project
	intakePath := dashboardPath + "/intentions"
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
	response := call("POST", intakePath, form, false)
	if response.StatusCode != 401 {
		t.Fatal("agent bearer authenticated intake")
	}
	response.Body.Close()
	form.Set("csrf_token", "wrong")
	response = call("POST", intakePath, form, true)
	if response.StatusCode != 403 {
		t.Fatal("invalid CSRF admitted")
	}
	response.Body.Close()
	form.Set("csrf_token", session.CSRF)
	response = call("POST", intakePath, form, true)
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
	response = call("GET", dashboardPath, nil, true)
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(raw), "Submit objective") || !strings.Contains(string(raw), "objective-description") || !strings.Contains(string(raw), "&lt;script&gt;") {
		t.Fatal("backlog view missing")
	}
	if !strings.Contains(string(raw), `<select id="project-switch" name="project">`) || !strings.Contains(string(raw), `value="`+project+`" selected`) || !strings.Contains(string(raw), `value="`+secondProject+`"`) || !strings.Contains(string(raw), "owner/second") {
		t.Fatal("authorized project switcher is incomplete")
	}
	response = call("GET", intakePath, nil, true)
	if response.StatusCode != http.StatusTemporaryRedirect || response.Header.Get("Location") != dashboardPath {
		t.Fatal("legacy backlog address did not redirect", response.StatusCode, response.Header.Get("Location"))
	}
	response.Body.Close()
	control := url.Values{"csrf_token": {"wrong"}, "expected_epoch": {"1"}, "action": {"pause"}}
	response = call("POST", location+"/control", control, true)
	if response.StatusCode != 403 {
		t.Fatal("control accepted invalid CSRF", response.StatusCode)
	}
	response.Body.Close()
	control.Set("csrf_token", session.CSRF)
	response = call("POST", location+"/control", control, false)
	if response.StatusCode != 401 {
		t.Fatal("agent controlled human root", response.StatusCode)
	}
	response.Body.Close()
	response = call("POST", location+"/control", control, true)
	if response.StatusCode != 303 {
		t.Fatal("human pause rejected", response.StatusCode)
	}
	response.Body.Close()
	response = call("GET", location, nil, true)
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(raw), "Resume for architect reconciliation") || !strings.Contains(string(raw), "<h2>History</h2>") || !strings.Contains(string(raw), "epoch 2") {
		t.Fatal("pause controls/history missing", string(raw))
	}
	control.Set("expected_epoch", "2")
	control.Set("action", "archive")
	response = call("POST", location+"/control", control, true)
	if response.StatusCode != 303 {
		t.Fatal("archive rejected", response.StatusCode)
	}
	response.Body.Close()
	response = call("GET", location, nil, true)
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(raw), "Archived — history is preserved.") || strings.Contains(string(raw), "Save new input revision") || strings.Contains(string(raw), "Cancel objective") || !strings.Contains(string(raw), "epoch 3") {
		t.Fatal("archive did not preserve read-only history", string(raw))
	}
	response = call("GET", dashboardPath, nil, true)
	raw, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(raw), "&lt;script&gt;") {
		t.Fatal("archived objective remained in backlog")
	}
}
