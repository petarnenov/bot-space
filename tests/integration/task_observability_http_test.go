package integration

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
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
	"github.com/petarnenov/bot-space/internal/tasks"
	"github.com/petarnenov/bot-space/internal/taskweb"
	management "github.com/petarnenov/bot-space/internal/web"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

func browserTaskWeb(t *testing.T) (context.Context, *pgxpool.Pool, *identity.Web, *httptest.Server, *workspaces.Store, *agents.Store, *tasks.Store) {
	t.Helper()
	ctx, dsn, pool := isolated(t)
	if err := database.Migrate(ctx, dsn, bundle(t)); err != nil {
		t.Fatal(err)
	}
	server := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, pool, bundle(t)) }, slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)))
	webServer := httptest.NewUnstartedServer(server.HTTP.Handler)
	web := &identity.Web{
		Config:     config.Identity{Enabled: true, ClientID: "x", ClientSecret: "y", BaseURL: "http://" + webServer.Listener.Addr().String()},
		Sessions:   &identity.Sessions{Pool: pool},
		Workspaces: &workspaces.Store{Pool: pool},
		Provider:   identity.GitHubProvider(),
	}
	web.Register(server)
	agentsStore := &agents.Store{Pool: pool}
	teams := &workspaces.Store{Pool: pool}
	(&agents.Web{Store: agentsStore}).Register(server, web)
	(&management.Management{Browser: web, Teams: teams, Agents: agentsStore, MailboxEnabled: true}).Register(server)
	mailboxStore, err := mailbox.New(pool, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	taskStore := tasks.New(mailboxStore)
	(&taskweb.Web{Browser: web, Tasks: taskStore}).Register(server)
	server.Handle("/mcp", mcpserver.NewWithTasks(mailboxStore, []string{web.Config.BaseURL}))
	webServer.Start()
	t.Cleanup(webServer.Close)
	return ctx, pool, web, webServer, teams, agentsStore, taskStore
}

