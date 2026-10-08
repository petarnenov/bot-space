CREATE TABLE mailbox.agents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES mailbox.workspaces(id),
    owner_user_id uuid NOT NULL REFERENCES mailbox.users(id),
    name text NOT NULL CHECK (octet_length(name) BETWEEN 1 AND 128),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id)
);
CREATE INDEX agents_owner ON mailbox.agents (workspace_id, owner_user_id, id);

CREATE TABLE mailbox.agent_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    secret_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    FOREIGN KEY (workspace_id, agent_id) REFERENCES mailbox.agents(workspace_id, id)
);
CREATE INDEX credentials_agent ON mailbox.agent_credentials (workspace_id, agent_id, id);

CREATE FUNCTION mailbox.revoke_removed_member_agents() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    removed_agent record;
    removed_credential record;
    actor uuid := NULLIF(current_setting('mailbox.audit_actor', true), '')::uuid;
    kind text := CASE WHEN actor IS NULL THEN 'system' ELSE 'human' END;
BEGIN
    FOR removed_agent IN
        SELECT id, active FROM mailbox.agents
        WHERE workspace_id = OLD.workspace_id AND owner_user_id = OLD.user_id
        ORDER BY id FOR UPDATE
    LOOP
        IF removed_agent.active THEN
            UPDATE mailbox.agents SET active = false
            WHERE workspace_id = OLD.workspace_id AND id = removed_agent.id;
            INSERT INTO mailbox.audit_events (workspace_id, actor_kind, actor_user_id, action, target_id)
            VALUES (OLD.workspace_id, kind, actor, 'agent_deactivated_by_member_removal', removed_agent.id::text);
        END IF;
        FOR removed_credential IN
            UPDATE mailbox.agent_credentials SET revoked_at = clock_timestamp()
            WHERE workspace_id = OLD.workspace_id AND agent_id = removed_agent.id AND revoked_at IS NULL
            RETURNING id
        LOOP
            INSERT INTO mailbox.audit_events (workspace_id, actor_kind, actor_user_id, action, target_id)
            VALUES (OLD.workspace_id, kind, actor, 'credential_revoked_by_member_removal', removed_credential.id::text);
        END LOOP;
    END LOOP;
    RETURN OLD;
END;
$$;

CREATE TRIGGER revoke_removed_member_agents
BEFORE DELETE ON mailbox.memberships
FOR EACH ROW EXECUTE FUNCTION mailbox.revoke_removed_member_agents();
