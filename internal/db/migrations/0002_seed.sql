-- Seed the workflow states and exclusive label groups.

INSERT INTO workflow_states (name, category, position, color) VALUES
    ('Triage',      'triage',    0, '#a1a1aa'),
    ('Backlog',     'backlog',   1, '#94a3b8'),
    ('Aligning',    'unstarted', 2, '#c084fc'),
    ('Ready',       'unstarted', 3, '#38bdf8'),
    ('In Progress', 'started',   4, '#facc15'),
    ('In Review',   'started',   5, '#fb923c'),
    ('Done',        'completed', 6, '#4ade80'),
    ('Canceled',    'canceled',  7, '#f87171')
ON CONFLICT (name) DO NOTHING;

-- Exclusive label groups per the taxonomy (one label per group per issue).
INSERT INTO label_groups (name, exclusive) VALUES
    ('repo',     true),
    ('platform', true),
    ('type',     true),
    ('domain',   true),
    ('triage',   true)
ON CONFLICT (name) DO NOTHING;

-- A few common type labels to start.
INSERT INTO labels (group_id, name, color)
SELECT g.id, v.name, v.color
FROM (VALUES
    ('bug',       '#f87171'),
    ('feature',   '#4ade80'),
    ('chore',     '#94a3b8'),
    ('tech-debt', '#fbbf24')
) AS v(name, color)
JOIN label_groups g ON g.name = 'type'
ON CONFLICT (name) DO NOTHING;

-- Triage role labels.
INSERT INTO labels (group_id, name, color)
SELECT g.id, v.name, v.color
FROM (VALUES
    ('needs-triage',    '#a1a1aa'),
    ('needs-info',      '#fbbf24'),
    ('ready-for-agent', '#38bdf8'),
    ('ready-for-human', '#c084fc'),
    ('wontfix',         '#f87171')
) AS v(name, color)
JOIN label_groups g ON g.name = 'triage'
ON CONFLICT (name) DO NOTHING;
