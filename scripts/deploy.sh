#!/usr/bin/env bash
# Deploy DoneWhen on the PC. Run it ON the PC, from any directory:
#   bash scripts/deploy.sh beta                 # follow origin/main, in ~/donewhen-cloud
#   bash scripts/deploy.sh prod [<tag>]         # private tracker in ~/donewhen (origin/main, or the tag)
#   bash scripts/deploy.sh rollback <beta|prod> [<commit>]
#   CONFIRM=<migration>.sql[,<migration>.sql] bash scripts/deploy.sh beta
#
# The steps, in order (docs/BETA-RUNBOOK.md, section "Deploy"):
#   1 refuse when the git state is wrong     5 start, wait for health (60 s)
#   2 back up the database and .env          6 check the live commit and the landing page
#   3 build the image donewhen:<commit>       7 on failure: start the previous image, exit 1
#   4 stop when a destructive migration is pending and CONFIRM does not name it
#
# Exit codes: 0 done, 1 the deploy failed and the previous image runs again,
# 2 refused (git state, usage), 3 destructive migration not confirmed,
# 4 the deploy failed and the rollback failed too.
#
# No ssh in here, and no password: docker runs as "sudo -n" (deploy/sudoers-donewhen).
# Every name is an environment variable, so a test can aim the script at a throwaway stack:
#   DEPLOY_DIR       clone to deploy                 COMPOSE_FILES   compose files, relative to DEPLOY_DIR
#   COMPOSE_PROJECT  compose project name            IMAGE_NAME      image repository (tag = commit)
#   HEALTH_URL       health route on the host        LANDING_URL     page that must return 200
#   HEALTH_TIMEOUT   seconds to wait (default 60)    BACKUP_DIR      where dumps and .env copies go
#   BACKUP_CMD       command that writes a *.sql.gz into BACKUP_DIR (run in DEPLOY_DIR);
#                    empty = dump DB_CONTAINER with pg_dump (PG_USER, PG_DB)
#   MIN_BYTES        smallest dump to accept         KEEP_IMAGES     images to keep (default 3)
#   ORIGIN_REF       the ref beta follows (default origin/main)
#   SUDO             command prefix for docker (default "sudo -n"; set to "" when not needed)
set -euo pipefail

usage() {
  echo "usage: deploy.sh <beta|prod [tag]>" >&2
  echo "       deploy.sh rollback <beta|prod> [commit]" >&2
  exit 2
}
die() { local code=$1; shift; echo "deploy: $*" >&2; exit "$code"; }
say() { echo "== $*"; }

mode=deploy
if [ "${1:-}" = rollback ]; then mode=rollback; shift; fi
target=${1:-}
extra=${2:-}
case "$target" in beta | prod) ;; *) usage ;; esac
if [ "$mode" = deploy ] && [ "$target" = beta ] && [ -n "$extra" ]; then usage; fi

# ---- settings per target; every one can be overridden ----
case "$target" in
  beta)
    : "${DEPLOY_DIR:=$HOME/donewhen-cloud}"
    : "${COMPOSE_FILES:=compose.yaml compose.beta.yaml}"
    : "${COMPOSE_PROJECT:=donewhen-beta}"
    : "${IMAGE_NAME:=donewhen-beta}"
    : "${HEALTH_URL:=http://127.0.0.1:8093/api/health}"
    : "${BACKUP_DIR:=$HOME/donewhen-cloud-backups}"
    : "${MIN_BYTES:=2000}"
    # beta-backup.sh does the dump; it reads BACKUP_DIR, BETA_DIR and DC.
    : "${BACKUP_CMD=bash scripts/beta-backup.sh}"
    ;;
  prod)
    : "${DEPLOY_DIR:=$HOME/donewhen}"
    : "${COMPOSE_FILES:=compose.yaml}"
    : "${COMPOSE_PROJECT:=donewhen}"
    : "${IMAGE_NAME:=donewhen}"
    : "${HEALTH_URL:=http://127.0.0.1:8090/api/health}"
    : "${BACKUP_DIR:=$HOME/raenil-backups}"
    : "${MIN_BYTES:=1000000}"
    : "${BACKUP_CMD=}"
    ;;
