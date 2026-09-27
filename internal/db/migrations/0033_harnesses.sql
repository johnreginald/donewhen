-- Terminal runs are gone: every run is headless, in the vendor's own harness.
ALTER TABLE agents DROP COLUMN IF EXISTS run_in_terminal;

-- Antigravity (Google's agy CLI) is a fourth harness.
ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_harness_chk;
ALTER TABLE agents ADD CONSTRAINT agents_harness_chk CHECK (harness IN ('claude', 'codex', 'opencode', 'antigravity'));