func browserWithSession(t *testing.T, baseURL, cookieName, secret string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	u, _ := url.Parse(baseURL)
	jar.SetCookies(u, []*http.Cookie{{Name: cookieName, Value: secret, Path: "/"}})
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

func TestTaskObservabilityPagesControlsAndPrivacy(t *testing.T) {
	ctx, pool, web, srv, teams, agentStore, taskStore := browserTaskWeb(t)
	ownerSession, ownerSecret, err := web.Sessions.Login(ctx, 101, "owner", "")
	if err != nil {
		t.Fatal(err)
	}
	memberSession, memberSecret, err := web.Sessions.Login(ctx, 102, "member", "")
	if err != nil {
		t.Fatal(err)
	}
	adminSession, adminSecret, err := web.Sessions.Login(ctx, 103, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := teams.Bootstrap(ctx, 101, "task-observability")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO mailbox.memberships (workspace_id,user_id,role) VALUES ($1,$2,'member'),($1,$3,'admin')", workspace.ID, memberSession.User.ID, adminSession.User.ID); err != nil {
		t.Fatal(err)
	}
	from, err := agentStore.Register(ctx, workspace.ID, ownerSession.User.ID, "owner-agent")
	if err != nil {
		t.Fatal(err)
	}
	to, err := agentStore.Register(ctx, workspace.ID, memberSession.User.ID, "member-agent")
	if err != nil {
		t.Fatal(err)
	}
	_, fromToken, err := agentStore.Issue(ctx, workspace.ID, ownerSession.User.ID, from.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, toToken, err := agentStore.Issue(ctx, workspace.ID, memberSession.User.ID, to.ID)
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := taskStore.Submit(ctx, fromToken, tasks.SubmitInput{
		ToAgentID:      to.ID,
		IdempotencyKey: "http-observability-1",
		Instruction:    "private-task-body-sentinel <script>alert(1)</script>",
	})
	if err != nil {
		t.Fatal(err)
	}
	ownership, err := taskStore.Heartbeat(ctx, toToken, "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := taskStore.Claim(ctx, toToken, "11111111-1111-4111-8111-111111111111", ownership.Generation)
	if err != nil || claimed == nil {
		t.Fatal("claim unavailable")
	}
	if _, err = taskStore.Complete(ctx, toToken, tasks.CompleteInput{
		TaskID:           claimed.ID,
		RunnerID:         "11111111-1111-4111-8111-111111111111",
		RunnerGeneration: ownership.Generation,
		Generation:       claimed.Generation,
		Result:           "private-task-result-sentinel <b>unsafe</b>",
		ErrorCode:        "requires_approval",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE mailbox.runner_ownership SET lease_until=clock_timestamp()-interval '1 second', last_seen_at=clock_timestamp()-interval '2 minutes' WHERE workspace_id=$1 AND agent_id=$2", workspace.ID, to.ID); err != nil {
		t.Fatal(err)
	}
	ownerClient := browserWithSession(t, srv.URL, web.CookieName(), ownerSecret)
	memberClient := browserWithSession(t, srv.URL, web.CookieName(), memberSecret)
	adminClient := browserWithSession(t, srv.URL, web.CookieName(), adminSecret)

	listResp, err := ownerClient.Get(srv.URL + "/workspaces/" + workspace.ID + "/tasks")
	if err != nil {
		t.Fatal(err)
	}
	listBody := readBody(t, listResp.Body)
	listResp.Body.Close()
	if listResp.StatusCode != 200 || listResp.Header.Get("Cache-Control") != "no-store" || listResp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("task list missing protected rendering")
	}
	if !strings.Contains(listBody, "offline") {
		t.Fatal("offline recipient state not shown")
	}
	if !strings.Contains(listBody, "Primary navigation") || !strings.Contains(listBody, "/profile") || !strings.Contains(listBody, "/settings") || !strings.Contains(listBody, "/workspaces/"+workspace.ID+"/settings") {
		t.Fatal("task navigation is incomplete")
	}
	if strings.Contains(listBody, "<script>alert(1)</script>") {
		t.Fatal("task instruction not escaped in list")
	}

	detailResp, err := memberClient.Get(srv.URL + "/workspaces/" + workspace.ID + "/tasks/" + submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	detailBody := readBody(t, detailResp.Body)
	detailResp.Body.Close()
	if detailResp.StatusCode != 200 || !strings.Contains(detailBody, "Approve retry") {
		t.Fatal("task detail/review control unavailable to participant")
	}
	if strings.Contains(detailBody, "<script>alert(1)</script>") || strings.Contains(detailBody, "<b>unsafe</b>") {
		t.Fatal("task body/result not escaped")
	}

	if code, _ := getBody(t, adminClient, srv.URL+"/workspaces/"+workspace.ID+"/tasks/"+submitted.ID); code != 403 {
		t.Fatal("unrelated administrator accessed private task")
	}
	if status, _ := formPost(t, memberClient, srv.URL, "/workspaces/"+workspace.ID+"/tasks/"+submitted.ID+"/review", "bad", url.Values{"action": {"retry"}}); status != 403 {
		t.Fatal("review accepted invalid CSRF")
	}
	if status, _ := formPost(t, memberClient, srv.URL, "/workspaces/"+workspace.ID+"/tasks/"+submitted.ID+"/review", memberSession.CSRF, url.Values{"action": {"retry"}}); status != 303 {
		t.Fatal("review retry rejected")
	}
	taskAfterReview, err := taskStore.Get(ctx, fromToken, submitted.ID)
	if err != nil || taskAfterReview.Status != "queued" {
		t.Fatal("review retry did not requeue task")
	}
	if status, _ := formPost(t, ownerClient, srv.URL, "/workspaces/"+workspace.ID+"/tasks/"+submitted.ID+"/cancel", ownerSession.CSRF, nil); status != 303 {
		t.Fatal("participant cancellation rejected")
	}
	taskAfterCancel, err := taskStore.Get(ctx, fromToken, submitted.ID)
	if err != nil || taskAfterCancel.Status != "cancelled" {
		t.Fatal("cancel control did not update task")
	}
	var auditRows []string
	rows, err := pool.Query(ctx, "SELECT metadata::text FROM mailbox.audit_events WHERE target_id=$1 AND action IN ('task_review_retry','task_cancelled') ORDER BY created_at", submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var metadata string
		if rows.Scan(&metadata) != nil {
			t.Fatal("audit read failed")
		}
		auditRows = append(auditRows, metadata)
	}
	rows.Close()
	if len(auditRows) < 2 {
		t.Fatal("expected review and cancellation audit rows")
	}
	for _, metadata := range auditRows {
		if strings.Contains(metadata, "private-task-body-sentinel") || strings.Contains(metadata, "private-task-result-sentinel") || strings.Contains(metadata, fromToken) || strings.Contains(metadata, toToken) {
			t.Fatal("audit stored task body or secret")
		}
	}
}

func readBody(t *testing.T, body io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
