CREATE TABLE mailbox.runner_deliveries (
 project_id uuid NOT NULL,
 runner_id uuid NOT NULL,
 issued bigint NOT NULL DEFAULT 0 CHECK(issued>=0),
 acknowledged bigint NOT NULL DEFAULT 0 CHECK(acknowledged>=0 AND acknowledged<=issued),
 PRIMARY KEY(project_id,runner_id),
 FOREIGN KEY(project_id,runner_id) REFERENCES mailbox.project_runners(project_id,id)
);
CREATE TABLE mailbox.control_outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL,
 runner_id uuid NOT NULL,
 sequence bigint NOT NULL CHECK(sequence>0),
 event_key text NOT NULL CHECK(event_key ~ '^[ -~]{1,128}$'),
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 payload bytea NOT NULL CHECK(octet_length(payload) BETWEEN 1 AND 131072),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(project_id,runner_id,sequence),
 UNIQUE(project_id,runner_id,event_key),
 FOREIGN KEY(project_id,runner_id) REFERENCES mailbox.runner_deliveries(project_id,runner_id)
);
CREATE TRIGGER outbox_immutable BEFORE UPDATE OR DELETE ON mailbox.control_outbox
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_council_evidence_mutation();
