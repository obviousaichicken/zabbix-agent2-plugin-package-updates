# Development

[Main README](../README.md) · [Template and trigger reference](TEMPLATES.md)

## Prerequisites

Development uses:

* Go 1.27.0 or newer for builds, tests, and static analysis
* Docker Engine with Docker Compose v2 for template integration tests and the local lab
* A POSIX shell for installer and syntax checks

## Build from source

```bash
CGO_ENABLED=0 go build -o zabbix-agent2-plugin-package-updates ./cmd/agent
```

## Checks

Run the core checks locally with:

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
sh -n install.sh
sh -n integration/installer/dnf
shellcheck install.sh integration/installer/dnf
go test -tags=integration ./internal/apt/
```

The last of those needs a real `dpkg` on the machine. It covers the kernel
comparison behind APT reboot detection, which containers cannot exercise:
`/proc/sys/kernel/osrelease` reports the host kernel, so no kernel package
inside a container will ever match the running release.

Linting runs `golangci-lint`, configured in `.golangci.yml`. Build it with this
module's toolchain rather than installing a release binary: published binaries
are built with an older Go than the `go` directive in `go.mod`, and
`golangci-lint` refuses to analyse a module targeting a newer language version
than it was built with.

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
"$(go env GOPATH)/bin/golangci-lint" run ./...
```

Every linter named by a `//nolint` directive in this repository is enabled, and
`nolintlint` rejects a directive that suppresses nothing or that omits an
explanation. Add the waiver and the reason together, or fix the finding.

The integration workflow additionally runs the collectors and installed plugin inside every supported distribution image, exercises representative Agent 2 versions, and validates the release installer paths.

## Template tests

`go test ./...` includes the template's YAML structure, UUID baseline, active/passive parity, item references, macros, preprocessing contracts, and trigger expressions. The YAML parser is a test-only dependency.

Run the real-Zabbix tests with:

```bash
docker compose -p package-updates-template-tests \
  -f integration/templates/docker-compose.yaml up -d

go test -tags=integration -race ./templates -count=1 -timeout=10m -v

docker compose -p package-updates-template-tests \
  -f integration/templates/docker-compose.yaml down -v --remove-orphans
```

The suite waits for the API, imports the original YAML twice, and verifies all four templates. It then changes the master items on the disposable imported templates to trapper mode and enables short-term history for the raw/projection values. Zabbix does not allow changing the type of an inherited host item. All dependent-item preprocessing, discovery rules, prototypes, default macros, and trigger expressions come from the shipped template.

Go sends fixture JSON through Zabbix 7.0's `history.push` API. Zabbix executes the JavaScript and JSONPath itself. Tests poll actual item values, unsupported-item errors, discovered items, trigger severities, and problem/recovery states, including `nodata()`. They cover both active/passive template variants, invalid macros, incomplete metadata, deterministic discovery, quoted advisory IDs, deduplication, disappearance, and APT package details. Collection duration values act as per-fixture receipt markers so stale item values cannot satisfy a new step. The existing Agent 2 integration tests separately exercise the real collection transport.

The test environment is separate from the local lab and uses an ephemeral database. Its API defaults to `http://127.0.0.1:17070/api_jsonrpc.php` with the disposable Zabbix credentials `Admin` / `zabbix`. To change the port, set `ZBX_TEST_PORT` for Compose and the corresponding `ZBX_TEST_API_URL` for Go. Point these tests only at the disposable test environment: they modify imported templates and create/delete fixture hosts.

For failure diagnostics:

```bash
docker compose -p package-updates-template-tests \
  -f integration/templates/docker-compose.yaml logs --no-color
```

## Local Zabbix lab

Start the complete local environment with:

```bash
docker compose \
  --env-file integration/lab/.env \
  -f integration/lab/docker-compose.yaml \
  up --build
```

The lab builds a representative DNF set, including Oracle Linux, plus Debian and Ubuntu APT hosts. It installs the plugin, starts passive and active Agent 2 hosts, imports all four templates, and queues an initial collection after the Zabbix configuration cache has synchronized.

