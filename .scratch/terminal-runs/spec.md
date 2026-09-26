# Run in terminal (per agent)

## Goal

Let an agent work a ticket in a live terminal session the user can watch and answer, instead of headless.

This adds:

- A per-agent **Run in terminal** setting (off by default).
- Terminal runs for Claude, Codex and OpenCode, each in its own cmux workspace named after the ticket.
- A check loop inside the session: turn ends → checks run → failures typed back in → repeat.

The user is involved only when an agent asks something, and at code review.

## 1. Agent setting

| Field | Type | Default |
|---|---|---|
| `run_in_terminal` | bool | `false` |

- Shown as a toggle on the agent's page.
- Headless runs are unchanged when it is off.

## 2. Terminal session

- Each run is a `tmux` session named `raenil-<KEY>` in the ticket's worktree.
- A cmux workspace named `<KEY>` attaches to it. No cmux → the session still runs; `tmux attach -t raenil-<KEY>` opens it.
- Closing the window does not stop the work.
- The session gets the same login, model, MCP and permission settings as a headless run.
- Claude keeps auto mode. Codex uses `workspace-write` with approvals on request. OpenCode uses its server's permissions.

## 3. Turn loop

| Turn ends with | Result |
|---|---|
| Last message is a question | Wait for the user. Mac notification + ticket comment "waiting for you in the terminal". |
| Anything else | Run the ticket's deterministic checks on the worktree. |
| Checks pass | Close the session. The attempt continues as a normal run: full checks, commit, In Review. |
| Checks fail | Type the failures into the session and keep going, like a goal. |
| Same checks fail on the same code 3 turns in a row | Stuck: close the session. The attempt ends blocked; the next attempt continues from its branch in a fresh session, and after the last one the ticket goes to Blocked. |

- No round limit: the checks decide when it is done.
- On finish the tmux session, the cmux workspace and a Terminal window the runner opened all close.
- Waiting for the user does not count against the attempt timeout. Terminal attempts are bounded at 12 hours.
- The prompt tells a terminal worker to ask questions in the terminal, not through `ask_user`.

## Implementation notes

- `models.Agent.RunInTerminal` + migration `agents.run_in_terminal`.
- `RunRequest.Terminal bool` and `RunRequest.Check func(ctx) (pass bool, feedback string)`.
- A shared `terminalSession` helper: start tmux, open cmux, paste text, wait for a turn-end marker file, close.
- Turn end:
  - Claude: a `Stop` hook passed with `--settings`, appending its JSON to `<runDir>/turns.jsonl`.
  - Codex: `-c notify=[…]` running a script that appends its JSON argument to the same file.
  - OpenCode: the existing session-idle watch; the TUI is `opencode attach <url> --dir <wt> --session <id>`.
- Secrets are never written into the launch script; it reads the token files.

## Acceptance tests

- Agent with `run_in_terminal` off → headless args, unchanged.
- Claude terminal launch → tmux session + Stop hook settings + auto mode.
- Turn ends with a question → no check runs, notification sent.
- Turn ends, checks fail → feedback pasted, next turn awaited.
- Turn ends, checks pass → session closed, `Run` returns.
- Failures keep changing → keeps going. Same failure on the same code 3 turns in a row → stuck, session closed.
- Live: one real ticket per harness runs to In Review in a visible terminal.

## Out of scope

- Raenil chat mirroring the terminal conversation → chat shows run status only.
- Running terminal sessions on the PC → Mac host only.
