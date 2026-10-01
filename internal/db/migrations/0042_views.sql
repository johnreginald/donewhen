-- Saved views (PP-205): a named filter, private to one user in one workspace.
CREATE TABLE views (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    query        text NOT NULL DEFAULT '',
    layout       text NOT NULL DEFAULT 'list' CHECK (layout IN ('board', 'list')),
    position     int  NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX views_owner_idx ON views (user_id, workspace_id, position);

-- One row per user and workspace once the default views were created, so a
-- default the user deleted does not come back on the next load.
CREATE TABLE views_seeded (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, workspace_id)
);
