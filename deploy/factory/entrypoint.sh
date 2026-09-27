#!/bin/bash
# Starts the factory: GitHub access, the repositories, OpenCode's server, and
# the orchestrator host. Secrets arrive as files under /secrets, never in the
# image or on a command line.
set -euo pipefail

S=/secrets
W="${FACTORY_WORK:-/work}"
mkdir -p "$W/repos" "$W/runs"
cd "$W"
if [ -f "$S/gh-token" ]; then
  export GH_TOKEN="$(cat "$S/gh-token")"
  gh auth setup-git >/dev/null
fi
git config --global user.name "${FACTORY_GIT_NAME:-Raenil Factory}"
git config --global user.email "${FACTORY_GIT_EMAIL:-factory@raenil.local}"
git config --global init.defaultBranch main
git config --global --add safe.directory '*'

# FACTORY_REPOS: label=owner/repo,... cloned over HTTPS under $FACTORY_WORK/repos.
repos=""
IFS=',' read -ra entries <<< "${FACTORY_REPOS:-}"
for e in "${entries[@]}"; do
  name="${e%%=*}"; slug="${e#*=}"
  [ -z "$name" ] && continue
  dir="$W/repos/$name"
  if [ ! -d "$dir/.git" ]; then
    echo "factory: cloning $slug"
    git clone --quiet "https://github.com/$slug.git" "$dir"
  else
    git -C "$dir" fetch --quiet origin || true
  fi
  # A repository's own gitignored .env (test databases) comes from secrets.
  [ -f "$S/repo-env/$name.env" ] && cp "$S/repo-env/$name.env" "$dir/.env"
  repos="${repos:+$repos,}$name=$dir"
done
export ORCHESTRATOR_REPOS="$repos"

# OpenCode's server, for OpenCode agents.
if command -v opencode >/dev/null; then
  opencode serve --port "${OPENCODE_PORT:-4096}" --hostname 127.0.0.1 >"$W/opencode.log" 2>&1 &
  export OPENCODE_URL="http://127.0.0.1:${OPENCODE_PORT:-4096}"
fi

exec orchestrator host --name "${FACTORY_NAME:-pc-factory}" --run-root "$W/runs" "$@"
