CREATE TABLE mailbox.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    github_id bigint NOT NULL UNIQUE CHECK (github_id > 0),
    username text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mailbox.workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE CHECK (length(slug) BETWEEN 1 AND 63),
    bootstrap_github_id bigint NOT NULL CHECK (bootstrap_github_id > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mailbox.memberships (
    workspace_id uuid NOT NULL REFERENCES mailbox.workspaces(id),
    user_id uuid NOT NULL REFERENCES mailbox.users(id),
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE TABLE mailbox.invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES mailbox.workspaces(id),
    target_github_id bigint NOT NULL CHECK (target_github_id > 0),
    role text NOT NULL CHECK (role IN ('admin', 'member')),
    secret_hash text NOT NULL UNIQUE,
    created_by uuid NOT NULL REFERENCES mailbox.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    cancelled_at timestamptz,
    UNIQUE (workspace_id, id),
    CHECK (expires_at > created_at),
    CHECK (accepted_at IS NULL OR cancelled_at IS NULL)
);

CREATE TABLE mailbox.oauth_attempts (
    state_hash text PRIMARY KEY,
    verifier text NOT NULL,
    return_path text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mailbox.sessions (
    id_hash text PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES mailbox.users(id),
    csrf_token text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX sessions_user_id ON mailbox.sessions (user_id);

CREATE TABLE mailbox.audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES mailbox.workspaces(id),
    actor_kind text NOT NULL CHECK (actor_kind IN ('system', 'human', 'agent')),
    actor_user_id uuid REFERENCES mailbox.users(id),
    action text NOT NULL,
    target_id text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_workspace_id ON mailbox.audit_events (workspace_id, id);
