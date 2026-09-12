#!/usr/bin/env bash

set -euo pipefail

project=package-updates-template-tests
compose_file=integration/templates/docker-compose.yaml

cleanup() {
  status=$?
  trap - EXIT

  if ((status != 0)); then
    docker compose \
      -p "$project" \
      -f "$compose_file" \
      logs --no-color || true
  fi

  if ! docker compose \
    -p "$project" \
    -f "$compose_file" \
    down -v --remove-orphans; then
    if ((status == 0)); then
      status=1
    fi
  fi

  exit "$status"
}
trap cleanup EXIT

docker compose \
  -p "$project" \
  -f "$compose_file" \
  up -d

go test -tags=integration -race ./templates \
  -run '^TestZabbixTemplateIntegration$' \
  -count=1 \
  -timeout=10m \
  -v
