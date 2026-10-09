-- Project access is GitHub-derived; legacy mailbox memberships are independent.
CREATE TABLE mailbox.orchestration_projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES mailbox.workspaces(id),
    repository_id bigint NOT NULL CHECK (repository_id > 0),
    repository_owner_id bigint NOT NULL CHECK (repository_owner_id > 0),
    repository_owner text NOT NULL,
    repository_name text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, repository_id)
);

CREATE TABLE mailbox.runner_enrollments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES mailbox.orchestration_projects(id),
    public_key bytea NOT NULL CHECK (octet_length(public_key) = 32),
    role text NOT NULL CHECK (role IN ('architect','executor')),
    client_nonce_hash text NOT NULL CHECK (client_nonce_hash ~ '^[0-9a-f]{64}$'),
    authorized_github_id bigint CHECK (authorized_github_id > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '10 minutes',
    authorized_at timestamptz,
    CHECK ((authorized_github_id IS NULL) = (authorized_at IS NULL)),
    UNIQUE (project_id, public_key, role, client_nonce_hash)
);
CREATE INDEX runner_enrollments_expiry ON mailbox.runner_enrollments(expires_at);

CREATE TABLE mailbox.project_runners (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES mailbox.orchestration_projects(id),
    owner_github_id bigint NOT NULL CHECK (owner_github_id > 0),
    public_key bytea NOT NULL CHECK (octet_length(public_key) = 32),
    role text NOT NULL CHECK (role IN ('architect','executor')),
    active boolean NOT NULL DEFAULT true,
    credential_epoch bigint NOT NULL DEFAULT 1 CHECK (credential_epoch > 0),
    credential_hash text UNIQUE CHECK (credential_hash ~ '^[0-9a-f]{64}$'),
    credential_expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, public_key, role),
    UNIQUE (project_id,id),
    CHECK ((credential_hash IS NULL) = (credential_expires_at IS NULL))
);

CREATE TABLE mailbox.runner_key_challenges (
    nonce_hash text PRIMARY KEY CHECK (nonce_hash ~ '^[0-9a-f]{64}$'),
    enrollment_id uuid REFERENCES mailbox.runner_enrollments(id),
    runner_id uuid REFERENCES mailbox.project_runners(id),
    purpose text NOT NULL CHECK (purpose IN ('claim','refresh')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '1 minute',
    CHECK ((purpose='claim' AND enrollment_id IS NOT NULL AND runner_id IS NULL) OR
           (purpose='refresh' AND runner_id IS NOT NULL AND enrollment_id IS NULL))
);
CREATE INDEX runner_key_challenges_expiry ON mailbox.runner_key_challenges(expires_at);
