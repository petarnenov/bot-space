package integration

import (
	"bytes"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpclient"
)

func getBody(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	r, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(body)
}
func field(t *testing.T, body, name string) string {
	t.Helper()
	match := regexp.MustCompile("name=\"" + name + "\"[^>]*value=\"([^\"]*)\"").FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("form field missing: %s", name)
	}
	return html.UnescapeString(match[1])
}
func codeField(t *testing.T, body, id string) string {
	t.Helper()
	match := regexp.MustCompile("<code id=\"" + id + "\">([^<]+)</code>").FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("result field missing: %s", id)
	}
	return html.UnescapeString(match[1])
}
func formPost(t *testing.T, c *http.Client, base, path, csrf string, values url.Values) (int, string) {
	t.Helper()
	if values == nil {
		values = url.Values{}
	}
	values.Set("csrf_token", csrf)
	req, _ := http.NewRequest("POST", base+path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", base)
	r, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(body)
}
func noRedirect(c *http.Client) {
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
}
func newBrowser() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }

func TestCompleteBrowserInvitationAgentsAndMCPAcceptance(t *testing.T) {
	ctx, pool, web, srv, ownerClient, fake := browserWeb(t)
	w, err := web.Workspaces.Bootstrap(ctx, 101, "complete-team")
	if err != nil {
		t.Fatal(err)
	}
	ownerBody := loginBrowser(t, ownerClient, srv.URL)
	ownerCSRF := field(t, ownerBody, "csrf_token")
	noRedirect(ownerClient)
	_, ownerPage := getBody(t, ownerClient, srv.URL+"/workspaces/"+w.ID)
	if !strings.Contains(ownerPage, "Members and roles") || !strings.Contains(ownerPage, "Create invitation") || !strings.Contains(ownerPage, "Your agents") {
		t.Fatal("management sections missing")
	}
	status, created := formPost(t, ownerClient, srv.URL, "/workspaces/"+w.ID+"/invitations", ownerCSRF, url.Values{"github_user_id": {"102"}, "role": {"member"}})
	if status != 200 {
		t.Fatal("browser invitation creation failed")
	}
	id, secret := codeField(t, created, "invitation-id"), codeField(t, created, "invitation-secret")
	link := srv.URL + "/invitations/accept?" + url.Values{"invitation_id": {id}, "secret": {secret}}.Encode()
	// A forwarded link does not consume or grant access to another account.
	wrongClient := newBrowser()
	fake.mu.Lock()
	fake.id = 103
	fake.username = "wrong-account"
	fake.mu.Unlock()
	wrongBody := loginBrowser(t, wrongClient, srv.URL)
	wrongCSRF := field(t, wrongBody, "csrf_token")
	noRedirect(wrongClient)
	if status, _ := getBody(t, wrongClient, link); status != 200 {
		t.Fatal("invitation view failed")
	}
	if status, _ := formPost(t, wrongClient, srv.URL, "/invitations/accept", wrongCSRF, url.Values{"invitation_id": {id}, "secret": {secret}}); status != 403 {
		t.Fatal("forwarded invitation accepted")
	}
	memberClient := newBrowser()
	if status, _ := getBody(t, memberClient, link); status != 200 {
		t.Fatal("prelogin invitation page unavailable")
	}
	fake.mu.Lock()
	fake.id = 102
	fake.username = "invited-member"
	fake.mu.Unlock()
	loginBrowser(t, memberClient, srv.URL)
	_, acceptPage := getBody(t, memberClient, srv.URL+"/invitations/accept")
	if field(t, acceptPage, "invitation_id") != id || field(t, acceptPage, "secret") != secret {
		t.Fatal("invitation not carried through login")
	}
	memberCSRF := field(t, acceptPage, "csrf_token")
	noRedirect(memberClient)
	if status, _ := formPost(t, memberClient, srv.URL, "/invitations/accept", memberCSRF, url.Values{"invitation_id": {id}, "secret": {secret}}); status != 303 {
		t.Fatal("target invitation acceptance failed")
	}
	_, memberPage := getBody(t, memberClient, srv.URL+"/workspaces/"+w.ID)
	if strings.Contains(memberPage, "Create invitation") || strings.Contains(memberPage, "Change role") || strings.Contains(memberPage, "Team agent access") {
		t.Fatal("member displayed administration controls")
	}
	_, ownerPage = getBody(t, ownerClient, srv.URL+"/workspaces/"+w.ID)
	if strings.Contains(ownerPage, secret) {
		t.Fatal("invitation secret redisplayed")
	}
	createAgent := func(client *http.Client, csrf, name string) (agents.Agent, string) {
		status, _ := formPost(t, client, srv.URL, "/workspaces/"+w.ID+"/agents", csrf, url.Values{"name": {name}})
		if status != 200 {
			t.Fatal("agent registration form failed")
		}
		session, err := web.Sessions.Authenticate(ctx, browserSecret(t, client, srv.URL, web.CookieName()))
		if err != nil {
			t.Fatal(err)
		}
		owned, err := (&agents.Store{Pool: pool}).Own(ctx, w.ID, session.User.ID)
		if err != nil || len(owned) != 1 {
			t.Fatal("registered ownership mismatch")
		}
		status, body := formPost(t, client, srv.URL, "/workspaces/"+w.ID+"/agents/"+owned[0].ID+"/credentials", csrf, nil)
		if status != 200 {
			t.Fatal("credential form failed")
		}
		return owned[0], codeField(t, body, "issued-token")
	}
	a, ta := createAgent(ownerClient, ownerCSRF, "owner worker")
	b, tb := createAgent(memberClient, memberCSRF, "member worker")
	alpha, err := mcpclient.Connect(ctx, srv.URL+"/mcp", ta)
	if err != nil {
		t.Fatal(err)
	}
	defer alpha.Close()
	beta, err := mcpclient.Connect(ctx, srv.URL+"/mcp", tb)
	if err != nil {
		t.Fatal(err)
	}
	defer beta.Close()
	var message mailbox.Message
	if err = alpha.Call(ctx, "send_message", map[string]any{"to_agent_id": b.ID, "idempotency_key": "complete-request", "text": "private-management-body-sentinel"}, &message); err != nil {
		t.Fatal(err)
	}
	var inbox mailbox.Page
	if err = beta.Call(ctx, "read_messages", map[string]any{}, &inbox); err != nil || len(inbox.Messages) != 1 {
		t.Fatal("complete HTTP inbox exchange failed")
	}
	var acknowledged mailbox.Message
	if err = beta.Call(ctx, "acknowledge_message", map[string]any{"message_id": message.ID}, &acknowledged); err != nil {
		t.Fatal(err)
	}
	var reply mailbox.Message
	if err = beta.Call(ctx, "send_message", map[string]any{"to_agent_id": a.ID, "idempotency_key": "complete-reply", "text": "synthetic reply", "in_reply_to": message.ID}, &reply); err != nil {
		t.Fatal(err)
	}
	if err = alpha.Call(ctx, "read_messages", map[string]any{}, &inbox); err != nil || len(inbox.Messages) != 1 || inbox.Messages[0].ID != reply.ID {
		t.Fatal("complete reply failed")
	}
	_, ownerPage = getBody(t, ownerClient, srv.URL+"/workspaces/"+w.ID)
	for _, private := range []string{message.Text, ta, tb, secret} {
		if strings.Contains(ownerPage, private) {
			t.Fatal("administration exposed inbox or secret content")
		}
	}
	// Native access is independent of browser session and cannot reach another team.
	other, err := web.Workspaces.Bootstrap(ctx, 201, "isolated-team")
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := getBody(t, memberClient, srv.URL+"/workspaces/"+other.ID); status != 403 {
		t.Fatal("foreign workspace page exposed")
	}
	metadata, err := (&agents.Store{Pool: pool}).Credentials(ctx, w.ID, b.OwnerUserID, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := formPost(t, memberClient, srv.URL, "/workspaces/"+w.ID+"/agents/"+b.ID+"/credentials/"+metadata[0].ID+"/revoke", memberCSRF, nil); status != 303 {
		t.Fatal("browser revocation failed")
	}
	if err = beta.Call(ctx, "whoami", map[string]any{}, &agents.Principal{}); err == nil {
		t.Fatal("revoked browser-issued credential retained MCP access")
	}
}

func TestManagementRoleCSRFAndEscaping(t *testing.T) {
	ctx, pool, web, srv, ownerClient, _ := browserWeb(t)
	w, err := web.Workspaces.Bootstrap(ctx, 101, "role-ui")
	if err != nil {
		t.Fatal(err)
	}
	body := loginBrowser(t, ownerClient, srv.URL)
	csrf := field(t, body, "csrf_token")
	noRedirect(ownerClient)
	adminSession, adminCookie, err := web.Sessions.Login(ctx, 102, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	i, secret, err := web.Workspaces.Invite(ctx, w.ID, adminSession.User.ID, 103, "member")
	_ = i
	_ = secret
	if err == nil {
		t.Fatal("unjoined admin session invited")
	}
	ownerSession, err := web.Sessions.Authenticate(ctx, browserSecret(t, ownerClient, srv.URL, web.CookieName()))
	if err != nil {
		t.Fatal(err)
	}
	i, secret, err = web.Workspaces.Invite(ctx, w.ID, ownerSession.User.ID, 102, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = web.Workspaces.AcceptInvitation(ctx, adminSession.User.ID, i.ID, secret); err != nil {
		t.Fatal(err)
	}
	adminClient := newBrowser()
	u, _ := url.Parse(srv.URL)
	adminClient.Jar.SetCookies(u, []*http.Cookie{{Name: web.CookieName(), Value: adminCookie, Path: "/"}})
	noRedirect(adminClient)
	_, adminPage := getBody(t, adminClient, srv.URL+"/workspaces/"+w.ID)
	if strings.Contains(adminPage, "value=\"owner\"") {
		t.Fatal("admin displayed owner grant")
	}
	path := "/workspaces/" + w.ID + "/members/" + adminSession.User.ID + "/role"
	if status, _ := formPost(t, adminClient, srv.URL, path, adminSession.CSRF, url.Values{"role": {"owner"}}); status != 403 {
		t.Fatal("admin self-owner promotion")
	}
	if status, _ := formPost(t, ownerClient, srv.URL, "/workspaces/"+w.ID+"/members/"+ownerSession.User.ID+"/role", csrf, url.Values{"role": {"member"}}); status != 409 {
		t.Fatal("last-owner UI demotion")
	}
	if status, _ := formPost(t, ownerClient, srv.URL, path, "wrong-csrf", url.Values{"role": {"member"}}); status != 403 {
		t.Fatal("missing UI CSRF check")
	}
	ownerRemove := "/workspaces/" + w.ID + "/members/" + ownerSession.User.ID + "/remove"
	if status, _ := formPost(t, adminClient, srv.URL, ownerRemove, adminSession.CSRF, nil); status != 403 {
		t.Fatal("admin removed an owner")
	}
	if status, _ := formPost(t, ownerClient, srv.URL, ownerRemove, csrf, nil); status != 409 {
		t.Fatal("browser removed last owner")
	}
	if status, _ := formPost(t, ownerClient, srv.URL, path, csrf, url.Values{"role": {"member"}}); status != 303 {
		t.Fatal("authorized role change failed")
	}
	if status, _ := formPost(t, ownerClient, srv.URL, "/workspaces/"+w.ID+"/members/"+adminSession.User.ID+"/remove", csrf, nil); status != 303 {
		t.Fatal("authorized member removal failed")
	}
	if status, _ := getBody(t, adminClient, srv.URL+"/workspaces/"+w.ID); status != 403 {
		t.Fatal("removed member still sees workspace")
	}
	store := &agents.Store{Pool: pool}
	if _, err = store.Register(ctx, w.ID, ownerSession.User.ID, "<script>evil</script>"); err != nil {
		t.Fatal(err)
	}
	_, page := getBody(t, ownerClient, srv.URL+"/workspaces/"+w.ID)
	if strings.Contains(page, "<script>evil") || !strings.Contains(page, "&lt;script&gt;evil") {
		t.Fatal("agent name not escaped")
	}
	response, err := ownerClient.Get(srv.URL + "/assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	style, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || !bytes.Contains(style, []byte("system-ui")) {
		t.Fatal("embedded stylesheet unavailable")
	}
}

func TestManagementInvitationCancellationExpiryAndReplay(t *testing.T) {
	ctx, pool, web, srv, ownerClient, _ := browserWeb(t)
	w, err := web.Workspaces.Bootstrap(ctx, 101, "invite-lifecycle-ui")
	if err != nil {
		t.Fatal(err)
	}
	ownerBody := loginBrowser(t, ownerClient, srv.URL)
	csrf := field(t, ownerBody, "csrf_token")
	noRedirect(ownerClient)
	target, cookie, err := web.Sessions.Login(ctx, 102, "target", "")
	if err != nil {
		t.Fatal(err)
	}
	targetClient := newBrowser()
	u, _ := url.Parse(srv.URL)
	targetClient.Jar.SetCookies(u, []*http.Cookie{{Name: web.CookieName(), Value: cookie, Path: "/"}})
	noRedirect(targetClient)
	for _, mode := range []string{"cancelled", "expired", "accepted"} {
		status, body := formPost(t, ownerClient, srv.URL, "/workspaces/"+w.ID+"/invitations", csrf, url.Values{"github_user_id": {"102"}, "role": {"member"}})
		if status != 200 {
			t.Fatal("invitation creation failed")
		}
		id, secret := codeField(t, body, "invitation-id"), codeField(t, body, "invitation-secret")
		r, err := ownerClient.Get(srv.URL + "/invitations/accept?" + url.Values{"invitation_id": {id}, "secret": {secret}}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("invitation page lacks secret handling headers")
		}
		switch mode {
		case "cancelled":
			if code, _ := formPost(t, ownerClient, srv.URL, "/workspaces/"+w.ID+"/invitations/"+id+"/cancel", csrf, nil); code != 303 {
				t.Fatal("browser cancellation failed")
			}
		case "expired":
			if _, err := pool.Exec(ctx, "UPDATE mailbox.invitations SET created_at=now()-interval '3 days',expires_at=now()-interval '1 day' WHERE id=$1", id); err != nil {
				t.Fatal(err)
			}
		}
		values := url.Values{"invitation_id": {id}, "secret": {secret}}
		expected := 403
		if mode == "accepted" {
			expected = 303
		}
		if code, _ := formPost(t, targetClient, srv.URL, "/invitations/accept", target.CSRF, values); code != expected {
			t.Fatalf("%s invitation returned %d", mode, code)
		}
		if mode == "accepted" {
			if code, _ := formPost(t, targetClient, srv.URL, "/invitations/accept", target.CSRF, values); code != 403 {
				t.Fatal("browser invitation replay accepted")
			}
		} else {
			memberships, err := web.Workspaces.List(ctx, target.User.ID)
			if err != nil || len(memberships) != 0 {
				t.Fatal("invalid invitation granted membership")
			}
		}
		_, listing := getBody(t, ownerClient, srv.URL+"/workspaces/"+w.ID)
		if strings.Contains(listing, secret) {
			t.Fatal("invitation listing redisplays its secret")
		}
	}
}
