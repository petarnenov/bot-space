package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

type runnerAuthority struct {
	allowed       map[int64]bool
	pool          *pgxpool.Pool
	project       string
	changeProject bool
	requireForce  bool
}

func (a *runnerAuthority) Verify(ctx context.Context, _ repositoryaccess.Repository, id int64, force bool) error {
	if a.requireForce && !force {
		return errors.New("enrollment must force current authority")
	}
	if a.changeProject {
		_, err := a.pool.Exec(ctx, "UPDATE mailbox.orchestration_projects SET repository_id=99 WHERE id=$1", a.project)
		if err != nil {
			return err
		}
	}
	if a.allowed[id] {
		return nil
	}
	return repositoryaccess.ErrDenied
}
func TestRunnerEnrollmentDurableKeyAndGitHubBinding(t *testing.T) {
	ctx, pool, _, w, _ := teams(t)
	var project string
	err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, w.ID).Scan(&project)
	if err != nil {
		t.Fatal(err)
	}
	authority := &runnerAuthority{allowed: map[int64]bool{101: true, 102: true}, pool: pool, project: project, requireForce: true}
	store := &runneridentity.Store{Pool: pool, Authority: authority}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	nonce, _ := security.Secret()
	message, _ := runneridentity.StartMessage(project, runneridentity.Executor, nonce)
	proof := runneridentity.StartProof{ProjectID: project, Role: runneridentity.Executor, Nonce: nonce, PublicKey: public, Signature: ed25519.Sign(private, message)}
	enrollment, err := store.Begin(ctx, proof)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &runneridentity.Store{Pool: pool, Authority: authority}
	repeated, err := restarted.Begin(ctx, proof)
	if err != nil || repeated.ID != enrollment.ID {
		t.Fatal("restart duplicated enrollment", err)
	}
	forged := proof
	forged.Role = runneridentity.Architect
	if _, err = store.Begin(ctx, forged); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("forged role admitted", err)
	}
	if err = store.AuthorizeGitHub(ctx, enrollment.ID, 999); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("public reader admitted", err)
	}
	if err = store.AuthorizeGitHub(ctx, enrollment.ID, 101); err != nil {
		t.Fatal(err)
	}
	if err = store.AuthorizeGitHub(ctx, enrollment.ID, 102); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("authorized person replaced", err)
	}
	var githubID int64
	if err = pool.QueryRow(ctx, "SELECT authorized_github_id FROM mailbox.runner_enrollments WHERE id=$1", enrollment.ID).Scan(&githubID); err != nil || githubID != 101 {
		t.Fatal("wrong OAuth identity persisted", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE mailbox.runner_enrollments SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", enrollment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Begin(ctx, proof); err == nil {
		t.Fatal("expired attempt reused")
	}
	if err = store.AuthorizeGitHub(ctx, enrollment.ID, 101); err == nil {
		t.Fatal("expired attempt authorized")
	}
	nonce, _ = security.Secret()
	message, _ = runneridentity.StartMessage(project, runneridentity.Executor, nonce)
	proof.Nonce = nonce
	proof.Signature = ed25519.Sign(private, message)
	fresh, err := store.Begin(ctx, proof)
	if err != nil {
		t.Fatal(err)
	}
	authority.changeProject = true
	if err = store.AuthorizeGitHub(ctx, fresh.ID, 101); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("changed repository admitted after old verification", err)
	}
}

