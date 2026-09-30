-- Raenil is a tracker again, not a control plane: agents, the machines that
-- ran them, their runs, conversations, reviews and evidence are gone. Tickets,
-- comments, criteria, commits, documents and "blocked by" links stay.
DROP TABLE IF EXISTS evidence;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS run_events;
DROP TABLE IF EXISTS interactions;
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS priority_agents;
DROP TABLE IF EXISTS routines;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS runner_hosts;

ALTER TABLE comments   DROP COLUMN IF EXISTS agent_id;
ALTER TABLE issues     DROP COLUMN IF EXISTS agent_id;
ALTER TABLE issues     DROP COLUMN IF EXISTS run_when_unblocked;
ALTER TABLE projects   DROP COLUMN IF EXISTS autorun;
ALTER TABLE workspaces DROP COLUMN IF EXISTS allowed_tools;

DROP TABLE IF EXISTS agents;
