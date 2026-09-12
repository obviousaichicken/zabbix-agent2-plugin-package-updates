#!/usr/bin/env bash

set -euo pipefail

dnf -q makecache
command -v dnf
command -v rpm
command -v uname
./integration.test -test.v -test.run '^TestDNFSmoke$'
