package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpserver"
	"github.com/petarnenov/bot-space/internal/tasks"
	"github.com/petarnenov/bot-space/internal/workspaces"
	"github.com/petarnenov/bot-space/migrations"
)

var runnerTestDBSeq uint64

func isolatedDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run real PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	seq := atomic.AddUint64(&runnerTestDBSeq, 1)
	name := fmt.Sprintf("bot_space_runner_test_%d_%d", time.Now().UnixNano(), seq)
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_, _ = admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		_ = admin.Close(cleanup)
	})
	replaced := replaceDB(url, name)
	bundle, err := migrations.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, replaced, bundle); err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(ctx, replaced, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func replaceDB(dsn, name string) string {
	parsed, _ := pgx.ParseConfig(dsn)
	parsed.Database = name
	return parsed.ConnString()
}

func TestTwoIndependentSupervisorsDelegateAndContinue(t *testing.T) {
	t.Run("copilot_to_copilot", func(t *testing.T) {
		testTwoIndependentSupervisorsDelegateAndContinue(t, "copilot", "copilot")
	})
	t.Run("codex_to_copilot", func(t *testing.T) {
		testTwoIndependentSupervisorsDelegateAndContinue(t, "codex", "copilot")
	})
	t.Run("copilot_to_codex", func(t *testing.T) {
		testTwoIndependentSupervisorsDelegateAndContinue(t, "copilot", "codex")
	})
}

func testTwoIndependentSupervisorsDelegateAndContinue(t *testing.T, providerA, providerB string) {
	t.Helper()
	ctx, pool := isolatedDB(t)
	teams := &workspaces.Store{Pool: pool}
	seq := atomic.AddUint64(&runnerTestDBSeq, 1)
	ownerGitHubID := int64(1000000 + seq*10)
	memberGitHubID := ownerGitHubID + 1
	workspaceSlug := fmt.Sprintf("runner-processes-%d", seq)
	ownerWorkspace, err := teams.Bootstrap(ctx, ownerGitHubID, workspaceSlug)
	if err != nil {
		t.Fatal(err)
	}
	var ownerID string
	if err = pool.QueryRow(ctx, "SELECT id::text FROM mailbox.users WHERE github_id=$1", ownerGitHubID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	var memberID string
	if err = pool.QueryRow(ctx, "INSERT INTO mailbox.users (github_id,username) VALUES ($1,'member') ON CONFLICT (github_id) DO UPDATE SET github_id=EXCLUDED.github_id RETURNING id::text", memberGitHubID).Scan(&memberID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO mailbox.memberships (workspace_id,user_id,role) VALUES ($1,$2,'member') ON CONFLICT (workspace_id,user_id) DO NOTHING", ownerWorkspace.ID, memberID); err != nil {
		t.Fatal(err)
	}
	agentsStore := &agents.Store{Pool: pool}
	ownerAgent, err := agentsStore.Register(ctx, ownerWorkspace.ID, ownerID, "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	memberAgent, err := agentsStore.Register(ctx, ownerWorkspace.ID, memberID, "member-b")
	if err != nil {
		t.Fatal(err)
	}
	_, ownerToken, err := agentsStore.Issue(ctx, ownerWorkspace.ID, ownerID, ownerAgent.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, memberToken, err := agentsStore.Issue(ctx, ownerWorkspace.ID, memberID, memberAgent.ID)
	if err != nil {
		t.Fatal(err)
	}
	store, err := mailbox.New(pool, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(mcpserver.NewWithTasks(store, nil))
	defer server.Close()

	root := t.TempDir()
	projectA := filepath.Join(root, "project-a")
	projectB := filepath.Join(root, "project-b")
	stateA := filepath.Join(root, "state-a")
	stateB := filepath.Join(root, "state-b")
	for _, path := range []string{projectA, projectB} {
		if err = os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("RUNNER_OWNER_TOKEN", ownerToken)
	t.Setenv("RUNNER_MEMBER_TOKEN", memberToken)
	configA := Config{
		Endpoint: server.URL + "/mcp",
		StateDir: stateA,
		Agents: []AgentConfig{{
			ID:             ownerAgent.ID,
			TokenEnv:       "RUNNER_OWNER_TOKEN",
			Provider:       providerA,
			Project:        projectA,
			AllowedSenders: []string{memberAgent.ID},
		}},
	}
	configB := Config{
		Endpoint: server.URL + "/mcp",
		StateDir: stateB,
		Agents: []AgentConfig{{
			ID:             memberAgent.ID,
			TokenEnv:       "RUNNER_MEMBER_TOKEN",
			Provider:       providerB,
			Project:        projectB,
			AllowedSenders: []string{ownerAgent.ID},
		}},
	}
	supervisorA, err := NewSupervisor(configA)
	if err != nil {
		t.Fatal(err)
	}
	defer supervisorA.Close()
	supervisorB, err := NewSupervisor(configB)
	if err != nil {
		t.Fatal(err)
	}
	defer supervisorB.Close()
	stateA = supervisorA.Config.StateDir
	stateB = supervisorB.Config.StateDir
	supervisorB.executor = func(context.Context, ProviderRequest) (ProviderResult, error) {
		return ProviderResult{Text: "B_ACTUAL_RESULT"}, nil
	}
	supervisorA.executor = func(execCtx context.Context, request ProviderRequest) (ProviderResult, error) {
		var child tasks.Task
		if e := bridgeCall(execCtx, request.StateDir, request.JobID, request.Capability, "delegate", DelegateInput{
			ToAgentID:      memberAgent.ID,
			Instruction:    "delegate and return actual result",
			RequestKey:     "child-proof",
			TimeoutSeconds: 120,
		}, &child); e != nil {
			return ProviderResult{ErrorCode: "provider_failed"}, e
		}
		if child.Result == nil {
			return ProviderResult{ErrorCode: "provider_failed"}, errors.New("missing delegated result")
		}
		return ProviderResult{Text: "A_CONTINUED_WITH:" + *child.Result}, nil
	}

	runCtx, stop := context.WithCancel(context.Background())
	defer stop()
	failures := make(chan error, 2)
	go func() { failures <- supervisorA.Serve(runCtx) }()
	go func() { failures <- supervisorB.Serve(runCtx) }()
	waitReady := func(state string) {
		deadline := time.Now().Add(10 * time.Second)
		last := ""
		for time.Now().Before(deadline) {
			var statuses []JobStatus
			err := LocalCall(context.Background(), state, "GET", "/status", nil, &statuses)
			if err == nil {
				return
			}
			last = err.Error()
			select {
			case serveErr := <-failures:
				if serveErr != nil {
					t.Fatalf("runner serve failed before ready: %v", serveErr)
				}
			default:
			}
			time.Sleep(100 * time.Millisecond)
		}
		if _, err := os.Stat(socketPath(state)); err != nil {
			t.Fatalf("local runner control did not start (socket missing: %v, last error: %s)", err, last)
		}
		t.Fatalf("local runner control did not start (last error: %s)", last)
	}
	waitReady(stateA)
	waitReady(stateB)

	var started JobStatus
	if err = LocalCall(context.Background(), stateA, "POST", "/start", StartInput{
		AgentID:        ownerAgent.ID,
		Instruction:    "delegate to member and continue",
		TimeoutSeconds: 300,
	}, &started); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		var status struct {
			JobStatus
			Result string `json:"result,omitempty"`
		}
		if err = LocalCall(context.Background(), stateA, "GET", "/jobs/"+started.ID, nil, &status); err == nil && status.Status == "completed" {
			if status.Result != "A_CONTINUED_WITH:B_ACTUAL_RESULT" {
				t.Fatalf("unexpected continuation result: %q", status.Result)
			}
			stop()
			for i := 0; i < 2; i++ {
				if serveErr := <-failures; serveErr != nil {
					t.Fatal(serveErr)
				}
			}
			return
		}
		select {
		case serveErr := <-failures:
			if serveErr != nil {
				t.Fatal(serveErr)
			}
		default:
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("delegated continuation did not complete in time")
}
