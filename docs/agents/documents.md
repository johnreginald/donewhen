# Documents = the AI's engineering journal

Documents in Raenil are **written by the AI, not by humans**. When Claude
implements or changes something, it records what it did so the codebase
accumulates an architecture wiki + changelog no one had to hand-write.

## The convention

When you (Claude) finish work on an issue — as you move it to **In Review**
or **Done** — write an implementation document with the MCP `save_document`
tool and **attach it to that issue**:

- `type: "change"` (implementation record) — the default for finishing an issue.
- `issue: "<key>"` — attach to the ticket you worked on.
- Reuse the ticket's mermaid diagram; expand it if the flow grew.

Structure the body:

```
# <Feature / change title>
## Summary — what changed and why (2–3 sentences)
## How it works — the mechanism; INCLUDE a ```mermaid diagram
## Key files — the files/functions that matter
## Decisions — trade-offs taken and why
## Related — issue keys, other docs
```

Other document types:
- `feature` — an evergreen doc for a whole feature/area (attach to the project).
- `decision` — an ADR.
- `overview` — a system map (attach to an initiative).
- `reference` — anything else.

## Coverage

`list_issues_missing_docs` returns completed issues with no attached document —
the gaps in the journal. Sweep it periodically and write the missing docs.
In the UI, Done issues with no doc show a faint `✦` marker.

## Human role

The human does **not** write documents. They read, browse, and curate
(re-attach, retag, set type, delete stale). There is no editor in the UI.
