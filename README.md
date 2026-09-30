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

## Quick start (local, no containers)

```bash
# 1. Postgres (any instance). Example with Docker (or Podman):
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

> Commands below use `docker compose`, which is what the production box runs.
> On a Podman dev machine use `podman compose` instead — or just `make up`,
> which picks the engine automatically. See [docs/PODMAN.md](docs/PODMAN.md).

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

You rarely have to say which workspace you mean. Listing tools span every
workspace the token can reach, and a call that names an existing issue, epic,
initiative or document has the workspace derived from it — issue keys are
globally unique, so `get_issue R-8` is a lookup, not a guess. Membership is
checked on whatever comes back, so deriving can never reach a workspace the
account is not in.

The one genuinely ambiguous case is creating something with no parent to inherit
from. There, pass `workspace: "globex"` (or belong to a single workspace).

Pin a token when an agent should be confined to one repo's tracker:

```bash
raenil token globex-agent globex    # sees only the 'globex' workspace
```

A pinned token needs no argument at all and is refused any other workspace.
`list_workspaces` shows what a token can reach.

### Mermaid convention

Backend-labeled issues should include a ```mermaid diagram in the description.
The global `~/.claude/hooks/require-mermaid.py` hook enforces this on `save_issue`;
add `mcp__raenil__save_issue` to its PreToolUse matcher (see `docs/DEPLOY.md`).

## Claude Code plugin: `/tasks`

`plugin/` is a Claude Code plugin. Its `/raenil:tasks` command prints a
workspace's open issues grouped by epic. It reads the REST API directly, so it
costs no model tokens to fetch.

```
Platform · 37 open

Raenil — Data integrity                 0/7 done
  ○ PP-181  Backlog      save_project and save_initiative update only the fields sent
  ◐ PP-185  In Progress  Issue page updates live and refuses to overwrite a newer edit

Raenil — Realtime & tests               0/6 done
  ○ PP-197  Backlog      Web live stream sends its workspace and catches up…  ⊘ PP-196
```

Install it from this repo, which is its own marketplace:

```bash
claude plugin marketplace add /path/to/raenil
claude plugin install raenil@raenil
```

Set a token. Any `raenil token` works, and a pinned token also picks the
workspace for you:

```bash
export RAENIL_TOKEN=raenil_…                     # required
export RAENIL_URL=https://tracker.example.com     # optional, this is the default
```

Give each repo a default workspace, and optionally a default project, in
`.claude/raenil.json`:

```json
{ "workspace": "platform", "project": "Platform" }
```

Usage:

```
/raenil:tasks [workspace] [--project <text>] [--all] [--epic <text>] [--state <name>]
/raenil:tasks workspaces
/raenil:tasks use <workspace> [--project <text>]
```

`workspaces` lists every workspace the token reaches, with open counts per
project. `▸` marks the current default:

```
  Workspace                 Prefix  Open  Projects
▸ platform          PP        70  Platform (62)
  acme-engineering          ACM      139  Acme Engineering (101)
  unsorted                  UNS        2  —
```

`use` switches the default. It writes `.claude/raenil.json` at the git root, or
`~/.config/raenil/default.json` outside a repo, and then shows the new view.
`--project` stores a default project too, and leaving it out clears one. An
unknown workspace, or a project name that matches nothing or several, is
refused and leaves the file untouched.

| Glyph | State |
|---|---|
| `◌` | Triage |
| `○` | Backlog, Aligning, Ready |
| `◐` | In Progress |
| `⊘` | Blocked |
| `◕` | In Review |
| `●` | Done |
| `×` | Canceled |

A trailing `⊘ KEY` means the issue still waits on that blocker. Done and
Canceled issues are hidden unless you pass `--all`. Run the tests with
`node --test 'plugin/scripts/*.test.mjs'`.

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
