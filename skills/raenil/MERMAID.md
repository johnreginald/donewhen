# Mermaid rules

Raenil renders diagrams with Mermaid 11. A diagram that fails to parse shows as a red "Diagram error" box, and is listed under **Artifacts → Diagram errors**. Follow these rules for every ```` ```mermaid ```` block in a ticket, comment or document.

## Node ids

- Use plain ids: letters, digits and `_`. Examples: `api`, `store`, `linksView`.
- Never use a reserved word as an id: `graph`, `flowchart`, `subgraph`, `end`, `style`, `class`, `classDef`, `click`, `default`, `direction`, `linkStyle`.
- Never start an id with `o` or `x` right after `--`. Mermaid reads `A --o B` and `A --x B` as arrow heads.

## Labels

- Put a label in double quotes when it has any of these: `/ \ : ; , ( ) [ ] { } < > # | + * ? ! & ' →`, a dot or a leading digit. Example: `api["GET /api/issues/:id"]`.
- Quoting is always safe. When in doubt, quote.
- Inside a quoted label, write `#quot;` for `"`. Never put a raw `"` in a label.
- Don't use the parallelogram shape `[/text/]` when the text has `/`. Use `["/route: text"]`.
- No Markdown in labels. No backticks, `**` or `_`.
- Edge labels follow the same rule: `A -->|"POST /archive"| B`.

## Sequence diagrams

- Write a message as `A->>B: text`. It needs the colon.
- Never put `;` in a message or a `Note`. Mermaid reads `;` as the end of a statement. Use `,` instead, as in `BEGIN, SELECT ...`.
- Give a participant with spaces or symbols an alias: `participant API as "api/booking"`.
- Close every `alt`, `opt`, `loop`, `par` and `rect` with `end`.

## Before you save

- Keep a diagram small: one idea, up to about 15 nodes.
- Read it back once against these rules.
- After saving, the document's reader page shows a red box if the diagram is broken. **Artifacts → Diagram errors** lists all broken diagrams.
