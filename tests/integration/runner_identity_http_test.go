package integration

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/security"
)

func TestRunnerOAuthCallbackAndMachineHTTPClaim(t *testing.T) {
	ctx, pool, teamsStore, w, _ := teams(t)
	var project string
	if err := pool.QueryRow(ctx, `INSERT INTO mailbox.orchestration_projects(workspace_id,repository_id,repository_owner_id,repository_owner,repository_name) VALUES($1,42,101,'owner','project') RETURNING id::text`, w.ID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	authority := &runnerAuthority{allowed: map[int64]bool{202: true}, pool: pool, project: project}
	store := &runneridentity.Store{Pool: pool, Authority: authority}
	provider := httptest.NewServer(http.HandlerFunc((&fakeGitHub{id: 202, username: "collaborator"}).handler))
	defer provider.Close()
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	defer server.Close()
	browser := &identity.Web{Config: config.Identity{Enabled: true, ClientID: "mock-client-id", ClientSecret: "mock-client-secret", BaseURL: "http://" + server.Listener.Addr().String()}, Sessions: &identity.Sessions{Pool: pool}, Workspaces: teamsStore, Provider: identity.Provider{AuthorizeURL: provider.URL + "/authorize", TokenURL: provider.URL + "/token", UserURL: provider.URL + "/user", Client: identity.GitHubProvider().Client}}
	browser.Register(mux)
	certificateSource := httptest.NewTLSServer(http.NotFoundHandler())
	pair := certificateSource.TLS.Certificates[0]
	publicCA := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateSource.TLS.Certificates[0].Certificate[0]}))
	certificateSource.Close()
	(&runneridentity.Web{Store: store, Browser: browser, ControlEndpoint: "native.example:9090", ControlCA: publicCA}).Register(mux)
	server.Start()
	machine := server.Client()
	post := func(path string, input any, origin string) *http.Response {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest("POST", server.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response, err := machine.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	nonce, _ := security.Secret()
	message, _ := runneridentity.StartMessage(project, runneridentity.Executor, nonce)
	response := post("/runners/enroll", runneridentity.StartProof{ProjectID: project, Role: runneridentity.Executor, Nonce: nonce, PublicKey: public, Signature: ed25519.Sign(private, message)}, "")
	if response.StatusCode != 200 {
		t.Fatal("enrollment begin status", response.StatusCode)
	}
	var started struct {
		runneridentity.Enrollment
		LoginURL string `json:"login_url"`
	}
	if json.NewDecoder(response.Body).Decode(&started) != nil {
		t.Fatal("invalid begin response")
	}
	response.Body.Close()
	response, err := machine.Get(server.URL + runneridentity.EnrollmentPath + started.ID)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	var authorized bool
	if err = pool.QueryRow(ctx, "SELECT authorized_github_id IS NOT NULL FROM mailbox.runner_enrollments WHERE id=$1", started.ID).Scan(&authorized); err != nil || authorized {
		t.Fatal("ordinary GET authorized enrollment", err)
	}
	jar, _ := cookiejar.New(nil)
	human := &http.Client{Jar: jar}
	response, err = human.Get(started.LoginURL)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatal("OAuth enrollment failed", response.StatusCode)
	}
	response.Body.Close()
	response = post("/runners/challenge", map[string]string{"id": started.ID, "purpose": "claim"}, "")
	var challenge runneridentity.Challenge
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&challenge) != nil {
		t.Fatal("challenge rejected", response.StatusCode)
	}
	response.Body.Close()
	message, _ = runneridentity.ChallengeMessage(started.ID, "claim", challenge.Nonce)
	input := map[string]any{"id": started.ID, "purpose": "claim", "nonce": challenge.Nonce, "signature": ed25519.Sign(private, message)}
	response = post("/runners/credential", input, "")
	var result struct {
		runneridentity.Credential
		Endpoint string `json:"control_endpoint"`
		CA       string `json:"control_ca"`
	}
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&result) != nil {
		t.Fatal("credential rejected", response.StatusCode)
	}
	response.Body.Close()
	if result.Endpoint != "native.example:9090" || result.CA != publicCA {
		t.Fatal("control trust not delivered")
	}
	if _, err = store.Authenticate(ctx, result.Token); err != nil {
		t.Fatal(err)
	}
	var memberships int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM mailbox.memberships m JOIN mailbox.users u ON u.id=m.user_id WHERE u.github_id=202`).Scan(&memberships); err != nil || memberships != 0 {
		t.Fatal("collaborator enrollment required legacy membership", err)
	}
	response = post("/runners/credential", input, "")
	if response.StatusCode != 401 {
		t.Fatal("HTTP challenge replay accepted", response.StatusCode)
	}
	response.Body.Close()
	response = post("/runners/credential", input, "https://attacker.example")
	if response.StatusCode != 403 {
		t.Fatal("browser origin accepted", response.StatusCode)
	}
	response.Body.Close()
	api, err := runneridentity.NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	state, err := runneridentity.OpenState(filepath.Join(t.TempDir(), "architect-state"), server.URL, runneridentity.Architect)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	session, err := runneridentity.NewSession(state, api)
	if err != nil {
		t.Fatal(err)
	}
	opened := false
	lease, err := session.Acquire(ctx, project, func(loginURL string) error {
		opened = true
		result, err := human.Get(loginURL)
		if err != nil {
			return err
		}
		defer result.Body.Close()
		if result.StatusCode != 200 {
			return runneridentity.ErrUnavailable
		}
		return nil
	})
	if err != nil || !opened || lease.Role != runneridentity.Architect {
		t.Fatal("startup client failed", err)
	}
	renewed, err := session.Refresh(ctx, project)
	if err != nil || renewed.Epoch != lease.Epoch+1 {
		t.Fatal("startup client refresh failed", err)
	}
	if _, err = store.Authenticate(ctx, lease.Token); err == nil {
		t.Fatal("client retained stale credential epoch")
	}
	if _, err = store.Authenticate(ctx, renewed.Token); err != nil {
		t.Fatal(err)
	}
	assertRunnerRevokedOnOpenTLSStream(t, ctx, pool, store, result.Credential, pair)
	saved, err := state.LoadLease(project)
	if err != nil || saved.Token != renewed.Token || saved.Epoch != renewed.Epoch {
		t.Fatal("startup session did not journal refreshed credential", err)
	}

}
