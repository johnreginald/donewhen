-- Recolor workflow states + core labels to the refined Raenil palette.

UPDATE workflow_states SET color = c.color
FROM (VALUES
    ('Triage',      '#8A8F9C'),
    ('Backlog',     '#7C8698'),
    ('Aligning',    '#A78BFA'),
    ('Ready',       '#38BDF8'),
    ('In Progress', '#FBBF24'),
    ('In Review',   '#FB923C'),
    ('Done',        '#34D399'),
    ('Canceled',    '#6B7280')
) AS c(name, color)
WHERE workflow_states.name = c.name;

UPDATE labels SET color = c.color
FROM (VALUES
    ('bug',       '#F87171'),
    ('feature',   '#34D399'),
    ('chore',     '#94A3B8'),
    ('tech-debt', '#FBBF24'),
    ('backend',   '#8B7BFF'),
    ('api',       '#8B7BFF')
) AS c(name, color)
WHERE lower(labels.name) = c.name;
