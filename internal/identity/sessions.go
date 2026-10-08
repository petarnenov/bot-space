// Package identity implements GitHub-only human identity and protected browser sessions.
package identity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/security"
	"github.com/petarnenov/bot-space/internal/workspaces"
)

const SessionLifetime = 8 * time.Hour
const SessionIdleLifetime = 30 * time.Minute
const AttemptLifetime = 10 * time.Minute

var ErrUnauthenticated = errors.New("unauthenticated")
var ErrUnavailable = errors.New("authentication temporarily unavailable")

type Sessions struct{ Pool *pgxpool.Pool }
type Session struct {
	User      workspaces.User
	CSRF      string
	ExpiresAt time.Time
}

func (s *Sessions) Login(ctx context.Context, githubID int64, username, oldSecret string) (Session, string, error) {
	if githubID < 1 || len(username) > 100 {
		return Session{}, "", ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	secret, err := security.Secret()
	if err != nil {
		return Session{}, "", ErrUnavailable
	}
	csrf, err := security.Secret()
	if err != nil {
		return Session{}, "", ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Session{}, "", ErrUnavailable
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(ctx)
	}()
	var session Session
	err = tx.QueryRow(ctx, "INSERT INTO mailbox.users (github_id,username) VALUES ($1,$2) ON CONFLICT (github_id) DO UPDATE SET username=EXCLUDED.username RETURNING id::text,github_id,username", githubID, username).Scan(&session.User.ID, &session.User.GitHubID, &session.User.Username)
	if err != nil {
		return Session{}, "", ErrUnavailable
	}
	if oldSecret != "" {
		if _, err = tx.Exec(ctx, "UPDATE mailbox.sessions SET revoked_at=now() WHERE id_hash=$1", security.Hash(oldSecret)); err != nil {
			return Session{}, "", ErrUnavailable
		}
	}
	session.CSRF = csrf
	err = tx.QueryRow(ctx, "INSERT INTO mailbox.sessions (id_hash,user_id,csrf_token,expires_at) VALUES ($1,$2,$3,now()+interval '8 hours') RETURNING expires_at", security.Hash(secret), session.User.ID, csrf).Scan(&session.ExpiresAt)
	if err != nil {
		return Session{}, "", ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Session{}, "", ErrUnavailable
	}
	return session, secret, nil
}

func (s *Sessions) Authenticate(ctx context.Context, secret string) (Session, error) {
	if len(secret) != 43 {
		return Session{}, ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var session Session
	err := s.Pool.QueryRow(ctx, `WITH active AS (
		UPDATE mailbox.sessions SET last_seen_at=now()
		WHERE id_hash=$1 AND revoked_at IS NULL AND expires_at>clock_timestamp()
		AND last_seen_at>clock_timestamp()-interval '30 minutes'
		RETURNING user_id,csrf_token,expires_at)
		SELECT u.id::text,u.github_id,u.username,a.csrf_token,a.expires_at
		FROM active a JOIN mailbox.users u ON u.id=a.user_id`, security.Hash(secret)).Scan(&session.User.ID, &session.User.GitHubID, &session.User.Username, &session.CSRF, &session.ExpiresAt)
	if err != nil {
		return Session{}, ErrUnauthenticated
	}
	return session, nil
}

func (s *Sessions) Logout(ctx context.Context, secret string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.Pool.Exec(ctx, "UPDATE mailbox.sessions SET revoked_at=now() WHERE id_hash=$1", security.Hash(secret))
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Sessions) Attempt(ctx context.Context, returnPath string) (string, string, error) {
	state, err := security.Secret()
	if err != nil {
		return "", "", ErrUnavailable
	}
	verifier, err := security.Secret()
	if err != nil {
		return "", "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = s.Pool.Exec(ctx, "INSERT INTO mailbox.oauth_attempts (state_hash,verifier,return_path,expires_at) VALUES ($1,$2,$3,now()+interval '10 minutes')", security.Hash(state), verifier, security.SafeReturn(returnPath))
	if err != nil {
		return "", "", ErrUnavailable
	}
	return state, verifier, nil
}

func (s *Sessions) ConsumeAttempt(ctx context.Context, state string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if len(state) != 43 {
		return "", "", ErrUnauthenticated
	}
	var verifier, path string
	err := s.Pool.QueryRow(ctx, "DELETE FROM mailbox.oauth_attempts WHERE state_hash=$1 AND expires_at>clock_timestamp() RETURNING verifier,return_path", security.Hash(state)).Scan(&verifier, &path)
	if err != nil {
		return "", "", ErrUnauthenticated
	}
	return verifier, security.SafeReturn(path), nil
}
