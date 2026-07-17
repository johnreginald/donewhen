-- Optional default repository per Project (initiative) and Epic (project).
-- An Epic inherits its Project's repo when unset. This is a DEFAULT for the UI +
-- bare-SHA auto-linking; cross-repo commits still carry their own full URL.

ALTER TABLE initiatives ADD COLUMN IF NOT EXISTS repo_url text;
ALTER TABLE projects    ADD COLUMN IF NOT EXISTS repo_url text;
