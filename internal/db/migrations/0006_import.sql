-- Support importing from external trackers (e.g. Linear).
-- parent_key preserves sub-issue hierarchy by human key (no FK: the parent may
-- import after the child, and keys are stable/unique).

ALTER TABLE issues ADD COLUMN IF NOT EXISTS parent_key text;
CREATE INDEX IF NOT EXISTS issues_parent_key_idx ON issues(parent_key);
