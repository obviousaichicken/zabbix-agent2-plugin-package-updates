[![Checks](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/checks.yaml/badge.svg)](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/checks.yaml)
[![Package Updates Integration](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/package-updates-integration.yaml/badge.svg)](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/package-updates-integration.yaml)
[![Release](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/release.yaml/badge.svg)](https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/actions/workflows/release.yaml)

# Package Updates for Zabbix Agent 2

A loadable `zabbix-agent2` plugin for monitoring package updates on DNF and APT systems.

It reports:

* Pending updates, grouped by repository and update type
* Security advisories and CVEs on DNF hosts
* Reboot status and the result of the last package transaction
* How long ago APT last refreshed its package indexes

It supports:

* RHEL/UBI 8, 9, and 10
* Fedora 43 and 44
* Rocky Linux 8, 9, and 10
* AlmaLinux 8, 9, and 10
* Oracle Linux 8, 9, and 10
* CentOS Stream 9 and 10
* Debian 11, 12, and 13
* Ubuntu 22.04, 24.04, 25.04, 25.10, and 26.04
* Linux Mint 21 and 22

Other Debian and RHEL derivatives are detected from `ID_LIKE` and work without being listed here. See [Derivatives](#derivatives).

Additionally the plugin works with all `zabbix-agent2` versions in the 7.0, 7.2, and 7.4 branches. Releases are `linux/amd64` binaries and the installer refuses any other architecture; other architectures need a source build.

<a href="docs/images/dnf-advisory-values-rocky8.png"><img width="100%" alt="DNF advisory collection values for a Rocky Linux 8 host in Zabbix" src="docs/images/dnf-advisory-values-rocky8.png"></a>

<details>
<summary><strong>View more screenshots (7)</strong></summary>
<br>
<table>
  <tr>
    <td width="50%" valign="top"><a href="docs/images/dnf-package-values-rocky8.png"><img width="100%" alt="DNF package update values for a Rocky Linux 8 host in Zabbix" src="docs/images/dnf-package-values-rocky8.png"></a></td>
    <td width="50%" valign="top"><a href="docs/images/distribution-lab-problems.png"><img width="100%" alt="Package update problems across the Zabbix distribution test lab" src="docs/images/distribution-lab-problems.png"></a></td>
  </tr>
  <tr>
    <td width="50%" valign="top"><a href="docs/images/dnf-template-items.png"><img width="100%" alt="DNF template items and item keys in Zabbix" src="docs/images/dnf-template-items.png"></a></td>
    <td width="50%" valign="top"><a href="docs/images/apt-template-items.png"><img width="100%" alt="APT template items and item keys in Zabbix" src="docs/images/apt-template-items.png"></a></td>
  </tr>
  <tr>
    <td width="50%" valign="top"><a href="docs/images/apt-triggers.png"><img width="100%" alt="APT package update triggers in Zabbix" src="docs/images/apt-triggers.png"></a></td>
    <td width="50%" valign="top"><a href="docs/images/dnf-triggers.png"><img width="100%" alt="DNF package update and advisory triggers in Zabbix" src="docs/images/dnf-triggers.png"></a></td>
  </tr>
  <tr>
    <td width="50%" valign="top"><a href="docs/images/apt-package-values-debian13.png"><img width="100%" alt="APT package update values for a Debian 13 host in Zabbix" src="docs/images/apt-package-values-debian13.png"></a></td>
    <td width="50%"></td>
  </tr>
</table>
</details>

## Quick start

### 1. Install the plugin

```bash
curl -fLO https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/latest/download/install.sh && sudo sh install.sh
```

The installer detects DNF or APT, verifies the downloaded binary, checks package-manager access as the `zabbix` user, validates the Agent 2 configuration, and restarts the service. It requires systemd and Linux on x86_64. On APT systems, package indexes must already exist; the installer does not run `apt-get update`.

### 2. Import the template

Download [template-package-updates-by-zabbix-agent2.yaml](template-package-updates-by-zabbix-agent2.yaml), import it from **Data collection > Templates > Import**, and link the matching passive or active DNF/APT template to the host.

### 3. Confirm collection

```bash
# Supported on DNF and APT hosts
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t packages.get

# DNF hosts also expose advisory collection
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t advisories.get
```

## Documentation

* [Templates and triggers](TEMPLATES.md) covers macros, items, triggers, repository discovery, per-advisory discovery, and advisory payload behavior.
* [Development](DEVELOPMENT.md) covers prerequisites, source builds, checks, the local Zabbix lab, project layout, and package-manager commands.

## Manual installation

```bash
# Download the latest binary and its SHA-256 checksum from GitHub Releases
curl -fL -o zabbix-agent2-plugin-package-updates https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/latest/download/zabbix-agent2-plugin-package-updates

# Download the checksum
curl -fL -o zabbix-agent2-plugin-package-updates.sha256 https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/latest/download/zabbix-agent2-plugin-package-updates.sha256

# Verify the binary
sha256sum --check zabbix-agent2-plugin-package-updates.sha256

# Install the binary
sudo install -D -m 0755 zabbix-agent2-plugin-package-updates /usr/sbin/zabbix-agent2-plugin/zabbix-agent2-plugin-package-updates

# Create the configuration file
sudo sh -c 'cat > /etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf' <<'EOF'
Plugins.PackageUpdates.System.Path=/usr/sbin/zabbix-agent2-plugin/zabbix-agent2-plugin-package-updates
Plugins.PackageUpdates.System.Capacity=1
PluginTimeout=30
EOF

# Set configuration file permissions
sudo chmod 0644 /etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf

# On systems with SELinux, apply the default installation-path contexts
sudo restorecon -Rv /usr/sbin/zabbix-agent2-plugin /etc/zabbix/zabbix_agent2.d/plugins.d/package-updates.conf

# Confirm that the zabbix user can query DNF on a DNF host. The --setopt
# override is what the collector runs, so an unreachable repository fails
# here rather than at collection time.
sudo -u zabbix dnf -q repolist
sudo -u zabbix dnf -q '--setopt=*.skip_if_unavailable=False' repoquery --upgrades --latest-limit=1

# Or confirm read-only APT access on a Debian/Ubuntu host. Populate indexes as
# root first if this host has never run apt-get update.
sudo -u zabbix apt-get indextargets
sudo -u zabbix dpkg-query --show
sudo -u zabbix dpkg --print-architecture
sudo -u zabbix apt-cache policy "dpkg:$(dpkg --print-architecture)"

# Restart the agent and check that it started correctly
sudo systemctl restart zabbix-agent2
systemctl status zabbix-agent2
```

Backend detection is automatic. Most installations do not need anything beyond the configuration shown above.

## Configuration

### Backend selection

The backend defaults to `auto`. The plugin reads `/etc/os-release`: it matches `ID` first and falls back to `ID_LIKE`, so Debian and Ubuntu derivatives are detected as APT and RHEL derivatives as DNF. Startup fails if the distribution is unsupported, if `ID_LIKE` names both families, or if the required commands are missing. The plugin itself does not check the release version at all.

The installer applies the same backend rule and additionally checks `VERSION_ID`, but only where this project has something to check against: an exact list for Debian, Ubuntu and Linux Mint, and a floor of major version 8 for Fedora, RHEL, CentOS Stream, Rocky Linux, AlmaLinux and Oracle Linux. A Fedora older than the two listed above therefore passes the installer even though it is not tested. Anything else reached through `ID_LIKE` numbers its releases on its own schedule, so the installer prints a note saying the version was not checked and continues.

Changing `Plugins.PackageUpdates.Backend` takes effect on an agent configuration reload; a full restart is not required.

You can force a backend when testing a controlled image:

```ini
Plugins.PackageUpdates.Backend=dnf
# or
Plugins.PackageUpdates.Backend=apt
```

Valid values are `auto`, `dnf`, and `apt`. A forced backend skips distribution-family detection but still checks the required commands. Leave this setting out for normal installations.

### Derivatives

Because detection falls back to `ID_LIKE`, distributions this project has never tested still work. On the APT side that covers LMDE, Pop!_OS, Zorin OS, elementary OS, Kali, Devuan and Raspberry Pi OS; on the DNF side Amazon Linux 2023, Nobara, EuroLinux and the other RHEL rebuilds. Anything whose `ID_LIKE` names neither family, such as openSUSE or Arch, is refused at startup rather than guessed at.

Two things are worth knowing before relying on one:

* None of them is in the test matrix, and the installer does not check their versions. It accepts any release with a note saying the version was not checked, because their numbering is their own. They are covered by the detection rule and nothing else.
* Security classification recognizes the official Debian and Ubuntu security pockets only. A derivative that serves its own security updates from its own repository, such as Devuan, Kali or Raspberry Pi OS, reports those updates as `other`, so a security count of zero on such a host means "none recognized", not "none pending". Pop!_OS, Zorin and elementary keep the Ubuntu pockets for the base system, so those are still classified correctly.

### Reboot detection

DNF reboot status is determined from reboot-sensitive RPM install times and installed kernel packages compared with the running kernel. The plugin supports DNF4 and DNF5 without depending on an optional DNF reboot-detection plugin.

APT reboot status combines two signals. `/run/reboot-required` is authoritative when present and is the only signal that covers library-only reboots such as libc, systemd or dbus, but it is written by `update-notifier-common` and by `unattended-upgrades`' kernel hook, which are optional packages absent from minimal Debian installs and from most container and cloud images. The plugin therefore also compares the running kernel against the installed `linux-image-*` packages of the same flavour, which needs nothing beyond dpkg and works on every host. Library-only reboots still go undetected where the marker file has no writer, so APT reports reboot detection as `best_effort` rather than `supported`.

### APT metadata

APT collection is read-only. It uses the installed-package database and local package indexes; it does not contact mirrors, download packages, take package-manager locks, or run `apt-get update`. A successful check means the local metadata was readable, not that a mirror is reachable.

The payload reports when APT last refreshed this host's indexes as `metadata.refreshed_at`, and how long ago that was as `metadata.age_seconds`. The value comes from the paths APT writes while refreshing (`/var/lib/apt/lists/partial`, and `/var/lib/apt/periodic/update-success-stamp` where periodic updates are enabled), using whichever is most recent.

It is deliberately **not** derived from index file modification times. APT stores each index with the `Last-Modified` time the mirror sent, so an index mtime is when the archive published that index, not when this host fetched it. Immutable release pockets such as `trixie/main` or `noble/main` are published once and keep their release-day timestamp forever, which would report years of staleness on a host that refreshes every hour.

If neither path exists, the check fails and says to run `apt-get update`: a container image built with `/var/lib/apt/lists` emptied leaves no evidence that this host has ever refreshed, and reporting an age would be inventing one.

Missing or unreadable indexes still fail the check, and old but readable indexes remain valid so the template can warn about stale metadata. Schedule `apt-get update` separately.

Only recognized official Debian and Ubuntu security pockets are counted as security updates; all other candidates are classified as `other`. Bugfix and enhancement classifications are unsupported. Update history is best effort and comes from retained `/var/log/apt/history.log*` files.

## Troubleshooting

```bash
# Test package collection on either backend
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t packages.get

# Test the independently scheduled advisory item on a DNF host
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t advisories.get

# Run package collection directly
sudo /usr/sbin/zabbix-agent2-plugin/zabbix-agent2-plugin-package-updates --test

# Check for SELinux policy denials
sudo ausearch -m AVC -ts recent

# A failed check logs why. Agent 2 records the failing operation, its exit
# status and a redacted first line of the command's stderr.
sudo journalctl -u zabbix-agent2 | grep -i package-updates
```

## Upgrade

Run the installer again, test the item, then import the template from the same release. Install the binary before importing the template so every referenced key is available.

The package and advisory item keys are `packages.get` and `advisories.get`.

## Limitations

### DNF

* DNF4 uses list-only advisory collection to stay below Agent 2's timeout. It reports advisory IDs, severities, and affected packages, but detail, CVE, and issue-date completeness remain false. Per-advisory discovery is therefore unavailable on DNF4.
* DNF5 supports the 5.2 and 5.3-or-newer JSON formats. Malformed or unknown formats fail the check instead of falling back to text parsing.
* Results are based on enabled repository metadata. They do not say whether a vulnerability is exploitable or reachable.
* Check the completeness items before treating a zero CVE count or a missing date as final.
* Advisory checks run hourly by default. A response over 8 MiB fails rather than being truncated, which is a limit on every item this plugin answers, not only advisories.
* Per-advisory discovery adds five items and one trigger for every selected advisory. IDs longer than 256 UTF-16 code units are not supported by discovery.

### APT

* The plugin does not refresh package indexes or check mirror health.
* `metadata.age_seconds` measures the last refresh run, not whether every repository was reachable during it. `apt-get update` exits successfully when some indexes fail and older copies are reused.
* Bugfix and enhancement classifications are unavailable.
* Package history is best effort because old APT logs may have been rotated away. A history log that cannot be read or parsed reports `not_recorded` and logs the reason rather than failing the check.
* Reboot detection is best effort. A newer installed kernel is always detected; a library-only reboot is detected only where `/run/reboot-required` has a writer installed.
* Collection is a point-in-time snapshot. A package installed, upgraded or removed while a check runs is reported as APT saw it, or omitted, and appears in the next collection.
* APT does not provide the per-advisory monitoring available on DNF.

## AI Disclaimer

This project was developed with guidance from GPT-5.6 Sol and Claude Opus 5 for tedious refactoring, code review, and documentation.

## License

This project is licensed under the terms in [LICENSE](LICENSE).
