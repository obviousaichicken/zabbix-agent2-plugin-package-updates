# Installation and configuration

[Main README](../README.md) · [Troubleshooting and limitations](TROUBLESHOOTING.md) · [Template and trigger reference](TEMPLATES.md)

For the automated installer and template import steps, see the [quick start](../README.md#quick-start). For source builds, see the [development guide](DEVELOPMENT.md#build-from-source).

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

Backend detection is automatic. Most installations do not need anything beyond the configuration shown above. The shipped configuration is [configs/package-updates.conf](../configs/package-updates.conf).

## Configuration

### Backend selection

The backend defaults to `auto`. The plugin reads `/etc/os-release`: it matches `ID` first and falls back to `ID_LIKE`, so Debian and Ubuntu derivatives are detected as APT and RHEL derivatives as DNF. Startup fails if the distribution is unsupported, if `ID_LIKE` names both families, or if the required commands are missing. The plugin itself does not check the release version at all.

The installer applies the same backend rule and additionally checks `VERSION_ID`, but only where this project has something to check against: an exact list for Debian, Ubuntu and Linux Mint, and a floor of major version 8 for Fedora, RHEL, CentOS Stream, Rocky Linux, AlmaLinux and Oracle Linux. A Fedora older than the two listed in the [README](../README.md) therefore passes the installer even though it is not tested. Anything else reached through `ID_LIKE` numbers its releases on its own schedule, so the installer prints a note saying the version was not checked and continues.

Changing `Plugins.PackageUpdates.Backend` takes effect on an agent configuration reload; a full restart is not required.

You can force a backend when testing a controlled image:

```ini
Plugins.PackageUpdates.Backend=dnf
# or
Plugins.PackageUpdates.Backend=apt
```

Valid values are `auto`, `dnf`, and `apt`. A forced backend skips distribution-family detection but still checks the required commands. Leave this setting out for normal installations.

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

## Upgrade

Run the installer again, test the item, then import the template from the same release. Install the binary before importing the template so every referenced key is available.

The package and advisory item keys are `packages.get` and `advisories.get`.
