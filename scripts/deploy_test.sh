#!/usr/bin/env bash
# Test scripts/deploy.sh against a throwaway stack. Needs local docker and python3.
#   bash scripts/deploy_test.sh
# The "app" is a tiny busybox image: it answers every request with a health JSON
# that holds the commit it was built from, and each commit chooses its own behaviour with two files
# (fail = exit at start, pending = a pending destructive migration).
# It uses its own compose project and a free localhost port, and removes both.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
deploy="$here/deploy.sh"

T=$(mktemp -d)
PROJ="donewhen-deploytest-$$"
IMG="donewhen-deploytest-$$"
PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
BASE_IMAGE=${BASE_IMAGE:-postgres:18-alpine}   # any image with busybox (sh, httpd)
export TEST_PORT=$PORT
pass=0

cleanup() {
  docker compose -p "$PROJ" -f "$T/dev/compose.yaml" --project-directory "$T/deploy" down -v --remove-orphans >/dev/null 2>&1 || true
  # shellcheck disable=SC2046
  docker image rm -f $(docker image ls "$IMG" -q 2>/dev/null) >/dev/null 2>&1 || true
  rm -rf "$T"
}
trap cleanup EXIT

ok() { pass=$((pass + 1)); echo "ok - $*"; }
fail() { echo "FAIL - $*" >&2; exit 1; }
live_commit() { curl -fsS "http://127.0.0.1:$PORT/api/health" | sed -n 's/.*"commit":"\([^"]*\)".*/\1/p'; }

# ---- the fake repo: origin, a dev clone to commit in, the deploy clone ----
git init -q --bare -b main "$T/origin.git"
git clone -q "$T/origin.git" "$T/dev" 2>/dev/null
cd "$T/dev"
git config user.email t@example.test
git config user.name test

cat > Dockerfile <<EOF
FROM $BASE_IMAGE
ARG GIT_COMMIT=dev
ARG BUILD_TIME=
ENV GIT_COMMIT=\$GIT_COMMIT BUILD_TIME=\$BUILD_TIME
COPY app.sh /app.sh
COPY fail pending /
ENTRYPOINT ["/bin/sh", "/app.sh"]
CMD ["serve"]
EOF
cat > app.sh <<'EOF'
case "$1" in
  migrate-pending) cat /pending ;;
  serve)
    if [ -s /fail ]; then echo "boom: the app cannot start"; exit 1; fi
    body=$(printf '{"status":"ok","commit":"%s","builtAt":"%s"}' "$GIT_COMMIT" "$BUILD_TIME")
    # Every path answers 200 with the health JSON: the landing page check only needs a 200.
    while true; do
      printf 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nConnection: close\r\n\r\n%s' "$body" | nc -l -p 8080 >/dev/null
    done ;;
esac
EOF
cat > compose.yaml <<'EOF'
services:
  db:
    image: ${BASE_IMAGE_REF:-postgres:18-alpine}
    entrypoint: ["sleep", "100000"]
    healthcheck:
      test: ["CMD", "true"]
      interval: 1s
  donewhen:
    build:
      context: .
      args:
        GIT_COMMIT: ${GIT_COMMIT:-dev}
        BUILD_TIME: ${BUILD_TIME:-}
    image: ${IMAGE_NAME:-donewhen}:${DONEWHEN_IMAGE_TAG:-latest}
    depends_on:
      db:
        condition: service_healthy
    ports: ["127.0.0.1:${TEST_PORT}:8080"]
    environment:
      DONEWHEN_BACKUP_CONFIRMED: ${DONEWHEN_BACKUP_CONFIRMED:-}
    restart: unless-stopped
EOF
export BASE_IMAGE_REF=$BASE_IMAGE

commit_with() { # <fail 0|1> <pending text> <message>
  if [ "$1" = 1 ]; then echo crash > fail; else : > fail; fi
  printf '%s' "$2" > pending
  echo "$3" > note
  git add -A
  git commit -q -m "$3"
  git push -q origin main
  git rev-parse --short HEAD
}
sync_deploy_dir() {
  git -C "$T/deploy" fetch -q origin
  git -C "$T/deploy" reset -q --hard origin/main
}

commit_with 0 "" "good A" >/dev/null
git clone -q "$T/origin.git" "$T/deploy" 2>/dev/null

export DEPLOY_DIR="$T/deploy" COMPOSE_FILES="compose.yaml" COMPOSE_PROJECT="$PROJ" IMAGE_NAME="$IMG"
export HEALTH_URL="http://127.0.0.1:$PORT/api/health" HEALTH_TIMEOUT=10 SUDO=""
export BACKUP_DIR="$T/backups" MIN_BYTES=1
# shellcheck disable=SC2016
export BACKUP_CMD='echo "dump $RANDOM" | gzip > "$BACKUP_DIR/test-$(date +%s)-$RANDOM.sql.gz"'
unset CONFIRM

run() { bash "$deploy" "$@" > "$T/out.txt" 2>&1; }   # sets $? for the caller via `|| rc=$?`
rc=0

