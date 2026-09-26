-- Commands every agent in a workspace may run without asking, on top of the
-- built-in defaults and each agent's own rules.
ALTER TABLE workspaces ADD COLUMN allowed_tools jsonb NOT NULL DEFAULT '[]';
