package integration

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/petarnenov/bot-space/internal/agents"
)

func TestAgentBrowserMutationsUseSessionOwnership(t *testing.T) {
	ctx, pool, web, srv, _, _ := browserWeb(t)
	w, err := web.Workspaces.Bootstrap(ctx, 101, "browser-team")
	if err != nil {
		t.Fatal(err)
	}
	owner, ownerCookie, err := web.Sessions.Login(ctx, 101, "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	member, memberCookie, err := web.Sessions.Login(ctx, 102, "member", "")
	if err != nil {
		t.Fatal(err)
	}
	i, secret, err := web.Workspaces.Invite(ctx, w.ID, owner.User.ID, 102, "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = web.Workspaces.AcceptInvitation(ctx, member.User.ID, i.ID, secret); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	post := func(cookie, csrf, path string, values url.Values) (int, string, http.Header) {
		t.Helper()
		if values == nil {
			values = url.Values{}
		}
		values.Set("csrf_token", csrf)
		req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", srv.URL)
		req.AddCookie(&http.Cookie{Name: web.CookieName(), Value: cookie})
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(body), response.Header
	}
	path := "/workspaces/" + w.ID + "/agents"
	if code, _, _ := post(memberCookie, "invalid", path, url.Values{"name": {"bad"}}); code != 403 {
		t.Fatal("registration accepted without CSRF")
	}
	if code, _, _ := post(memberCookie, member.CSRF, path, url.Values{"name": {"browser worker"}, "owner_user_id": {owner.User.ID}}); code != 200 {
		t.Fatal("browser registration failed")
	}
	s := &agents.Store{Pool: pool}
	list, err := s.Own(ctx, w.ID, member.User.ID)
	if err != nil || len(list) != 1 || list[0].OwnerUserID != member.User.ID {
		t.Fatal("browser actor spoof changed owner")
	}
	a := list[0]
	credentialsPath := path + "/" + a.ID + "/credentials"
	if code, _, _ := post(ownerCookie, owner.CSRF, credentialsPath, nil); code != 403 {
		t.Fatal("admin issued another member's token over HTTP")
	}
	code, body, headers := post(memberCookie, member.CSRF, credentialsPath, nil)
	match := regexp.MustCompile(`<code id="issued-token">([^<]+)</code>`).FindStringSubmatch(body)
	if code != 200 || len(match) != 2 || headers.Get("Cache-Control") != "no-store" || headers.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("unsafe one-time token page")
	}
	oldToken := match[1]
	metadata, err := s.Credentials(ctx, w.ID, member.User.ID, a.ID)
	if err != nil || len(metadata) != 1 {
		t.Fatal("missing credential metadata")
	}
	oldID := metadata[0].ID
	code, body, _ = post(memberCookie, member.CSRF, credentialsPath+"/"+oldID+"/rotate", nil)
	match = regexp.MustCompile(`<code id="issued-token">([^<]+)</code>`).FindStringSubmatch(body)
	if code != 200 || len(match) != 2 || match[1] == oldToken {
		t.Fatal("browser rotation failed")
	}
	newToken := match[1]
	if _, err = s.Authenticate(ctx, oldToken); err == nil {
		t.Fatal("old browser-issued token valid")
	}
	if _, err = s.Authenticate(ctx, newToken); err != nil {
		t.Fatal(err)
	}
	metadata, err = s.Credentials(ctx, w.ID, member.User.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	newID := ""
	for _, c := range metadata {
		if c.RevokedAt == nil {
			newID = c.ID
		}
	}
	if code, _, _ := post(memberCookie, member.CSRF, credentialsPath+"/"+newID+"/revoke", nil); code != 303 {
		t.Fatal("browser revoke failed")
	}
	if _, err = s.Authenticate(ctx, newToken); err == nil {
		t.Fatal("revoked browser-issued token valid")
	}
	if code, _, _ := post(ownerCookie, owner.CSRF, path+"/"+a.ID+"/deactivate", nil); code != 303 {
		t.Fatal("admin browser deactivation failed")
	}
}
