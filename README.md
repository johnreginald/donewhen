# Kanri

Self-hosted, single-user issue tracker in the spirit of Linear — driven by you in
the browser and by Claude over MCP. Continuous-flow Kanban, no sprints/estimates.

- **Stack:** Go (single binary) · PostgreSQL · SvelteKit PWA
- **Realtime:** SSE for live board updates · Web Push (VAPID) for background phone notifications
- **AI:** an MCP server whose tools mirror Linear's verbs (`save_issue`, `list_issues`, …)
- **Docs:** markdown with rendered ```mermaid diagrams
- **Hierarchy:** Initiatives → Projects → Issues
- **States:** Triage → Backlog → Aligning → Ready → In Progress → In Review → Done → Canceled

## Quick start (local, no Docker)

```bash
# 1. Postgres (any instance). Example with Docker:
docker run -d --name kanri-pg -e POSTGRES_USER=kanri -e POSTGRES_PASSWORD=kanri \
  -e POSTGRES_DB=kanri -p 5432:5432 postgres:18-alpine

# 2. Config
cp .env.example .env      # edit KANRI_SESSION_SECRET at least
export $(grep -v '^#' .env | xargs)

# 3. Build frontend + binary
make web
make build

# 4. Migrate + create your account + an API token for Claude
./kanri migrate
./kanri user you@example.com 'a-strong-password'
./kanri genvapid          # paste the two lines into .env, then re-export
./kanri token claude      # copy the printed token

# 5. Run
./kanri serve             # http://localhost:8080
```

## Quick start (Docker Compose — the 24/7 box)

```bash
cp .env.example .env
# set KANRI_SESSION_SECRET, KANRI_SITE_ADDRESS=tracker.yourdomain.com,
# KANRI_BASE_URL=https://tracker.yourdomain.com, and VAPID keys (./kanri genvapid)
docker compose up -d --build

# first-time account + token (exec into the running container)
docker compose exec kanri /app/kanri user you@example.com 'a-strong-password'
docker compose exec kanri /app/kanri token claude
```

Caddy terminates TLS automatically for `KANRI_SITE_ADDRESS`. Point your domain's
DNS at the box and open 80/443.

## Connecting Claude Code (MCP)

Register the remote MCP server (served at `<BASE_URL>/mcp`, Streamable HTTP):

```bash
claude mcp add --transport http kanri https://tracker.yourdomain.com/mcp \
  --header "Authorization: Bearer <the-token-from-kanri-token>"
```

Tools appear as `mcp__kanri__save_issue`, `mcp__kanri__list_issues`, etc. — the
same verbs as the Linear MCP, so existing workflow habits carry over.

### Mermaid convention

Backend-labeled issues should include a ```mermaid diagram in the description.
The global `~/.claude/hooks/require-mermaid.py` hook enforces this on `save_issue`;
add `mcp__kanri__save_issue` to its PreToolUse matcher (see `docs/DEPLOY.md`).

## Commands

| Command | Purpose |
| --- | --- |
| `kanri serve` | API + SSE + Web Push + MCP endpoint |
| `kanri migrate` | apply DB migrations |
| `kanri mcp` | MCP over stdio (local fallback transport) |
| `kanri user <email> <pass>` | create the account |
| `kanri token <name>` | create an API token (shown once) |
| `kanri genvapid` | print a fresh VAPID keypair |

## Backups

```bash
make backup     # pg_dump | gzip -> ./backups/kanri-<timestamp>.sql.gz
```

Add to cron for nightly dumps — see `docs/DEPLOY.md`.
