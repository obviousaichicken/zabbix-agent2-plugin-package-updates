#!/usr/bin/env bash

set -euo pipefail

: "${BACKEND:?BACKEND must be set}"
: "${TEST_IMAGE:?TEST_IMAGE must be set}"

case "$BACKEND" in
  apt) item_key=packages.get ;;
  dnf) item_key=advisories.get ;;
  *)
    printf 'unsupported backend: %s\n' "$BACKEND" >&2
    exit 1
    ;;
esac

output="$(docker run --rm \
  --user zabbix \
  --entrypoint /usr/sbin/zabbix_agent2 \
  "$TEST_IMAGE" \
  -c /etc/zabbix/zabbix_agent2.conf \
  -t "$item_key")"

printf '%s\n' "$output" |
  python3 integration/ci/assert-installed-output.py "$BACKEND"
printf '%s\n' "$output"
