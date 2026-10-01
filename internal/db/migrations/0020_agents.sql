-- Agents, the machines that run them, and the work queued between the two.
--
-- DoneWhen runs on one machine and the agents on another (where the repos and
-- the logged-in CLIs are). So DoneWhen never runs an agent itself: it queues a
-- job, a runner host claims it, and reports back. The host also reports what
-- it can run, so the UI shows a connection as what a machine has proved, not
-- as a stored credential.

CREATE TABLE agents (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name            text NOT NULL,
    slug            text NOT NULL,
    role            text NOT NULL DEFAULT '',
    harness         text NOT NULL,               -- claude | codex | opencode
    model           text NOT NULL DEFAULT '',    -- empty = the harness's own default
    effort          text NOT NULL DEFAULT '',
    instructions_md text NOT NULL DEFAULT '',
    allowed_tools   jsonb NOT NULL DEFAULT '[]', -- extra permission rules, e.g. "Bash(go test *)"
    max_turns       int  NOT NULL DEFAULT 0,     -- 0 = no limit
    status          text NOT NULL DEFAULT 'active',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agents_harness_chk CHECK (harness IN ('claude', 'codex', 'opencode')),
    CONSTRAINT agents_status_chk CHECK (status IN ('active', 'paused')),
    UNIQUE (workspace_id, slug)
);

CREATE TABLE runner_hosts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL,
    harnesses    jsonb NOT NULL DEFAULT '[]',   -- what the host last proved it can run
    version      text NOT NULL DEFAULT '',
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE TABLE jobs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind         text NOT NULL,                -- test_env | run_ticket
    agent_id     uuid REFERENCES agents(id) ON DELETE CASCADE,
    issue_id     uuid REFERENCES issues(id) ON DELETE CASCADE,
    status       text NOT NULL DEFAULT 'queued',
    host         text NOT NULL DEFAULT '',
    result       jsonb NOT NULL DEFAULT '{}',
    error        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    claimed_at   timestamptz,
    finished_at  timestamptz,
    CONSTRAINT jobs_kind_chk CHECK (kind IN ('test_env', 'run_ticket')),
    CONSTRAINT jobs_status_chk CHECK (status IN ('queued', 'claimed', 'succeeded', 'failed', 'canceled'))
);
CREATE INDEX jobs_queue_idx ON jobs(workspace_id, status, created_at);

ALTER TABLE issues ADD COLUMN agent_id uuid REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE runs   ADD COLUMN agent_id uuid REFERENCES agents(id) ON DELETE SET NULL;
