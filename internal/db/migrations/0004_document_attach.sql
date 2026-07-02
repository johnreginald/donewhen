-- Documents can attach to an initiative, a project (epic), or an issue (ticket),
-- and carry labels like issues do.

ALTER TABLE documents
    ADD COLUMN initiative_id uuid REFERENCES initiatives(id) ON DELETE SET NULL,
    ADD COLUMN issue_id      uuid REFERENCES issues(id) ON DELETE SET NULL;

CREATE INDEX documents_initiative_idx ON documents(initiative_id);
CREATE INDEX documents_issue_idx ON documents(issue_id);

CREATE TABLE document_labels (
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    label_id    uuid NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (document_id, label_id)
);
