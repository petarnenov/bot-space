CREATE TABLE mailbox.work_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 project_id uuid NOT NULL,
 assignment_id uuid NOT NULL,
 number integer NOT NULL CHECK(number>0),
 session_id text CHECK(octet_length(session_id) BETWEEN 1 AND 256),
 state text NOT NULL DEFAULT 'starting' CHECK(state IN ('starting','running','question_wait','review','interrupted','completed','cancelled')),
 authority_epoch bigint NOT NULL DEFAULT 1 CHECK(authority_epoch>0),
 authority_until timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(project_id,id),
 UNIQUE(project_id,assignment_id,number),
 FOREIGN KEY(project_id,assignment_id) REFERENCES mailbox.work_assignments(project_id,id)
);
CREATE FUNCTION mailbox.guard_attempt_session() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.session_id IS NOT NULL AND NEW.session_id IS DISTINCT FROM OLD.session_id THEN
  RAISE EXCEPTION 'Attempt session is immutable';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER attempt_session_immutable BEFORE UPDATE ON mailbox.work_attempts
 FOR EACH ROW EXECUTE FUNCTION mailbox.guard_attempt_session();
