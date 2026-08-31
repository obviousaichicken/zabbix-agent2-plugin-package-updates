# Development

[Main README](README.md) · [Template and trigger reference](TEMPLATES.md)

## Prerequisites

Development uses:

* Go 1.27.0 or newer for builds, tests, and static analysis
* Docker Engine with Docker Compose v2 for the local integration lab
* Ruby for template validation
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
ruby .github/ci/validate_templates.rb
sh -n install.sh
sh -n .dev/installer-test/dnf
```

The integration workflow additionally runs the collectors and installed plugin inside every supported distribution image, exercises representative Agent 2 versions, and validates the release installer paths.

## Local Zabbix lab

Start the complete local environment with:

```bash
docker compose \
  --env-file .dev/.env \
  -f .dev/docker-compose.yaml \
  up --build
```

The lab builds a representative DNF set, including Oracle Linux, plus Debian and Ubuntu APT hosts. It installs the plugin, starts passive and active Agent 2 hosts, imports all four templates, and queues an initial collection after the Zabbix configuration cache has synchronized.

Open <http://localhost:7070> and sign in with `Admin` / `zabbix`.

The broader DNF and APT distribution matrix runs in CI. See [package-updates-integration.yaml](.github/workflows/package-updates-integration.yaml) for the exact images and Zabbix Agent 2 versions.

## Project layout

|Path|Purpose|
|----|-------|
|`cmd/agent/`|External Agent 2 plugin entry point, lifecycle, backend selection, and public item keys|
|`internal/dnf/`|DNF package, advisory, history, and reboot collection|
|`internal/apt/`|APT package, repository, history, metadata, and reboot collection|
|`internal/packageinfo/`|Backend-neutral package update types and capabilities|
|`internal/results/`|Public JSON payload construction and validation|
|`template-package-updates-by-zabbix-agent2.yaml`|Passive and active DNF/APT Zabbix templates|
|`.dev/`|Local multi-distribution Zabbix lab and installer fixtures|
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
