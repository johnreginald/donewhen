-- The agent a task gets when nobody chose one: one per priority, per
-- workspace. Choosing an agent on the task overrides it.
CREATE TABLE priority_agents (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    priority     int  NOT NULL CHECK (priority BETWEEN 0 AND 4),
    agent_id     uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    PRIMARY KEY (workspace_id, priority)
);
