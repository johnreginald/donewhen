-- Record HOW it was done: link issues to the actual code (branch, PR, commits),
-- and make WHAT'S READY concrete with a done-when acceptance checklist.

ALTER TABLE issues ADD COLUMN IF NOT EXISTS git_branch text;
ALTER TABLE issues ADD COLUMN IF NOT EXISTS pr_url text;

CREATE TABLE issue_commits (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id   uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    sha        text NOT NULL,
    message    text NOT NULL DEFAULT '',
    url        text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX issue_commits_issue_idx ON issue_commits(issue_id, created_at);

-- Acceptance criteria ("done-when") — the checkable gate for Ready/Done.
CREATE TABLE issue_criteria (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id   uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    body       text NOT NULL,
    done       boolean NOT NULL DEFAULT false,
    position   int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX issue_criteria_issue_idx ON issue_criteria(issue_id, position);
