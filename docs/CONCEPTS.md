# Concepts

This page explains what DoneWhen is and why it exists. Read it first. It has no install steps.

## The problem

AI agents build fast. A feature can take minutes.

But the record of what was built, and why, is lost. The chat is gone. The commit says "fix". Nobody knows what "done" meant.

You review code that you did not watch while the agent wrote it. You need two things:

- A clear definition of "done", written before the work starts.
- A record of what was done, kept after the work ends.

## The idea

DoneWhen is a self-hosted issue tracker. Its main part is the **done-when checklist**.

- Every ticket has a done-when checklist. The checklist has 3 to 6 items.
- You write the items during **Aligning**, before the build starts. The items come from the spec.
- Each item is one result that you can check. Example: "`docs/CONCEPTS.md` explains the nine states".
- The AI ticks each item the moment it is met. If an item becomes false, the AI un-ticks it.
- **The checklist gates In Review and Done.** If one item is open, the ticket does not move.
- If a ticket cannot meet an item, you decide what to do. Fix the work, or change the item and say why.

When the work ends, DoneWhen keeps a **record**:

- The commits that did the work.
- The branch and the pull request.
- An engineering document: what changed and how, with a diagram.
- An activity log: who did what, and when. "Who" is you or the AI.

Today the gate is a rule that the agent follows. The server does not enforce the rule yet (planned, ticket PP-203). See [AI-WORKFLOW.md](AI-WORKFLOW.md#the-done-when-gate).

## Vocabulary

| Term | Meaning |
|---|---|
| Workspace | One tracker with its own issue key prefix (for example `ACM`). It owns all its data. Members have a role: owner, admin or member. |
| Project | A group of epics. In the database and in MCP tools, it is called an **initiative**. |
| Epic | A group of issues for one feature or goal. In the database and in MCP tools, it is called a **project**. |
| Issue | One ticket. It has a key such as `ACM-42`, a title, a Markdown description, a state, a priority and labels. |
| Sub-issue | An issue that has a parent issue. |
| State | The position of the issue in the flow. See the next section. |
| Done-when criterion | One item in the checklist of an issue. You can check the item. It is done or not done. |
| Document | An engineering note attached to an issue, epic or project. Types: `feature`, `change`, `decision`, `reference`, `overview`. |
| Label | A tag. Labels are in groups. A group can be exclusive: one label for each issue. |
| Blocker | A link that says "this issue waits for that issue". |
| Actor | The source of a change: `human` (you, in the browser) or `ai` (a call with an API token). |

The words "project" and "epic" are different on the screen and in the API. This is intentional. The screen uses the words that most people expect. The API keeps the names from Linear.

The hierarchy is: Workspace, Project, Epic, Issue, Sub-issue.

## States

An issue moves through nine states.

| State | Meaning |
|---|---|
| Triage | Captured as a one-line note. Nobody has looked at it. |
| Backlog | Accepted, but not planned. |
| Aligning | You and the AI agree on the goal, the scope and the done-when checklist. |
| Ready | The spec is locked. This is the queue. Pick the next work from here. |
| In Progress | Someone builds it. |
| Blocked | Work started and cannot finish without you. It needs a decision or an answer. The ticket says why. Do not use it to park "hard" work. |
| In Review | The checklist is complete. You review the code. |
| Done | Reviewed and merged. |
| Canceled | Nobody will do it. |

The usual path is: Triage, Backlog, Aligning, Ready, In Progress, In Review, Done.

A ticket in Blocked has a branch and commits. It does not go back to Aligning.

The AI moves a ticket to In Review. You move it to Done, after you merge.

Each workspace gets these states from the migrations in `internal/db/migrations/` (`0002_seed.sql`, `0016_blocked_state.sql`).

## What it is not

- It is not a clone of Linear or Jira. It uses the same verbs and the same shape. It does not copy the list of features.
- It has no sprints, no estimates and no story points. Work flows. The signal is the priority and the position in the Ready queue.
- It is not an agent runner. DoneWhen does not start or run agents. It stores the plan and the record. The agent runs in another place, such as Claude Code, and uses MCP to talk to DoneWhen.
- It is not a hosted service. You run it yourself, with one Go binary and Postgres.

## Next

- [AI-WORKFLOW.md](AI-WORKFLOW.md) shows how an AI agent drives a ticket.
- [ARCHITECTURE.md](ARCHITECTURE.md) shows how the system is built.
