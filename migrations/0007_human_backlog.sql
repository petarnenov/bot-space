ALTER TABLE mailbox.orchestration_projects ADD UNIQUE (workspace_id,id);
CREATE TABLE mailbox.human_intentions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL,
 project_id uuid NOT NULL,
 creator_user_id uuid NOT NULL REFERENCES mailbox.users(id),
 idempotency_key text NOT NULL CHECK (idempotency_key ~ '^[ -~]{1,128}$'),
 fingerprint text NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
 current_revision integer NOT NULL DEFAULT 1 CHECK (current_revision > 0),
 state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','discussing','capacity_wait','executing','question_wait','review','blocked','completed','cancelled')),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id,id),
 UNIQUE(project_id,creator_user_id,idempotency_key),
 FOREIGN KEY(workspace_id,project_id) REFERENCES mailbox.orchestration_projects(workspace_id,id)
);
CREATE TABLE mailbox.human_intention_revisions (
 project_id uuid NOT NULL,
 intention_id uuid NOT NULL,
 revision integer NOT NULL CHECK (revision>0),
 author_user_id uuid NOT NULL REFERENCES mailbox.users(id),
 title text NOT NULL CHECK (octet_length(title) BETWEEN 1 AND 256),
 description text NOT NULL CHECK (octet_length(description) BETWEEN 1 AND 32768),
 ticket_reference text NOT NULL DEFAULT '' CHECK (octet_length(ticket_reference)<=2048),
 priority integer NOT NULL CHECK (priority BETWEEN 0 AND 4),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(project_id,intention_id,revision),
 FOREIGN KEY(project_id,intention_id) REFERENCES mailbox.human_intentions(project_id,id)
);
ALTER TABLE mailbox.human_intentions ADD CONSTRAINT human_current_revision
 FOREIGN KEY(project_id,id,current_revision) REFERENCES mailbox.human_intention_revisions(project_id,intention_id,revision)
 DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX human_backlog_project ON mailbox.human_intentions(project_id,created_at,id);
