-- A ticket that was specified, attempted, and cannot finish without a human is
-- not back at the spec stage. It has a branch, commits and a partial worktree,
-- so parking it in Aligning conflates "not yet specified" with "built and
-- stuck" — and hides which tickets are actually waiting on someone.
--
-- Blocked sits between In Progress and In Review, in the started category,
-- because the work exists and has not gone backwards.
--
-- Both statements are scoped per workspace and guarded on Blocked being absent,
-- so a workspace that already has it is left alone and re-running shifts
-- nothing a second time.

UPDATE workflow_states s
   SET position = s.position + 1
 WHERE s.position >= 5
   AND NOT EXISTS (
     SELECT 1 FROM workflow_states b
      WHERE b.workspace_id = s.workspace_id AND b.name = 'Blocked'
   );

INSERT INTO workflow_states (workspace_id, name, category, position, color)
SELECT w.id, 'Blocked', 'started', 5, '#F87171'
  FROM workspaces w
 WHERE NOT EXISTS (
   SELECT 1 FROM workflow_states s
    WHERE s.workspace_id = w.id AND s.name = 'Blocked'
 );
