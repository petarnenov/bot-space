ALTER TABLE mailbox.human_intentions DROP CONSTRAINT human_intentions_state_check;
ALTER TABLE mailbox.human_intentions ADD CONSTRAINT human_intentions_state_check
 CHECK (state IN ('queued','discussing','capacity_wait','executing','question_wait','review','blocked','completed','cancelled','paused'));
ALTER TABLE mailbox.human_intentions
 ADD COLUMN lifecycle_epoch bigint NOT NULL DEFAULT 1 CHECK (lifecycle_epoch>0),
 ADD COLUMN archived boolean NOT NULL DEFAULT false,
 ADD COLUMN reconciliation_required boolean NOT NULL DEFAULT false;
CREATE TABLE mailbox.human_intention_lifecycle (
 project_id uuid NOT NULL,
 intention_id uuid NOT NULL,
 epoch bigint NOT NULL CHECK (epoch>1),
 actor_user_id uuid NOT NULL REFERENCES mailbox.users(id),
 action text NOT NULL CHECK (action IN ('pause','resume','cancel','archive')),
 previous_state text NOT NULL,
 resulting_state text NOT NULL,
 archived boolean NOT NULL,
 reconciliation_required boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(project_id,intention_id,epoch),
 FOREIGN KEY(project_id,intention_id) REFERENCES mailbox.human_intentions(project_id,id)
);
CREATE FUNCTION mailbox.reject_lifecycle_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'Lifecycle history is immutable';
END;
$$;
CREATE TRIGGER lifecycle_immutable BEFORE UPDATE OR DELETE ON mailbox.human_intention_lifecycle
 FOR EACH ROW EXECUTE FUNCTION mailbox.reject_lifecycle_mutation();
