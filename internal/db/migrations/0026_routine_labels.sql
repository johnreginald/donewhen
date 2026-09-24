-- A routine's tickets carry its labels — its repo label above all, since a
-- ticket without one in a multi-repo workspace is worked in the wrong place.
ALTER TABLE routines ADD COLUMN labels jsonb NOT NULL DEFAULT '[]';
