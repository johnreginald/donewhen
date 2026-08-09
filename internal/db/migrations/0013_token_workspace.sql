-- Pin an API token to a single workspace, so an agent working in one repo can
-- never read another workspace's issues over MCP. NULL keeps the old behaviour:
-- the token may act on any workspace its owner belongs to, but must then name
-- which one when the owner belongs to more than one.

ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS workspace_id uuid
    REFERENCES workspaces(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS api_tokens_workspace_idx ON api_tokens(workspace_id);
