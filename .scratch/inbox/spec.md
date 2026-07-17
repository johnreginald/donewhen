# Inbox — AI review queue

## Objective
The human's landing page. After the AI works for a while, you come back and see:
1. a **review queue** of what needs your decision, and
2. a **timeline** of recent AI activity,
with unread tracking so "come back after a few hours" shows what's new.

This is the *consumption* half of the record-keeper (we built the recording half — the activity log). It maps 1:1 to the Kanban **In Review** gate.

## Scope (v1)
- **Needs review** tier — issues currently in `In Review` that the **AI** moved there. Card per issue: key, title, epic, short AI-activity summary (state/commits/artifacts), actions:
  - **Open** (→ issue detail)
  - **Approve** (→ state `Done`)
  - **Bounce** (→ state `In Progress`)
  Both actions go through the existing update path → auto-logged to activity.
- **Recent AI activity** tier — cross-issue `actor=ai` feed, newest first; items newer than `inbox_seen_at` flagged **new**.
- **Unread marker** — `users.inbox_seen_at`. Visiting `/inbox` marks seen. Sidebar **Inbox** item (first nav row) with a **badge** = count of needs-review issues.

## Data
- Migration `0010_inbox.sql`: `ALTER TABLE users ADD COLUMN IF NOT EXISTS inbox_seen_at timestamptz;`

## Store
- `InboxNeedsReview(ctx) ([]InboxItem, error)` — issues in `In Review` whose latest `state_changed → In Review` activity had `actor=ai`; include a small recent-activity slice per issue.
- reuse `ListRecentActivity(ActivityFilter{Actor:"ai", Limit})` for the timeline.
- `GetInboxSeen(ctx, userID) (*time.Time, error)` / `SetInboxSeen(ctx, userID) error`.

## API (both `guard`-ed; user via `auth.UserFrom`)
- `GET /api/inbox` → `{ needsReview:[InboxItem], recent:[Activity], seenAt }`
- `POST /api/inbox/seen` → sets `inbox_seen_at = now()`, returns `{seenAt}`

## Frontend
- `/inbox` route — two sections (Needs review cards + Recent AI activity feed w/ new highlight). On mount → `POST /api/inbox/seen`.
- Sidebar: **Inbox** as first nav item (before All Issues) + badge from `needsReview.length`.
- `api.js`: `inbox()`, `inboxSeen()`.

## Done-when
- `/inbox` lists In-Review-by-AI issues; **Approve**→Done and **Bounce**→In Progress work and are logged.
- Recent AI activity lists with new-since-seen highlight.
- Sidebar badge shows needs-review count.
- Builds; deployed; visible at https://raenil.burmese.dev.

## Non-goals (v1)
- comment prompt on bounce (just moves state) · per-item seen (whole-inbox only) · push/notifications.
