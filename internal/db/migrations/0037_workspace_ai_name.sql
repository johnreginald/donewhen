-- What the AI actor is called in this workspace's UI and notifications.
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS ai_name text NOT NULL DEFAULT 'Clanker';
