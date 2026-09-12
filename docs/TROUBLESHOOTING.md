# Troubleshooting and limitations

[Main README](../README.md) · [Installation and configuration](INSTALLATION.md) · [Template and trigger reference](TEMPLATES.md)

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

See [APT metadata](INSTALLATION.md#apt-metadata) and [reboot detection](INSTALLATION.md#reboot-detection) for more detail on these signals.
