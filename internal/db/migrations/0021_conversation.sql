-- The conversation on a ticket: agents reply as themselves, ask structured
-- questions, and propose tickets; a chat turn resumes the agent's session.

ALTER TABLE comments ADD COLUMN agent_id uuid REFERENCES agents(id) ON DELETE SET NULL;

ALTER TABLE runs ADD COLUMN kind text NOT NULL DEFAULT 'work';
ALTER TABLE runs ADD CONSTRAINT runs_kind_chk CHECK (kind IN ('work', 'chat'));

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_chk;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_chk CHECK (kind IN ('test_env', 'run_ticket', 'chat'));
ALTER TABLE jobs ADD COLUMN input jsonb NOT NULL DEFAULT '{}';

-- An interaction is something an agent asked of a human on a ticket: a set of
-- questions, or a proposal to approve.
CREATE TABLE interactions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    issue_id     uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    agent_id     uuid REFERENCES agents(id) ON DELETE SET NULL,
    kind         text NOT NULL,
    payload      jsonb NOT NULL,
    status       text NOT NULL DEFAULT 'open',
    response     jsonb NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now(),
    resolved_at  timestamptz,
    CONSTRAINT interactions_kind_chk CHECK (kind IN ('questions', 'proposal')),
    CONSTRAINT interactions_status_chk CHECK (status IN ('open', 'answered', 'approved', 'rejected', 'canceled'))
);
CREATE INDEX interactions_issue_idx ON interactions(issue_id, created_at);

-- The agent's session on a ticket's conversation, so a turn resumes rather
-- than starting cold. cwd is kept because a session only resumes where it began.
CREATE TABLE agent_sessions (
    agent_id   uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    issue_id   uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    session_id text NOT NULL,
    cwd        text NOT NULL DEFAULT '',
    turns      int  NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, issue_id)
);
