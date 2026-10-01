<p align="center"><img src="docs/images/logo.svg" width="88" alt="DoneWhen logo"></p>

<h1 align="center">DoneWhen</h1>

<p align="center">A self-hosted issue tracker that keeps the record of AI-built work.</p>

![The DoneWhen board with the demo workspace](docs/images/board-light.png)

## What and why

AI agents build fast, but the record of what was built gets lost. The chat is gone and the commit says "fix".

DoneWhen fixes this. Every ticket has a **done-when checklist**, written before the work starts. The AI ticks each item as it is met. A ticket cannot move to In Review or Done while an item is open.

When the work ends, DoneWhen keeps the commits, the branch and an engineering document. You review code you did not watch being written, and you know what "done" meant.

Read [docs/CONCEPTS.md](docs/CONCEPTS.md) for the full idea.

## Features

- **Board and states.** Continuous-flow Kanban: Triage, Backlog, Aligning, Ready, In Progress, Blocked, In Review, Done, Canceled. No sprints, no estimates.
- **Done-when gate.** Each ticket has a checklist. Open items block In Review and Done.
- **Commits and engineering docs.** Link commits, branches and PRs to a ticket. Save a document (with Mermaid diagrams) for each change.
- **MCP for AI agents.** An MCP server at `/mcp`. Any MCP client works, including Claude Code.
- **Live updates.** The board updates in real time with Server-Sent Events.
- **Installable PWA with Web Push.** Add it to your phone. Get notified in the background.
- **Light and dark theme.**
- **Multi-workspace.** Each workspace has its own issues, epics, labels and members. It is a hard boundary.

## Quick start

You need Docker with the Compose plugin, and Git. On Podman, see [docs/PODMAN.md](docs/PODMAN.md).

1. Clone the repo.

   ```bash
   git clone https://github.com/johnreginald/donewhen.git
   cd donewhen
   ```

2. Copy the example config.

   ```bash
   cp .env.example .env
   ```

3. Set the secrets in `.env`. Generate a session secret and put it in `DONEWHEN_SESSION_SECRET`.

   ```bash
   openssl rand -hex 32
   ```

   The placeholder secret from `.env.example` is rejected unless `DONEWHEN_ENV=dev`, so replace it. Also change `POSTGRES_PASSWORD`. If you do, change the password inside `DONEWHEN_DATABASE_URL` too (only the local, non-Docker use reads that line).

4. For a local try-out, set the public URL to the port that Compose publishes:

   ```
   DONEWHEN_BASE_URL=http://localhost:8090
   ```

   Leave `DONEWHEN_ENV=dev`. In `prod` mode cookies need HTTPS. For a real server, follow [docs/SELF-HOSTING.md](docs/SELF-HOSTING.md).

5. Start it.

   ```bash
   docker compose up -d --build
   ```

6. Create the first user.

   ```bash
   docker compose exec donewhen /app/donewhen user you@example.com 'a-strong-password'
   ```

   The password needs at least 8 characters.

7. Open <http://localhost:8090> and sign in.

   Compose publishes the app on `127.0.0.1:8090` only. Change the port with `DONEWHEN_HOST_PORT`.

Next, create a workspace in the app, or run `docker compose exec donewhen /app/donewhen workspace create "My Work" MYW`.

## Try the demo

Three commands start DoneWhen with sample data:

```bash
docker compose up -d --build
docker compose exec donewhen /app/donewhen user you@example.com 'a-strong-password'
docker compose exec donewhen /app/donewhen demo
```

`donewhen demo` makes a workspace called Demo (key `DEMO`). It holds about 24 tickets in every state, done-when checklists, blockers, three engineering documents with diagrams, and 3 tickets waiting for your review in the Inbox. The data is fictional. Run it again and it says "demo already exists" and changes nothing.

Prefer to seed on start? Set `DONEWHEN_DEMO=1` in `.env`. Compose then seeds the demo at start, once a user exists. Create the user first, then run `docker compose restart donewhen`.