esac
: "${DB_CONTAINER:=${COMPOSE_PROJECT}-db-1}"
: "${PG_USER:=donewhen}"
: "${PG_DB:=donewhen}"
: "${LANDING_URL:=${HEALTH_URL%/api/health}/}"
: "${HEALTH_TIMEOUT:=60}"
: "${KEEP_IMAGES:=3}"
: "${ORIGIN_REF:=origin/main}"
SUDO=${SUDO-sudo -n}
CONFIRM=${CONFIRM:-}

[ -d "$DEPLOY_DIR" ] || die 2 "DEPLOY_DIR does not exist: $DEPLOY_DIR"
DEPLOY_DIR=$(cd "$DEPLOY_DIR" && pwd)
HISTORY="$DEPLOY_DIR/.deploy-history"

# ---- helpers ----
dkr() {
  # $SUDO is a word list on purpose ("sudo -n").
  # shellcheck disable=SC2086
  $SUDO docker "$@"
}
# docker compose, aimed at the project folder. sudoers matches on the first arguments.
dc() {
  local args=() f
  for f in $COMPOSE_FILES; do args+=(-f "$DEPLOY_DIR/$f"); done
  dkr compose --project-directory "$DEPLOY_DIR" -p "$COMPOSE_PROJECT" "${args[@]}" "$@"
}

