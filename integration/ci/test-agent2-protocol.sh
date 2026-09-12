#!/usr/bin/env bash

set -euo pipefail

: "${AGENT_IMAGE:?AGENT_IMAGE must be set}"
: "${FAKE_DNF_HISTORY:?FAKE_DNF_HISTORY must be set}"

workspace=${GITHUB_WORKSPACE:-$PWD}

chmod +x integration/zabbix-agent2/fakebin/*
docker pull "$AGENT_IMAGE"

started=$SECONDS
output="$(docker run --rm \
  --user zabbix \
  --env FAKE_DNF_HISTORY \
  --volume "$workspace/zabbix-agent2-plugin-package-updates:/tmp/zabbix-agent2-plugin-package-updates:ro" \
  --volume "$workspace/integration/zabbix-agent2/package-updates.conf:/etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf:ro" \
  --volume "$workspace/integration/zabbix-agent2/fakebin:/tmp/fakebin:ro" \
  "$AGENT_IMAGE" \
  /bin/sh -c '
    PATH=/tmp/fakebin:$PATH zabbix_agent2 -t packages.get
    PATH=/tmp/fakebin:$PATH zabbix_agent2 -t advisories.get
  ')"
duration=$((SECONDS - started))

if ((duration >= 20)); then
  printf 'collection took %d seconds; release threshold is below 20 seconds\n' \
    "$duration" >&2
  exit 1
fi

printf '%s\n' "$output" | python3 integration/ci/assert-agent2-output.py
printf '%s\n' "$output"
printf 'collection completed in %d seconds\n' "$duration"