![A short walkthrough: board, ticket, done-when, inbox, artifacts](docs/images/walkthrough.gif)

### Screenshots

| | Light | Dark |
|---|---|---|
| Board | [light](docs/images/board-light.png) | [dark](docs/images/board-dark.png) |
| Issue and done-when | [light](docs/images/issue-light.png) | [dark](docs/images/issue-dark.png) |
| Inbox | [light](docs/images/inbox-light.png) | [dark](docs/images/inbox-dark.png) |
| Artifacts reader | [light](docs/images/artifacts-light.png) | [dark](docs/images/artifacts-dark.png) |
| List | [light](docs/images/list-light.png) | [dark](docs/images/list-dark.png) |

## Connect Claude Code

DoneWhen has an MCP endpoint at `<your-server>/mcp` (Streamable HTTP, bearer token).

1. Mint a token.

   ```bash
   docker compose exec donewhen /app/donewhen token claude
   ```

   The token is shown once. Copy it.

2. Register the server.

   ```bash
   claude mcp add --transport http donewhen http://localhost:8090/mcp \
     --header "Authorization: Bearer <token>"
   ```

   Use your real server address in place of `http://localhost:8090`, for example `https://tracker.example.com`. The tools appear as `mcp__donewhen__*`.

### Pinned tokens

A normal token reaches all of your workspaces. A **pinned** token reaches one. Give the workspace slug as the second argument:

```bash
docker compose exec donewhen /app/donewhen token acme-agent acme
```

Use a pinned token for an agent that works in one repo.

### The plugin: `/donewhen:tasks`

The plugin adds one command. It lists open issues grouped by epic. It reads the REST API directly, so it costs no model tokens.

1. Install it. This repo is its own marketplace.

   ```bash
   claude plugin marketplace add johnreginald/donewhen
   claude plugin install donewhen@donewhen
   ```

2. Set two environment variables, for example in your shell profile.

   ```bash
   export DONEWHEN_URL=https://tracker.example.com   # your server
   export DONEWHEN_TOKEN=donewhen_...                # a token from `donewhen token`
   ```

3. Run `/donewhen:tasks` in Claude Code.

A pinned token also picks the workspace for you. Otherwise, set a default for a repo in `.claude/donewhen.json`:

```json
{ "workspace": "acme", "project": "Core" }
```

Usage:

```
/donewhen:tasks [workspace] [--project <text>] [--all] [--epic <text>] [--state <name>]
/donewhen:tasks workspaces
/donewhen:tasks use <workspace> [--project <text>]
```

Done and Canceled issues are hidden unless you pass `--all`.

The repo also has a skill that teaches Claude how to use the tools well. See [skills/README.md](skills/README.md). For the full loop, see [docs/AI-WORKFLOW.md](docs/AI-WORKFLOW.md).

## Configuration

Set these in `.env` (Compose) or in the environment. The project was called Raenil before. The old `RAENIL_*` names keep working for one release and log a deprecation warning. If both are set, the `DONEWHEN_*` name wins.

