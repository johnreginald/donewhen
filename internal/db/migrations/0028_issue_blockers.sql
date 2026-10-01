-- "Blocked by" as data, not prose: a ticket does not run while any ticket
-- blocking it is not Done.
CREATE TABLE issue_blockers (
    issue_id   uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    blocker_id uuid NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, blocker_id),
    CHECK (issue_id <> blocker_id)
);
CREATE INDEX issue_blockers_blocker_idx ON issue_blockers(blocker_id);

-- Tickets written before this say who blocks them in a structured line:
--   **Blocked by:** ACM-47 (WEB-02), ACM-52 (WEB-07)
-- Those keys become links, within the same workspace. Prose ("blocked by the
-- models ticket") and keys that do not exist are left alone.
INSERT INTO issue_blockers (issue_id, blocker_id)
SELECT DISTINCT i.id, b.id
FROM issues i
CROSS JOIN LATERAL substring(i.description_md from '\*\*Blocked by:\*\*([^\n]*)') AS line
CROSS JOIN LATERAL regexp_matches(line, '([A-Z][A-Z0-9]*-[0-9]+)', 'g') AS m
JOIN issues b ON b.key = m[1] AND b.workspace_id = i.workspace_id AND b.id <> i.id
WHERE line IS NOT NULL
ON CONFLICT DO NOTHING;

-- An epic set running starts each of its Ready tickets as soon as nothing
-- blocks it, until it is stopped.
ALTER TABLE projects ADD COLUMN autorun boolean NOT NULL DEFAULT false;
