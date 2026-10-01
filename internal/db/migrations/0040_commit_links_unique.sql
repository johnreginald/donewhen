-- donewhen:destructive
-- PP-187: a commit is linked to an issue once. This DELETES duplicate rows of
-- issue_commits (same issue, same sha), so the backup gate asks for a backup
-- first.
--
-- 1. Store every sha lowercase, so "ABC1234" and "abc1234" are the same commit.
-- 2. Keep the earliest row of each (issue_id, sha) and delete the later ones.
-- 3. Add UNIQUE (issue_id, sha) and an index for prefix lookups by sha.
--
-- Safe to run twice: step 1 and 2 find nothing the second time, and the
-- constraint and index are replaced rather than added.

UPDATE issue_commits SET sha = lower(sha) WHERE sha <> lower(sha);

DO $$
DECLARE
    removed bigint;
BEGIN
    DELETE FROM issue_commits d
    USING issue_commits k
    WHERE d.issue_id = k.issue_id
      AND d.sha = k.sha
      AND (d.created_at, d.id) > (k.created_at, k.id);
    GET DIAGNOSTICS removed = ROW_COUNT;
    RAISE NOTICE 'migration 0040: % duplicate commit link(s) deleted', removed;
END
$$;

ALTER TABLE issue_commits DROP CONSTRAINT IF EXISTS issue_commits_issue_sha_key;
ALTER TABLE issue_commits ADD CONSTRAINT issue_commits_issue_sha_key UNIQUE (issue_id, sha);

CREATE INDEX IF NOT EXISTS issue_commits_sha_idx ON issue_commits (sha text_pattern_ops);