# (g) refuse when HEAD is behind origin/main
B=$(commit_with 0 "" "good B")
run beta || rc=$?
[ "$rc" = 2 ] || fail "behind origin/main: exit $rc, want 2"
grep -q "HEAD is not at" "$T/out.txt" || fail "no refusal message"
ok "refuses when the clone is not at origin/main (exit 2)"
sync_deploy_dir

# (a) good deploy of B, with no image before it
rc=0; run beta || rc=$?
[ "$rc" = 0 ] || { cat "$T/out.txt"; fail "good deploy: exit $rc"; }
[ "$(live_commit)" = "$B" ] || fail "live commit is $(live_commit), want $B"
curl -fsS "http://127.0.0.1:$PORT/api/health" | grep -q '"builtAt":"20' || fail "builtAt not set"
ok "good deploy: $B is live, builtAt is set"

# deploy a second good commit; both images stay
C=$(commit_with 0 "" "good C"); sync_deploy_dir
rc=0; run beta || rc=$?
[ "$rc" = 0 ] || { cat "$T/out.txt"; fail "second deploy: exit $rc"; }
[ "$(live_commit)" = "$C" ] || fail "live commit is not $C"
docker image inspect "$IMG:$B" >/dev/null 2>&1 || fail "old image $B was not kept"
ok "second good deploy: $C is live and the old image $B is kept"

# images beyond the last 3 are removed
commit_with 0 "" "good D" >/dev/null; sync_deploy_dir; run beta || fail "deploy D"
E=$(commit_with 0 "" "good E"); sync_deploy_dir; run beta || fail "deploy E"[ "$(docker image ls "$IMG" -q | sort -u | wc -l | tr -d ' ')" = 3 ] || fail "want 3 images, have: $(docker image ls "$IMG" --format '{{.Tag}}' | tr '\n' ' ')"
docker image inspect "$IMG:$B" >/dev/null 2>&1 && fail "image $B should be pruned; images: $(docker image ls "$IMG" --format '{{.Tag}}@{{.CreatedAt}}' | tr '\n' ' ')"
ok "only the last 3 images are kept"

# (b) a commit whose container exits at start
F=$(commit_with 1 "" "bad F, exits at start"); sync_deploy_dir
rc=0; run beta || rc=$?
[ "$rc" != 0 ] || fail "failed deploy exited 0"
[ "$rc" = 1 ] || { cat "$T/out.txt"; fail "failed deploy: exit $rc, want 1 (rolled back)"; }
[ "$(live_commit)" = "$E" ] || fail "after the rollback the live commit is '$(live_commit)', want $E"
grep -q "rolled back\|roll back" "$T/out.txt" || fail "no rollback message"
grep -q "A migration cannot go backwards" "$T/out.txt" || fail "no migration note"
docker image inspect "$IMG:$F" >/dev/null 2>&1 && fail "the failed image $F should be removed"
ok "a container that exits at start: rolled back to $E, exit $rc"

# (c) a pending destructive migration without CONFIRM stops before any change
G=$(commit_with 0 "9999_drop_things.sql" "good G, destructive migration"); sync_deploy_dir
rc=0; run beta || rc=$?
[ "$rc" = 3 ] || { cat "$T/out.txt"; fail "unconfirmed migration: exit $rc, want 3"; }
grep -q "9999_drop_things.sql" "$T/out.txt" || fail "the migration name is not printed"
[ "$(live_commit)" = "$E" ] || fail "the live commit changed to $(live_commit)"
ok "pending destructive migration without CONFIRM: stopped, name printed, $E still live"

# the same, with CONFIRM
rc=0; CONFIRM=9999_drop_things.sql run beta || rc=$?
[ "$rc" = 0 ] || { cat "$T/out.txt"; fail "confirmed migration: exit $rc"; }
[ "$(live_commit)" = "$G" ] || fail "live commit is not $G"
ok "pending destructive migration with CONFIRM: deployed, $G is live"

# (d) rollback command: back to the previous image, and again with a commit
rc=0; run rollback beta || rc=$?
[ "$rc" = 0 ] || { cat "$T/out.txt"; fail "rollback: exit $rc"; }
[ "$(live_commit)" = "$E" ] || fail "after rollback the live commit is '$(live_commit)', want $E; images: $(docker image ls "$IMG" --format '{{.Tag}}@{{.CreatedAt}}' | tr '\n' ' ') $(cat "$T/out.txt" | head -3)"
ok "rollback beta: the previous commit $E is live"
rc=0; run rollback beta "$G" || rc=$?
[ "$rc" = 0 ] && [ "$(live_commit)" = "$G" ] || fail "rollback to a named commit"
ok "rollback beta <commit>: $G is live"

# a dirty tree is refused
echo x >> "$T/deploy/note"
rc=0; run beta || rc=$?
[ "$rc" = 2 ] || fail "dirty tree: exit $rc, want 2"
git -C "$T/deploy" checkout -q -- note
ok "a dirty working tree is refused"

echo "all $pass checks passed"
