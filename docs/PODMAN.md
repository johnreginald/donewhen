# Running DoneWhen under Podman

Docker and Podman both read the same `compose.yaml`. Nothing in it is
engine-specific, on purpose. This file collects the Podman-only
friction so `compose.yaml` stays portable.

`make up` / `make down` / `make logs` auto-detect the engine (Podman if it's on
PATH, else Docker). Check with:

```bash
make engine          # -> COMPOSE = podman compose
make up COMPOSE="docker compose"   # force the other one
```

## 1. Do not put `:U` in compose.yaml

Podman's `:U` mount option chowns a bind mount into the rootless user-namespace
range. It solves the Postgres permissions problem below — but **Docker rejects
`:U` outright**, so putting it in `compose.yaml` breaks Docker users.

Fix the host directory instead, once, before the first `make up`:

```bash
# postgres:18-alpine runs as uid/gid 70
podman unshare chown -R 70:70 ./data/pg
```

`podman unshare` enters the user namespace, so the chown lands on the mapped
uid rather than host uid 70. Without this, the DB container fails at boot with a
permissions error on `/var/lib/postgresql`.

## 2. Rootless Podman cannot bind ports below 1024

Only affects the `edge` profile (the bundled Caddy on 80/443). The main service
publishes on `127.0.0.1:8090`, which is fine rootless.

```bash
sudo sysctl -w net.ipv4.ip_unprivileged_port_start=80
echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/99-podman-ports.conf
```

Or leave the profile off and let a host reverse proxy own 80/443 — which is the
recommended setup anyway.

## 3. Stale Docker Desktop credential helper breaks every pull

If Docker Desktop was uninstalled, `~/.docker/config.json` can still say:

```json
"credsStore": "desktop"
```

Podman reads that file and shells out to `docker-credential-desktop`, which now
fails — so even anonymous pulls of public images die with:

```
Error: error getting credentials - err: exit status 1, out: ``
```

Permanent fix — point it at a helper that still exists:

```json
"credsStore": "osxkeychain"
```

Per-command workaround if you'd rather not touch the global file:

```bash
echo '{"auths":{}}' > /tmp/podman-auth.json
podman pull --authfile /tmp/podman-auth.json docker.io/library/postgres:18-alpine
podman run --pull=never --authfile /tmp/podman-auth.json ...
```

Note `podman run` consults the helper even when the image is already local, so
`--authfile` is needed on `run` too, not just `pull`.

## 4. `podman compose` delegates to an external provider

Podman 6 has no built-in compose engine; it shells out to whatever compose
binary is on PATH and prints a notice saying so. Harmless. Silence it with
`compose_warning_logs=false` under `[engine]` in
`~/.config/containers/containers.conf`.

## 5. `restart: unless-stopped` is weaker under Podman

Compose restart policies only hold while the engine is supervising. For a
machine that must come back after reboot, Quadlet + systemd is the real answer:

```bash
podman generate systemd --new --files --name donewhen   # one-off units
# or write .container files under ~/.config/containers/systemd/ (Quadlet)
systemctl --user enable --now donewhen.service
loginctl enable-linger "$USER"     # so user units start without a login
```

Use this when a Podman host must restart the service after a reboot.

## Verifying a migration locally

Migrations are embedded and auto-apply on boot, so the fastest check is a
throwaway database rather than the full stack:

```bash
podman run -d --name donewhen-mig-test --pull=never \
  -e POSTGRES_USER=donewhen -e POSTGRES_PASSWORD=donewhen -e POSTGRES_DB=donewhen \
  -p 55432:5432 docker.io/library/postgres:18-alpine

go build -o ./donewhen ./cmd/donewhen
DONEWHEN_DATABASE_URL='postgres://donewhen:donewhen@localhost:55432/donewhen?sslmode=disable' \
  ./donewhen migrate

podman rm -f donewhen-mig-test
```

Port 55432 avoids colliding with a real Postgres on 5432.
