package workallocation

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/controlevents"
	"github.com/petarnenov/bot-space/internal/councilstore"
	"github.com/petarnenov/bot-space/internal/security"
)

type Answer struct{ Question, Decision, Session, Text string }

// AcceptAnswer persists one majority answer for the current executor session.
// Actual native delivery must deduplicate this record before resuming the client.
func (s *Store) AcceptAnswer(ctx context.Context, token, id, question, decision, session string, epoch int64) (Answer, error) {
	if !security.ValidUUID(id) || !security.ValidUUID(question) || !security.ValidUUID(decision) || len(session) < 1 || len(session) > 256 || epoch < 1 {
		return Answer{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_EXECUTOR)
	if err != nil {
		return Answer{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Answer{}, err
	}
	if _, err = execution(ctx, tx, p, id); err != nil {
		return Answer{}, err
	}
	a, err := attempt(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Answer{}, err
	}
	if a.Session != session || a.AuthorityEpoch != epoch || (a.State != "running" && a.State != "question_wait") {
		return Answer{}, ErrInvalid
	}
	var fingerprint, contract, digest string
	var live bool
	err = tx.QueryRow(ctx, `SELECT q.fingerprint,c.id::text,c.content_hash,t.authority_until>clock_timestamp() FROM mailbox.executor_questions q
 JOIN mailbox.work_assignments a ON a.project_id=q.project_id AND a.id=q.assignment_id JOIN mailbox.work_contracts c ON c.project_id=a.project_id AND c.id=a.contract_id
 JOIN mailbox.work_attempts t ON t.project_id=q.project_id AND t.id=q.attempt_id
 WHERE q.project_id=$1 AND q.id=$2 AND q.assignment_id=$3 AND q.attempt_id=$4 AND q.session_id=$5 AND q.authority_epoch=$6`, p.ProjectID, question, id, a.ID, session, epoch).Scan(&fingerprint, &contract, &digest, &live)
	if err != nil {
		return Answer{}, ErrInvalid
	}
	if !live {
		return Answer{}, ErrAuthorityLost
	}
	out := Answer{Question: question, Decision: decision, Session: session}
	var oldDecision string
	err = tx.QueryRow(ctx, `SELECT decision_id::text,answer FROM mailbox.executor_question_answers WHERE project_id=$1 AND question_id=$2`, p.ProjectID, question).Scan(&oldDecision, &out.Text)
	if err == nil {
		if oldDecision != decision {
			return Answer{}, ErrQuestionConflict
		}
		if tx.Commit(ctx) != nil {
			return Answer{}, ErrUnavailable
		}
		return out, nil
	}
	if err != pgx.ErrNoRows {
		return Answer{}, ErrUnavailable
	}
	record, err := councilstore.LockAccepted(ctx, tx, p.ProjectID, decision, "answer", "question:"+question, digest)
	if err != nil {
		return Answer{}, err
	}
	if record.Contract != contract || record.Material.EvidenceDigest != fingerprint {
		return Answer{}, ErrInvalid
	}
	round := record.Snapshot.Rounds[len(record.Snapshot.Rounds)-1]
	if len(round.Proposal.Actions) != 1 || round.Proposal.Actions[0].Kind != "answer" || round.Proposal.Actions[0].Target != question {
		return Answer{}, ErrInvalid
	}
	out.Text = round.Proposal.Actions[0].Value
	if _, err = tx.Exec(ctx, `INSERT INTO mailbox.executor_question_answers(project_id,question_id,decision_id,answer) VALUES($1,$2,$3,$4)`, p.ProjectID, question, decision, out.Text); err != nil {
		return Answer{}, ErrUnavailable
	}
	var pending bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.executor_questions q WHERE q.project_id=$1 AND q.attempt_id=$2 AND NOT EXISTS(SELECT 1 FROM mailbox.executor_question_answers r WHERE r.project_id=q.project_id AND r.question_id=q.id))`, p.ProjectID, a.ID).Scan(&pending)
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	if !pending {
		if _, err = tx.Exec(ctx, `UPDATE mailbox.work_attempts SET state='running' WHERE id=$1`, a.ID); err != nil {
			return Answer{}, ErrUnavailable
		}
		if _, err = tx.Exec(ctx, `UPDATE mailbox.work_assignments SET state='running' WHERE project_id=$1 AND id=$2`, p.ProjectID, id); err != nil {
			return Answer{}, ErrUnavailable
		}
	}
	_, err = controlevents.PublishTx(ctx, tx, p.ProjectID, p.RunnerID, "answer:"+question, &pb.ServerFrame{Body: &pb.ServerFrame_Answer{Answer: &pb.Answer{QuestionId: question, AssignmentId: id, AttemptEpoch: uint64(epoch), ContractHash: digest, SessionId: session, Answer: out.Text, DecisionId: decision}}})
	if err != nil {
		return Answer{}, err
	}
	if tx.Commit(ctx) != nil {
		return Answer{}, ErrUnavailable
	}
	return out, nil
}
