CREATE TABLE mailbox.conversations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES mailbox.workspaces(id),
    participant_a uuid NOT NULL,
    participant_b uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, participant_a) REFERENCES mailbox.agents(workspace_id, id),
    FOREIGN KEY (workspace_id, participant_b) REFERENCES mailbox.agents(workspace_id, id),
    CHECK (participant_a <= participant_b)
);

CREATE TABLE mailbox.inbox_counters (
    workspace_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    PRIMARY KEY (workspace_id, agent_id),
    FOREIGN KEY (workspace_id, agent_id) REFERENCES mailbox.agents(workspace_id, id)
);

CREATE TABLE mailbox.messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    from_agent_id uuid NOT NULL,
    to_agent_id uuid NOT NULL,
    thread_id uuid NOT NULL,
    in_reply_to uuid,
    kind text NOT NULL CHECK (kind ~ '^[A-Za-z0-9_.:-]{1,64}$'),
    text text NOT NULL CHECK (octet_length(text) BETWEEN 1 AND 16384),
    metadata json NOT NULL DEFAULT '{}' CHECK (json_typeof(metadata) = 'object' AND octet_length(metadata::text) <= 8192),
    created_at timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    inbox_sequence bigint NOT NULL CHECK (inbox_sequence > 0),
    idempotency_key text NOT NULL CHECK (idempotency_key ~ '^[ -~]{1,128}$'),
    fingerprint text NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, from_agent_id, idempotency_key),
    UNIQUE (workspace_id, to_agent_id, inbox_sequence),
    FOREIGN KEY (workspace_id, from_agent_id) REFERENCES mailbox.agents(workspace_id, id),
    FOREIGN KEY (workspace_id, to_agent_id) REFERENCES mailbox.agents(workspace_id, id),
    FOREIGN KEY (workspace_id, thread_id) REFERENCES mailbox.conversations(workspace_id, id),
    FOREIGN KEY (workspace_id, in_reply_to) REFERENCES mailbox.messages(workspace_id, id)
);
CREATE INDEX messages_unacknowledged ON mailbox.messages (workspace_id, to_agent_id, inbox_sequence) WHERE acknowledged_at IS NULL;
CREATE INDEX messages_thread ON mailbox.messages (workspace_id, to_agent_id, thread_id, inbox_sequence);
