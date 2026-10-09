package integration

import (
	"errors"
	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/contracts"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
	"strings"
	"testing"
)

func TestContractPublicationAuthorityRevisionAndRestart(t *testing.T) {
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
	prefix := "openspec/changes/feature/"
	artifacts := map[string]string{}
	for _, name := range []string{"proposal.md", "design.md", "tasks.md", "specs/work/spec.md"} {
		artifacts[prefix+name] = strings.Repeat("a", 64)
	}
	content := contracts.Content{ProjectID: project, RootID: root.ID, RootRevision: 1, RepositoryID: 42, BaseCommit: strings.Repeat("b", 40), Change: "feature", Tasks: []string{"1.1"}, Scenarios: []string{"verified feature"}, Artifacts: artifacts}
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
	// Explicit trusted validation-record fixture isolates the SQL gate; actual
	// Git/OpenSpec verification is covered by real-CLI tests in contracts.
	if _, err = pool.Exec(ctx, `INSERT INTO mailbox.contract_validations(project_id,contract_id,contract_hash,validator_runner_id) VALUES($1,$2,$3,$4)`, project, first.ID, first.Hash, first.Publisher); err != nil {
		t.Fatal(err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, first.Hash); err != nil {
		t.Fatal("verified bound record rejected", err)
	}
	if err = store.RequireValidated(ctx, project, first.ID, strings.Repeat("c", 64)); !errors.Is(err, contracts.ErrStale) {
		t.Fatal("different hash inherited evidence", err)
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
