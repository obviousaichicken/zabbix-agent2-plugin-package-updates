# Zabbix Templates and Triggers

[Main README](README.md) · [Development guide](DEVELOPMENT.md)

The release asset [template-package-updates-by-zabbix-agent2.yaml](template-package-updates-by-zabbix-agent2.yaml) contains four templates:

|Host|Passive checks|Active checks|
|----|--------------|-------------|
|DNF|DNF by Zabbix agent 2|DNF by Zabbix agent 2 active|
|APT|APT by Zabbix agent 2|APT by Zabbix agent 2 active|

Import the file from **Data collection > Templates > Import**, then link one template to each host. Repository discovery and alerts are included. Per-advisory discovery on DNF is disabled by default.

The passive and active variants share macros, dependent items, triggers, and discovery rules. Their master items differ only in the Zabbix check type and the corresponding template path used by expressions.

## DNF template

### Macros

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

### Items

|Name|Description|Type|Key and additional info|
|----|-----------|----|-----------------------|
|DNF: Get update data|Collects package, repository, classification, reboot, and update-history data. Dependent items extract the values, so the raw JSON is not stored.|Zabbix agent|packages.get|
|DNF: Get advisory data|Collects applicable DNF security advisories independently from package-update monitoring.|Zabbix agent|advisories.get, interval `{$DNF.ADVISORY.UPDATE.INTERVAL}`, history disabled, 30-second timeout|
|DNF: Advisory discovery data|Projects compact discovery records. With every selection macro disabled it returns an empty array; enabled discovery requires complete advisory metadata and fails closed on incomplete input.|Dependent item|dnf.advisory.discovery.data, history disabled|
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
|DNF: Last update time|Unix timestamp of the most recent completed DNF transaction that upgraded a package, or `0` when none is recorded.|Dependent item|dnf.last_update.timestamp|
|DNF: Advisory collection health|Independent completion and duration telemetry.|Dependent items|dnf.advisory.collection.complete, dnf.advisory.collection.duration|
|DNF: Advisory metadata completeness|Reports whether detail, CVE, and true issue-date metadata are complete.|Dependent items|dnf.advisory.details.complete, dnf.advisory.cves.complete, dnf.advisory.issue_dates.complete|
|DNF: Advisory and CVE totals|Counts unique advisory IDs and known CVE IDs.|Dependent items|dnf.advisory.total, dnf.advisory.cves|
|DNF: Advisories by severity|Counts Critical, Important, Moderate, Low, and Unknown advisory IDs.|Dependent items|dnf.advisory.critical, dnf.advisory.important, dnf.advisory.moderate, dnf.advisory.low, dnf.advisory.unknown|
|DNF: Affected updates by severity|Counts unique pending package updates once at their highest linked advisory severity.|Dependent items|dnf.advisory.packages.critical, dnf.advisory.packages.important, dnf.advisory.packages.moderate, dnf.advisory.packages.low, dnf.advisory.packages.unknown|
|DNF: Oldest advisory vendor time|Reports the oldest preferred vendor timestamp and age, using `0` when unknown; the basis item reports `issued`, `updated`, or `none`.|Dependent items|dnf.advisory.oldest.timestamp, dnf.advisory.oldest.age, dnf.advisory.oldest.basis|

### Triggers

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

### Repository discovery

|Name|Description|Type|Key and additional info|
|----|-----------|----|-----------------------|
|DNF: Repository discovery|Discovers enabled DNF repositories. Items for repositories that disappear are disabled immediately and retained for 30 days before deletion.|Dependent item|dnf.repository|

The rule creates these item prototypes:

