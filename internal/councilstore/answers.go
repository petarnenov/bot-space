package councilstore

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/petarnenov/bot-space/internal/council"
	"github.com/petarnenov/bot-space/internal/security"
)

func (s *Store) OpenAnswer(ctx context.Context, token, question string, proposal council.Proposal) (Record, error) {
	return s.answer(ctx, token, question, proposal, false)
}
func (s *Store) ReconsiderAnswer(ctx context.Context, token, question string, proposal council.Proposal) (Record, error) {
	return s.answer(ctx, token, question, proposal, true)
}
func (s *Store) answer(ctx context.Context, token, question string, proposal council.Proposal, reconsider bool) (Record, error) {
	if !security.ValidUUID(question) {
		return Record{}, council.ErrInvalid
	}
	proposal.Actions = append([]council.Action(nil), proposal.Actions...)
	for i := range proposal.Actions {
		proposal.Actions[i].Value = strings.TrimSpace(proposal.Actions[i].Value)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token)
	if err != nil {
		return Record{}, err
	}
	var contract string
	err = s.Pool.QueryRow(ctx, `SELECT a.contract_id::text FROM mailbox.executor_questions q JOIN mailbox.work_assignments a ON a.project_id=q.project_id AND a.id=q.assignment_id WHERE q.project_id=$1 AND q.id=$2`, p.ProjectID, question).Scan(&contract)
	if err != nil {
		return Record{}, ErrForbidden
	}
	return s.openDecision(ctx, token, contract, "answer", "question:"+question, proposal, reconsider)
}

// questionSource binds a decision to immutable executor context and the same
// live attempt/session/slot. It adds no elapsed deliberation deadline.
func questionSource(ctx context.Context, tx pgx.Tx, project, contract, subject string) (string, error) {
	if !strings.HasPrefix(subject, "question:") {
		return "", council.ErrInvalid
	}
	id := strings.TrimPrefix(subject, "question:")
	if !security.ValidUUID(id) {
		return "", council.ErrInvalid
	}
	var fingerprint string
	err := tx.QueryRow(ctx, `SELECT q.fingerprint FROM mailbox.executor_questions q
 JOIN mailbox.work_assignments a ON a.project_id=q.project_id AND a.id=q.assignment_id
 JOIN mailbox.work_attempts t ON t.project_id=q.project_id AND t.id=q.attempt_id AND t.assignment_id=q.assignment_id AND t.session_id=q.session_id AND t.authority_epoch=q.authority_epoch
 JOIN mailbox.executor_slots s ON s.assignment_id=a.id AND s.owner_github_id=a.owner_github_id AND s.public_key=a.public_key AND s.generation=a.slot_generation
 WHERE q.project_id=$1 AND q.id=$2 AND a.contract_id=$3 AND a.state='question_wait' AND t.state='question_wait'
 AND t.number=(SELECT max(number) FROM mailbox.work_attempts WHERE project_id=q.project_id AND assignment_id=q.assignment_id)`, project, id, contract).Scan(&fingerprint)
	if err != nil {
		return "", ErrStale
	}
	return fingerprint, nil
}
func answerProposal(proposal council.Proposal, subject string) bool {
	if len(proposal.Actions) != 1 {
		return false
	}
	action := proposal.Actions[0]
	return action.Kind == "answer" && action.Target == strings.TrimPrefix(subject, "question:") && len(strings.TrimSpace(action.Value)) > 0 && len(action.Value) <= 16384 && utf8.ValidString(action.Value) && !strings.ContainsRune(action.Value, 0)
}
