---
name: donewhen
description: Work with DoneWhen, a self-hosted issue tracker, over its MCP tools (mcp__donewhen__*). Use whenever creating, writing, updating, moving or finishing a ticket, epic or project in DoneWhen; turning a spec or plan into tickets; writing or ticking done-when criteria; or recording commits and documents on a ticket.
argument-hint: "What to do in DoneWhen, e.g. 'turn specs/x.md into tickets' or 'finish ACM-42'"
---

# DoneWhen

DoneWhen is a self-hosted tracker and record-keeper. Its MCP endpoint is `<your-server>/mcp` (see the README for `claude mcp add`). Its tools are `mcp__donewhen__*`; if they are deferred, load the ones you need with ToolSearch in one call.

The hard rules (confirm before creating, the checklist gates In Review and Done, `repo` labels) are in CLAUDE.md and always apply. This skill is the how.

## The model

- Hierarchy: **Workspace → Initiative → Epic → Issue → Sub-issue**. The UI calls initiatives "Projects" and projects "Epics"; the tools say `initiative` and `project`.
- States: Triage → Backlog → Aligning → Ready → In Progress → Blocked → In Review → Done → Canceled.
- An issue key's prefix names its workspace (`ACM-42` → the workspace with prefix ACM). Tools infer the workspace from a key; pass `workspace` only when creating something with no parent.
- Per-project wiring (label values, epic names) lives in that project's memory. First use in a project: ask once, save it there.

## Tools, like the Linear MCP

| Need | Tool |
|---|---|
| Find issues | `list_issues` (state, project, label, initiative, parent, query) |
| Read one | `get_issue` |
| Create / update / move | `save_issue` (no id = create; `state` = a status name; `blockedBy` = keys it waits on) |
| Epics and initiatives | `list_projects`, `get_project`, `save_project`, `list_initiatives`, `save_initiative` |
| Comments | `list_comments`, `save_comment` |
| Labels and states | `list_issue_labels`, `create_issue_label`, `list_issue_statuses` |
| Done-when | `get_criteria`, `set_criteria`, `check_criterion` |
| Record | `link_commit`, `save_document`, `set_issue_dev`, `get_activity`, `list_issues_missing_docs` |

## Writing a ticket

Read [TICKETS.md](TICKETS.md) before writing or rewriting any ticket, and follow it strictly. Any ```mermaid``` block in a ticket, comment or document follows [MERMAID.md](MERMAID.md).

## From a spec to tickets

1. Read the spec (wherever it lives, for example `specs/<feature>.md`) and the project's memory for its labels and epics.
2. Split by feature: one ticket per feature end to end, one per UI screen. A multi-ticket feature gets an epic.
3. Propose the list — titles, epic, `blockedBy` links, labels — and confirm with the user before creating anything.
4. Create each ticket with `save_issue` in the TICKETS.md format, with its labels (a `repo` label on every ticket when the project has more than one repository) and `blockedBy`.
5. Give each its done-when checklist with `set_criteria` (below).
6. Link the spec on the tickets or the epic.

## Done-when checklists

- Every ticket has one: 3–6 items derived from its own spec, never generic. Set it in Aligning with `set_criteria {issue, items:[{text, done:false}]}`.
- Each item is one observable result someone can check: a named test passing, a behaviour, a file or rule in place.
- Give each item the right `kind`. The default is `manual`; set another kind only when it fits:
  - Proof is a command (`make test`, `go vet`, a build) → `deterministic`, with `check` `{"cmd": "make test", "expect_exit": 0}`. Run the command yourself, then tick the item with the result as evidence.
  - A rule about which files change → `policy`, with `check` `{"policy": "paths_within", "args": ["internal/**"]}`. `args` are globs: `*` is one path segment, `**` is any number.
  - A question a reader answers from the code or the result, with a wording of its own → `judgment`, with `check` `{"prompt": "Does the page explain the error to the user?"}`. It is advice, never the only gate.
  - Behaviour a reviewer reads in the code → `manual`.
- Tick each item **the moment it is met**, not at the end: `check_criterion {issue, index, done:true}`. A dropped session then leaves real progress recorded.
- Un-tick an item that stops being true. A tick always means it holds now.
- Before moving to **In Review** or **Done**: `get_criteria` and confirm every item is done. If one is not, the ticket does not move.
- An item that genuinely cannot be met is a conversation with the user: fix the work, or change the item and say why.

## Finishing a ticket

1. Every criterion ticked (`get_criteria` to confirm).
2. Move it with `save_issue` (`state: "In Review"`, or `"Done"` after the user's merge).
3. `link_commit` for the commit(s) that did it.
4. `set_issue_dev` with the branch and PR.
5. `save_document`: the engineering record — what changed and how, with a mermaid diagram for backend flows. Plain, short English. Follow [MERMAID.md](MERMAID.md) for every diagram.

## Moving tickets as work happens

Drive the states yourself as the work moves: Triage when captured, Aligning while discussing, Ready when the spec is locked, In Progress while building, Blocked (with a comment saying why) when it cannot finish without the user, In Review at the PR, Done at merge.
