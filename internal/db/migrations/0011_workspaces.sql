-- Workspaces — the top of the hierarchy and the tenancy boundary.
--
--   Workspace → Project (initiative) → Epic (project) → Issue
--
-- Every root table gains a workspace_id; workspace_members decides who may see
-- it. Child tables (issue_labels, document_labels, comments, issue_criteria,
-- issue_commits) get no column: they are only ever reached through an already
-- scoped parent and cascade from it.
--
-- issues.workspace_id has to be denormalized rather than derived — an issue may
-- have project_id IS NULL, so there is no join path upward.
--
-- Backfill rule: one workspace per existing initiative. An issue with no epic is
-- placed by its key prefix, learned from the issues that do have one and used
-- only where that prefix resolves to a single workspace. Whatever is still
-- unplaceable (orphan epics, unattached documents, activity whose issue was
-- already deleted) lands in an "Unsorted" workspace, created only if such rows
-- exist.
--
-- No existing issue key is rewritten. Where a legacy prefix belongs
-- unambiguously to one workspace it is adopted, so new issues continue the
-- imported lineage; each counter is then seeded past everything already taken.
--
-- The runner applies each migration file in its own transaction, so a failure
-- anywhere below rolls the whole thing back.

CREATE TABLE workspaces (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text NOT NULL UNIQUE,          -- url-safe handle, e.g. 'zlan-engineering'
    name       text NOT NULL,
    key_prefix text NOT NULL UNIQUE,          -- prefix for NEW issue keys, e.g. 'ZLA'
    issue_seq  bigint NOT NULL DEFAULT 0,     -- per-workspace issue counter
    position   int  NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workspace_members (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         text NOT NULL DEFAULT 'member',   -- owner | admin | member
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX workspace_members_user_idx ON workspace_members(user_id);

-- ---------------------------------------------------------------------------
-- 1. Tenant columns, nullable for now so the backfill can fill them in.
-- ---------------------------------------------------------------------------

ALTER TABLE initiatives     ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;
ALTER TABLE projects        ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;
ALTER TABLE issues          ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;
ALTER TABLE documents       ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;
ALTER TABLE labels          ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;
ALTER TABLE label_groups    ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;
ALTER TABLE workflow_states ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;
ALTER TABLE activity        ADD COLUMN workspace_id uuid REFERENCES workspaces(id) ON DELETE CASCADE;

-- The global name/number uniqueness has to go before the per-workspace clones
-- below can be inserted. Per-workspace equivalents are added at the end.
ALTER TABLE workflow_states DROP CONSTRAINT workflow_states_name_key;
ALTER TABLE labels          DROP CONSTRAINT labels_name_key;
ALTER TABLE label_groups    DROP CONSTRAINT label_groups_name_key;
ALTER TABLE issues          DROP CONSTRAINT issues_number_key;
-- issues.key stays GLOBALLY unique on purpose: it keeps GetIssueByKey,
-- parent_key and the commit reverse-lookup unambiguous with no workspace
-- argument. It holds because workspaces.key_prefix is itself unique.

-- ---------------------------------------------------------------------------
-- 2. One workspace per existing initiative.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    ini       RECORD;
    ws_id     uuid;
    base      text;
    cand      text;
    n         int;
    reserved  text[];
BEGIN
    -- Every prefix already burned into a legacy key is off limits, so a new
    -- workspace can never mint a key that collides with an existing issue.
    SELECT coalesce(array_agg(DISTINCT upper(split_part(key, '-', 1))), '{}')
      INTO reserved
      FROM issues;

    FOR ini IN SELECT id, name, position, created_at FROM initiatives
               ORDER BY position, created_at, name
    LOOP
        base := upper(substring(regexp_replace(ini.name, '[^a-zA-Z0-9]', '', 'g') FROM 1 FOR 3));
        IF base IS NULL OR base = '' THEN
            base := 'WSP';
        END IF;
        cand := base;
        n := 1;
        WHILE cand = ANY(reserved)
              OR EXISTS (SELECT 1 FROM workspaces WHERE key_prefix = cand)
        LOOP
            n := n + 1;
            cand := base || n::text;
        END LOOP;

        INSERT INTO workspaces (slug, name, key_prefix, position)
        VALUES (
            -- slug: lowercase, non-alphanumerics collapsed to dashes
            nullif(trim(BOTH '-' FROM lower(regexp_replace(ini.name, '[^a-zA-Z0-9]+', '-', 'g'))), '')
                || CASE WHEN EXISTS (
                        SELECT 1 FROM workspaces
                        WHERE slug = trim(BOTH '-' FROM lower(regexp_replace(ini.name, '[^a-zA-Z0-9]+', '-', 'g')))
                   ) THEN '-' || lower(cand) ELSE '' END,
            ini.name,
            cand,
            ini.position
        )
        RETURNING id INTO ws_id;

        UPDATE initiatives SET workspace_id = ws_id WHERE id = ini.id;
    END LOOP;
END $$;

-- Nobody loses access at the migration boundary: existing accounts own
-- everything that existed before workspaces did.
INSERT INTO workspace_members (workspace_id, user_id, role)
SELECT w.id, u.id, 'owner' FROM workspaces w CROSS JOIN users u
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Backfill top-down: initiative → epic → issue, then the attachments.
-- ---------------------------------------------------------------------------

UPDATE projects p SET workspace_id = i.workspace_id
  FROM initiatives i WHERE i.id = p.initiative_id;

UPDATE issues s SET workspace_id = p.workspace_id
  FROM projects p WHERE p.id = s.project_id AND p.workspace_id IS NOT NULL;

-- An issue with no epic has no join path upward, but its key prefix is real
-- signal: the Linear import preserved the originating team's prefix. Learn the
-- prefix → workspace mapping from the issues that DO have an epic, and use it
-- only for prefixes that resolve to exactly one workspace. An ambiguous prefix
-- (one whose issues are spread across workspaces) teaches nothing and is left
-- alone, to fall through to "Unsorted" below.
UPDATE issues t SET workspace_id = m.workspace_id
  FROM (
    SELECT pfx, (array_agg(DISTINCT workspace_id))[1] AS workspace_id
      FROM (
        SELECT upper(split_part(key, '-', 1)) AS pfx, workspace_id
          FROM issues WHERE workspace_id IS NOT NULL
      ) s
     GROUP BY pfx
    HAVING count(DISTINCT workspace_id) = 1
  ) m
 WHERE t.workspace_id IS NULL
   AND upper(split_part(t.key, '-', 1)) = m.pfx;

UPDATE documents d SET workspace_id = COALESCE(
    (SELECT i.workspace_id FROM issues      i WHERE i.id = d.issue_id),
    (SELECT p.workspace_id FROM projects    p WHERE p.id = d.project_id),
    (SELECT n.workspace_id FROM initiatives n WHERE n.id = d.initiative_id)
);

UPDATE activity a SET workspace_id = i.workspace_id
  FROM issues i WHERE i.id = a.issue_id;

-- ---------------------------------------------------------------------------
-- 4. "Unsorted" — only if something genuinely has no initiative ancestry.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    ws_id    uuid;
    cand     text;
    n        int;
    reserved text[];
    orphans  boolean;
BEGIN
    SELECT EXISTS (SELECT 1 FROM projects    WHERE workspace_id IS NULL)
        OR EXISTS (SELECT 1 FROM issues      WHERE workspace_id IS NULL)
        OR EXISTS (SELECT 1 FROM documents   WHERE workspace_id IS NULL)
        OR EXISTS (SELECT 1 FROM activity    WHERE workspace_id IS NULL)
        OR EXISTS (SELECT 1 FROM initiatives WHERE workspace_id IS NULL)
      INTO orphans;

    IF NOT orphans THEN
        RETURN;
    END IF;

    SELECT coalesce(array_agg(DISTINCT upper(split_part(key, '-', 1))), '{}')
      INTO reserved FROM issues;

    cand := 'UNS';
    n := 1;
    WHILE cand = ANY(reserved) OR EXISTS (SELECT 1 FROM workspaces WHERE key_prefix = cand) LOOP
        n := n + 1;
        cand := 'UNS' || n::text;
    END LOOP;

    INSERT INTO workspaces (slug, name, key_prefix, position)
    VALUES ('unsorted', 'Unsorted', cand, 999)
    RETURNING id INTO ws_id;

    INSERT INTO workspace_members (workspace_id, user_id, role)
    SELECT ws_id, u.id, 'owner' FROM users u ON CONFLICT DO NOTHING;

    UPDATE initiatives SET workspace_id = ws_id WHERE workspace_id IS NULL;
    UPDATE projects    SET workspace_id = ws_id WHERE workspace_id IS NULL;
    UPDATE issues      SET workspace_id = ws_id WHERE workspace_id IS NULL;
    UPDATE documents   SET workspace_id = ws_id WHERE workspace_id IS NULL;
    UPDATE activity    SET workspace_id = ws_id WHERE workspace_id IS NULL;
END $$;

-- ---------------------------------------------------------------------------
-- 5. Adopt the legacy key prefix where it unambiguously belongs to one
--    workspace, so new issues continue the imported lineage (ZLA-288 rather
--    than ZLA2-1) instead of starting a parallel numbering scheme.
-- ---------------------------------------------------------------------------

DO $$
DECLARE r RECORD;
BEGIN
    FOR r IN
        SELECT pfx, (array_agg(DISTINCT workspace_id))[1] AS workspace_id
          FROM (SELECT upper(split_part(key, '-', 1)) AS pfx, workspace_id FROM issues) s
         GROUP BY pfx
        HAVING count(DISTINCT workspace_id) = 1
    LOOP
        -- Never steal a prefix another workspace already derived from its name.
        IF NOT EXISTS (SELECT 1 FROM workspaces WHERE key_prefix = r.pfx AND id <> r.workspace_id) THEN
            UPDATE workspaces SET key_prefix = r.pfx WHERE id = r.workspace_id;
        END IF;
    END LOOP;
END $$;

-- Seed each counter past everything already taken, on BOTH axes it has to stay
-- unique on: the numeric half of an existing key with this prefix (issues.key is
-- globally unique) and the largest number already used inside this workspace
-- (issues.number becomes UNIQUE per workspace below). Note issues.number was a
-- global sequence, so it does NOT match the key's numeric half for imported rows.
UPDATE workspaces w SET issue_seq = GREATEST(
    coalesce((SELECT max(coalesce(nullif(regexp_replace(split_part(i.key, '-', 2), '[^0-9]', '', 'g'), ''), '0')::bigint)
                FROM issues i
               WHERE upper(split_part(i.key, '-', 1)) = w.key_prefix), 0),
    coalesce((SELECT max(i.number) FROM issues i WHERE i.workspace_id = w.id), 0)
);

-- ---------------------------------------------------------------------------
-- 6. Clone the workflow states per workspace and repoint every issue.
-- ---------------------------------------------------------------------------

INSERT INTO workflow_states (workspace_id, name, category, position, color)
SELECT w.id, s.name, s.category, s.position, s.color
  FROM workspaces w CROSS JOIN workflow_states s
 WHERE s.workspace_id IS NULL;

UPDATE issues i SET state_id = ns.id
  FROM workflow_states os
  JOIN workflow_states ns ON ns.name = os.name
 WHERE os.id = i.state_id
   AND os.workspace_id IS NULL
   AND ns.workspace_id = i.workspace_id;

-- ---------------------------------------------------------------------------
-- 7. Clone the label groups + labels per workspace and repoint every link.
-- ---------------------------------------------------------------------------

INSERT INTO label_groups (workspace_id, name, exclusive)
SELECT w.id, g.name, g.exclusive
  FROM workspaces w CROSS JOIN label_groups g
 WHERE g.workspace_id IS NULL;

INSERT INTO labels (workspace_id, group_id, name, color)
SELECT w.id,
       (SELECT ng.id FROM label_groups ng
         WHERE ng.workspace_id = w.id AND ng.name = og.name),
       l.name, l.color
  FROM workspaces w
 CROSS JOIN labels l
  LEFT JOIN label_groups og ON og.id = l.group_id
 WHERE l.workspace_id IS NULL;

UPDATE issue_labels il SET label_id = nl.id
  FROM labels ol, issues i, labels nl
 WHERE ol.id = il.label_id
   AND ol.workspace_id IS NULL
   AND i.id = il.issue_id
   AND nl.workspace_id = i.workspace_id
   AND nl.name = ol.name;

UPDATE document_labels dl SET label_id = nl.id
  FROM labels ol, documents d, labels nl
 WHERE ol.id = dl.label_id
   AND ol.workspace_id IS NULL
   AND d.id = dl.document_id
   AND nl.workspace_id = d.workspace_id
   AND nl.name = ol.name;

-- ---------------------------------------------------------------------------
-- 8. Refuse to drop the legacy rows while anything still points at them.
--    labels cascade to issue_labels, so a missed repoint would silently delete
--    label assignments — fail loudly instead.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    stragglers int;
BEGIN
    SELECT count(*) INTO stragglers
      FROM issue_labels il JOIN labels l ON l.id = il.label_id
     WHERE l.workspace_id IS NULL;
    IF stragglers > 0 THEN
        RAISE EXCEPTION '0011: % issue_labels rows still reference a pre-workspace label', stragglers;
    END IF;

    SELECT count(*) INTO stragglers
      FROM document_labels dl JOIN labels l ON l.id = dl.label_id
     WHERE l.workspace_id IS NULL;
    IF stragglers > 0 THEN
        RAISE EXCEPTION '0011: % document_labels rows still reference a pre-workspace label', stragglers;
    END IF;

    SELECT count(*) INTO stragglers
      FROM issues i JOIN workflow_states s ON s.id = i.state_id
     WHERE s.workspace_id IS NULL;
    IF stragglers > 0 THEN
        RAISE EXCEPTION '0011: % issues still reference a pre-workspace workflow state', stragglers;
    END IF;
END $$;

DELETE FROM labels          WHERE workspace_id IS NULL;
DELETE FROM label_groups    WHERE workspace_id IS NULL;
DELETE FROM workflow_states WHERE workspace_id IS NULL;

-- ---------------------------------------------------------------------------
-- 9. Lock it down.
-- ---------------------------------------------------------------------------

ALTER TABLE initiatives     ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE projects        ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE issues          ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE documents       ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE labels          ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE label_groups    ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE workflow_states ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE activity        ALTER COLUMN workspace_id SET NOT NULL;

ALTER TABLE workflow_states ADD CONSTRAINT workflow_states_ws_name_key UNIQUE (workspace_id, name);
ALTER TABLE labels          ADD CONSTRAINT labels_ws_name_key          UNIQUE (workspace_id, name);
ALTER TABLE label_groups    ADD CONSTRAINT label_groups_ws_name_key    UNIQUE (workspace_id, name);
-- Per-workspace sequences collide by design (ZLA-1 and LUM-1 are both number 1).
ALTER TABLE issues          ADD CONSTRAINT issues_ws_number_key        UNIQUE (workspace_id, number);

CREATE INDEX initiatives_workspace_idx     ON initiatives(workspace_id);
CREATE INDEX projects_workspace_idx        ON projects(workspace_id);
CREATE INDEX issues_workspace_idx          ON issues(workspace_id);
CREATE INDEX documents_workspace_idx       ON documents(workspace_id);
CREATE INDEX labels_workspace_idx          ON labels(workspace_id);
CREATE INDEX label_groups_workspace_idx    ON label_groups(workspace_id);
CREATE INDEX workflow_states_workspace_idx ON workflow_states(workspace_id);
CREATE INDEX activity_workspace_idx        ON activity(workspace_id, created_at DESC);

-- Superseded by workspaces.issue_seq.
DROP SEQUENCE issue_number_seq;
