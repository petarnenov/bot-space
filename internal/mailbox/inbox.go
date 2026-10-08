package mailbox

import (
	"context"

	"github.com/petarnenov/bot-space/internal/security"
)

func (s *Store) Read(ctx context.Context, token string, input ReadInput) (Page, error) {
	scope, limit, err := filters(input)
	if err != nil {
		return Page{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Page{}, ErrUnavailable
	}
	defer rollback(tx)
	p, err := s.Auth.AuthenticateTx(ctx, tx, token)
	if err != nil {
		return Page{}, authError(err)
	}
	scope.Agent = p.AgentID
	scope.Workspace = p.WorkspaceID
	cursor := Cursor{Scope: scope}
	if input.Cursor != "" {
		cursor, err = s.Signer.Parse(input.Cursor, scope)
		if err != nil {
			return Page{}, err
		}
		if cursor.AfterAgent != "" {
			return Page{}, ErrInvalid
		}
	} else {
		if tx.QueryRow(ctx, "SELECT COALESCE((SELECT last_sequence FROM mailbox.inbox_counters WHERE workspace_id=$1 AND agent_id=$2),0)", p.WorkspaceID, p.AgentID).Scan(&cursor.Upper) != nil {
			return Page{}, ErrUnavailable
		}
	}
	var thread, kind any
	if scope.Thread != "" {
		thread = scope.Thread
	}
	if scope.Kind != "" {
		kind = scope.Kind
	}
	rows, err := tx.Query(ctx, "SELECT "+messageColumns+` FROM mailbox.messages WHERE workspace_id=$1 AND to_agent_id=$2 AND inbox_sequence>$3 AND inbox_sequence<=$4
		AND ($5='all' OR ($5='unacknowledged' AND acknowledged_at IS NULL) OR ($5='acknowledged' AND acknowledged_at IS NOT NULL))
		AND ($6::uuid IS NULL OR thread_id=$6) AND ($7::text IS NULL OR kind=$7) ORDER BY inbox_sequence LIMIT $8`, p.WorkspaceID, p.AgentID, cursor.Last, cursor.Upper, scope.Acknowledged, thread, kind, limit+1)
	if err != nil {
		return Page{}, ErrUnavailable
	}
	var candidates []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			rows.Close()
			return Page{}, ErrUnavailable
		}
		candidates = append(candidates, m)
	}
	if rows.Err() != nil {
		rows.Close()
		return Page{}, ErrUnavailable
	}
	rows.Close()
	page := Page{Messages: []Message{}}
	for i, m := range candidates {
		if len(page.Messages) >= limit {
			break
		}
		page.Messages = append(page.Messages, m)
		page.NextCursor = nil
		if i+1 < len(candidates) {
			next, err := s.Signer.Sign(Cursor{Scope: scope, Last: m.Sequence, Upper: cursor.Upper})
			if err != nil {
				return Page{}, err
			}
			page.NextCursor = &next
		}
		if !fits(page) {
			page.Messages = page.Messages[:len(page.Messages)-1]
			if len(page.Messages) == 0 {
				return Page{}, ErrUnavailable
			}
			last := page.Messages[len(page.Messages)-1].Sequence
			next, err := s.Signer.Sign(Cursor{Scope: scope, Last: last, Upper: cursor.Upper})
			if err != nil {
				return Page{}, err
			}
			page.NextCursor = &next
			break
		}
	}
	return page, commit(ctx, tx)
}

func (s *Store) Acknowledge(ctx context.Context, token, messageID string) (Message, error) {
	if !security.ValidUUID(messageID) {
		return Message{}, ErrInvalid
	}
	messageID = canonicalID(messageID)
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Message{}, ErrUnavailable
	}
	defer rollback(tx)
	p, err := s.Auth.AuthenticateTx(ctx, tx, token)
	if err != nil {
		return Message{}, authError(err)
	}
	m, err := scanMessage(tx.QueryRow(ctx, "UPDATE mailbox.messages SET acknowledged_at=COALESCE(acknowledged_at,clock_timestamp()) WHERE workspace_id=$1 AND to_agent_id=$2 AND id=$3 RETURNING "+messageColumns, p.WorkspaceID, p.AgentID, messageID))
	if err != nil {
		return Message{}, lookupError(err)
	}
	return m, commit(ctx, tx)
}

func (s *Store) Directory(ctx context.Context, token string, input DirectoryInput) (DirectoryPage, error) {
	limit, err := pageLimit(input.Limit)
	if err != nil || len(input.Cursor) > MaxCursorBytes {
		return DirectoryPage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, OperationTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DirectoryPage{}, ErrUnavailable
	}
	defer rollback(tx)
	p, err := s.Auth.AuthenticateTx(ctx, tx, token)
	if err != nil {
		return DirectoryPage{}, authError(err)
	}
	scope := CursorScope{Purpose: "agents", Agent: p.AgentID, Workspace: p.WorkspaceID}
	var after any
	if input.Cursor != "" {
		cursor, err := s.Signer.Parse(input.Cursor, scope)
		if err != nil || !security.ValidUUID(cursor.AfterAgent) || cursor.Last != 0 || cursor.Upper != 0 {
			return DirectoryPage{}, ErrInvalid
		}
		after = cursor.AfterAgent
	}
	rows, err := tx.Query(ctx, `SELECT a.id::text,a.workspace_id::text,a.owner_user_id::text,a.name,u.username,u.github_id
		FROM mailbox.agents a JOIN mailbox.memberships m ON m.workspace_id=a.workspace_id AND m.user_id=a.owner_user_id JOIN mailbox.users u ON u.id=a.owner_user_id
		WHERE a.workspace_id=$1 AND a.active AND ($2::uuid IS NULL OR a.id>$2) ORDER BY a.id LIMIT $3`, p.WorkspaceID, after, limit+1)
	if err != nil {
		return DirectoryPage{}, ErrUnavailable
	}
	var candidates []AgentInfo
	for rows.Next() {
		var a AgentInfo
		if rows.Scan(&a.ID, &a.WorkspaceID, &a.OwnerUserID, &a.Name, &a.OwnerUsername, &a.OwnerGitHubID) != nil {
			rows.Close()
			return DirectoryPage{}, ErrUnavailable
		}
		candidates = append(candidates, a)
	}
	if rows.Err() != nil {
		rows.Close()
		return DirectoryPage{}, ErrUnavailable
	}
	rows.Close()
	page := DirectoryPage{Agents: []AgentInfo{}}
	for i, a := range candidates {
		if len(page.Agents) >= limit {
			break
		}
		page.Agents = append(page.Agents, a)
		page.NextCursor = nil
		if i+1 < len(candidates) {
			next, err := s.Signer.Sign(Cursor{Scope: scope, AfterAgent: a.ID})
			if err != nil {
				return DirectoryPage{}, err
			}
			page.NextCursor = &next
		}
		if !fits(page) {
			page.Agents = page.Agents[:len(page.Agents)-1]
			if len(page.Agents) == 0 {
				return DirectoryPage{}, ErrUnavailable
			}
			next, err := s.Signer.Sign(Cursor{Scope: scope, AfterAgent: page.Agents[len(page.Agents)-1].ID})
			if err != nil {
				return DirectoryPage{}, err
			}
			page.NextCursor = &next
			break
		}
	}
	return page, commit(ctx, tx)
}
