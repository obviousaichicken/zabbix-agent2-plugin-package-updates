#!/usr/bin/env bash

set -euo pipefail

: "${FOREIGN_ARCHITECTURE:?FOREIGN_ARCHITECTURE must be set}"

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends passwd util-linux

# apt-cache policy qualifies foreign architectures but not the native one.
# Installing a foreign package ensures every host exercises both forms.
dpkg --add-architecture "$FOREIGN_ARCHITECTURE"
apt-get update
apt-get install -y --no-install-recommends "libgcc-s1:${FOREIGN_ARCHITECTURE}"

foreign="$(dpkg-query --show --showformat='${binary:Package} ${Architecture}\n' |
  grep -c " ${FOREIGN_ARCHITECTURE}$" || true)"
if ((foreign == 0)); then
  printf 'no %s package is installed\n' "$FOREIGN_ARCHITECTURE" >&2
  exit 1
fi
printf 'multi-arch verified: %d %s packages installed\n' \
  "$foreign" "$FOREIGN_ARCHITECTURE"

command -v apt-get
command -v apt-cache
command -v dpkg-query
command -v dpkg

if ! id zabbix >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin zabbix
fi

runuser -u zabbix -- \
  ./integration.test -test.v -test.run '^TestAPTSmoke$'
runuser -u zabbix -- \
  ./apt-dpkg.test -test.v -test.run '^TestRebootPendingKernelComparisonAgainstRealDpkg$'
