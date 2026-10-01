# Running DoneWhen under Podman

Docker and Podman both read the same `compose.yaml`. Nothing in it is
engine-specific, on purpose. This file lists the Podman-only
problems, so that `compose.yaml` stays portable.

`make up` / `make down` / `make logs` find the engine automatically. They use Podman if it is on
PATH, and Docker if it is not. To check the engine, do this:

```bash
make engine          # -> COMPOSE = podman compose
make up COMPOSE="docker compose"   # force the other one
```

## 1. Do not put `:U` in compose.yaml

The Podman mount option `:U` changes the owner of a bind mount to the rootless user-namespace
range. It solves the Postgres permissions problem below, but **Docker rejects
`:U` outright**. If you put it in `compose.yaml`, Docker users cannot start the stack.

Instead, fix the host directory one time, before the first `make up`:

```bash
# postgres:18-alpine runs as uid/gid 70
podman unshare chown -R 70:70 ./data/pg
```

`podman unshare` enters the user namespace. The chown then applies to the mapped
uid, not to host uid 70. If you do not do this, the DB container fails at boot with a
permissions error on `/var/lib/postgresql`.

## 2. Rootless Podman cannot bind ports below 1024

This problem affects only the `edge` profile (the bundled Caddy on 80/443). The main service
publishes on `127.0.0.1:8090`. Rootless Podman allows this port.

```bash
sudo sysctl -w net.ipv4.ip_unprivileged_port_start=80
echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/99-podman-ports.conf
```

You can also leave the profile off. Then a host reverse proxy owns 80/443. This is
the recommended setup.

## 3. Stale Docker Desktop credential helper breaks every pull

If you uninstalled Docker Desktop, `~/.docker/config.json` can still contain:

```json
"credsStore": "desktop"
```

Podman reads that file and runs `docker-credential-desktop`. This helper now
fails. Because of this, even anonymous pulls of public images fail with:

```
Error: error getting credentials - err: exit status 1, out: ``
```

For a permanent fix, point the setting at a helper that still exists:

```json
"credsStore": "osxkeychain"
```

To avoid a change to the global file, use this workaround for each command:

```bash
echo '{"auths":{}}' > /tmp/podman-auth.json
podman pull --authfile /tmp/podman-auth.json docker.io/library/postgres:18-alpine
podman run --pull=never --authfile /tmp/podman-auth.json ...
```

Note that `podman run` uses the helper even when the image is already local. Add
`--authfile` to `run` and to `pull`.

## 4. `podman compose` delegates to an external provider

Podman 6 has no built-in compose engine. It runs the compose
binary that is on PATH, and it prints a notice about this. The notice is harmless. To hide it, set
`compose_warning_logs=false` under `[engine]` in
`~/.config/containers/containers.conf`.

## 5. `restart: unless-stopped` is weaker under Podman

Compose restart policies work only while the engine supervises the container. If a
machine must start the service again after a reboot, use Quadlet and systemd:

```bash
podman generate systemd --new --files --name donewhen   # one-off units
# or write .container files under ~/.config/containers/systemd/ (Quadlet)
systemctl --user enable --now donewhen.service
loginctl enable-linger "$USER"     # so user units start without a login
```

Use this when a Podman host must restart the service after a reboot.

## Verifying a migration locally

The binary contains the migrations and applies them automatically at boot. The fastest check is a
throwaway database. It is faster than the full stack.

```bash
podman run -d --name donewhen-mig-test --pull=never \
  -e POSTGRES_USER=donewhen -e POSTGRES_PASSWORD=donewhen -e POSTGRES_DB=donewhen \
  -p 55432:5432 docker.io/library/postgres:18-alpine

go build -o ./donewhen ./cmd/donewhen
DONEWHEN_DATABASE_URL='postgres://donewhen:donewhen@localhost:55432/donewhen?sslmode=disable' \
  ./donewhen migrate

podman rm -f donewhen-mig-test
```

Port 55432 prevents a collision with a real Postgres on 5432.
