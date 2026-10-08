package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

const ProviderBodyLimit = 64 << 10

type Provider struct {
	AuthorizeURL, TokenURL, UserURL string
	Client                          *http.Client
}

func GitHubProvider() Provider {
	return Provider{AuthorizeURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", UserURL: "https://api.github.com/user", Client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (p Provider) Exchange(ctx context.Context, cfg config.Identity, code, verifier string) (workspaces.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if code == "" || len(code) > 2048 {
		return workspaces.User{}, ErrUnauthenticated
	}
	form := url.Values{"client_id": {cfg.ClientID}, "client_secret": {cfg.ClientSecret}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {cfg.BaseURL + "/auth/github/callback"}}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return workspaces.User{}, ErrUnavailable
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
	}
	if err = p.decode(r, &token); err != nil || token.AccessToken == "" || token.Error != "" || !strings.EqualFold(token.TokenType, "bearer") {
		return workspaces.User{}, ErrUnauthenticated
	}
	r, err = http.NewRequestWithContext(ctx, http.MethodGet, p.UserURL, nil)
	if err != nil {
		return workspaces.User{}, ErrUnavailable
	}
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err = p.decode(r, &user); err != nil || user.ID < 1 || user.Login == "" || len(user.Login) > 100 {
		return workspaces.User{}, ErrUnauthenticated
	}
	return workspaces.User{GitHubID: user.ID, Username: user.Login}, nil
}

func (p Provider) decode(r *http.Request, destination any) error {
	client := p.Client
	if client == nil {
		client = GitHubProvider().Client
	}
	response, err := client.Do(r)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrUnauthenticated
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, ProviderBodyLimit+1))
	if err != nil || len(body) > ProviderBodyLimit {
		return ErrUnavailable
	}
	if json.Unmarshal(body, destination) != nil {
		return errors.New("invalid identity provider response")
	}
	return nil
}
