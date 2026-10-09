ALTER TABLE mailbox.council_decisions ADD UNIQUE(project_id,id,kind,subject);
CREATE TABLE mailbox.executor_question_answers (
 project_id uuid NOT NULL,
 question_id uuid NOT NULL,
 decision_id uuid NOT NULL,
 decision_kind text GENERATED ALWAYS AS ('answer'::text) STORED,
 decision_subject text GENERATED ALWAYS AS ('question:'::text || question_id::text) STORED,
 answer text NOT NULL CHECK(octet_length(answer) BETWEEN 1 AND 16384),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(project_id,question_id),
 FOREIGN KEY(project_id,question_id) REFERENCES mailbox.executor_questions(project_id,id),
 FOREIGN KEY(project_id,decision_id) REFERENCES mailbox.council_decisions(project_id,id),
 FOREIGN KEY(project_id,decision_id,decision_kind,decision_subject) REFERENCES mailbox.council_decisions(project_id,id,kind,subject)
);
CREATE TRIGGER answer_immutable BEFORE UPDATE OR DELETE ON mailbox.executor_question_answers
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_council_evidence_mutation();
