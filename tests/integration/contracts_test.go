package integration

import (
	"errors"
	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/contracts"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractPublicationAuthorityRevisionAndRestart(t *testing.T) {
	cli, err := exec.LookPath("openspec")
	if err != nil {
		t.Skip("OpenSpec CLI required for integrated verification")
	}
	cli, err = filepath.Abs(cli)
	if err != nil {
		t.Fatal(err)
	}
	ctx, pool, _, w, owner := teams(t)
	var project string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, w.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	sessions := &identity.Sessions{Pool: pool}
	_, humanSecret, err := sessions.Login(ctx, owner.GitHubID, owner.Username, "")
	if err != nil {
		t.Fatal(err)
	}
	queue := &backlog.Store{Pool: pool, Sessions: sessions, Authority: &backlogAuthority{true}}
	input := backlog.Input{ProjectID: project, Key: "contract-root", Title: "Feature", Description: "Implement verified feature"}
	root, err := queue.Create(ctx, humanSecret, input)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for _, role := range []string{"architect", "executor"} {
		secret, _ := security.Secret()
		token := runneridentity.TokenPrefix + secret
		tokens[role] = token
		if _, err = pool.Exec(ctx, `INSERT INTO mailbox.project_runners(project_id,owner_github_id,public_key,role,credential_hash,credential_expires_at) VALUES($1,101,decode(repeat('ab',32),'hex'),$2,$3,clock_timestamp()+interval '15 minutes')`, project, role, security.Hash(token)); err != nil {
			t.Fatal(err)
		}
	}
	identities := &runneridentity.Store{Pool: pool, Authority: &runnerAuthority{allowed: map[int64]bool{101: true}}}
	store := &contracts.Store{Pool: pool, Identities: identities}
	checkout := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, e := exec.Command("git", append([]string{"-C", checkout}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatal("Git fixture failed")
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.invalid")
	files := map[string]string{
		"proposal.md":        "# Feature\n\n## Why\nProvide feature.\n\n## What Changes\n- Add feature.\n\n## Capabilities\n\n### New Capabilities\n- work: Verified behavior.\n\n### Modified Capabilities\n- None.\n\n## Impact\nImplementation.\n",
		"design.md":          "# Design\n\nImplement feature.\n",
		"tasks.md":           "# Tasks\n\n## 1. Implementation\n\n- [ ] 1.1 Implement feature and verify output.\n",
		"specs/work/spec.md": "# Work\n\n## ADDED Requirements\n\n### Requirement: Verified output\nThe feature SHALL return verified output.\n\n#### Scenario: Success\n- **GIVEN** valid input\n- **WHEN** feature runs\n- **THEN** output is verified.\n",
	}
	artifacts := map[string]string{}
	for name, body := range files {
		relative := "openspec/changes/feature/" + name
		p := filepath.Join(checkout, filepath.FromSlash(relative))
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		artifacts[relative] = security.Hash(body)
	}
	git("add", ".")
	git("commit", "-qm", "Create specification")
	content := contracts.Content{ProjectID: project, RootID: root.ID, RootRevision: 1, RepositoryID: 42, BaseCommit: git("rev-parse", "HEAD"), Change: "feature", Tasks: []string{"1.1"}, Scenarios: []string{"work::Verified output::Success"}, Artifacts: artifacts}
	if _, err = store.Publish(ctx, tokens["executor"], content); !errors.Is(err, contracts.ErrForbidden) {
		t.Fatal("executor published contract", err)
	}
	first, err := store.Publish(ctx, tokens["architect"], content)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &contracts.Store{Pool: pool, Identities: identities}
	same, err := restarted.Publish(ctx, tokens["architect"], content)
	if err != nil || same.ID != first.ID || same.Hash != first.Hash {
		t.Fatal("restart duplicated contract", err)
	}
	loaded, err := store.Get(ctx, tokens["architect"], project, first.ID)
	if err != nil || loaded.Hash != first.Hash {
		t.Fatal("durable contract missing", err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, first.Hash); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("unverified contract passed allocator gate", err)
	}
	validator := contracts.Validator{Store: store, OpenSpec: cli, Checkout: func(p string, repository int64) (string, error) {
		if p != project || repository != 42 {
			return "", contracts.ErrForbidden
		}
		return checkout, nil
	}}
	validation, err := validator.Validate(ctx, tokens["architect"], project, first.ID)
	if err != nil || validation == "" {
		t.Fatal("real verification-to-storage pipeline failed", err)
	}
	repeated, err := validator.Validate(ctx, tokens["architect"], project, first.ID)
	if err != nil || repeated != validation {
		t.Fatal("validation retry duplicated evidence", err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, first.Hash); err != nil {
		t.Fatal("verified bound record rejected", err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, strings.Repeat("c", 64)); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("different hash inherited evidence", err)
	}
	paused, err := queue.Control(ctx, humanSecret, project, root.ID, "pause", 1)
	if err != nil || paused.Epoch != 2 {
		t.Fatal("pause contract root", err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, first.Hash); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("paused contract executable", err)
	}
	if _, err = validator.Validate(ctx, tokens["architect"], project, first.ID); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("paused validation accepted", err)
	}
	if _, err = store.Publish(ctx, tokens["architect"], content); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("paused contract publication accepted", err)
	}
	if _, err = queue.Control(ctx, humanSecret, project, root.ID, "resume", 2); err != nil {
		t.Fatal(err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, first.Hash); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("unreconciled resumed contract executable", err)
	}
	input.Description = "Revised human scope"
	if _, err = queue.Revise(ctx, humanSecret, root.ID, 1, input); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Publish(ctx, tokens["architect"], content); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("old input revision republished", err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, first.Hash); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("changed root inherited validation evidence", err)
	}
	loaded, err = store.Get(ctx, tokens["architect"], project, first.ID)
	if err != nil || loaded.Content.RootRevision != 1 {
		t.Fatal("history rewritten", err)
	}
	var audits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.audit_events WHERE action='contract.published' AND target_id=$1", first.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("duplicate publication audit", err)
	}
}
