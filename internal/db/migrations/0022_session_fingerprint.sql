-- A session resumes only into the setup it began in: same repository, same
-- harness and model, same instructions. The fingerprint says which setup.
-- created_at lets an old session be retired and started afresh.
ALTER TABLE agent_sessions ADD COLUMN fingerprint text NOT NULL DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
