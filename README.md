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
* Debian 12 and 13
* Ubuntu 22.04, 24.04, 25.04, 25.10, and 26.04
* Linux Mint 21 and 22

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

Download [template-package-updates-by-zabbix-agent2.yaml](templates/template-package-updates-by-zabbix-agent2.yaml), import it from **Data collection > Templates > Import**, and link the matching passive or active DNF/APT template to the host.

### 3. Confirm collection

```bash
# Supported on DNF and APT hosts
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t packages.get

# DNF hosts also expose advisory collection
sudo -u zabbix zabbix_agent2 -c /etc/zabbix/zabbix_agent2.conf -t advisories.get
```

## Documentation

* [Installation and configuration](docs/INSTALLATION.md) covers manual installation, backend selection, reboot detection, APT metadata, and upgrades.
* [Troubleshooting and limitations](docs/TROUBLESHOOTING.md) covers diagnostic commands and DNF/APT collection limitations.
* [Templates and triggers](docs/TEMPLATES.md) covers macros, items, triggers, repository discovery, per-advisory discovery, and advisory payload behavior.
* [Development](docs/DEVELOPMENT.md) covers prerequisites, source builds, checks, the local Zabbix lab, project layout, and package-manager commands.

## AI Disclaimer

This project was developed with guidance from GPT-5.6 Sol and Claude Opus 5 for tedious refactoring, code review, and documentation.

## License

This project is licensed under the terms in [LICENSE](LICENSE).
