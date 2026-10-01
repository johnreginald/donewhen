# Self-hosting DoneWhen

This guide puts DoneWhen on a server you control. For a quick local try-out, use the [README](../README.md).

You need:

- A machine that is always on (a small VPS or a home server).
- Docker with the Compose plugin. For Podman, read [PODMAN.md](PODMAN.md).
- Git.

DoneWhen is one Go binary. It serves the API, live updates (SSE), Web Push, MCP and the web app. Postgres stores the data.

## 1. Why you need HTTPS

Use HTTPS on a real server. Three features need it:

| Feature | Why |
|---|---|
| Login cookies | In `prod` mode the cookies are `Secure`. Browsers only send them over HTTPS. |
| Installable PWA | Browsers only offer "Add to home screen" on HTTPS (localhost is the exception). |
| Web Push | Push needs a service worker, and that needs HTTPS. |

Pick one of the three ways in step 3 to get HTTPS.

## 2. Configure

```bash
git clone https://github.com/johnreginald/donewhen.git
cd donewhen
cp .env.example .env
```

Set at least these in `.env`:

```
DONEWHEN_ENV=prod
DONEWHEN_BASE_URL=https://tracker.example.com
DONEWHEN_SESSION_SECRET=<output of: openssl rand -hex 32>
POSTGRES_PASSWORD=<something strong>
```

`DONEWHEN_BASE_URL` must be the exact public URL, with `https://` and no trailing slash. It is used for cookies, the push origin and links.

