# Deploying Kanri on the Local PC (24/7)

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
KANRI_ENV=prod
KANRI_SITE_ADDRESS=tracker.yourdomain.com     # Caddy auto-TLS
KANRI_BASE_URL=https://tracker.yourdomain.com # cookies, push origin, deep links
KANRI_SESSION_SECRET=<openssl rand -hex 32>
POSTGRES_PASSWORD=<something strong>
```

Generate VAPID keys once and paste both lines in:

```bash
docker compose run --rm kanri /app/kanri genvapid
# -> KANRI_VAPID_PRIVATE=... / KANRI_VAPID_PUBLIC=...
```

## 3. Bring it up

```bash
docker compose up -d --build
docker compose exec kanri /app/kanri user you@example.com 'a-strong-password'
docker compose exec kanri /app/kanri token claude   # copy the token
```

Open `https://tracker.yourdomain.com`, sign in, and (Settings → Notifications)
enable push on your phone after installing the PWA (Chrome → Add to home screen).

## 4. Wire the mermaid hook to Kanri's MCP

The global hook currently gates only Linear's `save_issue`. To also gate Kanri,
extend the PreToolUse matcher in `~/.claude/settings.json`:

```json
{
  "matcher": "mcp__plugin_linear_linear__save_issue|mcp__kanri__save_issue",
  "hooks": [{ "type": "command", "command": "python3 \"$HOME/.claude/hooks/require-mermaid.py\"" }]
}
```

The hook already inspects the `labels` + `description` fields, which Kanri's
`save_issue` provides in the same shape, so no hook code changes are needed.

## 5. Register the MCP server

```bash
claude mcp add --transport http kanri https://tracker.yourdomain.com/mcp \
  --header "Authorization: Bearer <token>"
```

## 6. Nightly backups (cron)

```cron
# m h dom mon dow
30 3 * * *  cd /path/to/kanri && /usr/bin/make backup >> /var/log/kanri-backup.log 2>&1
```

Keep the `data/` volume (Postgres + Caddy certs) on durable storage. To restore:

```bash
gunzip -c backups/kanri-YYYYMMDD-HHMMSS.sql.gz | \
  docker compose exec -T db psql -U kanri kanri
```

## Updating

```bash
git pull
docker compose up -d --build   # migrations run automatically on start
```
