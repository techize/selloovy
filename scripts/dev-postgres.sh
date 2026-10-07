#!/usr/bin/env bash
set -euo pipefail
umask 077

# Uses only a dedicated, ignored directory; never starts a system-wide service.
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
dev_root="$repo_root/local"
dev_data="$dev_root/postgres"
dev_port=55432
mkdir -p "$dev_root"

case "${1:-}" in
  start)
    for tool in initdb pg_ctl psql createdb openssl; do
      command -v "$tool" >/dev/null || { echo "Missing required local tool: $tool" >&2; exit 1; }
    done
    if [[ ! -f "$dev_data/PG_VERSION" ]]; then
      openssl rand -hex 24 > "$dev_root/postgres-password"
      initdb -D "$dev_data" -U selloovy --auth-local=scram-sha-256 --auth-host=scram-sha-256 --pwfile="$dev_root/postgres-password" > "$dev_root/postgres-init.log" 2>&1
    fi
    [[ -f "$dev_root/postgres-password" ]] || { echo 'Local password file is missing; refusing to replace it.' >&2; exit 1; }
    if ! pg_ctl -D "$dev_data" status >/dev/null 2>&1; then
      pg_ctl -D "$dev_data" -l "$dev_root/postgres.log" -o "-h 127.0.0.1 -p $dev_port -c unix_socket_directories=''" -w start > "$dev_root/postgres-start.log" 2>&1
    fi
    dev_password="$(cat "$dev_root/postgres-password")"
    export PGPASSWORD="$dev_password"
    for dev_database in selloovy_development selloovy_test; do
      present="$(psql -X -h 127.0.0.1 -p "$dev_port" -U selloovy -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$dev_database'")"
      if [[ "$present" != 1 ]]; then
        createdb -h 127.0.0.1 -p "$dev_port" -U selloovy "$dev_database"
      fi
    done
    printf "export SELLOOVY_DATABASE_URL='postgres://selloovy:%s@127.0.0.1:%s/selloovy_development?sslmode=disable'\nexport SELLOOVY_TEST_DATABASE_URL='postgres://selloovy:%s@127.0.0.1:%s/selloovy_test?sslmode=disable'\n" "$dev_password" "$dev_port" "$dev_password" "$dev_port" > "$dev_root/database.env"
    echo 'Dedicated PostgreSQL is running on loopback port 55432. Source local/database.env to configure this shell.'
    ;;
  stop)
    pg_ctl -D "$dev_data" -m fast -w stop > "$dev_root/postgres-stop.log" 2>&1
    echo 'Dedicated PostgreSQL stopped; data and configuration retained.'
    ;;
  status)
    pg_ctl -D "$dev_data" status
    ;;
  *)
    echo 'Usage: bash scripts/dev-postgres.sh start|stop|status' >&2
    exit 1
    ;;
esac
