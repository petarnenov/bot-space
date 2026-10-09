CREATE TABLE mailbox.work_contracts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL,
 root_id uuid NOT NULL,
 root_revision integer NOT NULL,
 repository_id bigint NOT NULL CHECK(repository_id>0),
 publisher_runner_id uuid NOT NULL,
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 content jsonb NOT NULL CHECK(octet_length(content::text)<=65536),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id,id),
 UNIQUE(project_id,content_hash),
 FOREIGN KEY(project_id,root_id,root_revision) REFERENCES mailbox.human_intention_revisions(project_id,intention_id,revision),
 FOREIGN KEY(project_id,publisher_runner_id) REFERENCES mailbox.project_runners(project_id,id)
);
-- A contract is immutable input. Council acceptance is a separate record, so
-- publishing or receiving a contract cannot itself authorize execution.
