# Raenil

Self-hosted, single-user issue tracker in the spirit of Linear — driven by you in
the browser and by Claude over MCP. Continuous-flow Kanban, no sprints/estimates.

- **Stack:** Go (single binary) · PostgreSQL · SvelteKit PWA
- **Realtime:** SSE for live board updates · Web Push (VAPID) for background phone notifications
- **AI:** an MCP server whose tools mirror Linear's verbs (`save_issue`, `list_issues`, …)
- **Docs:** markdown with rendered ```mermaid diagrams
- **Hierarchy:** Workspaces → Projects (initiatives) → Epics → Issues
- **Tenancy:** every workspace is a hard boundary — its own issues, epics, labels, board columns, artifacts and activity log, visible only to its members
- **States:** Triage → Backlog → Aligning → Ready → In Progress → In Review → Done → Canceled

## Quick start (local, no Docker)

```bash
# 1. Postgres (any instance). Example with Docker:
docker run -d --name raenil-pg -e POSTGRES_USER=raenil -e POSTGRES_PASSWORD=raenil \
  -e POSTGRES_DB=raenil -p 5432:5432 postgres:18-alpine

# 2. Config
cp .env.example .env      # edit RAENIL_SESSION_SECRET at least
export $(grep -v '^#' .env | xargs)

# 3. Build frontend + binary
make web
make build

# 4. Migrate + create your account + an API token for Claude
./raenil migrate
./raenil user you@example.com 'a-strong-password'
./raenil workspace create "My Work" MYW   # first workspace; keys become MYW-1, MYW-2, …
./raenil genvapid          # paste the two lines into .env, then re-export
./raenil token claude      # copy the printed token

# 5. Run
./raenil serve             # http://localhost:8080
```

## Quick start (Docker Compose — the 24/7 box)

```bash
cp .env.example .env
# set RAENIL_SESSION_SECRET, RAENIL_SITE_ADDRESS=tracker.yourdomain.com,
# RAENIL_BASE_URL=https://tracker.yourdomain.com, and VAPID keys (./raenil genvapid)
docker compose up -d --build

# first-time account + token (exec into the running container)
docker compose exec raenil /app/raenil user you@example.com 'a-strong-password'
docker compose exec raenil /app/raenil workspace create "My Work" MYW
docker compose exec raenil /app/raenil token claude
```

Caddy terminates TLS automatically for `RAENIL_SITE_ADDRESS`. Point your domain's
DNS at the box and open 80/443.

## Connecting Claude Code (MCP)

Register the remote MCP server (served at `<BASE_URL>/mcp`, Streamable HTTP):

```bash
claude mcp add --transport http raenil https://tracker.yourdomain.com/mcp \
  --header "Authorization: Bearer <the-token-from-raenil-token>"
```

Tools appear as `mcp__raenil__save_issue`, `mcp__raenil__list_issues`, etc. — the
same verbs as the Linear MCP, so existing workflow habits carry over.

### Workspaces and MCP

Every tool acts on exactly one workspace. Pin a token to one so an agent working
in a given repo only ever sees that repo's tracker:

```bash
raenil token globex-agent globex    # pinned to the 'globex' workspace
```

A pinned token is refused if it asks for any other workspace. An unpinned token
whose owner belongs to several must name one (`workspace: "globex"` on the tool
call, or the `X-Workspace` header over REST) rather than having one guessed for
it. `list_workspaces` shows what a token can reach.

### Mermaid convention

Backend-labeled issues should include a ```mermaid diagram in the description.
The global `~/.claude/hooks/require-mermaid.py` hook enforces this on `save_issue`;
add `mcp__raenil__save_issue` to its PreToolUse matcher (see `docs/DEPLOY.md`).

## Commands

| Command | Purpose |
| --- | --- |
| `raenil serve` | API + SSE + Web Push + MCP endpoint |
| `raenil migrate` | apply DB migrations |
| `raenil mcp` | MCP over stdio (local fallback transport) |
| `raenil user <email> <pass>` | create the account |
| `raenil token <name> [workspace]` | create an API token (shown once); pass a workspace slug to pin it |
| `raenil workspace list` | show workspaces and member counts |
| `raenil workspace create <name> <prefix>` | create a workspace |
| `raenil workspace add <slug> <email> [role]` | grant a user access (owner/admin/member) |
| `raenil genvapid` | print a fresh VAPID keypair |

## Backups

```bash
make backup     # pg_dump | gzip -> ./backups/raenil-<timestamp>.sql.gz
```

Add to cron for nightly dumps — see `docs/DEPLOY.md`.
