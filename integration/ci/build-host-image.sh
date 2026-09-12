#!/usr/bin/env bash

set -euo pipefail

: "${BUILD_ARGS:?BUILD_ARGS must be set}"
: "${DOCKERFILE:?DOCKERFILE must be set}"
: "${TEST_IMAGE:?TEST_IMAGE must be set}"

build_args=()
for argument in $BUILD_ARGS; do
  build_args+=(--build-arg "$argument")
done

docker build \
  --file "$DOCKERFILE" \
  "${build_args[@]}" \
  --tag "$TEST_IMAGE" \
  .
