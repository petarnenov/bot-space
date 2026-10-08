package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

func agent(t *testing.T, ctx context.Context, s *agents.Store, w workspaces.Workspace, owner workspaces.User, name string) (agents.Agent, agents.Credential, string) {
	t.Helper()
	a, err := s.Register(ctx, w.ID, owner.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	c, token, err := s.Issue(ctx, w.ID, owner.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a, c, token
}

func TestAgentOwnershipCredentialHashesAndDeactivation(t *testing.T) {
	ctx, pool, teams, w, owner := teams(t)
	member := join(t, ctx, pool, teams, w, owner, 102, "member")
	admin := join(t, ctx, pool, teams, w, owner, 103, "admin")
	s := &agents.Store{Pool: pool}
	one, _, oneToken := agent(t, ctx, s, w, owner, "Codex on machine one")
	two, _, twoToken := agent(t, ctx, s, w, member, "Custom client on machine two")
	if one.ID == two.ID || one.OwnerUserID == two.OwnerUserID || oneToken == twoToken {
		t.Fatal("agents or credentials not independent")
	}
	random, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(twoToken, agents.TokenPrefix))
	if err != nil || len(random) != 32 {
		t.Fatal("token does not contain 32 random bytes")
	}
	if p, err := s.Authenticate(ctx, twoToken); err != nil || p.AgentID != two.ID || p.OwnerUserID != member.ID || p.WorkspaceID != w.ID {
		t.Fatal("invalid principal")
	}
	var hash string
	if pool.QueryRow(ctx, "SELECT secret_hash FROM mailbox.agent_credentials WHERE workspace_id=$1 AND agent_id=$2", w.ID, two.ID).Scan(&hash) != nil || hash != security.Hash(twoToken) || hash == twoToken {
		t.Fatal("credential not hashed")
	}
	list, err := s.Own(ctx, w.ID, member.ID)
	if err != nil || len(list) != 1 || list[0].ID != two.ID {
		t.Fatal("own-agent listing leaked another owner")
	}
	for _, actor := range []workspaces.User{owner, admin} {
		if _, _, err = s.Issue(ctx, w.ID, actor.ID, two.ID); !errors.Is(err, agents.ErrForbidden) {
			t.Fatal("administrator issued another owner's credentials")
		}
		if _, err = s.Credentials(ctx, w.ID, actor.ID, two.ID); !errors.Is(err, agents.ErrForbidden) {
			t.Fatal("administrator listed another owner's credentials")
		}
	}
	metadata, err := s.Credentials(ctx, w.ID, member.ID, two.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(metadata)
	if strings.Contains(string(encoded), twoToken) || strings.Contains(string(encoded), hash) {
		t.Fatal("secret redisplayed")
	}
	if err = s.Deactivate(ctx, w.ID, member.ID, one.ID); !errors.Is(err, agents.ErrForbidden) {
		t.Fatal("member substituted another agent ID")
	}
	for _, name := range []string{"", strings.Repeat("x", 129), "agent\nname", string([]byte{0xff})} {
		if _, err = s.Register(ctx, w.ID, member.ID, name); !errors.Is(err, agents.ErrInvalid) {
			t.Fatal("invalid display name registered")
		}
	}
	other, err := teams.Bootstrap(ctx, 201, "foreign-team")
	if err != nil {
		t.Fatal(err)
	}
	outsider := user(t, ctx, pool, 201)
	if _, err = s.Register(ctx, other.ID, member.ID, "foreign"); !errors.Is(err, agents.ErrForbidden) {
		t.Fatal("foreign registration allowed")
	}
	if err = s.Deactivate(ctx, w.ID, outsider.ID, two.ID); !errors.Is(err, agents.ErrForbidden) {
		t.Fatal("foreign administrator deactivated an agent")
	}
	_, extraToken, err := s.Issue(ctx, w.ID, member.ID, two.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Deactivate(ctx, w.ID, admin.ID, two.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Deactivate(ctx, w.ID, admin.ID, two.ID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{twoToken, extraToken} {
		if _, err = s.Authenticate(ctx, token); err == nil {
			t.Fatal("deactivated credential still valid")
		}
	}
	if _, _, err = s.Issue(ctx, w.ID, member.ID, two.ID); !errors.Is(err, agents.ErrConflict) {
		t.Fatal("inactive issuance allowed")
	}
	var exists, active bool
	if pool.QueryRow(ctx, "SELECT true,active FROM mailbox.agents WHERE workspace_id=$1 AND id=$2", w.ID, two.ID).Scan(&exists, &active) != nil || !exists || active {
		t.Fatal("historical agent deleted or active")
	}
	var deactivations int
	if pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.audit_events WHERE workspace_id=$1 AND action='agent_deactivated' AND target_id=$2", w.ID, two.ID).Scan(&deactivations) != nil || deactivations != 1 {
		t.Fatal("duplicate deactivation audit")
	}
	var registrations, issued, revoked int
	if pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE action='agent_registered'),count(*) FILTER (WHERE action='credential_issued'),count(*) FILTER (WHERE action='credential_revoked') FROM mailbox.audit_events WHERE workspace_id=$1", w.ID).Scan(&registrations, &issued, &revoked) != nil || registrations != 2 || issued != 3 || revoked != 2 {
		t.Fatal("agent lifecycle audit mismatch")
	}
}

func TestCredentialRotationRevocationAndAudit(t *testing.T) {
	ctx, pool, teams, w, owner := teams(t)
	member := join(t, ctx, pool, teams, w, owner, 102, "member")
	s := &agents.Store{Pool: pool}
	a, first, oldToken := agent(t, ctx, s, w, member, "worker")
	second, unchangedToken, err := s.Issue(ctx, w.ID, member.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Rotate(ctx, w.ID, owner.ID, a.ID, first.ID); !errors.Is(err, agents.ErrForbidden) {
		t.Fatal("owner rotated another member's credentials")
	}
	replacement, newToken, err := s.Rotate(ctx, w.ID, member.ID, a.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == first.ID || newToken == oldToken {
		t.Fatal("rotation reused token or ID")
	}
	if _, err = s.Authenticate(ctx, oldToken); err == nil {
		t.Fatal("old token remains active")
	}
	for _, value := range []string{newToken, unchangedToken} {
		if _, err = s.Authenticate(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = s.Rotate(ctx, w.ID, member.ID, a.ID, first.ID); !errors.Is(err, agents.ErrConflict) {
		t.Fatal("revoked token rotated")
	}
	for i := 0; i < 2; i++ {
		if err = s.Revoke(ctx, w.ID, member.ID, a.ID, second.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Authenticate(ctx, unchangedToken); err == nil {
		t.Fatal("revoked token active")
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.audit_events WHERE workspace_id=$1 AND action='credential_revoked' AND target_id=$2", w.ID, second.ID).Scan(&count) != nil || count != 1 {
		t.Fatal("duplicate revoke audit")
	}
	var secrets bool
	if pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mailbox.audit_events WHERE workspace_id=$1 AND (row_to_json(audit_events)::text LIKE $2 OR row_to_json(audit_events)::text LIKE $3))", w.ID, "%"+newToken+"%", "%"+security.Hash(newToken)+"%").Scan(&secrets) != nil || secrets {
		t.Fatal("secret audit material")
	}
	if err = s.Deactivate(ctx, w.ID, member.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Rotate(ctx, w.ID, member.ID, a.ID, replacement.ID); !errors.Is(err, agents.ErrConflict) {
		t.Fatal("inactive agent rotated")
	}
}

func TestAgentHTTPAuthenticationBoundary(t *testing.T) {
	ctx, pool, _, w, owner := teams(t)
	s := &agents.Store{Pool: pool}
	a, credential, token := agent(t, ctx, s, w, owner, "probe")
	_, browserSecret, err := (&identity.Sessions{Pool: pool}).Login(ctx, owner.GitHubID, "probe-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	var called atomic.Int32
	srv := httptest.NewServer(s.Middleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		called.Add(1)
		p, ok := agents.FromContext(r.Context())
		if !ok {
			t.Error("missing principal")
		}
		_ = json.NewEncoder(rw).Encode(p)
	})))
	defer srv.Close()
	request := func(headers []string, cookie bool) (int, string) {
		t.Helper()
		r, _ := http.NewRequest("GET", srv.URL+"?from_agent_id=spoof&workspace_id=foreign", nil)
		for _, h := range headers {
			r.Header.Add("Authorization", h)
		}
		if cookie {
			r.AddCookie(&http.Cookie{Name: "bot_space_session", Value: browserSecret})
		}
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(body)
	}
	for _, headers := range [][]string{nil, {"Basic secret"}, {"Bearer unknown"}, {"Bearer " + agents.TokenPrefix + strings.Repeat("x", 43)}, {"Bearer " + token, "Bearer " + token}} {
		status, _ := request(headers, true)
		if status != 401 {
			t.Fatal("invalid credentials accepted")
		}
	}
	status, body := request([]string{"Bearer " + token}, false)
	var principal agents.Principal
	if status != 200 || json.Unmarshal([]byte(body), &principal) != nil || principal.AgentID != a.ID || principal.WorkspaceID != w.ID || strings.Contains(body, "spoof") {
		t.Fatal("request-controlled principal")
	}
	if called.Load() != 1 {
		t.Fatal("unauthorized protected handler ran")
	}
	if err := s.Revoke(ctx, w.ID, owner.ID, a.ID, credential.ID); err != nil {
		t.Fatal(err)
	}
	if status, _ := request([]string{"Bearer " + token}, false); status != 401 {
		t.Fatal("revoked HTTP request accepted")
	}
	pool.Close()
	if status, _ := request([]string{"Bearer " + token}, false); status != 503 {
		t.Fatal("database failure did not deny access")
	}
}

func TestRemovalPermanentRevocationAndWorkspaceIsolation(t *testing.T) {
	ctx, pool, teams, w, owner := teams(t)
	member := join(t, ctx, pool, teams, w, owner, 102, "member")
	other, err := teams.Bootstrap(ctx, 102, "another-team")
	if err != nil {
		t.Fatal(err)
	}
	s := &agents.Store{Pool: pool}
	one, _, token := agent(t, ctx, s, w, member, "first workspace")
	_, _, secondToken := agent(t, ctx, s, w, member, "another first-workspace agent")
	_, extraToken, err := s.Issue(ctx, w.ID, member.ID, one.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, otherToken := agent(t, ctx, s, other, member, "second workspace")
	if err = teams.Remove(ctx, w.ID, owner.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{token, secondToken, extraToken} {
		if _, err = s.Authenticate(ctx, value); err == nil {
			t.Fatal("removed member token valid")
		}
	}
	if _, err = s.Authenticate(ctx, otherToken); err != nil {
		t.Fatal("other-workspace token affected")
	}
	i, secret, err := teams.Invite(ctx, w.ID, owner.ID, member.GitHubID, "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = teams.AcceptInvitation(ctx, member.ID, i.ID, secret); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{token, secondToken, extraToken} {
		if _, err = s.Authenticate(ctx, value); err == nil {
			t.Fatal("rejoin revived token")
		}
	}
	var active bool
	if pool.QueryRow(ctx, "SELECT active FROM mailbox.agents WHERE workspace_id=$1 AND id=$2", w.ID, one.ID).Scan(&active) != nil || active {
		t.Fatal("historical agent lost or reactivated")
	}
	var auditCount int
	if pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.audit_events WHERE workspace_id=$1 AND action IN ('agent_deactivated_by_member_removal','credential_revoked_by_member_removal') AND actor_user_id=$2", w.ID, owner.ID).Scan(&auditCount) != nil || auditCount != 5 {
		t.Fatal("removal lifecycle audit missing")
	}
}

func TestCredentialTransactionLockAndStaleContext(t *testing.T) {
	for _, action := range []string{"revoke", "deactivate", "remove"} {
		t.Run(action, func(t *testing.T) {
			ctx, pool, teams, w, owner := teams(t)
			member := join(t, ctx, pool, teams, w, owner, 102, "member")
			s := &agents.Store{Pool: pool}
			a, c, token := agent(t, ctx, s, w, member, "locking")
			p, err := s.Authenticate(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err = s.RecheckTx(ctx, tx, p); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				switch action {
				case "revoke":
					result <- s.Revoke(ctx, w.ID, member.ID, a.ID, c.ID)
				case "deactivate":
					result <- s.Deactivate(ctx, w.ID, owner.ID, a.ID)
				case "remove":
					result <- teams.Remove(ctx, w.ID, owner.ID, member.ID)
				}
			}()
			deadline := time.Now().Add(2 * time.Second)
			locked := false
			for time.Now().Before(deadline) {
				_, _ = tx.Exec(ctx, "SELECT pg_stat_clear_snapshot()")
				if tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock')").Scan(&locked) != nil {
					t.Fatal("cannot inspect lock wait")
				}
				if locked {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !locked {
				t.Fatal("access termination did not wait for authorization lock")
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; err != nil {
				t.Fatal(err)
			}
			if _, err = s.Authenticate(ctx, token); err == nil {
				t.Fatal("terminated token accepted")
			}
			check, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer check.Rollback(context.Background())
			if _, err = s.RecheckTx(ctx, check, p); !errors.Is(err, agents.ErrUnauthenticated) {
				t.Fatal("stale principal reused")
			}
		})
	}
}

func TestAgentAuditFailureRollsBack(t *testing.T) {
	ctx, pool, teams, w, owner := teams(t)
	member := join(t, ctx, pool, teams, w, owner, 102, "member")
	s := &agents.Store{Pool: pool}
	a, c, token := agent(t, ctx, s, w, member, "audited")
	_, err := pool.Exec(ctx, `CREATE FUNCTION mailbox.reject_agent_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$;
	CREATE TRIGGER reject_agent_test_audit BEFORE INSERT ON mailbox.audit_events FOR EACH ROW EXECUTE FUNCTION mailbox.reject_agent_test_audit();`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Register(ctx, w.ID, member.ID, "rejected"); !errors.Is(err, agents.ErrUnavailable) {
		t.Fatal("registration ignored audit failure")
	}
	if _, _, err = s.Rotate(ctx, w.ID, member.ID, a.ID, c.ID); !errors.Is(err, agents.ErrUnavailable) {
		t.Fatal("rotation ignored audit failure")
	}
	if err = s.Deactivate(ctx, w.ID, member.ID, a.ID); !errors.Is(err, agents.ErrUnavailable) {
		t.Fatal("deactivation ignored audit failure")
	}
	if err = teams.Remove(ctx, w.ID, owner.ID, member.ID); !errors.Is(err, workspaces.ErrUnavailable) {
		t.Fatal("removal ignored audit failure")
	}
	if _, err = s.Authenticate(ctx, token); err != nil {
		t.Fatal("failed audit partially revoked access")
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.agents WHERE workspace_id=$1 AND owner_user_id=$2", w.ID, member.ID).Scan(&count) != nil || count != 1 {
		t.Fatal("failed registration committed")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.agent_credentials WHERE workspace_id=$1 AND agent_id=$2", w.ID, a.ID).Scan(&count) != nil || count != 1 {
		t.Fatal("failed rotation issued token")
	}
}
