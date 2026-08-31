[![Checks](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/checks.yaml/badge.svg)](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/checks.yaml)
[![Package Updates Integration](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/package-updates-integration.yaml/badge.svg)](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/package-updates-integration.yaml)
[![Release](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/release.yaml/badge.svg)](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/release.yaml)

# Package Updates for Zabbix Agent 2

A loadable `zabbix-agent2` plugin for monitoring package updates on DNF and APT systems.

It reports:

* Pending updates, grouped by repository and update type
* Security advisories and CVEs on DNF hosts
* Reboot status and the result of the last package transaction
* The age of local package metadata on APT hosts

Use `packages.get` on either backend. The older `dnf.get` key remains available for existing DNF templates, and `dnf.advisories.get` collects DNF advisory details on its own schedule.

<img width="1905" height="1016" alt="Screenshot From 2026-08-21 20-38-47" src="https://github.com/user-attachments/assets/ea31ef4a-861f-4865-8748-1dcf3f474762" />

<img width="2106" height="634" alt="Screenshot From 2026-08-21 20-39-21" src="https://github.com/user-attachments/assets/cf966977-d3b7-49e6-9b3b-d984cd8b0a97" />

<img width="2202" height="520" alt="Screenshot From 2026-08-21 20-39-53" src="https://github.com/user-attachments/assets/51f4dcac-2fab-4973-816d-c9e225839fc3" />

<img width="2199" height="290" alt="Screenshot From 2026-08-21 20-39-37" src="https://github.com/user-attachments/assets/d891dfe2-7b07-482c-8279-ed0770003751" />

## Installation

