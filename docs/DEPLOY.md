# Deploying Raenil on the Local PC (24/7)

Target: your always-on machine with a domain and 1000/500 Mbps. Caddy handles
TLS; the Go binary serves everything else (API, SSE, Web Push, MCP, static PWA).

## 1. DNS + firewall

- Point `tracker.yourdomain.com` (A/AAAA) at the box's public IP.
- Forward ports 80 and 443 to the box. Caddy needs 80 for the ACME challenge.
- Web Push and PWA install both require HTTPS — the domain is mandatory in prod.

## 2. Environment

```bash
cp .env.example .env
```

Set at minimum:

```
RAENIL_ENV=prod
RAENIL_SITE_ADDRESS=tracker.yourdomain.com     # Caddy auto-TLS
RAENIL_BASE_URL=https://tracker.yourdomain.com # cookies, push origin, deep links
RAENIL_SESSION_SECRET=<openssl rand -hex 32>
POSTGRES_PASSWORD=<something strong>
```

Generate VAPID keys once and paste both lines in:

```bash
docker compose run --rm raenil /app/raenil genvapid
# -> RAENIL_VAPID_PRIVATE=... / RAENIL_VAPID_PUBLIC=...
```

## 3. Bring it up

```bash
docker compose up -d --build
docker compose exec raenil /app/raenil user you@example.com 'a-strong-password'
docker compose exec raenil /app/raenil token claude   # copy the token
# Optional: pin a token to one workspace so an agent sees only that tracker
docker compose exec raenil /app/raenil token globex-agent globex
```

Open `https://tracker.yourdomain.com`, sign in, and (Settings → Notifications)
enable push on your phone after installing the PWA (Chrome → Add to home screen).

## 4. Wire the mermaid hook to Raenil's MCP

The global hook currently gates only Linear's `save_issue`. To also gate Raenil,
extend the PreToolUse matcher in `~/.claude/settings.json`:

```json
{
  "matcher": "mcp__plugin_linear_linear__save_issue|mcp__raenil__save_issue",
  "hooks": [{ "type": "command", "command": "python3 \"$HOME/.claude/hooks/require-mermaid.py\"" }]
}
```

The hook already inspects the `labels` + `description` fields, which Raenil's
`save_issue` provides in the same shape, so no hook code changes are needed.

## 5. Register the MCP server

```bash
claude mcp add --transport http raenil https://tracker.yourdomain.com/mcp \
  --header "Authorization: Bearer <token>"
```

## 6. Nightly backups (cron)

```cron
# m h dom mon dow
30 3 * * *  cd /path/to/raenil && /usr/bin/make backup >> /var/log/raenil-backup.log 2>&1
```

Keep the `data/` volume (Postgres + Caddy certs) on durable storage. To restore:

```bash
gunzip -c backups/raenil-YYYYMMDD-HHMMSS.sql.gz | \
  docker compose exec -T db psql -U raenil raenil
```

## Updating

```bash
git pull
docker compose up -d --build   # migrations run automatically on start
```


## Workspaces

Raenil is multi-tenant: `Workspace → Project (initiative) → Epic → Issue`. A
workspace owns its issues, epics, labels, board columns, artifacts and activity
log, and only its members can read them — over REST, over MCP, and over the SSE
stream alike.

```bash
raenil workspace list                          # what exists, and how many members
raenil workspace create "Client Work" CLI      # new issues become CLI-1, CLI-2, …
raenil workspace add client-work sam@x.com member
```

New issues take their workspace's key prefix. **Existing keys are never
rewritten** — a workspace that already holds `ACM-*` issues adopts `ACM` and
continues from the highest number in use, so nothing that references an old key
breaks.

### Upgrading an existing install

Migration `0011` introduces workspaces and backfills one per existing
initiative. Issues with no epic are placed by their key prefix where that prefix
belongs to a single workspace; anything genuinely unplaceable lands in an
`Unsorted` workspace, which is only created if such rows exist. Every existing
account becomes an owner of every workspace, so nobody loses access.

It rewrites `issues.state_id` and every label link, so **take a backup first and
rehearse against it**:

```bash
make backup                                   # ./backups/raenil-<ts>.sql.gz
createdb raenil_rehearsal
gunzip -c backups/raenil-<ts>.sql.gz | psql raenil_rehearsal
RAENIL_DATABASE_URL=postgres://…/raenil_rehearsal ./raenil migrate
```

Then compare row counts for `issues`, `labels`, `issue_labels`, `documents` and
`activity` before and after — they must be identical, with only foreign keys
repointed. The whole migration runs in one transaction, so a failure rolls back
cleanly.
