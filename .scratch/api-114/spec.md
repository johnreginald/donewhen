# API-114 — worker receives the real worktree

## Objective

Make the orchestrator worker run inside the worktree that Git actually created when `RunRoot` is relative, and ensure cleanup targets that same resolved path.

## Scope

- Resolve the worktree path to an absolute path before it is shared with Git, the worker runner, or cleanup.
- Add a regression test that uses a relative `RunRoot` and observes the worker CWD while the worktree exists.
- Preserve the existing lease, heartbeat, concurrency, and worker API behavior.
- Keep the unrelated `propose` workspace and asker-quality findings out of this fix.

## Done when

1. A regression test proves the worker receives an absolute, existing worktree path when `RunRoot` is relative.
2. Cleanup uses the same resolved worktree path and cannot resolve relative to the orchestrator process CWD.
3. The orchestrator test suite passes on the committed fix.
4. A live worker invocation records non-zero input/output tokens rather than timing out with an empty assistant message.
5. Review confirms worktree creation, worker CWD, and cleanup share one resolved path without changing lease or concurrency semantics.

## Out of scope

- Fixing workspace inference in `propose`.
- Changing the default criteria asker.
- Reworking lease acquisition or heartbeat policy.