|Name|Description|Type|Key and additional info|
|----|-----------|----|-----------------------|
|DNF: Repository [{#REPO_NAME}]: Available update count|Number of available package updates from repository {#REPO_NAME} ({#REPO_ID}).|Dependent item|dnf.repository.updates["{#REPO_ID}"]|
|DNF: Repository [{#REPO_NAME}]: Pending package details|Comma-separated NEVRA identifiers for packages with available updates from repository {#REPO_NAME} ({#REPO_ID}). The value is blank when there are no pending packages.|Dependent item|dnf.repository.update.details["{#REPO_ID}"]|

### Advisory discovery

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

With all five discovery macros at their default `0`, the discovery data item remains supported and returns an empty array even when a repository cannot provide complete titles, CVEs, or issue dates. Enabling any severity keeps the stricter metadata requirement so incomplete input cannot make existing advisories disappear falsely.

To enable discovery on a DNF5 host:

1. Confirm that `advisories.get` is supported and that `dnf.advisory.collection.complete`, `dnf.advisory.details.complete`, `dnf.advisory.cves.complete`, and `dnf.advisory.issue_dates.complete` all report `1`.
2. Change the host or template macro for each severity you want from `0` to `1`; for example, set `{$DNF.ADVISORY.LLD.CRITICAL}` to `1`.
3. Run or wait for `DNF: Advisory discovery`, then check discovered-object counts before enabling more severities.

To turn discovery off, restore all five macros to `0`. Presence items return to zero, then Zabbix disables the discovered objects after one day and removes them after 30 days.

### Collection behavior

`advisories.get` is separate from `packages.get`, so package snapshots can run every 15 minutes while the more expensive advisory check runs hourly. Advisory output is limited to 8 MiB. The collector does not run a command for every advisory:

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

## APT template

The APT template uses `packages.get` as its master item. It monitors the common collection, repository, update, reboot, and best-effort history fields plus the age of the oldest local binary package index used in the collection.

### Macros

|Name|Description|Default|
|----|-----------|-------|
|{$APT.COLLECTION.DURATION.MAX}|Maximum acceptable average collection duration, in seconds.|`20`|
|{$APT.METADATA.AGE.MAX}|Maximum acceptable age of the oldest local APT binary index.|`2d`|
|{$APT.NODATA.TIME}|Time without a successful collection before an availability problem is raised.|`30m`|
|{$APT.SECURITY.MIN}|Minimum number of positively identified security updates that raises a problem.|`1`|
|{$APT.UPDATE.INTERVAL}|Interval between complete APT collections.|`15m`|

### Items

|Purpose|Keys|
|-------|----|
|Collection health|`apt.collection.complete`, `apt.collection.duration`|
|Repository and update totals|`apt.repositories`, `apt.updates`, `apt.updates.pending`|
|Supported classifications|`apt.updates.security`, `apt.updates.other`|
|Host state and history|`apt.reboot.pending`, `apt.last_update.timestamp`, `apt.last_update.result`|
|Cached-index freshness|`apt.metadata.refreshed`, `apt.metadata.age`|

The APT template has no bugfix or enhancement items because the collector cannot reliably make those classifications. It also has no failed-history trigger because retained APT history is best effort.

### Triggers

|Name|Description|Expression|Severity|
|----|-----------|----------|--------|
|APT: Collection is unavailable|APT update data is stale or unavailable until collection succeeds.|`last(/APT by Zabbix agent 2/apt.collection.complete)=0 or nodata(/APT by Zabbix agent 2/apt.collection.complete,{$APT.NODATA.TIME})=1`|High|
|APT: Collection is slow|The average collection duration is approaching the hard plugin and item timeout.|`avg(/APT by Zabbix agent 2/apt.collection.duration,30m)>{$APT.COLLECTION.DURATION.MAX}`|Warning|
|APT: Reboot is required|A reboot is required to complete package updates.|`last(/APT by Zabbix agent 2/apt.reboot.pending)=1`|Warning|
|APT: Security updates are available|One or more positively identified security updates are available.|`last(/APT by Zabbix agent 2/apt.updates.security)>={$APT.SECURITY.MIN}`|High|
|APT: Package metadata is stale|The cached APT indexes are older than the configured threshold.|`last(/APT by Zabbix agent 2/apt.metadata.age)>{$APT.METADATA.AGE.MAX}`|Warning|

### Repository discovery

The `apt.repository` rule discovers repositories represented in the package snapshot. Items for repositories that disappear are disabled immediately and retained for 30 days before deletion.

The rule creates these dependent item prototypes:

|Purpose|Key|
|-------|---|
|Available update count|`apt.repository.updates["{#REPO_ID}"]`|
|Pending package details|`apt.repository.update.details["{#REPO_ID}"]`|

### Collection behavior

Pending package details use the Debian identifier `name:architecture=full-version`.

An update classified as `other` was not positively identified as coming from a trusted official Debian, Ubuntu, or Ubuntu ESM security pocket; `other` is not proof that the update has no security impact.
