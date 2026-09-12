#!/bin/sh
set -eu

docker_bin=/usr/bin/docker
container_name=account-manager-159
app_path=/opt/account-manager/app.py

probe=$(
  "$docker_bin" exec "$container_name" \
    python3 "$app_path" \
    --dry-run-auto-refresh \
    --weekly-only \
    --limit 0
)

case "$probe" in
  *'"status":"dry_run"'*'"mode":"weekly_only"'*)
    ;;
  *'"status":"skipped_busy"'*)
    echo "$probe"
    exit 0
    ;;
  *)
    echo "weekly refresh blocked: container does not prove weekly-only support" >&2
    exit 78
    ;;
esac

exec "$docker_bin" exec "$container_name" \
  python3 "$app_path" \
  --run-auto-refresh \
  --weekly-only \
  --limit 15
