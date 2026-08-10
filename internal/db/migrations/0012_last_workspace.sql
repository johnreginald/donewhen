-- Remember which workspace a user was last in, so a fresh session lands where
-- they left off instead of defaulting to whichever membership sorts first.
-- SET NULL rather than CASCADE: losing a workspace should not delete the user.

ALTER TABLE users ADD COLUMN IF NOT EXISTS last_workspace_id uuid
    REFERENCES workspaces(id) ON DELETE SET NULL;
