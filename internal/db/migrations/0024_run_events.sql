-- A run's transcript as it happens: readable, redacted lines posted by the
-- machine running it, so the ticket shows an agent working live.
CREATE TABLE run_events (
    run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    seq    int  NOT NULL,
    text   text NOT NULL,
    at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, seq)
);

-- Monthly caps per agent, as Paperclip's budgets: tokens (any billing) and
-- metered dollars. Zero means no cap.
ALTER TABLE agents ADD COLUMN budget_tokens bigint NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN budget_usd numeric(12,2) NOT NULL DEFAULT 0;
