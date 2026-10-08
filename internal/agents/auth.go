package agents

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/security"
)

type Principal struct {
	AgentID        string `json:"agent_id"`
	WorkspaceID    string `json:"workspace_id"`
	OwnerUserID    string `json:"owner_user_id"`
	Name           string `json:"name"`
	CredentialID   string `json:"-"`
	credentialHash string
}
type principalKey struct{}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

func (s *Store) Authenticate(ctx context.Context, token string) (Principal, error) {
	if !validToken(token) {
		return Principal{}, ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	defer rollback(tx)
	p, err := s.AuthenticateTx(ctx, tx, token)
	if err != nil {
		return Principal{}, err
	}
	if err = commit(ctx, tx); err != nil {
		return Principal{}, err
	}
	return p, nil
}

func (s *Store) AuthenticateTx(ctx context.Context, tx pgx.Tx, token string) (Principal, error) {
	if !validToken(token) {
		return Principal{}, ErrUnauthenticated
	}
	return authenticateHash(ctx, tx, security.Hash(token))
}

func (s *Store) RecheckTx(ctx context.Context, tx pgx.Tx, p Principal) (Principal, error) {
	return authenticateHash(ctx, tx, p.credentialHash)
}

func authenticateHash(ctx context.Context, tx pgx.Tx, hash string) (Principal, error) {
	if hash == "" {
		return Principal{}, ErrUnauthenticated
	}
	var p Principal
	err := tx.QueryRow(ctx, "SELECT a.id::text,a.workspace_id::text,a.owner_user_id::text,c.id::text FROM mailbox.agent_credentials c JOIN mailbox.agents a ON a.workspace_id=c.workspace_id AND a.id=c.agent_id WHERE c.secret_hash=$1", hash).Scan(&p.AgentID, &p.WorkspaceID, &p.OwnerUserID, &p.CredentialID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	var role string
	if err = tx.QueryRow(ctx, "SELECT role FROM mailbox.memberships WHERE workspace_id=$1 AND user_id=$2 FOR SHARE", p.WorkspaceID, p.OwnerUserID).Scan(&role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{}, ErrUnavailable
	}
	var active bool
	err = tx.QueryRow(ctx, "SELECT name,active FROM mailbox.agents WHERE workspace_id=$1 AND id=$2 AND owner_user_id=$3 FOR SHARE", p.WorkspaceID, p.AgentID, p.OwnerUserID).Scan(&p.Name, &active)
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	if !active {
		return Principal{}, ErrUnauthenticated
	}
	err = tx.QueryRow(ctx, "SELECT revoked_at IS NULL FROM mailbox.agent_credentials WHERE workspace_id=$1 AND agent_id=$2 AND id=$3 AND secret_hash=$4 FOR SHARE", p.WorkspaceID, p.AgentID, p.CredentialID, hash).Scan(&active)
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	if !active {
		return Principal{}, ErrUnauthenticated
	}
	p.credentialHash = hash
	return p, nil
}

func (s *Store) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Cache-Control", "no-store")
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			unauthorized(rw)
			return
		}
		fields := strings.Fields(values[0])
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			unauthorized(rw)
			return
		}
		p, err := s.Authenticate(r.Context(), fields[1])
		if err != nil {
			if errors.Is(err, ErrUnavailable) {
				http.Error(rw, "Authentication unavailable", http.StatusServiceUnavailable)
			} else {
				unauthorized(rw)
			}
			return
		}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func unauthorized(rw http.ResponseWriter) {
	rw.Header().Set("WWW-Authenticate", `Bearer realm="bot-space"`)
	http.Error(rw, "Authentication required", http.StatusUnauthorized)
}