func TestRunnerCredentialOneUseRotationAndRevocation(t *testing.T) {
	ctx, pool, _, w, _ := teams(t)
	var project string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, w.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	authority := &runnerAuthority{allowed: map[int64]bool{101: true}, pool: pool, project: project}
	store := &runneridentity.Store{Pool: pool, Authority: authority}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	nonce, _ := security.Secret()
	message, _ := runneridentity.StartMessage(project, runneridentity.Executor, nonce)
	e, err := store.Begin(ctx, runneridentity.StartProof{ProjectID: project, Role: runneridentity.Executor, Nonce: nonce, PublicKey: public, Signature: ed25519.Sign(private, message)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Challenge(ctx, e.ID, "claim"); err == nil {
		t.Fatal("challenge issued before OAuth authorization")
	}
	if err = store.AuthorizeGitHub(ctx, e.ID, 101); err != nil {
		t.Fatal(err)
	}
	challenge, err := store.Challenge(ctx, e.ID, "claim")
	if err != nil {
		t.Fatal(err)
	}
	message, _ = runneridentity.ChallengeMessage(e.ID, "claim", challenge.Nonce)
	signature := ed25519.Sign(private, message)
	credential, err := store.Issue(ctx, e.ID, "claim", challenge.Nonce, signature)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Issue(ctx, e.ID, "claim", challenge.Nonce, signature); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("claim proof replayed", err)
	}
	principal, err := store.Authenticate(ctx, credential.Token)
	if err != nil || principal.RunnerID != credential.RunnerID || principal.CredentialEpoch != 1 {
		t.Fatal("new credential rejected", err)
	}
	var stored string
	if err = pool.QueryRow(ctx, "SELECT credential_hash FROM mailbox.project_runners WHERE id=$1", credential.RunnerID).Scan(&stored); err != nil || stored != security.Hash(credential.Token) || stored == credential.Token {
		t.Fatal("credential not hashed", err)
	}
	challenge, err = store.Challenge(ctx, credential.RunnerID, "refresh")
	if err != nil {
		t.Fatal(err)
	}
	message, _ = runneridentity.ChallengeMessage(credential.RunnerID, "refresh", challenge.Nonce)
	signature = ed25519.Sign(private, message)
	bad := append([]byte{}, signature...)
	bad[0] ^= 1
	if _, err = store.Issue(ctx, credential.RunnerID, "refresh", challenge.Nonce, bad); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("bad key proof accepted", err)
	}
	refreshed, err := store.Issue(ctx, credential.RunnerID, "refresh", challenge.Nonce, signature)
	if err != nil || refreshed.Epoch != 2 {
		t.Fatal("refresh failed after forged proof", err)
	}
	if _, err = store.Authenticate(ctx, credential.Token); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("old epoch authenticated", err)
	}
	if _, err = store.Authenticate(ctx, refreshed.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Issue(ctx, credential.RunnerID, "refresh", challenge.Nonce, signature); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("refresh proof replayed", err)
	}
	expired, err := store.Challenge(ctx, credential.RunnerID, "refresh")
	if err != nil {
		t.Fatal(err)
	}
	expiredMessage, _ := runneridentity.ChallengeMessage(credential.RunnerID, "refresh", expired.Nonce)
	if _, err = pool.Exec(ctx, "UPDATE mailbox.runner_key_challenges SET expires_at=clock_timestamp()-interval '1 second' WHERE nonce_hash=$1", security.Hash(expired.Nonce)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Issue(ctx, credential.RunnerID, "refresh", expired.Nonce, ed25519.Sign(private, expiredMessage)); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("expired challenge accepted", err)
	}
	concurrent, err := store.Challenge(ctx, credential.RunnerID, "refresh")
	if err != nil {
		t.Fatal(err)
	}
	concurrentMessage, _ := runneridentity.ChallengeMessage(credential.RunnerID, "refresh", concurrent.Nonce)
	concurrentSignature := ed25519.Sign(private, concurrentMessage)
	type issued struct {
		credential runneridentity.Credential
		err        error
	}
	results := make(chan issued, 2)
	for i := 0; i < 2; i++ {
		go func() {
			c, e := store.Issue(ctx, credential.RunnerID, "refresh", concurrent.Nonce, concurrentSignature)
			results <- issued{c, e}
		}()
	}
	successes := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err == nil {
			successes++
			refreshed = result.credential
		} else if !errors.Is(result.err, runneridentity.ErrUnauthenticated) {
			t.Fatal(result.err)
		}
	}
	if successes != 1 || refreshed.Epoch != 3 {
		t.Fatal("concurrent challenge issued multiple credentials")
	}
	authority.allowed[101] = false
	if _, err = store.Authenticate(ctx, refreshed.Token); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("removed collaborator authenticated", err)
	}
	authority.allowed[101] = true
	if _, err = pool.Exec(ctx, "UPDATE mailbox.project_runners SET active=false WHERE id=$1", credential.RunnerID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Authenticate(ctx, refreshed.Token); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("revoked runner authenticated", err)
	}
	if _, err = store.Challenge(ctx, credential.RunnerID, "refresh"); err == nil {
		t.Fatal("revoked runner requested refresh")
	}
	var audits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM mailbox.audit_events WHERE action='runner.credential_issued' AND target_id=$1", credential.RunnerID).Scan(&audits); err != nil || audits != 3 {
		t.Fatal("wrong credential audit count", err)
	}
}

