# Concepts

This page explains what DoneWhen is and why it exists. Read it first. It has no install steps.

## The problem

AI agents build fast. A feature can take minutes.

But the record of what was built, and why, gets lost. The chat is gone. The commit says "fix". Nobody knows what "done" meant.

You review code you did not watch being written. You need two things:

- A clear definition of "done", written before the work starts.
- A record of what was done, kept after it ends.

## The idea

DoneWhen is a self-hosted issue tracker. Its centre is the **done-when checklist**.

- Every ticket has a done-when checklist. It has 3 to 6 items.
- The items are written during **Aligning**, before any building. They come from the spec.
- Each item is one result you can check. Example: "`docs/CONCEPTS.md` explains the nine states".
- The AI ticks each item the moment it is met. It un-ticks an item that stops being true.
- **The checklist gates In Review and Done.** If any item is open, the ticket does not move.
- A ticket that cannot meet an item is a conversation with you. Fix the work, or change the item and say why.

When the work ends, DoneWhen keeps a **record**:

- The commits that did the work.
- The branch and pull request.
- An engineering document: what changed and how, with a diagram.
- An activity log: who did what, and when. "Who" is you or the AI.

Today the gate is a rule the agent follows. The server does not enforce it yet (planned, ticket PP-203). See [AI-WORKFLOW.md](AI-WORKFLOW.md#the-done-when-gate).

## Vocabulary

| Term | Meaning |
|---|---|
| Workspace | One tracker with its own issue key prefix (for example `ACM`). It owns all its data. Members have a role: owner, admin or member. |
| Project | A group of epics. In the database and in MCP tools it is called an **initiative**. |
| Epic | A group of issues for one feature or goal. In the database and in MCP tools it is called a **project**. |
| Issue | One ticket. It has a key such as `ACM-42`, a title, a Markdown description, a state, a priority and labels. |
| Sub-issue | An issue with a parent issue. |
| State | Where the issue is in the flow. See the next section. |
| Done-when criterion | One checkable item in an issue's checklist. It is done or not done. |
| Document | An engineering note attached to an issue, epic or project. Types: `feature`, `change`, `decision`, `reference`, `overview`. |
| Label | A tag. Labels live in groups. A group can be exclusive: one label per issue. |
| Blocker | A link that says "this issue waits on that issue". |
| Actor | Who made a change: `human` (you, in the browser) or `ai` (a call with an API token). |

The words "project" and "epic" swap between the screen and the API. This is on purpose. The screen uses the words most people expect. The API keeps the names from Linear.

The hierarchy is: Workspace, Project, Epic, Issue, Sub-issue.

## States

An issue moves through nine states.

| State | Meaning |
|---|---|
| Triage | Captured as a one-liner. Nobody has looked at it yet. |
| Backlog | Accepted, but not planned. |
| Aligning | You and the AI are agreeing on the goal, the scope and the done-when checklist. |
| Ready | The spec is locked. This is the queue. Next work is picked from here. |
| In Progress | Someone is building it. |
| Blocked | Work started and cannot finish without you. It needs a decision or an answer. The ticket says why. It is not a place to park "hard" work. |
| In Review | The checklist is complete. You review the code. |
| Done | Reviewed and merged. |
| Canceled | Will not be done. |

The usual path is: Triage, Backlog, Aligning, Ready, In Progress, In Review, Done.

A ticket in Blocked has a branch and commits behind it. It does not go back to Aligning.

The AI moves a ticket to In Review. You move it to Done, after you merge.

Each workspace gets these states from the migrations in `internal/db/migrations/` (`0002_seed.sql`, `0016_blocked_state.sql`).

## What it is not

- It is not a clone of Linear or Jira. It borrows the verbs and the shape. It does not copy the feature list.
- It has no sprints, no estimates and no story points. Work flows. The signal is priority and position in the Ready queue.
- It is not an agent runner. DoneWhen does not start or run agents. It stores the plan and the record. The agent runs somewhere else, such as Claude Code, and talks to DoneWhen over MCP.
- It is not a hosted service. You run it yourself, with one Go binary and Postgres.

## Next

- [AI-WORKFLOW.md](AI-WORKFLOW.md) shows how an AI agent drives a ticket.
- [ARCHITECTURE.md](ARCHITECTURE.md) shows how the system is built.
