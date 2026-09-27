-- Agent reviews: a ticket qualified before it is built, and a change reviewed
-- by a different vendor before the user sees it.
CREATE TABLE IF NOT EXISTS reviews (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    issue_id     uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('ticket', 'code')),
    reviewer     text NOT NULL DEFAULT '',   -- harness/model that reviewed
    builder      text NOT NULL DEFAULT '',   -- harness that built (code reviews)
    verdict      text NOT NULL CHECK (verdict IN ('pass', 'changes')),
    summary      text NOT NULL DEFAULT '',
    findings     jsonb NOT NULL DEFAULT '[]',
    guide_md     text NOT NULL DEFAULT '',   -- the review guide for the user (code reviews)
    content_hash text NOT NULL DEFAULT '',   -- what was reviewed (ticket reviews)
    round        int  NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS reviews_issue_idx ON reviews (issue_id, created_at DESC);
