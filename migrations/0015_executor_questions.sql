ALTER TABLE mailbox.work_attempts ADD UNIQUE(project_id,id,assignment_id,session_id);
CREATE TABLE mailbox.executor_questions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL,
 assignment_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 session_id text NOT NULL,
 authority_epoch bigint NOT NULL CHECK(authority_epoch>0),
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 context jsonb NOT NULL CHECK(octet_length(context::text)<=65536),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(project_id,id),
 UNIQUE(project_id,id,attempt_id),
 UNIQUE(project_id,attempt_id,fingerprint),
 FOREIGN KEY(project_id,attempt_id,assignment_id,session_id) REFERENCES mailbox.work_attempts(project_id,id,assignment_id,session_id)
);
CREATE TABLE mailbox.executor_question_requests (
 project_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 request_key text NOT NULL CHECK(request_key ~ '^[ -~]{1,128}$'),
 question_id uuid NOT NULL,
 PRIMARY KEY(project_id,attempt_id,request_key),
 FOREIGN KEY(project_id,question_id,attempt_id) REFERENCES mailbox.executor_questions(project_id,id,attempt_id)
);
CREATE TRIGGER question_immutable BEFORE UPDATE OR DELETE ON mailbox.executor_questions
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_council_evidence_mutation();
CREATE TRIGGER question_request_immutable BEFORE UPDATE OR DELETE ON mailbox.executor_question_requests
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_council_evidence_mutation();