func TestMachineRoleRetainsGitHubActorAcrossProjects(t *testing.T) {
	ctx, pool, _, w, _ := teams(t)
	var projects [2]string
	for i := range projects {
		if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,$2,101,'owner','project') RETURNING id::text`, w.ID, 42+i).Scan(&projects[i]); err != nil {
			t.Fatal(err)
		}
	}
	authority := &runnerAuthority{allowed: map[int64]bool{101: true, 202: true}, pool: pool}
	store := &runneridentity.Store{Pool: pool, Authority: authority}
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	issue := func(project string, actor int64) (runneridentity.Credential, error) {
		nonce, _ := security.Secret()
		message, _ := runneridentity.StartMessage(project, runneridentity.Executor, nonce)
		e, err := store.Begin(ctx, runneridentity.StartProof{ProjectID: project, Role: runneridentity.Executor, Nonce: nonce, PublicKey: public, Signature: ed25519.Sign(key, message)})
		if err != nil {
			return runneridentity.Credential{}, err
		}
		if err = store.AuthorizeGitHub(ctx, e.ID, actor); err != nil {
			return runneridentity.Credential{}, err
		}
		challenge, err := store.Challenge(ctx, e.ID, "claim")
		if err != nil {
			return runneridentity.Credential{}, err
		}
		message, _ = runneridentity.ChallengeMessage(e.ID, "claim", challenge.Nonce)
		return store.Issue(ctx, e.ID, "claim", challenge.Nonce, ed25519.Sign(key, message))
	}
	first, err := issue(projects[0], 101)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = issue(projects[1], 202); !errors.Is(err, runneridentity.ErrUnauthenticated) {
		t.Fatal("same machine changed GitHub actor across projects", err)
	}
	second, err := issue(projects[1], 101)
	if err != nil {
		t.Fatal("same actor rejected in second project", err)
	}
	if first.RunnerID == second.RunnerID || first.ProjectID == second.ProjectID {
		t.Fatal("project scopes mixed")
	}
	for _, credential := range []runneridentity.Credential{first, second} {
		principal, err := store.Authenticate(ctx, credential.Token)
		if err != nil || principal.ProjectID != credential.ProjectID {
			t.Fatal("scoped authority lost", err)
		}
	}
}

func TestConfiguredProjectBootstrapIsStableAndFailClosed(t *testing.T) {
	ctx, pool, _, w, _ := teams(t)
	store := &runneridentity.Store{Pool: pool}
	repo := repositoryaccess.Repository{ID: 42, OwnerID: 101, Owner: "owner", Name: "project"}
	first, err := store.ConfigureProject(ctx, w.Slug, repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ConfigureProject(ctx, w.Slug, repo)
	if err != nil || first != second {
		t.Fatal("bootstrap replaced project identity", err)
	}
	changed := repo
	changed.OwnerID = 202
	if _, err = store.ConfigureProject(ctx, w.Slug, changed); err == nil {
		t.Fatal("repository owner changed silently")
	}
	if _, err = store.ConfigureProject(ctx, "unknown-workspace", repo); err == nil {
		t.Fatal("unknown workspace configured")
	}
}
