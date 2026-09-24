-- Claude Code joins opencode and codex as a runner a ticket can ask for.

INSERT INTO labels (workspace_id, group_id, name, color)
SELECT w.id, g.id, 'claude', '#d97757'
  FROM workspaces w
  JOIN label_groups g ON g.workspace_id = w.id AND g.name = 'runner'
 WHERE NOT EXISTS (
   SELECT 1 FROM labels l
    WHERE l.workspace_id = w.id AND l.name = 'claude'
 );