| Name | Default | Meaning |
|---|---|---|
| `DONEWHEN_BASE_URL` | `http://localhost:8080` | Public URL of the app. Used for cookies, the Web Push origin and links. |
| `DONEWHEN_LISTEN_ADDR` | `:8080` | Address the server listens on. Compose sets `:8080` inside the container. |
| `DONEWHEN_DATABASE_URL` | `postgres://donewhen:donewhen@localhost:5432/donewhen?sslmode=disable` | Postgres connection string. Compose builds it from the `POSTGRES_*` values. |
| `DONEWHEN_ENV` | `dev` (Compose: `prod`) | `dev` or `prod`. `prod` needs a session secret and always sets Secure cookies (HTTPS). |
| `DONEWHEN_SESSION_SECRET` | none | Secret for sessions. At least 16 characters. Required in `prod`. Generate with `openssl rand -hex 32`. |
| `DONEWHEN_ISSUE_PREFIX` | `R` | Key prefix of the legacy default workspace. New workspaces choose their own prefix. |
| `DONEWHEN_TRUSTED_PROXY_HEADER` | empty | Header your proxy sets to the real client IP. Used for login rate limiting. See [SELF-HOSTING.md](docs/SELF-HOSTING.md#4-trusted-proxy-header). |
| `DONEWHEN_VAPID_PUBLIC` | empty | Web Push public key. Push is off if this or the private key is empty. |
| `DONEWHEN_VAPID_PRIVATE` | empty | Web Push private key. Make a pair with `donewhen genvapid`. |
| `DONEWHEN_VAPID_SUBJECT` | `mailto:admin@localhost` | Contact for push services. Use `mailto:you@example.com`. |
| `DONEWHEN_DEMO` | empty | Compose passes it through. `1` seeds the Demo workspace at start, once a user exists. |
| `DONEWHEN_BACKUP_CONFIRMED` | empty | Names of destructive migrations you have backed up for, comma-separated. See [SELF-HOSTING.md](docs/SELF-HOSTING.md#migration-safety). |
| `DONEWHEN_HOST_PORT` | `8090` | Compose only. Host port on `127.0.0.1` for the app. |
| `DONEWHEN_SITE_ADDRESS` | `:80` | Compose only. Domain for the bundled Caddy (`edge` profile). |
| `POSTGRES_USER` | `donewhen` | Compose only. Database user. |
| `POSTGRES_PASSWORD` | `donewhen` | Compose only. Database password. Change it. |
| `POSTGRES_DB` | `donewhen` | Compose only. Database name. |
| `DONEWHEN_URL` | none | Plugin only. Your server address. Set in your shell. |
| `DONEWHEN_TOKEN` | none | Plugin only. An API token. Set in your shell. |

## Upgrading and backups

Upgrade:

```bash
git pull
docker compose up -d --build
```

Migrations run when the app starts. Back up first. If a destructive migration is pending, the app refuses to start and tells you what to do. See [Migration safety](docs/SELF-HOSTING.md#migration-safety).

Upgrading from before the rename to DoneWhen? The Compose service was called `raenil`, so run `docker compose up -d --build --remove-orphans`. The old container holds the port until you do. Keep `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB` at `raenil` in `.env`.

Back up the database:

```bash
docker compose exec -T db pg_dump -U donewhen donewhen | gzip > donewhen-$(date +%Y%m%d-%H%M%S).sql.gz
```

Or run `make backup`, which writes to `./backups` and uses your `POSTGRES_USER` and `POSTGRES_DB`. `make` picks Podman when it is installed. To force Docker, run `make backup COMPOSE="docker compose"`.

Restore on a new machine or an empty `data/pg`. Start only the database, load the dump, then start the app:

```bash
docker compose up -d db
# wait until `docker compose ps db` says healthy (about 20 seconds on a first start)
gunzip -c donewhen-YYYYMMDD-HHMMSS.sql.gz | docker compose exec -T db psql -U donewhen donewhen
docker compose up -d --build
```

Start the app only after the load. If the app ran first, it has already created the tables, and the load fails on them. To restore over a running install, see [SELF-HOSTING.md](docs/SELF-HOSTING.md#restore).

More in [docs/SELF-HOSTING.md](docs/SELF-HOSTING.md).

## Development

You need Go, Node 22 and Docker. See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, tests and the PR process.

## Documentation

| Page | What it covers |
|---|---|
| [docs/CONCEPTS.md](docs/CONCEPTS.md) | The idea, vocabulary and states |
| [docs/AI-WORKFLOW.md](docs/AI-WORKFLOW.md) | How an AI agent drives a ticket |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Parts, packages and data flow |
| [docs/SELF-HOSTING.md](docs/SELF-HOSTING.md) | Put it on a server, with HTTPS |
| [docs/PODMAN.md](docs/PODMAN.md) | Run it with Podman |
| [skills/README.md](skills/README.md) | The Claude skill |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |

## Security

Report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## License

[AGPL-3.0](LICENSE).
