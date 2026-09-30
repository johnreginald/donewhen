# Ticket writing standard

A ticket is an **execution spec**, not an essay. Optimise for, in order:

1. Scanability — a reader scans it, they don't read it.
2. Implementation precision — exact codes, limits, names.
3. Low cognitive load — one idea per line.
4. Complete, but no extra prose.

Readers: the agent that builds it, and the product owner. English is not the owner's first language.

Research and reasoning stay out of the ticket. The ticket holds the **decision** and the **contract** the builder needs.

## Sizing

- One ticket = one feature end to end (database + API + tests together).
- Every UI screen is its own ticket.
- Don't split one feature into a ticket per migration, endpoint or helper.

## Sections, in order

```
# <Title in plain words>

**Epic:** … · **Priority:** … · **Blocked by:** …

## Goal
## 1. <Part> … ## 2. <Part> …     (one section per capability: interface + rules)
## Implementation notes
## Acceptance tests
## Out of scope
```

### Goal

- One or two short sentences: what this adds.
- If it adds several things, list them as bullets under "This ticket adds:".
- No requirements, files or signatures here.

### Numbered parts (the contract)

One `## N. <Part>` per capability (an endpoint, a guard, a command, a screen). Inside each:

- One line saying what it is for.
- The interface: route, command, event, table, screen.
- **Tables** for structured data: request fields, validation, config, permissions, state transitions, input → output.
- **Rules** as short bullets under the table.

A single-capability ticket may use one `## Requirements` section instead.

### Implementation notes

Only decisions already made. Otherwise state the behaviour and let the builder choose.

- Files that must change, as `` `path` `` — what it does.
- Signatures, types or columns other tickets depend on, in a short code block.
- Database constraints and architecture decisions.

Don't invent files or signatures to make the ticket look complete. Keep file paths when a `paths_within` criterion depends on them.

### Acceptance tests

- Grouped under `###` headings per part.
- One scenario per bullet, with its expected result.
- Prefer `scenario → result`: `61-character display name → rejected.`
- Cover the rules and edge cases. Don't restate the whole ticket.

### Out of scope

- Adjacent work someone might expect here, and which ticket does it.
- Format: `Avatar upload → **Packages and discovery backend**`.

No "Done when" section: the done-when checklist (criteria) is the one place for it.

## Writing rules

- **One testable behaviour per bullet or sentence.** Two behaviours → split.
- No semicolons joining rules. No long comma chains. No nested clauses.
- Conditional behaviour as `condition → result`.
- Put exact values in code: `` `400 validation_failed` ``, `` `display_name` ``, `` `GET` ``.
- Plain verbs: add, update, return, reject, allow, require, clear, store.
- Avoid: facilitate, utilize, in order to, with regard to, the corresponding, the aforementioned, "what this gives us", "provides the platform with".
- Don't make simple behaviour sound architectural.
- Use the product's and codebase's own terms.
- Prose only for context or a reason ("This is needed because there is no admin console yet.").

## Self-check before saving

Read every line and ask:

1. Can I understand this on first read, without holding another clause in my head? If no, restructure.
2. Does it hold more than one testable behaviour? If yes, split.

Then:

1. Remove unneeded prose.
2. Turn structured prose into tables or bullets.
3. Keep requirements and implementation notes apart.
4. Give every acceptance test an explicit result.
5. Remove repeats.
6. Check nothing was lost while simplifying.

## Done-when checklist

The checklist lives in the ticket's criteria, not its description. 3–6 items, derived from this ticket, each one observable result (see SKILL.md).

## Bad → good

**Bad** (one sentence, five behaviours):

> `RequireStaff(roles...)` returns 401 when nobody is signed in, always allows an `admin`, and otherwise returns 403 `forbidden` unless the signed-in user's staff role is in the given list; calling it with no roles at all means any staff member is allowed.

**Good:**

> - No signed-in user → `401`
> - Admin → always allowed
> - Staff role in `roles` → allowed
> - Staff role not in `roles` → `403 forbidden`
> - No roles given → any staff member allowed

**Bad** (fields, limits, normalising and errors in one paragraph):

> PATCH /me accepts any subset of display name (1-60 characters), locale (`my` or `en`), customer type (`individual` or `business`), business name (1-120 characters) and city (1-60 characters); every string is trimmed first and measured in characters, not bytes, so Burmese text is counted correctly.

**Good:**

> | Field | Accepted value | Validation |
> |---|---|---|
> | `display_name` | String | 1–60 characters |
> | `locale` | `my`, `en` | Allowed values only |
> | `customer_type` | `individual`, `business` | Allowed values only |
> | `business_name` | String | 1–120 characters |
> | `city` | String | 1–60 characters |
>
> - Trim all strings before validation.
> - Count Unicode characters, not bytes.
> - Unknown field → `400 validation_failed`.

## Full example

MYINF-255 "Profile updates and staff roles" in Raenil is the reference ticket for this format.
