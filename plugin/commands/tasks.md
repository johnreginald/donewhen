---
description: Browse your DoneWhen tickets interactively. Pick a workspace, narrow by project, epic or state, open a ticket.
argument-hint: "[workspace] [--plain] [--project <text>] [--epic <text>] [--state <name>] [--all] | workspaces | use <workspace> [--project <text>]"
allowed-tools: Bash(node:*), Bash(printf:*), AskUserQuestion, mcp__donewhen__get_issue, mcp__donewhen__get_criteria
---

Arguments: `$ARGUMENTS`

Call the script `S` = `node "${CLAUDE_PLUGIN_ROOT}/scripts/tasks.mjs"`.

Workspaces, read when this command started (JSON):

!`node "${CLAUDE_PLUGIN_ROOT}/scripts/tasks.mjs" workspaces --json`

## If the data above is not JSON

It is an error message, for example a missing token. Print it exactly as it is inside a single ```text block. Write nothing else. Stop.

## Plain mode

If the arguments contain `--plain`, or the first argument is `workspaces` or `use`, do not ask anything. Run `S` with the arguments (leave out `--plain`). Print the output exactly as it is inside a single ```text block. Write nothing before or after the block. Stop.

## Interactive mode

Keep the replies short. Use the question tool for every choice, one question at a time. Never make up tickets, counts or names: everything comes from `S` or from the DoneWhen tools.

### Step 1: the workspace

- If the arguments name a workspace (the first word that does not start with `--`), use it.
- Else, if `current` in the JSON is not empty, use it. Say in one line which workspace you use.
- Else ask "Which workspace?" (header `Workspace`). Offer the first four entries of `workspaces`, which are already sorted by open count. Label: `<name> (<prefix>)`. Description: `<open> open · <project names>`. If there are more than four, list the rest by slug in the question text. The user can type any slug or prefix with the "Other" answer.

### Step 2: show the tickets

Run `S <slug>` plus the filters you hold: `--epic "<name>"`, `--state "<name>"`, `--project "<name>"`, `--all`. Start with the flags from the arguments. Print the output exactly as it is inside a single ```text block. If you hold filters, write them in one line above the block, for example `Filters: epic = BahtBook`.

### Step 3: ask what next

Ask "What next?" (header `Next`) with these four options:

1. **Narrow it down**: pick an epic, a state or a project.
2. **Look at a ticket**: open one ticket with its done-when checklist.
3. **Switch workspace**: go back to step 1 and ignore the default.
4. **Done**: stop here.

Then do the following, and come back to step 3 after each action.

- **Narrow it down.** Ask "Narrow by what?" (header `Filter`) with the options Epic, State, Project and "Show Done and Canceled too". Run `S outline <slug>`. It prints JSON with `projects`, `epics`, `states` and `hot`.
  - Epic: ask "Which epic?" with the first four `epics` (label: the name, description: `<open> open · <project>`). Typing a name with "Other" works too. Add `--epic "<name>"`.
  - State: ask "Which state?" with the first four `states` (label: the name, description: `<open> open`). Add `--state "<name>"`.
  - Project: ask "Which project?" with the first four `projects`. Add `--project "<name>"`.
  - "Show Done and Canceled too": add `--all`.
  - A new filter of the same kind replaces the old one. Then go to step 2.
- **Look at a ticket.** Run `S outline <slug>`. Ask "Which ticket?" with the first four `hot` tickets (label: the key, description: `<state> · <title cut at 60 characters>`). The user can type any key, such as `PP-12`, with "Other". Call `mcp__donewhen__get_issue` and `mcp__donewhen__get_criteria` for that key. Then show a short card:
  - the key, title, state and epic,
  - the first three lines of the description,
  - the done-when list with `☑` for done and `☐` for open items,
  - the branch or pull request if set,
  - the link `<URL>/issue/<KEY>`. Get `<URL>` with `printf %s "$DONEWHEN_URL"`.
  If the DoneWhen tools are not connected, say so in one line and show only the link. To move a ticket, tell the user to ask in the chat: the server checks the done-when list.
- **Switch workspace.** Go to step 1 and ask, even when a default exists.
- **Done.** Stop. Write one line: `Make a workspace the default for this repo with /donewhen:tasks use <workspace>.`
