package integration

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestInvitationHTTPRequiresSessionCSRFAndTarget(t *testing.T) {
	ctx, _, web, srv, client, _ := browserWeb(t)
	w, err := web.Workspaces.Bootstrap(ctx, 101, "http-team")
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := web.Sessions.Login(ctx, 101, "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	target, targetSecret, err := web.Sessions.Login(ctx, 102, "target", "")
	if err != nil {
		t.Fatal(err)
	}
	wrong, wrongSecret, err := web.Sessions.Login(ctx, 103, "other", "")
	if err != nil {
		t.Fatal(err)
	}
	i, secret, err := web.Workspaces.Invite(ctx, w.ID, owner.User.ID, 102, "member")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.URL)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	post := func(cookie, csrf string) int {
		t.Helper()
		client.Jar.SetCookies(u, []*http.Cookie{{Name: web.CookieName(), Value: cookie, Path: "/"}})
		form := url.Values{"csrf_token": {csrf}, "invitation_id": {i.ID}, "secret": {secret}, "user_id": {owner.User.ID}}
		r, _ := http.NewRequest("POST", srv.URL+"/invitations/accept", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", srv.URL)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	if post(wrongSecret, wrong.CSRF) != 403 {
		t.Fatal("wrong account accepted forwarded link")
	}
	if post(targetSecret, "wrong-csrf") != 403 {
		t.Fatal("invitation accepted without CSRF")
	}
	if post(targetSecret, target.CSRF) != 303 {
		t.Fatal("target acceptance failed")
	}
	list, err := web.Workspaces.List(ctx, target.User.ID)
	if err != nil || len(list) != 1 || list[0].Role != "member" {
		t.Fatal("caller identity spoof changed membership")
	}
	response, err := client.Get(srv.URL + "/invitations/accept")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("GET invitation form unavailable")
	}
	newInvite, newSecret, err := web.Workspaces.Invite(ctx, w.ID, owner.User.ID, 103, "member")
	if err != nil {
		t.Fatal(err)
	}
	client.Jar.SetCookies(u, []*http.Cookie{{Name: web.CookieName(), Value: wrongSecret, Path: "/"}})
	response, err = client.Get(srv.URL + "/invitations/accept?" + url.Values{"invitation_id": {newInvite.ID}, "secret": {newSecret}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	memberships, err := web.Workspaces.List(ctx, wrong.User.ID)
	if err != nil || len(memberships) != 0 {
		t.Fatal("GET invitation page granted membership")
	}
}

func TestIdentityHTMLIsEscaped(t *testing.T) {
	_, _, _, srv, client, fake := browserWeb(t)
	fake.mu.Lock()
	fake.username = "<script>alert(1)</script>"
	fake.mu.Unlock()
	response, err := client.Get(srv.URL + "/auth/github/login")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(body), "<script>") || !strings.Contains(string(body), "&lt;script&gt;") {
		t.Fatal("external identity HTML not escaped")
	}
}
