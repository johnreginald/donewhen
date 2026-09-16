-- The runner labels were created as "runner:codex" and "runner:opencode", which
-- does not match how this tracker names labels. Every other group holds a bare
-- value — the type group holds "bug", not "type:bug" — and the group already
-- says what the value means.
--
-- Guarded on the target name being free, so a workspace that already has a
-- "codex" label keeps it rather than colliding, and the migration can be
-- re-applied without effect.

UPDATE labels l
   SET name = 'codex'
 WHERE l.name = 'runner:codex'
   AND EXISTS (SELECT 1 FROM label_groups g WHERE g.id = l.group_id AND g.name = 'runner')
   AND NOT EXISTS (
     SELECT 1 FROM labels x WHERE x.workspace_id = l.workspace_id AND x.name = 'codex'
   );

UPDATE labels l
   SET name = 'opencode'
 WHERE l.name = 'runner:opencode'
   AND EXISTS (SELECT 1 FROM label_groups g WHERE g.id = l.group_id AND g.name = 'runner')
   AND NOT EXISTS (
     SELECT 1 FROM labels x WHERE x.workspace_id = l.workspace_id AND x.name = 'opencode'
   );
