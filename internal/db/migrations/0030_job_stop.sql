-- A person can stop a job. A queued one is canceled at once; one a host is
-- working on is marked here, and the host ends it when it next looks.
ALTER TABLE jobs ADD COLUMN stop_requested_at timestamptz;
