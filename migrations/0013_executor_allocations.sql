CREATE TABLE mailbox.executor_presence (
 project_id uuid NOT NULL,
 runner_id uuid NOT NULL,
 credential_epoch bigint NOT NULL CHECK(credential_epoch>0),
 available boolean NOT NULL,
 client jsonb NOT NULL CHECK(octet_length(client::text)<=8192),
 client_hash text NOT NULL CHECK(client_hash ~ '^[0-9a-f]{64}$'),
 lease_until timestamptz NOT NULL,
 PRIMARY KEY(project_id,runner_id),
 FOREIGN KEY(project_id,runner_id) REFERENCES mailbox.project_runners(project_id,id)
);
CREATE TABLE mailbox.work_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL,
 root_id uuid NOT NULL,
 contract_id uuid NOT NULL,
 decision_id uuid NOT NULL,
 task_id text NOT NULL CHECK(task_id ~ '^[0-9]+(\.[0-9]+)+$'),
 executor_id uuid NOT NULL,
 owner_github_id bigint NOT NULL CHECK(owner_github_id>0),
 public_key bytea NOT NULL CHECK(octet_length(public_key)=32),
 slot_generation bigint NOT NULL CHECK(slot_generation>0),
 client jsonb NOT NULL CHECK(octet_length(client::text)<=8192),
 client_hash text NOT NULL CHECK(client_hash ~ '^[0-9a-f]{64}$'),
 decision_kind text GENERATED ALWAYS AS ('allocation'::text) STORED,
 decision_subject text GENERATED ALWAYS AS ('task:'::text || task_id) STORED,
 state text NOT NULL DEFAULT 'reserved' CHECK(state IN ('reserved','running','question_wait','review','interrupted','completed','cancelled')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(project_id,id),
 UNIQUE(project_id,decision_id),
 UNIQUE(id,owner_github_id,public_key),
 FOREIGN KEY(project_id,root_id) REFERENCES mailbox.human_intentions(project_id,id),
 FOREIGN KEY(project_id,contract_id,root_id) REFERENCES mailbox.work_contracts(project_id,id,root_id),
 FOREIGN KEY(project_id,decision_id) REFERENCES mailbox.council_decisions(project_id,id),
 FOREIGN KEY(project_id,decision_id,root_id,decision_kind,decision_subject) REFERENCES mailbox.council_decisions(project_id,id,root_id,kind,subject),
 FOREIGN KEY(project_id,executor_id) REFERENCES mailbox.project_runners(project_id,id)
);
CREATE UNIQUE INDEX work_assignments_active_task ON mailbox.work_assignments(project_id,root_id,task_id)
 WHERE state IN ('reserved','running','question_wait','review','interrupted');
CREATE TABLE mailbox.executor_slots (
 owner_github_id bigint NOT NULL CHECK(owner_github_id>0),
 public_key bytea NOT NULL CHECK(octet_length(public_key)=32),
 generation bigint NOT NULL DEFAULT 0 CHECK(generation>=0),
 assignment_id uuid,
 PRIMARY KEY(owner_github_id,public_key),
 FOREIGN KEY(assignment_id,owner_github_id,public_key) REFERENCES mailbox.work_assignments(id,owner_github_id,public_key)
);
