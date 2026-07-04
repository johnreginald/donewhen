-- Documents are the AI's engineering journal: typed + attributed.

ALTER TABLE documents
    ADD COLUMN type   text NOT NULL DEFAULT 'reference',
    ADD COLUMN author text NOT NULL DEFAULT 'human';

-- type: feature | change | decision | reference | overview
CREATE INDEX documents_type_idx ON documents(type);