The plugin is compatible with `zabbix-agent2` 7.0, 7.2, and 7.4 on the distributions in the [support matrix](#compatibility).

### Automated installation

```bash
curl -fLO https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/latest/download/install.sh && sudo sh install.sh
```

The installer detects DNF or APT, verifies the downloaded binary, checks package-manager access as the `zabbix` user, validates the Agent 2 configuration, and restarts the service. On APT systems, package indexes must already exist; the installer does not run `apt-get update`.

### Manual installation

```bash
# Download the latest binary and its SHA-256 checksum from GitHub Releases:
curl -fL -o zabbix-agent2-plugin-package-updates https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/latest/download/zabbix-agent2-plugin-package-updates

# Download the checksum
curl -fL -o zabbix-agent2-plugin-package-updates.sha256 https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/latest/download/zabbix-agent2-plugin-package-updates.sha256

# Verify binary
sha256sum --check zabbix-agent2-plugin-package-updates.sha256

# Install the binary
sudo install -D -m 0755 zabbix-agent2-plugin-package-updates /usr/sbin/zabbix-agent2-plugin/zabbix-agent2-plugin-package-updates

# Create config file
sudo sh -c 'cat > /etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf' <<'EOF'
Plugins.PackageUpdates.System.Path=/usr/sbin/zabbix-agent2-plugin/zabbix-agent2-plugin-package-updates
Plugins.PackageUpdates.System.Capacity=1
PluginTimeout=30
EOF

# Set config file rights
sudo chmod 0644 /etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf

# On systems with SELinux, apply the default contexts for the installation paths
sudo restorecon -Rv /usr/sbin/zabbix-agent2-plugin /etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf

# Confirm that the zabbix user can query DNF on a DNF host
sudo -u zabbix dnf -q repolist
sudo -u zabbix dnf -q repoquery --upgrades

# Or confirm read-only APT access on a Debian/Ubuntu host. Populate indexes as
# root first if this host has never run apt-get update.
sudo -u zabbix apt-get indextargets
sudo -u zabbix dpkg-query --show
sudo -u zabbix apt-cache policy dpkg:$(dpkg --print-architecture)

# Restart the agent and check that it started correctly
sudo systemctl restart zabbix-agent2
systemctl status zabbix-agent2
```

Backend detection is automatic. Most installations do not need anything beyond the configuration shown above.

### Troubleshooting

```bash
# Test the neutral item on either backend
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t packages.get

# Test the legacy DNF item on a DNF host
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t dnf.get

# Test the independently scheduled advisory item on a DNF host
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t dnf.advisories.get

# Run the collector directly on a DNF host
sudo /usr/sbin/zabbix-agent2-plugin/zabbix-agent2-plugin-package-updates --test

# Check for SELinux policy denials
sudo ausearch -m AVC -ts recent
```

## Zabbix templates

Download `template-package-updates-by-zabbix-agent2.yaml` from the GitHub release. It contains four templates:

|Host|Passive checks|Active checks|
|----|--------------|-------------|
|DNF|DNF by Zabbix agent 2|DNF by Zabbix agent 2 active|
|APT|APT by Zabbix agent 2|APT by Zabbix agent 2 active|

Import the file from **Data collection > Templates > Import**, then link one template to each host. Repository discovery and alerts are included. Per-advisory discovery on DNF is disabled by default.

### DNF template

**Macros**

|Name|Description|Default|
|----|-----------|-------|
|{$DNF.ADVISORY.NODATA.TIME}|Time without a successful advisory collection before an availability problem is raised.|`2h`|
|{$DNF.ADVISORY.LLD.CRITICAL}|Enable per-advisory discovery for Critical advisories. Must be `0` or `1`.|`0`|
|{$DNF.ADVISORY.LLD.IMPORTANT}|Enable per-advisory discovery for Important advisories. Must be `0` or `1`.|`0`|
|{$DNF.ADVISORY.LLD.MODERATE}|Enable per-advisory discovery for Moderate advisories. Must be `0` or `1`.|`0`|
|{$DNF.ADVISORY.LLD.LOW}|Enable per-advisory discovery for Low advisories. Must be `0` or `1`.|`0`|
|{$DNF.ADVISORY.LLD.UNKNOWN}|Enable per-advisory discovery for Unknown-severity advisories. Must be `0` or `1`.|`0`|
|{$DNF.ADVISORY.UPDATE.INTERVAL}|Interval between independent DNF advisory collections.|`1h`|
|{$DNF.COLLECTION.DURATION.MAX}|Maximum acceptable average collection duration, in seconds.|`20`|
|{$DNF.COLLECTION.DURATION.WINDOW}|Evaluation window for the average collection duration.|`30m`|
|{$DNF.NODATA.TIME}|Time without a successful collection before an availability problem is raised.|`30m`|
|{$DNF.SECURITY.ADVISORY.MAX.AGE}|Maximum acceptable age of the oldest known vendor timestamp for an applicable advisory.|`7d`|
|{$DNF.SECURITY.CRITICAL.MIN}|Minimum number of Critical advisories that raises a Disaster problem.|`1`|
|{$DNF.SECURITY.IMPORTANT.MIN}|Minimum number of Important advisories that raises a High problem when no Critical threshold is met.|`1`|
|{$DNF.SECURITY.MIN}|Minimum number of security updates that raises a problem.|`1`|
|{$DNF.SECURITY.UNKNOWN.MAX}|Maximum accepted number of advisories or affected package updates with Unknown severity.|`0`|
|{$DNF.UPDATE.INTERVAL}|Interval between complete DNF collections.|`15m`|

**Items**

|Name|Description|Type|Key and additional info|
|----|-----------|----|-----------------------|
|DNF: Get update data|Collects package, repository, advisory, reboot, and update-history data. Dependent items extract the values, so the raw JSON is not stored.|Zabbix agent|dnf.get|
|DNF: Get advisory data|Collects applicable DNF security advisories independently from package-update monitoring.|Zabbix agent|dnf.advisories.get, interval `{$DNF.ADVISORY.UPDATE.INTERVAL}`, history disabled, 30-second timeout|
|DNF: Advisory discovery data|Validates completeness and the five selection macros, then projects compact records for discovery. It becomes unsupported on invalid or incomplete input instead of publishing a false empty list.|Dependent item|dnf.advisory.discovery.data, history disabled|
|DNF: Collection complete|Indicates whether the plugin completed every required DNF query. Failed collections make the master item unsupported and are also detected by the no-data condition.|Dependent item|dnf.collection.complete|
|DNF: Collection duration|Time spent collecting the complete DNF payload, in seconds.|Dependent item|dnf.collection.duration|
|DNF: Advisory classification complete|Indicates that all advisory categories were classified successfully. Strict classification failures make the master collection fail instead of publishing partial category counts.|Dependent item|dnf.classification.complete|
|DNF: Updates pending total|Total number of available package updates across all enabled repositories.|Dependent item|dnf.updates|
|DNF: Enabled repositories|Number of enabled DNF repositories included in the collection.|Dependent item|dnf.repositories|
|DNF: Updates pending|Indicates whether one or more package updates are available.|Dependent item|dnf.updates.pending|
|DNF: Reboot pending|Indicates whether installed reboot-sensitive packages or a newer installed kernel require the host to be rebooted.|Dependent item|dnf.reboot.pending|
|DNF: Updates pending security|Number of available package updates classified by repository advisory metadata as security updates.|Dependent item|dnf.updates.security|
|DNF: Updates pending bugfix|Number of available package updates classified by repository advisory metadata as bug fixes.|Dependent item|dnf.updates.bugfix|
|DNF: Updates pending enhancement|Number of available package updates classified by repository advisory metadata as enhancements.|Dependent item|dnf.updates.enhancement|
|DNF: Updates pending other|Number of available package updates with no supported advisory category. This can include missing or unrecognized advisory metadata and does not necessarily mean non-security updates.|Dependent item|dnf.updates.other|
|DNF: Last update result|Result of the most recent completed DNF transaction that upgraded a package: 0 means not recorded, 1 means success, and 2 means failed.|Dependent item|dnf.last_update.result|
|DNF: Last update time|Unix timestamp of the most recent completed DNF transaction that upgraded a package. No value is stored when no such transaction exists.|Dependent item|dnf.last_update.timestamp|
|DNF: Advisory collection health|Independent completion and duration telemetry.|Dependent items|dnf.advisory.collection.complete, dnf.advisory.collection.duration|
|DNF: Advisory metadata completeness|Reports whether detail, CVE, and true issue-date metadata are complete.|Dependent items|dnf.advisory.details.complete, dnf.advisory.cves.complete, dnf.advisory.issue_dates.complete|
|DNF: Advisory and CVE totals|Counts unique advisory IDs and known CVE IDs.|Dependent items|dnf.advisory.total, dnf.advisory.cves|
|DNF: Advisories by severity|Counts Critical, Important, Moderate, Low, and Unknown advisory IDs.|Dependent items|dnf.advisory.critical, dnf.advisory.important, dnf.advisory.moderate, dnf.advisory.low, dnf.advisory.unknown|
|DNF: Affected updates by severity|Counts unique pending package updates once at their highest linked advisory severity.|Dependent items|dnf.advisory.packages.critical, dnf.advisory.packages.important, dnf.advisory.packages.moderate, dnf.advisory.packages.low, dnf.advisory.packages.unknown|
|DNF: Oldest advisory vendor time|Reports the oldest preferred vendor timestamp, its age, and whether its basis is `issued`, `updated`, or `none`.|Dependent items|dnf.advisory.oldest.timestamp, dnf.advisory.oldest.age, dnf.advisory.oldest.basis|

**Triggers**

|Name|Description|Expression|Severity|Dependencies and additional info|
|----|-----------|----------|--------|--------------------------------|
|DNF: Collection is unavailable|DNF update data is stale or unavailable until collection succeeds.|`last(/DNF by Zabbix agent 2/dnf.collection.complete)=0 or nodata(/DNF by Zabbix agent 2/dnf.collection.complete,{$DNF.NODATA.TIME})=1`|High||
|DNF: Collection is slow|The average collection duration is approaching the hard plugin and item timeout. Check repository availability and DNF performance.|`avg(/DNF by Zabbix agent 2/dnf.collection.duration,{$DNF.COLLECTION.DURATION.WINDOW})>{$DNF.COLLECTION.DURATION.MAX}`|Warning||
|DNF: Reboot is required|A reboot is required to complete package updates.|`last(/DNF by Zabbix agent 2/dnf.reboot.pending)=1`|Warning||
|DNF: Security updates are available|One or more security updates are available.|`last(/DNF by Zabbix agent 2/dnf.updates.security)>={$DNF.SECURITY.MIN}`|High||
|DNF: Last package update failed|The most recent package update transaction did not complete successfully.|`last(/DNF by Zabbix agent 2/dnf.last_update.result)=2`|Warning||
|DNF: Advisory collection is unavailable|The independent advisory item is stale or unsupported.|`last(.../dnf.advisory.collection.complete)=0 or nodata(...,{$DNF.ADVISORY.NODATA.TIME})=1`|High||
|DNF: Critical security advisories are applicable|The Critical advisory threshold is met.|`last(.../dnf.advisory.critical)>={$DNF.SECURITY.CRITICAL.MIN}`|Disaster||
|DNF: Important security advisories are applicable|The Important threshold is met while the Critical threshold is not.|`last(.../dnf.advisory.critical)<{$DNF.SECURITY.CRITICAL.MIN} and last(.../dnf.advisory.important)>={$DNF.SECURITY.IMPORTANT.MIN}`|High|Mutually exclusive with the Critical problem.|
|DNF: Applicable security advisory is old|The oldest known vendor timestamp exceeds the configured age.|`last(.../dnf.advisory.oldest.age)>{$DNF.SECURITY.ADVISORY.MAX.AGE}`|Warning||
|DNF: Security advisory severity is unknown|Unknown advisory or affected-package severity exceeds the configured maximum.|Checks both `dnf.advisory.unknown` and `dnf.advisory.packages.unknown`.|High||
|DNF: Advisory metadata is incomplete|Detail, CVE, or true issue-date metadata is incomplete.|Checks all three advisory completeness items.|Warning|Expected on DNF4, which uses list-only collection.|
|DNF: Security package updates lack advisory objects|The package classifier reports more security updates than the advisory collector can link.|Compares `dnf.updates.security` with the sum of all five advisory package-severity counts.|Warning||

**LLD rule DNF: Repository discovery**

|Name|Description|Type|Key and additional info|
|----|-----------|----|-----------------------|
|DNF: Repository discovery|Discovers enabled DNF repositories. Items for repositories that disappear are disabled immediately and retained for 30 days before deletion.|Dependent item|dnf.repository|

**Item prototypes for DNF: Repository discovery**

|Name|Description|Type|Key and additional info|
|----|-----------|----|-----------------------|
|DNF: Repository [{#REPO_NAME}]: Available update count|Number of available package updates from repository {#REPO_NAME} ({#REPO_ID}).|Dependent item|dnf.repository.updates["{#REPO_ID}"]|
|DNF: Repository [{#REPO_NAME}]: Pending package details|Comma-separated NEVRA identifiers for packages with available updates from repository {#REPO_NAME} ({#REPO_ID}). The value is blank when there are no pending packages.|Dependent item|dnf.repository.update.details["{#REPO_ID}"]|

#### Per-advisory discovery

All five severity macros default to `0`, so no advisory items or triggers are discovered unless you enable them. The host-level counts, age checks, and alerts still work with discovery disabled.

Discovery requires complete detail, CVE, and issue-date data, which normally means DNF5. On DNF4 or an incomplete DNF5 response, discovery becomes unsupported instead of returning an empty list and clearing existing problems. When an advisory disappears from a later valid response, its presence item changes to `0`. Zabbix disables the discovered objects after one day and deletes them after 30 days.

Each selected advisory creates five item instances and one trigger instance:

|Prototype|Value|Key|
|---------|-----|---|
|Presence|`1` while applicable, then `0` on a valid disappearance.|dnf.advisory.presence[{#ADVISORY_SAFE_ID}]|
|Vendor timestamp|Unix issue timestamp from the complete vendor record.|dnf.advisory.vendor.timestamp[{#ADVISORY_SAFE_ID}]|
|Affected package count|Number of unique pending package NEVRAs.|dnf.advisory.packages.count[{#ADVISORY_SAFE_ID}]|
|Affected package list|Sorted JSON array of pending package NEVRAs.|dnf.advisory.packages.list[{#ADVISORY_SAFE_ID}]|
|CVE list|Sorted JSON array of known CVE IDs.|dnf.advisory.cves.list[{#ADVISORY_SAFE_ID}]|

Trigger severity follows advisory severity: Disaster for Critical, High for Important, Warning for Moderate, Information for Low, and High for Unknown. Advisory IDs remain visible in names and tags. Prototype keys use a lowercase hexadecimal encoding so quotes and backslashes cannot break their syntax. IDs longer than 256 UTF-16 code units are rejected.

Each matching advisory adds five items and one trigger. Start with Critical only, or Critical and Important, and watch the number of discovered objects. Advisory payloads are limited to 8 MiB.

To enable discovery on a DNF5 host:

1. Confirm that `dnf.advisories.get` is supported and that `dnf.advisory.collection.complete`, `dnf.advisory.details.complete`, `dnf.advisory.cves.complete`, and `dnf.advisory.issue_dates.complete` all report `1`.
2. Change the host or template macro for each severity you want from `0` to `1`; for example, set `{$DNF.ADVISORY.LLD.CRITICAL}` to `1`.
3. Run or wait for `DNF: Advisory discovery`, then check discovered-object counts before enabling more severities.

To turn discovery off, restore all five macros to `0`. Presence items return to zero, then Zabbix disables the discovered objects after one day and removes them after 30 days.

### APT template

The APT template uses `packages.get` as its master item. It monitors the common collection, repository, update, reboot, and best-effort history fields plus the age of the oldest local binary package index used in the collection.

**Macros**

|Name|Description|Default|
|----|-----------|-------|
|{$APT.COLLECTION.DURATION.MAX}|Maximum acceptable average collection duration, in seconds.|`20`|
|{$APT.METADATA.AGE.MAX}|Maximum acceptable age of the oldest local APT binary index.|`2d`|
|{$APT.NODATA.TIME}|Time without a successful collection before an availability problem is raised.|`30m`|
|{$APT.SECURITY.MIN}|Minimum number of positively identified security updates that raises a problem.|`1`|
|{$APT.UPDATE.INTERVAL}|Interval between complete APT collections.|`15m`|

**Items and discovery**

|Purpose|Keys|
|-------|----|
|Collection health|`apt.collection.complete`, `apt.collection.duration`|
|Repository and update totals|`apt.repositories`, `apt.updates`, `apt.updates.pending`|
|Supported classifications|`apt.updates.security`, `apt.updates.other`|
|Host state and history|`apt.reboot.pending`, `apt.last_update.timestamp`, `apt.last_update.result`|
|Cached-index freshness|`apt.metadata.refreshed`, `apt.metadata.age`|
|Repository discovery|`apt.repository`, with per-repository count and package-detail prototypes|

Pending package details use the Debian identifier `name:architecture=full-version`. The APT template has no bugfix or enhancement items because the collector cannot reliably make those classifications. It also has no failed-history trigger because retained APT history is best effort.

The APT alerts cover unavailable or slow collection, pending security updates, a required reboot, and stale package indexes. An update classified as `other` was not positively identified as coming from a trusted official Debian, Ubuntu, or Ubuntu ESM security pocket; `other` is not proof that the update has no security impact.

## Compatibility

All listed distributions are supported on Linux x86_64 and exercised in CI:

|Backend|Distribution versions|
|-------|---------------------|
|DNF|RHEL/UBI 8, 9, and 10; Fedora 43 and 44; Rocky Linux 9 and 10; AlmaLinux 9 and 10; CentOS Stream 9 and 10|
|APT|Debian 12 and 13; Ubuntu 22.04, 24.04, and 26.04|

One plugin binary supports the released `zabbix-agent2` versions in the 7.0, 7.2, and 7.4 branches.

DNF reboot status is determined from reboot-sensitive RPM install times and installed kernel packages compared with the running kernel. The plugin supports DNF4 and DNF5 without depending on an optional DNF reboot-detection plugin. APT reboot status follows `/run/reboot-required`.

### How DNF advisories work

`dnf.advisories.get` is separate from `dnf.get` and `packages.get`, so package snapshots can run every 15 minutes while the more expensive advisory check runs hourly. Advisory output is limited to 8 MiB. The collector does not run a command for every advisory:

* DNF4 runs `updateinfo list --updates --security` after checking the DNF version. Fetching full details can exceed Agent 2's 30-second deadline on some releases, so DNF4 reports IDs, severities, and affected packages but not titles, CVEs, or dates.
* DNF5 runs one JSON list command and one JSON info command. Only advisories and packages from the applicable-update list are included in the result.

The schema-version-1 summary uses these counting rules:

* `summary.advisories` and `advisories_by_severity` count unique advisory IDs.
* `package_updates_by_severity` counts unique pending NEVRAs. When several advisories affect one update, that package is counted once at the highest severity: Critical, Important, Moderate, Low, then Unknown.
* `summary.unique_cves` is deduplicated. Trust a zero value only when `metadata.cves_complete` is true.
* Unrecognized vendor severity values are reported as `unknown`, not Low.
* Issue and update timestamps are normalized to UTC. A true issue date is preferred; otherwise an available Updated timestamp is used and the summary basis is `updated`. Missing timestamps remain `null`, and future timestamps produce age zero.

DNF5 prefers structured CVE references. Some vendor JSON puts CVE IDs in Bugzilla records, so the collector also recognizes strict `CVE-YYYY-NNNN...` tokens. Descriptions are not kept. Command output and the final advisory payload return an error instead of being truncated at 8 MiB.

A compact DNF4 result therefore looks like:

```json
{
  "metadata": {
    "details_complete": false,
    "cves_complete": false,
    "issue_dates_complete": false
  },
  "summary": {
    "advisories": 1,
    "unique_cves": 0,
    "oldest_vendor_timestamp": null,
    "oldest_vendor_timestamp_basis": "none"
  },
  "advisories": [{
    "id": "RLSA-2026:22314",
    "severity": "moderate",
    "cve_ids": [],
    "affected_update_nevras": ["openssl-libs-1:3.5.5-3.el10_2.x86_64"]
  }]
}
```

DNF5 returns the same fields, normally with complete metadata, plus `title`, `issued_at`, known CVEs, and applicable binary NEVRAs. Arrays are always JSON arrays, never `null`.

### Backend selection

The backend defaults to `auto`. The plugin reads `/etc/os-release`: Debian and Ubuntu families use APT, while Fedora and RHEL families use DNF. Startup fails if the distribution is unsupported or the required commands are missing. The installer also checks the distribution version against the support matrix above.

You can force a backend when testing a controlled image:

```ini
Plugins.PackageUpdates.Backend=dnf
# or
Plugins.PackageUpdates.Backend=apt
```

Valid values are `auto`, `dnf`, and `apt`. A forced backend skips distribution-family detection but still checks the required commands. Leave this setting out for normal installations.

### APT metadata

APT collection is read-only. It uses the installed-package database and local package indexes; it does not contact mirrors, download packages, take package-manager locks, or run `apt-get update`. A successful check means the local metadata was readable, not that a mirror is reachable.

The payload reports the oldest participating binary index as `metadata.refreshed_at` and its age as `metadata.age_seconds`. Missing or unreadable indexes fail the check. Old but readable indexes remain valid so the template can warn about stale metadata. Schedule `apt-get update` separately.

Only recognized official security pockets are counted as security updates; all other candidates are classified as `other`. Bugfix and enhancement classifications are unsupported. Update history is best effort and comes from retained `/var/log/apt/history.log*` files. Reboot detection uses `/run/reboot-required`.

## Upgrade and rollback

### Upgrade

Run the installer again, test the item keys, then import the template from the same release. Install the binary before importing the template so its keys are already available.

The `dnf.get`, `packages.get`, and `dnf.advisories.get` keys and template UUIDs are kept between releases. If you are upgrading from the predecessor plugin, remove its Agent 2 configuration before enabling `package-updates.conf`. Loading both plugins registers duplicate metric keys.

### Rollback

Use the binary, checksum, configuration, and template from the same earlier release. Import the older template before replacing the binary. A newer binary can serve the older template, but an older binary may not provide keys used by the newer template.

For example, download the selected tagged assets, verify the checksum, stop Agent 2, replace the plugin binary and configuration, then restart and test `dnf.get`:

```bash
release=vX.Y.Z
base_url="https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/download/${release}"
curl -fLO "${base_url}/zabbix-agent2-plugin-package-updates"
curl -fLO "${base_url}/zabbix-agent2-plugin-package-updates.sha256"
sha256sum --check zabbix-agent2-plugin-package-updates.sha256
curl -fL -o package-updates.conf "${base_url}/package-updates.conf"

sudo systemctl stop zabbix-agent2
sudo install -m 0755 zabbix-agent2-plugin-package-updates /usr/sbin/zabbix-agent2-plugin/zabbix-agent2-plugin-package-updates
sudo install -m 0644 package-updates.conf /etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf
sudo systemctl start zabbix-agent2
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t dnf.get
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t packages.get
```

Remove `Plugins.PackageUpdates.Backend` before rolling back to a release that predates backend selection. Rolling back to a DNF-only release stops APT monitoring but does not change packages or APT metadata on the host.

## Notes and limitations

### DNF

* DNF4 uses list-only advisory collection to stay below Agent 2's timeout. It reports advisory IDs, severities, and affected packages, but detail, CVE, and issue-date completeness remain false. Per-advisory discovery is therefore unavailable on DNF4.
* DNF5 supports the 5.2 and 5.3-or-newer JSON formats. Malformed or unknown formats fail the check instead of falling back to text parsing.
* Results are based on enabled repository metadata. They do not say whether a vulnerability is exploitable or reachable.
* Check the completeness items before treating a zero CVE count or a missing date as final.
* Advisory checks run hourly by default and are limited to 8 MiB.
* Per-advisory discovery adds five items and one trigger for every selected advisory. IDs longer than 256 UTF-16 code units are not supported by discovery.

### APT

* The plugin does not refresh package indexes or check mirror health.
* Bugfix and enhancement classifications are unavailable.
* Package history is best effort because old APT logs may have been rotated away.
* APT does not provide the per-advisory monitoring available on DNF.

## Development

Start the local Zabbix/DNF lab with:

```bash
docker compose -f .dev/docker-compose.yaml up --build
```

The lab builds the supported DNF images, installs the plugin, starts passive and active Agent 2 hosts, and imports all four templates. APT distribution coverage runs in CI.

## Build from source

Building the plugin requires Go 1.27.0 or newer.

```bash
CGO_ENABLED=0 go build -o zabbix-agent2-plugin-package-updates ./cmd/agent
```

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

# List DNF transactions.
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
