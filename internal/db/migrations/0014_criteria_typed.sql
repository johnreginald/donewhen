-- Typed done-when criteria: let a criterion carry how it is verified, and a
-- pointer to the evidence that verified it. Existing rows become kind='manual',
-- which behaves exactly as before.

ALTER TABLE issue_criteria
    ADD COLUMN kind         text NOT NULL DEFAULT 'manual',
    ADD COLUMN check_spec   jsonb,
    ADD COLUMN evidence_ref text;

-- manual        — a human ticks it (legacy behaviour)
-- deterministic — {"cmd": "...", "expect_exit": 0}
-- policy        — {"policy": "paths_within", "args": ["src/**"]}
-- judgment      — {"prompt": "...", "model": "..."}  advisory only, never a sole gate
ALTER TABLE issue_criteria
    ADD CONSTRAINT issue_criteria_kind_chk
    CHECK (kind IN ('manual', 'deterministic', 'policy', 'judgment'));

-- A non-manual criterion must say how it is checked.
ALTER TABLE issue_criteria
    ADD CONSTRAINT issue_criteria_check_spec_chk
    CHECK (kind = 'manual' OR check_spec IS NOT NULL);
