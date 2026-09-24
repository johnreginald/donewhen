-- Runs: one row per agent attempt, so the work an agent did is visible where the
-- ticket is instead of in a log file on the machine that ran it.
--
-- Tokens are kept by kind. One total misleads: a cache read costs a tenth of
-- fresh input and is most of what a long session consumes.

CREATE TABLE runs (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id          uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    issue_id              uuid REFERENCES issues(id) ON DELETE CASCADE,
    runner                text NOT NULL,                 -- claude | codex | opencode
    model                 text NOT NULL DEFAULT '',
    attempt               int  NOT NULL DEFAULT 1,
    status                text NOT NULL DEFAULT 'running',
    verdict               text NOT NULL DEFAULT '',      -- what the criteria decided
    session_id            text NOT NULL DEFAULT '',
    exit_code             int,
    agent_error           text NOT NULL DEFAULT '',
    tokens_input          int  NOT NULL DEFAULT 0,
    tokens_cache_read     int  NOT NULL DEFAULT 0,
    tokens_cache_creation int  NOT NULL DEFAULT 0,
    tokens_output         int  NOT NULL DEFAULT 0,
    tokens_total          int  NOT NULL DEFAULT 0,
    cost_usd              numeric(12,6) NOT NULL DEFAULT 0,  -- counted against budgets
    notional_cost_usd     numeric(12,6) NOT NULL DEFAULT 0,  -- list price of a subscription run
    billing               text NOT NULL DEFAULT 'unknown',
    denied_tools          jsonb NOT NULL DEFAULT '[]',
    log_tail              text NOT NULL DEFAULT '',          -- redacted, capped
    host                  text NOT NULL DEFAULT '',
    started_at            timestamptz NOT NULL DEFAULT now(),
    finished_at           timestamptz,
    CONSTRAINT runs_status_chk  CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'aborted')),
    CONSTRAINT runs_billing_chk CHECK (billing IN ('subscription', 'api', 'unknown'))
);
CREATE INDEX runs_issue_idx ON runs(issue_id, started_at DESC);
CREATE INDEX runs_workspace_idx ON runs(workspace_id, started_at DESC);
