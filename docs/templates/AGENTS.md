## Tracker: DoneWhen

DoneWhen is the issue tracker. Use it over its MCP tools (`mcp__donewhen__*`) and the `donewhen` skill. Load the skill before any ticket work.

Flow: Triage → Backlog → Aligning → Ready → In Progress → Blocked → In Review → Done → Canceled.

Rules:
- Confirm the title and scope with me before you create a ticket or an epic.
- Every ticket has a done-when checklist of 3 to 6 items, set with `set_criteria`. Derive the items from the spec. Do not invent generic ones.
- Tick each item with `check_criterion` the moment it is met. Do not wait for the end.
- The checklist gates In Review and Done. If an item is not done, the ticket does not move.
- If you cannot finish without me, move the ticket to Blocked and write why in a comment.
- I review all code. Never move a ticket to Done. Stop at In Review.
- When you finish: `link_commit`, `set_issue_dev`, then `save_document` with what changed and how.

Where things live:
- Workspace: `<your-workspace-slug>`
- Epics: `<your epic names>`
- Labels: one `type` label per ticket (`bug`, `feature`, `chore`, `tech-debt`).
