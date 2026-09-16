-- Every workspace gets a `runner` label group, so any project can say which
-- agent works a ticket rather than only the one this was first tried in.
--
-- The labels are bare values, like every other group here: the type group holds
-- "bug", not "type:bug", and the group already says what the value means.

INSERT INTO label_groups (workspace_id, name, exclusive)
SELECT w.id, 'runner', true
  FROM workspaces w
 WHERE NOT EXISTS (
   SELECT 1 FROM label_groups g
    WHERE g.workspace_id = w.id AND g.name = 'runner'
 );

INSERT INTO labels (workspace_id, group_id, name, color)
SELECT w.id, g.id, v.name, v.color
  FROM workspaces w
  JOIN label_groups g ON g.workspace_id = w.id AND g.name = 'runner'
 CROSS JOIN (VALUES
   ('opencode', '#38bdf8'),
   ('codex',    '#8b87ff')
 ) AS v(name, color)
 WHERE NOT EXISTS (
   SELECT 1 FROM labels l
    WHERE l.workspace_id = w.id AND l.name = v.name
 );