# The tag of the image the donewhen container runs now. Empty when there is none.
current_tag() {
  dkr ps -a \
    --filter "label=com.docker.compose.project=$COMPOSE_PROJECT" \
    --filter "label=com.docker.compose.service=donewhen" \
    --filter "label=com.docker.compose.oneoff=False" \
    --format '{{.Image}}' | head -n1 | sed -n "s|^$IMAGE_NAME:||p"
}
# Image tags that are commits, most recently live first. The order comes from the
# history file: image dates are not reliable (a cached build keeps an old date).
# Images that are not in the history follow, in name order.
list_tags() {
  local have t seen=" "
  have=$(dkr image ls "$IMAGE_NAME" --format '{{.Tag}}' | grep -E '^[0-9a-f]{7,40}$' | sort -u || true)
  for t in $({
    if [ -f "$HISTORY" ]; then awk '{a[NR]=$0} END{for(i=NR;i>=1;i--)print a[i]}' "$HISTORY"; fi
    echo "$have"
  }); do
    case "$seen" in *" $t "*) continue ;; esac
    if printf '%s\n' "$have" | grep -qxF -- "$t"; then
      echo "$t"
      seen="$seen$t "
    fi
  done
}
# Note that commit $1 is live now. The file is untracked in the clone (.gitignore).
record_live() {
  echo "$1" >> "$HISTORY"
  tail -n 50 "$HISTORY" > "$HISTORY.tmp" && mv "$HISTORY.tmp" "$HISTORY"
}
health_commit() {
  curl -fsS --max-time 5 "$HEALTH_URL" 2>/dev/null | sed -n 's/.*"commit" *: *"\([^"]*\)".*/\1/p' || true
}
# Wait until the health route answers. Prints nothing; returns 1 on timeout.
wait_healthy() {
  local i=0 max=$((HEALTH_TIMEOUT / 2))
  while [ "$i" -lt "$max" ]; do
    if [ -n "$(health_commit)" ]; then return 0; fi
    sleep 2
    i=$((i + 1))
  done
  return 1
}
latest_backup() {
  { ls -t "$BACKUP_DIR"/*.sql.gz 2>/dev/null || true; } | head -n1
}
migration_note() {
  local backup
  backup=$(latest_backup)
  echo "A migration cannot go backwards. The new image may have changed the schema." >&2
  echo "The old image runs on the schema that exists now." >&2
  if [ -n "$backup" ]; then echo "Backup from before the deploy: $backup" >&2; fi
}

# Start image tag $1 without a build and check it. Returns 0 when it is healthy
# and reports that commit.
start_image() {
  local tag=$1 got
  DONEWHEN_IMAGE_TAG=$tag dc up -d --no-build --no-deps donewhen || return 1
  wait_healthy || return 1
  got=$(health_commit)
  [ "$got" = "$tag" ] || { echo "deploy: the container reports commit '$got', not '$tag'" >&2; return 1; }
  record_live "$tag"
}

export IMAGE_NAME

# ---- rollback command ----
if [ "$mode" = rollback ]; then
  cur=$(current_tag)
  want=$extra
  if [ -z "$want" ]; then
    for t in $(list_tags); do
      if [ "$t" != "$cur" ]; then want=$t; break; fi
    done
  fi
  [ -n "$want" ] || die 2 "no previous image of $IMAGE_NAME to start"
  dkr image inspect "$IMAGE_NAME:$want" >/dev/null 2>&1 || die 2 "image $IMAGE_NAME:$want does not exist"
  say "rollback $target: $cur -> $want"
  if start_image "$want"; then
    migration_note
    echo "The old image $want is healthy on the current schema."
    say "done: $want is live"
    exit 0
  fi
  migration_note
  echo "The old image $want does NOT start on the current schema. Restore the backup." >&2
  exit 4
fi

# ---- 1. git state ----
say "1. check the git state ($target)"
cd "$DEPLOY_DIR"
git fetch --quiet --tags origin
[ -z "$(git status --porcelain --untracked-files=no)" ] || die 2 "the working tree has changes. Stop."
head_commit=$(git rev-parse HEAD)
if [ "$target" = prod ] && [ -n "$extra" ]; then
  want_commit=$(git rev-parse --verify --quiet "refs/tags/$extra^{commit}") || die 2 "tag $extra does not exist"
  label="tag $extra"
else
  want_commit=$(git rev-parse --verify --quiet "$ORIGIN_REF^{commit}") || die 2 "$ORIGIN_REF does not exist"
  label=$ORIGIN_REF
fi
[ "$head_commit" = "$want_commit" ] || die 2 "HEAD is not at $label. Run: git -C $DEPLOY_DIR reset --hard $label"
commit=$(git rev-parse --short HEAD)
build_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
git log --oneline -1

prev=$(current_tag)
if [ -z "$prev" ] || [ "$prev" = "$commit" ]; then
  prev=
  for t in $(list_tags); do
    if [ "$t" != "$commit" ]; then prev=$t; break; fi
  done
fi
echo "deploying $commit; the previous image is ${prev:-none}"

# ---- 2. backup ----
say "2. back up the database and .env"
stamp=$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
backup_before=$(latest_backup)
if [ -n "$BACKUP_CMD" ]; then
  # beta-backup.sh settings. Other commands ignore them.
  (
    cd "$DEPLOY_DIR"
    export BACKUP_DIR BETA_DIR="$DEPLOY_DIR" MIN_BYTES
    # --project-directory first, so the sudoers rule matches this command too.
    export DC="${SUDO:+$SUDO }docker compose --project-directory $DEPLOY_DIR"
    bash -c "$BACKUP_CMD"
  ) || die 1 "the backup command failed. Nothing changed."
  backup_file=$(latest_backup)
  [ -n "$backup_file" ] && [ "$backup_file" != "$backup_before" ] || die 1 "the backup command wrote no new dump. Nothing changed."
else
  backup_file="$BACKUP_DIR/pre-deploy-$stamp.sql.gz"
  dkr exec "$DB_CONTAINER" pg_dump -U "$PG_USER" -d "$PG_DB" | gzip > "$backup_file" || die 1 "pg_dump failed. Nothing changed."
  chmod 600 "$backup_file"
fi
size=$(wc -c < "$backup_file")
[ "$size" -ge "$MIN_BYTES" ] || die 1 "the dump is too small ($size bytes, limit $MIN_BYTES). Nothing changed."
if [ -f "$DEPLOY_DIR/.env" ]; then
  cp "$DEPLOY_DIR/.env" "$BACKUP_DIR/env-$stamp.bak"
  chmod 600 "$BACKUP_DIR/env-$stamp.bak"
  # The .env copies hold secrets: keep the last 5.
  { ls -t "$BACKUP_DIR"/env-*.bak 2>/dev/null || true; } | tail -n +6 | while read -r old; do rm -f "$old"; done
fi
echo "backup: $backup_file ($size bytes)"

# ---- 3. build ----
say "3. build $IMAGE_NAME:$commit"
export GIT_COMMIT=$commit BUILD_TIME=$build_time
DONEWHEN_IMAGE_TAG=$commit dc build donewhen || die 1 "the build failed. Nothing changed."

# ---- 4. migrations ----
say "4. check pending migrations"
dc up -d --wait db || die 1 "the database did not start. Nothing changed."
pending=$(DONEWHEN_IMAGE_TAG=$commit dc run --rm -T --no-deps donewhen migrate-pending | tail -n1 | tr -d '\r') \
  || die 1 "migrate-pending failed. Nothing changed."
if [ -n "$pending" ]; then
  echo "destructive migration(s) pending: $pending"
  missing=
  confirmed=${CONFIRM// /}
  for n in $(echo "$pending" | tr ',' ' '); do
    case ",$confirmed," in
      *",$n,"*) ;;
      *) missing="${missing:+$missing,}$n" ;;
    esac
  done
  if [ -n "$missing" ]; then
    echo "deploy: STOP. Not confirmed: $missing. Nothing changed." >&2
    echo "Read the migration. The backup is $backup_file. Then run again with:" >&2
    echo "  CONFIRM=$pending bash scripts/deploy.sh $target" >&2
    exit 3
  fi
  echo "confirmed by CONFIRM."
fi

# ---- 5 and 6. start and check ----
fail=
say "5. start $commit and wait for health (up to ${HEALTH_TIMEOUT}s)"
if ! DONEWHEN_IMAGE_TAG=$commit DONEWHEN_BACKUP_CONFIRMED=$CONFIRM dc up -d --no-build --remove-orphans; then
  fail="compose could not start the stack"
elif ! wait_healthy; then
  fail="no health answer from $HEALTH_URL in ${HEALTH_TIMEOUT}s"
fi
if [ -z "$fail" ]; then
  say "6. check the live commit and the landing page"
  got=$(health_commit)
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$LANDING_URL" || true)
  if [ "$got" != "$commit" ]; then
    fail="the live commit is '$got', not '$commit'"
  elif [ "$code" != 200 ]; then
    fail="the landing page $LANDING_URL returned '$code', not 200"
  fi
fi

# ---- 7. rollback ----
if [ -n "$fail" ]; then
  echo "deploy: FAILED: $fail" >&2
  dc logs --tail 20 donewhen >&2 || true
  if [ -z "$prev" ]; then
    echo "deploy: there is no previous image to start. The failed container still runs." >&2
    exit 4
  fi
  say "7. roll back to $prev"
  if start_image "$prev"; then
    migration_note
    echo "deploy: the old image $prev is live again and healthy on the current schema." >&2
    dkr image rm "$IMAGE_NAME:$commit" >/dev/null 2>&1 || true
    exit 1
  fi
  migration_note
  echo "deploy: the old image $prev does NOT start on the current schema. Restore the backup." >&2
  exit 4
fi

record_live "$commit"

# ---- keep the last images ----
n=0
for t in $(list_tags); do
  n=$((n + 1))
  if [ "$n" -gt "$KEEP_IMAGES" ] && [ "$t" != "$commit" ]; then
    dkr image rm "$IMAGE_NAME:$t" >/dev/null 2>&1 || echo "could not remove old image $t"
  fi
done
say "done: $commit is live ($(health_commit)); kept: $(list_tags | tr '\n' ' ')"
