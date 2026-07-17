-- Persisted activity log — the record of what happened, by whom (human/ai), when.
-- This is the backbone of Raenil as a record-keeper: every meaningful transition
-- and action is written here so the timeline, changelog, and human/AI split can
-- be reconstructed. issue_id is SET NULL (not CASCADE) + key/title snapshotted so
-- the history survives even if the issue is later deleted.

CREATE TABLE activity (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id    uuid REFERENCES issues(id) ON DELETE SET NULL,
    issue_key   text,          -- snapshot, survives deletion
    issue_title text,          -- snapshot
    actor       text NOT NULL DEFAULT 'human',  -- human | ai
    kind        text NOT NULL, -- created | state_changed | priority_changed | epic_changed | title_changed | commented | artifact_written | deleted
    field       text,          -- optional field name
    from_val    text,          -- optional (e.g. old state name)
    to_val      text,          -- optional (e.g. new state name)
    detail      text,          -- optional freeform (comment excerpt, artifact title)
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_issue_idx ON activity(issue_id, created_at);
CREATE INDEX activity_created_idx ON activity(created_at DESC);
