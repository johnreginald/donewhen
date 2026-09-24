-- Routines: tickets that make themselves on a schedule, and agents that wake
-- on a timer to answer what is waiting for them.

CREATE TABLE routines (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name           text NOT NULL,
    agent_id       uuid REFERENCES agents(id) ON DELETE SET NULL,
    project_id     uuid REFERENCES projects(id) ON DELETE SET NULL,
    title          text NOT NULL,              -- {date} becomes the day it fires
    description_md text NOT NULL DEFAULT '',
    criteria       jsonb NOT NULL DEFAULT '[]', -- typed done-when for each ticket
    schedule       text NOT NULL,              -- five-field cron
    timezone       text NOT NULL DEFAULT 'UTC',
    enabled        boolean NOT NULL DEFAULT true,
    auto_run       boolean NOT NULL DEFAULT false, -- also queue a run for the agent
    next_run_at    timestamptz,
    last_run_at    timestamptz,
    last_issue_id  uuid REFERENCES issues(id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX routines_due_idx ON routines(next_run_at) WHERE enabled;

ALTER TABLE agents ADD COLUMN heartbeat_minutes int NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN last_heartbeat_at timestamptz;
