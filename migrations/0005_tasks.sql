CREATE TABLE mailbox.runner_ownership (
    workspace_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    runner_id uuid NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    lease_until timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, agent_id),
    FOREIGN KEY (workspace_id, agent_id) REFERENCES mailbox.agents(workspace_id, id)
);

CREATE TABLE mailbox.tasks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    from_agent_id uuid NOT NULL,
    to_agent_id uuid NOT NULL,
    parent_task_id uuid,
    root_task_id uuid NOT NULL,
    depth integer NOT NULL DEFAULT 1 CHECK (depth BETWEEN 1 AND 4),
    ancestor_agents uuid[] NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key ~ '^[ -~]{1,128}$'),
    fingerprint text NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    instruction text NOT NULL CHECK (octet_length(instruction) BETWEEN 1 AND 16384),
    retry_safe boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','waiting_dependency','completed','failed','cancelled','expired','interrupted','requires_approval')),
    deadline timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    generation bigint NOT NULL DEFAULT 0 CHECK (generation >= 0),
    runner_id uuid,
    runner_generation bigint,
    lease_until timestamptz,
    result text CHECK (octet_length(result) <= 65536),
    error_code text CHECK (error_code ~ '^[a-z_]{1,64}$'),
    request_message_id uuid NOT NULL,
    reply_message_id uuid,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, from_agent_id, idempotency_key),
    UNIQUE (workspace_id, request_message_id),
    FOREIGN KEY (workspace_id, from_agent_id) REFERENCES mailbox.agents(workspace_id, id),
    FOREIGN KEY (workspace_id, to_agent_id) REFERENCES mailbox.agents(workspace_id, id),
    FOREIGN KEY (workspace_id, parent_task_id) REFERENCES mailbox.tasks(workspace_id, id),
    FOREIGN KEY (workspace_id, root_task_id) REFERENCES mailbox.tasks(workspace_id, id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id, request_message_id) REFERENCES mailbox.messages(workspace_id, id),
    FOREIGN KEY (workspace_id, reply_message_id) REFERENCES mailbox.messages(workspace_id, id),
    CHECK (from_agent_id <> to_agent_id),
    CHECK (cardinality(ancestor_agents) = depth),
    CHECK (deadline > created_at),
    CHECK ((status = 'completed' AND result IS NOT NULL AND error_code IS NULL) OR status <> 'completed'),
    CHECK ((runner_id IS NULL AND runner_generation IS NULL AND lease_until IS NULL) OR (runner_id IS NOT NULL AND runner_generation > 0 AND lease_until IS NOT NULL))
);
CREATE INDEX tasks_recipient_queue ON mailbox.tasks (workspace_id, to_agent_id, created_at, id) WHERE status = 'queued';
CREATE INDEX tasks_parent ON mailbox.tasks (workspace_id, parent_task_id);

CREATE TABLE mailbox.task_attempts (
    workspace_id uuid NOT NULL,
    task_id uuid NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    runner_id uuid NOT NULL,
    runner_generation bigint NOT NULL CHECK (runner_generation > 0),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status text NOT NULL CHECK (status IN ('running','completed','failed','cancelled','expired','interrupted','requires_approval')),
    error_code text CHECK (error_code ~ '^[a-z_]{1,64}$'),
    PRIMARY KEY (workspace_id, task_id, generation),
    FOREIGN KEY (workspace_id, task_id) REFERENCES mailbox.tasks(workspace_id, id)
);
