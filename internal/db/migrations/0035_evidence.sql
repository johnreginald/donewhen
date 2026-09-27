-- What a ticket's change looks like, for the user's review: screenshots of
-- each screen state (and the prototype screen beside them) and a preview
-- link. Made by the repo's `make evidence`, uploaded by the runner host.
CREATE TABLE IF NOT EXISTS evidence (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    issue_id     uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    name         text NOT NULL,
    content_type text NOT NULL,
    data         bytea NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS evidence_issue_idx ON evidence (issue_id, created_at DESC);