The full list is in the [README](../README.md#configuration).

### The database password

Compose reads `POSTGRES_PASSWORD` and builds the database URL from it. Postgres stores the password in `data/pg` the first time it starts. If you change `POSTGRES_PASSWORD` later, the database keeps the old one. See [Troubleshooting](#troubleshooting).

### VAPID keys (Web Push)

Push needs a key pair. Make it once:

```bash
docker compose run --rm donewhen /app/donewhen genvapid
```

Paste both lines into `.env`:

```
DONEWHEN_VAPID_PRIVATE=...
DONEWHEN_VAPID_PUBLIC=...
```

Set a contact in `DONEWHEN_VAPID_SUBJECT`, for example `mailto:you@example.com`.

Keep the same keys for the life of the install. If you change them, phones must subscribe again. If you leave the keys empty, push is off and everything else works.

## 3. Put it on the network

The app listens on `127.0.0.1:8090` (change with `DONEWHEN_HOST_PORT`). Only the same machine can reach it. Put a proxy in front. Choose one way.

| Way | Use it when | HTTPS comes from |
|---|---|---|
| A. Domain with Caddy | You have a domain and can open ports 80 and 443 | Caddy, automatic (Let's Encrypt) |
| B. Cloudflare Tunnel | You cannot open ports, or you do not want to | Cloudflare |
| C. Private network (Tailscale) | Only you and your team need access | Tailscale |

### A. A domain with Caddy (bundled)

1. Point a DNS record (`A` or `AAAA`) for `tracker.example.com` at the server's public IP.
2. Open ports 80 and 443 on the firewall and router. Caddy needs port 80 for the certificate check.
3. Add to `.env`:

   ```
   DONEWHEN_SITE_ADDRESS=tracker.example.com
   DONEWHEN_TRUSTED_PROXY_HEADER=X-Forwarded-For
   ```

4. Start with the `edge` profile:

   ```bash
   docker compose --profile edge up -d --build
   ```

   Or run `make up PROFILE=edge`.

Caddy gets the certificate by itself and renews it. It keeps the certificates in `data/caddy`. The `Caddyfile` turns off buffering for `/api/events`, so live updates work.

If another proxy already owns ports 80 and 443 on the machine, do not use the `edge` profile. Point that proxy at `127.0.0.1:8090`. Turn off buffering for `/api/events`.

### B. A Cloudflare Tunnel

1. In Cloudflare, create a tunnel and a public hostname, for example `tracker.example.com`.
2. Point the hostname at `http://localhost:8090`.
3. Run `cloudflared` on the same machine.
4. Add to `.env`:

   ```
   DONEWHEN_BASE_URL=https://tracker.example.com
   DONEWHEN_TRUSTED_PROXY_HEADER=CF-Connecting-IP
   ```

5. Start without the `edge` profile:

   ```bash
   docker compose up -d --build
   ```

No ports are opened. Cloudflare provides HTTPS.

### C. A private network (Tailscale)

1. Install Tailscale on the server and on your devices.
2. Start the app: `docker compose up -d --build`.
3. Share the port over HTTPS with Tailscale Serve:

   ```bash
   tailscale serve --bg 8090
   ```

   The command prints your address, like `https://myserver.tailnet-name.ts.net`.
4. Set `DONEWHEN_BASE_URL` in `.env` to that address, then run `docker compose up -d`.
5. Leave `DONEWHEN_TRUSTED_PROXY_HEADER` empty.

Only devices on your tailnet can open the app. Push and the PWA work because the address is HTTPS.

## 4. Trusted proxy header

`DONEWHEN_TRUSTED_PROXY_HEADER` tells DoneWhen which request header holds the real client IP.

Why it matters: login is rate limited per client IP. Behind a proxy, every request comes from the proxy's address. Without the header, all visitors share one limit. With a wrong header, an attacker can fake the header and skip the limit.

| Setup | Value |
|---|---|
| Cloudflare Tunnel | `CF-Connecting-IP` |
| Bundled Caddy, or another proxy that appends to `X-Forwarded-For` | `X-Forwarded-For` (DoneWhen uses the right-most entry) |
| No proxy, or a private network | leave empty (the TCP address is used) |

Rules:

- Set it only when that proxy is the only way to reach the app. The header is believed as sent.
- If the header is missing or not an IP, DoneWhen uses the proxy's address. All callers then share one limit. This is safe, but strict.

## 5. Start and create the first user

```bash
docker compose up -d --build        # add --profile edge for way A
docker compose exec donewhen /app/donewhen user you@example.com 'a-strong-password'
docker compose exec donewhen /app/donewhen workspace create "My Work" MYW
```

Open `https://tracker.example.com` and sign in.

To get push on your phone, open the site in your phone browser and install it (Add to home screen). Then enable push in Settings, then Notifications.

To connect an AI agent, see [Connect Claude Code](../README.md#connect-claude-code).

## 6. Workspaces

A workspace owns its issues, epics, labels, board columns, documents and activity. Only its members can read them. This holds over the web app, REST, MCP and the live stream.

```bash
docker compose exec donewhen /app/donewhen workspace list
docker compose exec donewhen /app/donewhen workspace create "Client Work" CLI
docker compose exec donewhen /app/donewhen workspace add client-work sam@example.com member
```

New issues get the workspace prefix: `CLI-1`, `CLI-2`. The roles are `owner`, `admin` and `member`.

## 7. Backups

Dump the database:

```bash
make backup
```

This writes `./backups/donewhen-<timestamp>.sql.gz`. It reads the database user and name from the `db` container, so an install that kept `POSTGRES_USER=raenil` works too. `make` uses Podman when it is installed. To force Docker, run `make backup COMPOSE="docker compose"`. The same thing without `make`:

```bash
docker compose exec -T db pg_dump -U donewhen donewhen | gzip > donewhen-backup.sql.gz
```

Run it every night with cron:

```cron
30 3 * * *  cd /path/to/donewhen && /usr/bin/make backup >> /var/log/donewhen-backup.log 2>&1
```

In the commands below, replace `donewhen` after `-U` and the database name with your `POSTGRES_USER` and `POSTGRES_DB` if you changed them.

Copy the files off the machine. Keep `data/` on durable storage. It holds Postgres (`data/pg`) and the Caddy certificates (`data/caddy`).

### Restore

To test a backup, restore it into a new database:

```bash
docker compose exec db createdb -U donewhen donewhen_rehearsal
gunzip -c donewhen-backup.sql.gz | docker compose exec -T db psql -U donewhen donewhen_rehearsal
```

To restore for real over a running install, stop the app, re-create the database, load the dump and start the app:

```bash
docker compose stop donewhen
docker compose exec db dropdb -U donewhen donewhen
docker compose exec db createdb -U donewhen donewhen
gunzip -c donewhen-backup.sql.gz | docker compose exec -T db psql -U donewhen donewhen
docker compose start donewhen
```

To restore on a new machine, or after you delete `data/pg`, start only the database, load the dump, then start the app. Do not start the app first: it creates the tables, and the load then fails on them.

```bash
docker compose up -d db
docker compose ps db        # wait until it says healthy
gunzip -c donewhen-backup.sql.gz | docker compose exec -T db psql -U donewhen donewhen
docker compose up -d --build
```

## 8. Upgrading

1. Back up (step 7).
2. Pull and rebuild:

   ```bash
   git pull
   docker compose up -d --build        # add --profile edge for way A
   ```

3. Check the logs: `docker compose logs -f --tail=100 donewhen`.

Migrations run when the app starts. Each file runs in its own transaction, so a failure rolls that file back. Run one instance only. You can also run `docker compose exec donewhen /app/donewhen migrate` by hand.

To go back, restore the backup and check out the older version.

Upgrading an install from before the rename to DoneWhen: the Compose service was called `raenil` and is now `donewhen`. Add `--remove-orphans` once, or the old container keeps the port:

```bash
docker compose up -d --build --remove-orphans
```

Keep `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB` set to `raenil` in `.env`. Postgres only reads them on first init, so the data stays under the old names. An old `.env` with `RAENIL_*` keys keeps working.

### Migration safety

- Migrations run under a Postgres advisory lock. If two processes start together, they take turns. The second finds nothing left to apply.
- A migration whose first line is `-- donewhen:destructive` drops or rewrites data. If one is pending on a database that already has data, DoneWhen refuses to start and names it. A fresh, empty database applies everything without this gate.
- To continue, make a backup. Then confirm it by name in `.env`, and run `docker compose up -d`:

  ```bash
  make backup
  DONEWHEN_BACKUP_CONFIRMED=0036_plain_tracker.sql
  ```

- The value is a comma-separated list of migration names. Remove the line after the migration is applied.
- `0039_security_cleanup.sql` deletes old push subscriptions that point at non-https or private addresses, and clears issue project or parent links that cross workspaces. Each change is logged as `migration 0039: ...`. If you run tests against a database that already has data, confirm it the same way.
- `make migrate` does both steps, but it runs the local `./donewhen` binary. It needs Go on the machine and `DONEWHEN_DATABASE_URL` pointing at a database it can reach. Compose does not publish Postgres, so on a Compose install use the steps above.

### Upgrading an old install to workspaces

Migration `0011` adds workspaces. It makes one workspace for each existing project (initiative). It places issues with no epic by their key prefix. Anything it cannot place goes to a workspace called `Unsorted`, created only if needed. Every existing account becomes an owner of every workspace. Existing issue keys never change.

This migration rewrites `issues.state_id` and every label link. Rehearse it on a copy:

```bash
make backup
docker compose exec db createdb -U donewhen donewhen_rehearsal
gunzip -c backups/donewhen-<timestamp>.sql.gz | docker compose exec -T db psql -U donewhen donewhen_rehearsal
```

Then compare the row counts of `issues`, `labels`, `issue_labels`, `documents` and `activity` before and after. They must be equal.

## Troubleshooting

| Problem | Cause and fix |
|---|---|
| Cannot log in, no error | `DONEWHEN_ENV=prod` sets Secure cookies, so use HTTPS. For local HTTP, use `DONEWHEN_ENV=dev`. Check that `DONEWHEN_BASE_URL` matches the address in the browser. |
| Login is refused after many tries | The login rate limit. Wait and try again. If every visitor hits it, set `DONEWHEN_TRUSTED_PROXY_HEADER` (step 4). |
| Login works, then you are logged out | The session secret changed, or the cookie was not saved. Keep `DONEWHEN_SESSION_SECRET` fixed. Use HTTPS. |
| App will not start: `DONEWHEN_SESSION_SECRET is required in prod` | Set the secret in `.env`. It needs at least 16 characters. |
| Push does not work | Check: HTTPS; both VAPID keys set (the log says `push=true` at start); the PWA is installed; notifications are allowed for the site. If you changed the VAPID keys, subscribe again. |
| Live updates stop after a short time | Your proxy buffers `/api/events`. Turn off buffering for that path. The bundled Caddy already does. |
| `port is already allocated` or `address already in use` | Another program uses the port. Change `DONEWHEN_HOST_PORT`. For the `edge` profile, ports 80 and 443 must be free. Do not use `edge` if another proxy owns them. |
| `password authentication failed for user "donewhen"` | `POSTGRES_PASSWORD` in `.env` differs from the password stored in `data/pg`. Postgres sets the password only the first time. Put the old password back in `.env`, or change it in the database: `docker compose exec db psql -U donewhen -c "ALTER USER donewhen PASSWORD 'new-password'"`. An install made before the rename uses the user `raenil`: set `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB` to `raenil` in `.env`. On a fresh install with no data, run `docker compose down` and delete `data/pg`. |
| `Bind for 127.0.0.1:8090 failed: port is already allocated` after an upgrade | The pre-rename `raenil` container is still running. Run `docker compose up -d --build --remove-orphans`. |
| `refusing to migrate: destructive migration(s) pending` | Back up, then set `DONEWHEN_BACKUP_CONFIRMED` to the name in the message and run `docker compose up -d`. See [Migration safety](#migration-safety). |
| `DONEWHEN_SESSION_SECRET is still the placeholder` | `.env` still has the secret from `.env.example`. Generate one with `openssl rand -hex 32`. |
| Database container fails with a permissions error | Rootless Podman. See [PODMAN.md](PODMAN.md). |
| Certificate is not issued (Caddy) | DNS does not point at this machine, or port 80 is closed. Check `docker compose logs caddy`. |

## More

- [PODMAN.md](PODMAN.md): running with Podman.
- [ARCHITECTURE.md](ARCHITECTURE.md): how the parts fit.
- [AI-WORKFLOW.md](AI-WORKFLOW.md): connect and drive an AI agent.
