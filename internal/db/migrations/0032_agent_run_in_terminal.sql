-- An agent can work its tickets in a live terminal session on the runner
-- host, where the user can watch and answer, instead of headless.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS run_in_terminal boolean NOT NULL DEFAULT false;
