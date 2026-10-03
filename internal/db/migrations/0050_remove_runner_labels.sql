-- donewhen:destructive
-- DW-91: remove the `runner` label group. It came from the agent control plane
-- (removed with 0036) and nothing reads it now.
--
-- It DELETES labels, and with them the links from issues and documents to those
-- labels (issue_labels and document_labels are ON DELETE CASCADE). Tickets and
-- documents stay. The backup gate asks for a backup first.
--
-- The labels go before the group: labels.group_id is ON DELETE SET NULL, so a group
-- delete alone would leave the labels behind with no group.
DELETE FROM labels
 WHERE group_id IN (SELECT id FROM label_groups WHERE name = 'runner');

DELETE FROM label_groups WHERE name = 'runner';
