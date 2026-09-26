-- A ticket an agent found waiting on another starts again on its own once
-- that one is In Review or Done, whether or not its epic is running.
ALTER TABLE issues ADD COLUMN run_when_unblocked boolean NOT NULL DEFAULT false;
