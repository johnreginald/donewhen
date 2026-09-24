-- Reviewing from the dashboard: a run keeps the diff it produced, and the
-- host takes verify and finish jobs — the same code `orchestrator verify` and
-- `orchestrator finish` run, against the kept worktree.
ALTER TABLE runs ADD COLUMN diff text NOT NULL DEFAULT '';

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_chk;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_chk CHECK (kind IN ('test_env', 'run_ticket', 'chat', 'verify', 'finish'));
