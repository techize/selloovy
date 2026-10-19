#!/usr/bin/env bash
# Foreground development supervisor; secrets stay in ignored local files.
set -euo pipefail
umask 077
if [[ $# -ne 0 ]]; then echo 'Usage: bash scripts/dev-run.sh (Ctrl-C to stop app and worker)' >&2; exit 1; fi
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"
if [[ ! -f local/database.env || ! -f local/auth.key || ! -f local/postgres/PG_VERSION || ! -f local/postgres-password ]]; then
  echo 'Configure the dedicated database and installation key first; see docs/development.md. Existing credentials are never replaced by this runner.' >&2
  exit 1
fi
for tool in go; do
  command -v "$tool" >/dev/null || { echo "Missing required tool: $tool" >&2; exit 1; }
done
if [[ ! -f web/admin/dist/index.html ]]; then
  echo 'Build the admin first: npm --prefix web/admin run build' >&2
  exit 1
fi
bash scripts/dev-postgres.sh start
# These two operator-controlled files contain shell exports. Do not source files
# from a merchant upload, checkout or another untrusted location.
source local/database.env
if [[ -f local/square.env ]]; then source local/square.env; fi
export SELLOOVY_AUTH_KEY_FILE="${SELLOOVY_AUTH_KEY_FILE:-local/auth.key}"
export SELLOOVY_PUBLIC_ORIGIN="${SELLOOVY_PUBLIC_ORIGIN:-http://localhost:8080}"
export SELLOOVY_HTTP_ADDR="${SELLOOVY_HTTP_ADDR:-127.0.0.1:8080}"
export SELLOOVY_ADMIN_DIR="${SELLOOVY_ADMIN_DIR:-web/admin/dist}"
mkdir -p local/bin
go build -o local/bin/selloovy ./cmd/selloovy
go build -o local/bin/worker ./cmd/worker
app_pid=''
worker_pid=''
cleanup() {
  trap - EXIT INT TERM
  for child in "$app_pid" "$worker_pid"; do
    if [[ -n "$child" ]] && kill -0 "$child" 2>/dev/null; then kill -TERM "$child" 2>/dev/null || true; fi
  done
  if [[ -n "$app_pid" ]]; then wait "$app_pid" 2>/dev/null || true; fi
  if [[ -n "$worker_pid" ]]; then wait "$worker_pid" 2>/dev/null || true; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
local/bin/selloovy &
app_pid=$!
local/bin/worker &
worker_pid=$!
echo 'Local app and worker started. Use the configured origin (default http://localhost:8080/admin/). Ctrl-C stops both; PostgreSQL data and service are retained.'
# Bash 3 compatible: terminate the sibling if either application process exits.
# Migrations and email activation remain explicit operator steps.
while kill -0 "$app_pid" 2>/dev/null && kill -0 "$worker_pid" 2>/dev/null; do sleep 1; done
echo 'An app or worker process exited; stopping its sibling. Check the preceding redacted error and database schema readiness.' >&2
exit 1
