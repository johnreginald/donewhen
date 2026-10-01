-- A comment is either a plain comment or the reason an issue was moved to
-- Blocked (PP-207). The Blocked page reads the latest blocked_reason.
ALTER TABLE comments ADD COLUMN kind text NOT NULL DEFAULT 'comment';
CREATE INDEX comments_blocked_reason_idx ON comments(issue_id, created_at DESC) WHERE kind = 'blocked_reason';
