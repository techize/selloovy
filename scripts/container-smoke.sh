#!/bin/sh
# Run only an unconfigured preview image; never mount a merchant installation.
set -eu
image=${1:-selloovy:container-check}
container=''
cleanup() {
  if [ -n "$container" ]; then docker rm -f "$container" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT HUP INT TERM
user=$(docker image inspect --format '{{.Config.User}}' "$image")
[ "$user" = '65532:65532' ] || { echo 'Container must run as the non-root application user.' >&2; exit 1; }
container=$(docker run --platform linux/amd64 -d --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m --cap-drop ALL --security-opt no-new-privileges -p 127.0.0.1::8080 "$image")
port=$(docker port "$container" 8080/tcp | sed 's/.*://')
origin="http://127.0.0.1:$port"
tries=0
while ! curl --max-time 5 --fail --silent "$origin/health/live" >/dev/null; do
  tries=$((tries+1))
  [ "$tries" -lt 30 ] || { echo 'Container failed liveness startup.' >&2; exit 1; }
  sleep 1
done
[ "$(curl --max-time 5 --silent --output /dev/null --write-out '%{http_code}' "$origin/health/ready")" = '503' ] || { echo 'Unconfigured preview must not claim database readiness.' >&2; exit 1; }
curl --max-time 5 --fail --silent "$origin/" >/dev/null
assets=$(curl --max-time 5 --fail --silent "$origin/admin/" | tr '"' '\n' | sed -n '\|^/admin/assets/|p')
[ -n "$assets" ] || { echo 'Bundled admin assets are missing.' >&2; exit 1; }
for asset in $assets; do
  curl --max-time 5 --fail --silent "$origin$asset" >/dev/null
done
# Database tooling is packaged, and fails safely without configuration.
if docker run --platform linux/amd64 --rm --read-only --cap-drop ALL --security-opt no-new-privileges --entrypoint /app/migrate "$image" >/dev/null 2>&1; then
  echo 'Unconfigured migration unexpectedly succeeded.' >&2; exit 1
fi
printf '%s\n' 'Non-root read-only container startup, public/admin pages and unconfigured readiness boundary passed.'
