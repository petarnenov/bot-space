package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/mailbox"
)

func TestPostgresBackupRestorePreservesMailboxAndCredentials(t *testing.T) {
	container := os.Getenv("PG_BACKUP_CONTAINER")
	if container == "" {
		t.Skip("set PG_BACKUP_CONTAINER to the PostgreSQL service container for the restore drill")
	}
	f := mailSetup(t)
	input := mailbox.SendInput{ToAgentID: f.b.ID, IdempotencyKey: "backup-key", Text: "synthetic restore payload"}
	message, err := f.store.Send(f.ctx, f.ta, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.agents.Revoke(f.ctx, f.workspace.ID, f.owner.ID, f.a.ID, f.ca.ID); err != nil {
		t.Fatal(err)
	}
	cfg := f.pool.Config().ConnConfig
	admin, err := pgx.ConnectConfig(f.ctx, cfg.Copy())
	if err != nil {
		t.Fatal("backup source connection failed")
	}
	name := fmt.Sprintf("bot_space_restore_%d", time.Now().UnixNano())
	if _, err = admin.Exec(f.ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		_ = admin.Close(f.ctx)
		t.Fatal("disposable restore database creation failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error("disposable restore database cleanup failed")
		}
		_ = admin.Close(ctx)
	})
	var dump bytes.Buffer
	backup := exec.CommandContext(f.ctx, "docker", "exec", container, "pg_dump", "-U", cfg.User, "-d", cfg.Database, "--format=custom")
	backup.Stdout = &dump
	if err := backup.Run(); err != nil {
		t.Fatal("PostgreSQL backup command failed")
	}
	restore := exec.CommandContext(f.ctx, "docker", "exec", "-i", container, "pg_restore", "-U", cfg.User, "-d", name, "--exit-on-error", "--no-owner")
	restore.Stdin = bytes.NewReader(dump.Bytes())
	if err := restore.Run(); err != nil {
		t.Fatal("PostgreSQL restore command failed")
	}
	for _, raw := range []string{f.ta, f.tb} {
		if bytes.Contains(dump.Bytes(), []byte(raw)) {
			t.Fatal("database backup contains a raw credential")
		}
	}
	restoredConfig := cfg.Copy()
	restoredConfig.Database = name
	pool, err := database.Open(f.ctx, restoredConfig.ConnString(), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := database.Ready(f.ctx, pool, bundle(t)); err != nil {
		t.Fatal("restored migration ledger is not ready")
	}
	store, err := mailbox.New(pool, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.Read(f.ctx, f.tb, mailbox.ReadInput{})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != message.ID || page.Messages[0].Text != input.Text {
		t.Fatal("restored message or active credential differs")
	}
	credentials := &agents.Store{Pool: pool}
	if _, err := credentials.Authenticate(f.ctx, f.ta); err == nil {
		t.Fatal("restored revoked credential is active")
	}
	_, token, err := credentials.Issue(f.ctx, f.workspace.ID, f.owner.ID, f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.Send(f.ctx, token, input)
	if err != nil || repeated.ID != message.ID {
		t.Fatal("restored idempotency state differs")
	}
}