Open <http://localhost:7070> and sign in with `Admin` / `zabbix`.

The broader DNF and APT distribution matrix runs in CI. See [compatibility.yaml](../.github/workflows/compatibility.yaml) for the exact images and Zabbix Agent 2 versions.

## Project layout

|Path|Purpose|
|----|-------|
|`cmd/agent/`|External Agent 2 plugin entry point, lifecycle, backend selection, and public item keys|
|`internal/dnf/`|DNF package, advisory, history, and reboot collection|
|`internal/apt/`|APT package, repository, history, metadata, and reboot collection|
|`internal/command/`|Shared command execution and failure handling|
|`internal/logging/`|Zabbix logging adapter|
|`internal/packageinfo/`|Backend-neutral package update types and capabilities|
|`internal/results/`|Public JSON payload construction and validation|
|`templates/`|Passive and active DNF/APT Zabbix templates, Go contract tests, and Zabbix-backed fixture tests|
|`configs/`|Installable plugin configuration|
|`docs/`|Installation, troubleshooting, development, and template documentation|
|`integration/`|DNF and APT smoke tests|
|`integration/lab/`|Local multi-distribution Zabbix lab and Dockerfiles|
|`integration/templates/`|Disposable Zabbix server, web/API, and database for template fixture tests|
|`integration/installer/`|Installer test fixtures|
|`integration/zabbix-agent2/`|Agent 2 compatibility-test configuration and command fixtures|
|`.github/workflows/`|Unit, integration, release, and smoke-test automation|

## Package-manager commands

### DNF

The DNF backend uses the host's enabled repositories and their configured URLs.

```bash
# List enabled repositories
dnf --assumeno -q repolist

# Query the latest available updates, optionally by advisory type
dnf --assumeno -q '--setopt=*.skip_if_unavailable=False' repoquery --upgrades [--security|--bugfix|--enhancement] --latest-limit=1 --queryformat '%{name}|%{epoch}|%{version}|%{release}|%{arch}|%{repoid}\n'

# Detect the installed DNF version
dnf --assumeno --version

# DNF4: list applicable security advisory/package relationships
dnf --assumeno -q '--setopt=*.skip_if_unavailable=False' updateinfo list --updates --security

# DNF5: list applicable relationships and read bulk detail as JSON
dnf --assumeno -q '--setopt=*.skip_if_unavailable=False' advisory list --updates --security --json
dnf --assumeno -q '--setopt=*.skip_if_unavailable=False' advisory info --updates --security --json

# List DNF transactions
dnf --assumeno -q history list [--json]

# Inspect one DNF transaction
dnf --assumeno -q history info TRANSACTION_ID [--json]

# Read package installation times for reboot detection
rpm -qa --qf '%{NAME}|%{INSTALLTIME}\n'

# List installed kernels for reboot detection
rpm -qa --qf '%{NAME}|%{VERSION}-%{RELEASE}.%{ARCH}\n' 'kernel*'

# Read the running kernel version
uname -r
```

The update query runs once without an advisory flag and once for each listed classification. Advisory collection is separate and fixed at one version probe plus one DNF4 command or two DNF5 commands. JSON history output is used when supported by DNF5.

### APT

The APT backend runs only read-only queries against the installed-package database and existing local indexes:

```bash
# Enumerate downloaded binary package indexes and repository metadata
apt-get indextargets

# Snapshot installed packages and their exact architecture/version identity
dpkg-query --show '--showformat=${binary:Package}|${Architecture}|${Version}|${db:Status-Status}\n'

# Resolve installed/candidate versions, priorities, phasing, and exact sources
apt-cache policy package:architecture ...

# Compare Debian versions
dpkg --compare-versions candidate gt installed
```

Policy queries are split by package count and command-line size. The backend also reads `/run/reboot-required`, retained APT history logs, and package-index modification times. It never runs interactive `apt`, `apt-get update`, package downloads, simulations, or installation commands.
