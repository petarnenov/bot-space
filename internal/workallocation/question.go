package workallocation

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	pb "github.com/petarnenov/bot-space/api/control/v1"
	"github.com/petarnenov/bot-space/internal/security"
)

var ErrQuestionConflict = errors.New("question request conflicts with saved context")
var questionCommit = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var questionCredential = regexp.MustCompile(`bsr_[A-Za-z0-9_-]{43}`)

type QuestionContext struct {
	Question string   `json:"question"`
	Branch   string   `json:"branch"`
	Commit   string   `json:"commit"`
	Tried    []string `json:"tried"`
	Checks   []string `json:"checks"`
	Diff     string   `json:"diff"`
}
type Question struct {
	ID, Assignment, Attempt, Session string
	Epoch                            int64
	Context                          QuestionContext
	Answered                         bool
}

func questionContext(in QuestionContext, token string) (QuestionContext, []byte, string, error) {
	in.Question = strings.Join(strings.Fields(in.Question), " ")
	in.Commit = strings.ToLower(in.Commit)
	if len(in.Question) < 1 || len(in.Question) > 8192 || len(in.Branch) > 128 || !questionCommit.MatchString(in.Commit) || len(in.Tried) < 1 || len(in.Tried) > 16 || len(in.Checks) > 16 || len(in.Diff) > 32768 {
		return in, nil, "", ErrInvalid
	}
	in.Tried = append([]string(nil), in.Tried...)
	in.Checks = append([]string(nil), in.Checks...)
	scrub := func(value string) (string, error) {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return "", ErrInvalid
		}
		if token != "" {
			value = strings.ReplaceAll(value, token, "[redacted]")
		}
		return questionCredential.ReplaceAllString(value, "[redacted]"), nil
	}
	var err error
	if in.Question, err = scrub(in.Question); err != nil {
		return in, nil, "", err
	}
	if in.Diff, err = scrub(in.Diff); err != nil {
		return in, nil, "", err
	}
	for _, values := range [][]string{in.Tried, in.Checks} {
		for i, value := range values {
			if len(value) < 1 || len(value) > 2048 {
				return in, nil, "", ErrInvalid
			}
			values[i], err = scrub(strings.Join(strings.Fields(value), " "))
			if err != nil {
				return in, nil, "", err
			}
		}
	}
	raw, _ := json.Marshal(in)
	if len(raw) > 65536 {
		return in, nil, "", ErrInvalid
	}
	return in, raw, security.Hash(string(raw)), nil
}

// Ask persists an exact-session question and enters question wait atomically.
// Request labels are aliases: identical context cannot create a new question
// identity merely to restart a blocked council discussion.
func (s *Store) Ask(ctx context.Context, token, id, session, key string, epoch int64, input QuestionContext) (Question, error) {
	if !security.ValidUUID(id) || len(session) < 1 || len(session) > 256 || epoch < 1 || len(key) < 1 || len(key) > 128 {
		return Question{}, ErrInvalid
	}
	for _, char := range []byte(key) {
		if char < 32 || char > 126 {
			return Question{}, ErrInvalid
		}
	}
	in, raw, fingerprint, err := questionContext(input, token)
	if err != nil {
		return Question{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := s.authenticate(ctx, token, pb.Role_ROLE_EXECUTOR)
	if err != nil {
		return Question{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Question{}, ErrUnavailable
	}
	defer rollback(tx)
	if err = actor(ctx, tx, p, token); err != nil {
		return Question{}, err
	}
	if _, err = execution(ctx, tx, p, id); err != nil {
		return Question{}, err
	}
	a, err := attempt(ctx, tx, p.ProjectID, id)
	if err != nil {
		return Question{}, err
	}
	if a.Session != session || a.AuthorityEpoch != epoch || (a.State != "running" && a.State != "question_wait") {
		return Question{}, ErrInvalid
	}
	var live bool
	var branch string
	err = tx.QueryRow(ctx, `SELECT t.authority_until>clock_timestamp(),'openspec/'||(c.content->>'change') FROM mailbox.work_attempts t JOIN mailbox.work_assignments a ON a.id=t.assignment_id AND a.project_id=t.project_id JOIN mailbox.work_contracts c ON c.id=a.contract_id AND c.project_id=a.project_id WHERE t.id=$1`, a.ID).Scan(&live, &branch)
	if err != nil {
		return Question{}, ErrUnavailable
	}
	if !live {
		return Question{}, ErrAuthorityLost
	}
	if in.Branch != branch {
		return Question{}, ErrInvalid
	}
	var qid, prior string
	err = tx.QueryRow(ctx, `SELECT q.id::text,q.fingerprint FROM mailbox.executor_question_requests r JOIN mailbox.executor_questions q ON q.project_id=r.project_id AND q.id=r.question_id WHERE r.project_id=$1 AND r.attempt_id=$2 AND r.request_key=$3`, p.ProjectID, a.ID, key).Scan(&qid, &prior)
	if err != nil && err != pgx.ErrNoRows {
		return Question{}, ErrUnavailable
	}
	if err == nil && prior != fingerprint {
		return Question{}, ErrQuestionConflict
	}
	if qid == "" {
		err = tx.QueryRow(ctx, `SELECT id::text FROM mailbox.executor_questions WHERE project_id=$1 AND attempt_id=$2 AND fingerprint=$3`, p.ProjectID, a.ID, fingerprint).Scan(&qid)
		if err != nil && err != pgx.ErrNoRows {
			return Question{}, ErrUnavailable
		}
		if qid == "" {
			err = tx.QueryRow(ctx, `INSERT INTO mailbox.executor_questions(project_id,assignment_id,attempt_id,session_id,authority_epoch,fingerprint,context) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, p.ProjectID, id, a.ID, session, epoch, fingerprint, raw).Scan(&qid)
			if err != nil {
				return Question{}, ErrUnavailable
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO mailbox.executor_question_requests(project_id,attempt_id,request_key,question_id) VALUES($1,$2,$3,$4)`, p.ProjectID, a.ID, key, qid); err != nil {
			return Question{}, ErrUnavailable
		}
	}
	var answered bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mailbox.executor_question_answers WHERE project_id=$1 AND question_id=$2)`, p.ProjectID, qid).Scan(&answered); err != nil {
		return Question{}, ErrUnavailable
	}
	if answered {
		if tx.Commit(ctx) != nil {
			return Question{}, ErrUnavailable
		}
		return Question{ID: qid, Assignment: id, Attempt: a.ID, Session: session, Epoch: epoch, Context: in, Answered: true}, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE mailbox.work_attempts SET state='question_wait' WHERE id=$1`, a.ID); err != nil {
		return Question{}, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `UPDATE mailbox.work_assignments SET state='question_wait' WHERE project_id=$1 AND id=$2`, p.ProjectID, id); err != nil {
		return Question{}, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Question{}, ErrUnavailable
	}
	return Question{ID: qid, Assignment: id, Attempt: a.ID, Session: session, Epoch: epoch, Context: in}, nil
}
