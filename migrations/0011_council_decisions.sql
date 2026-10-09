ALTER TABLE mailbox.work_contracts ADD UNIQUE(project_id,id,root_id);
CREATE TABLE mailbox.council_decisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL,
 root_id uuid NOT NULL,
 contract_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('plan','allocation','answer','review','retry','integration')),
 previous_id uuid,
 material jsonb NOT NULL,
 snapshot jsonb NOT NULL CHECK(octet_length(snapshot::text)<=1048576),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(project_id,id),
 UNIQUE(project_id,id,root_id,kind),
 FOREIGN KEY(project_id,root_id) REFERENCES mailbox.human_intentions(project_id,id),
 FOREIGN KEY(project_id,contract_id) REFERENCES mailbox.work_contracts(project_id,id),
 FOREIGN KEY(project_id,contract_id,root_id) REFERENCES mailbox.work_contracts(project_id,id,root_id),
 FOREIGN KEY(project_id,previous_id) REFERENCES mailbox.council_decisions(project_id,id)
);
CREATE TABLE mailbox.council_heads (
 project_id uuid NOT NULL,
 root_id uuid NOT NULL,
 kind text NOT NULL,
 decision_id uuid NOT NULL,
 PRIMARY KEY(project_id,root_id,kind),
 FOREIGN KEY(project_id,decision_id) REFERENCES mailbox.council_decisions(project_id,id),
 FOREIGN KEY(project_id,decision_id,root_id,kind) REFERENCES mailbox.council_decisions(project_id,id,root_id,kind),
 FOREIGN KEY(project_id,root_id) REFERENCES mailbox.human_intentions(project_id,id)
);
CREATE TABLE mailbox.council_rounds (
 project_id uuid NOT NULL,
 decision_id uuid NOT NULL,
 round integer NOT NULL CHECK(round BETWEEN 1 AND 3),
 proposal_hash text NOT NULL CHECK(proposal_hash ~ '^[0-9a-f]{64}$'),
 proposal jsonb NOT NULL,
 PRIMARY KEY(project_id,decision_id,round),
 UNIQUE(project_id,decision_id,round,proposal_hash),
 FOREIGN KEY(project_id,decision_id) REFERENCES mailbox.council_decisions(project_id,id)
);
CREATE TABLE mailbox.council_votes (
 project_id uuid NOT NULL,
 decision_id uuid NOT NULL,
 round integer NOT NULL,
 runner_id uuid NOT NULL,
 choice text NOT NULL CHECK(choice IN ('approve','object','need_information')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(project_id,decision_id,round,runner_id),
 FOREIGN KEY(project_id,decision_id,round) REFERENCES mailbox.council_rounds(project_id,decision_id,round),
 FOREIGN KEY(project_id,runner_id) REFERENCES mailbox.project_runners(project_id,id)
);
CREATE TABLE mailbox.council_commits (
 project_id uuid NOT NULL,
 decision_id uuid NOT NULL,
 round integer NOT NULL,
 proposal_hash text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(project_id,decision_id),
 FOREIGN KEY(project_id,decision_id,round,proposal_hash) REFERENCES mailbox.council_rounds(project_id,decision_id,round,proposal_hash)
);
CREATE TABLE mailbox.council_coordinators (
 project_id uuid NOT NULL,
 decision_id uuid NOT NULL,
 runner_id uuid NOT NULL,
 epoch bigint NOT NULL DEFAULT 1 CHECK(epoch>0),
 expires_at timestamptz NOT NULL,
 PRIMARY KEY(project_id,decision_id),
 FOREIGN KEY(project_id,decision_id) REFERENCES mailbox.council_decisions(project_id,id),
 FOREIGN KEY(project_id,runner_id) REFERENCES mailbox.project_runners(project_id,id)
);
CREATE FUNCTION mailbox.reject_council_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'Council evidence is immutable';
END;
$$;
CREATE TRIGGER council_round_immutable BEFORE UPDATE OR DELETE ON mailbox.council_rounds
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_council_evidence_mutation();
CREATE TRIGGER council_vote_immutable BEFORE UPDATE OR DELETE ON mailbox.council_votes
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_council_evidence_mutation();
CREATE TRIGGER council_commit_immutable BEFORE UPDATE OR DELETE ON mailbox.council_commits
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_council_evidence_mutation();
