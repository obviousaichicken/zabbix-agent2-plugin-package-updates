#!/bin/sh

set -eu

release_url="${RELEASE_URL:-https://github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/releases/latest/download}"
plugin_dir="/usr/sbin/zabbix-agent2-plugin"
plugin_path="${plugin_dir}/zabbix-agent2-plugin-package-updates"
config_dir="/etc/zabbix/zabbix_agent2.d/plugins.d"
config_path="${config_dir}/package-updates.conf"
agent_config="/etc/zabbix/zabbix_agent2.conf"

fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

check_agent_version() {
	agent_version_output="$(zabbix_agent2 --version 2>&1)" ||
		fail "cannot read Zabbix Agent 2 version"

	# shellcheck disable=SC2086 # Splitting the version banner into fields is the point.
	set -- $agent_version_output
	agent_version="${3:-}"
	previous_ifs=$IFS
	IFS=.
	# shellcheck disable=SC2086 # Splitting the dotted version into fields is the point.
	set -- $agent_version
	IFS=$previous_ifs

	if [ "$#" -ne 3 ]; then
		fail "cannot parse Zabbix Agent 2 version: ${agent_version:-unknown}"
	fi

	for component in "$@"; do
		case "$component" in
		'' | *[!0-9]*)
			fail "cannot parse Zabbix Agent 2 version: $agent_version"
			;;
		esac
	done

	case "$1.$2" in
	7.0 | 7.2 | 7.4) ;;
	*)
		fail "unsupported Zabbix Agent 2 version $agent_version; require 7.0, 7.2, or 7.4"
		;;
	esac
}

# Mirrors detectOSBackend in cmd/agent/backend.go: match ID first, then fall
# back to ID_LIKE. Keeping only an ID list here made the installer refuse
# hosts the plugin supports, such as Linux Mint, Pop!_OS and Raspbian, with
# the misleading message "required command not found: dnf".
detect_package_backend() {
	backend_id="$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')"
	backend_id_like="$(printf '%s' "${2:-}" | tr '[:upper:]' '[:lower:]')"

	case "$backend_id" in
	debian | ubuntu)
		printf 'apt\n'
		return 0
		;;
	fedora | rhel | centos | rocky | almalinux)
		printf 'dnf\n'
		return 0
		;;
	esac

	apt_family=0
	dnf_family=0
	for family in $backend_id_like; do
		case "$family" in
		debian | ubuntu) apt_family=1 ;;
		fedora | rhel | centos) dnf_family=1 ;;
		esac
	done

	if [ "$apt_family" -eq 1 ] && [ "$dnf_family" -eq 1 ]; then
		fail "ambiguous package-manager family for ID=\"$backend_id\" ID_LIKE=\"$backend_id_like\"; set Plugins.PackageUpdates.Backend explicitly"
	fi
	if [ "$apt_family" -eq 1 ]; then
		printf 'apt\n'
		return 0
	fi
	if [ "$dnf_family" -eq 1 ]; then
		printf 'dnf\n'
		return 0
	fi

	fail "unsupported operating-system package manager for ID=\"$backend_id\" ID_LIKE=\"$backend_id_like\""
}

# supported_version_check reports whether VERSION_ID is a release this
# project documents and tests:
#
#   0  supported
#   1  unsupported; the requirement is printed on stdout
#   2  the distribution is not version gated here
#
# A derivative reached through ID_LIKE numbers its releases on its own
# schedule, which cannot be checked against a list of the distributions the
# README names, so it returns 2 rather than pretending the version was
# validated.
supported_version_check() {
	check_id=$1
	check_version=$2
	check_major="${check_version%%.*}"

	case "$check_id" in
	debian)
		case "$check_version" in
		11 | 12 | 13) return 0 ;;
		esac
		printf 'Debian 11, 12, or 13\n'
		;;
	ubuntu)
		case "$check_version" in
		22.04 | 24.04 | 25.04 | 25.10 | 26.04) return 0 ;;
		esac
		printf 'Ubuntu 22.04, 24.04, 25.04, 25.10, or 26.04\n'
		;;
	linuxmint)
		# Mint numbers its releases independently of the Ubuntu base it is
		# built from: 21.x is Ubuntu 22.04 and 22.x is Ubuntu 24.04. Point
		# releases keep the base, so only the major is checked.
		case "$check_major" in
		21 | 22) return 0 ;;
		esac
		printf 'Linux Mint 21 or 22\n'
		;;
	fedora | rhel | centos | rocky | almalinux | ol)
		case "$check_major" in
		'' | *[!0-9]*) ;;
		*)
			if [ "$check_major" -ge 8 ]; then
				return 0
			fi
			;;
		esac
		printf 'a DNF-based version 8 or newer\n'
		;;
	*)
		return 2
		;;
	esac

	return 1
}

check_operating_system() {
	[ -r /etc/os-release ] || fail "cannot read /etc/os-release"

	# shellcheck disable=SC1091 # The operating system provides this file.
	. /etc/os-release
	os_id="${ID:-}"
	os_id_like="${ID_LIKE:-}"
	os_version="${VERSION_ID:-}"

	package_backend="$(detect_package_backend "$os_id" "$os_id_like")"

	version_status=0
	version_requirement="$(supported_version_check "$os_id" "$os_version")" ||
		version_status=$?

	case "$version_status" in
	0) return ;;
	1)
		fail "unsupported ${os_id} version ${os_version:-unknown}; require ${version_requirement}"
		;;
	esac

	printf 'Note: %s is not in the supported distribution list; continuing with the %s backend detected from ID_LIKE.\n' \
		"${os_id:-unknown}" "$package_backend"
}

check_dnf_access() {
	printf '%s\n' 'Checking DNF access...'
	dnf_path="$(command -v dnf)"
	# --assumeyes is deliberate here and must not be "aligned" with the
	# collector's --assumeno: this preflight is the first DNF run as the
	# unprivileged zabbix user, on a host whose repository GPG keys may not be
	# imported yet, and only --assumeyes lets that import happen without a
	# prompt. .dev/installer-test/dnf asserts it on every image build.
	#
	# The --setopt override does match the collector, so a repository that is
	# unreachable fails here rather than silently passing preflight and
	# failing at collection time.
	run_as_zabbix "$dnf_path" --assumeyes -q repolist </dev/null >/dev/null ||
		fail "the zabbix user cannot list DNF repositories"
	run_as_zabbix "$dnf_path" --assumeyes -q '--setopt=*.skip_if_unavailable=False' \
		repoquery --upgrades --latest-limit=1 </dev/null >/dev/null ||
		fail "the zabbix user cannot query DNF updates; check that every enabled repository is reachable"
}

check_apt_access() {
	printf '%s\n' 'Checking APT access...'
	apt_get_path="$(command -v apt-get)"
	apt_cache_path="$(command -v apt-cache)"
	dpkg_query_path="$(command -v dpkg-query)"
	dpkg_path="$(command -v dpkg)"

	apt_index_output="$(run_as_zabbix "$apt_get_path" indextargets)" ||
		fail "the zabbix user cannot inspect APT package indexes"
	case "$apt_index_output" in
	*'Identifier: Packages'*) ;;
	*)
		fail "APT package indexes are not populated; run apt-get update as root and retry"
		;;
	esac

	# shellcheck disable=SC2016 # ${...} here is dpkg's own format syntax.
	run_as_zabbix "$dpkg_query_path" --show \
		'--showformat=${binary:Package}|${Architecture}|${Version}|${db:Status-Status}\n' \
		>/dev/null || fail "the zabbix user cannot query installed packages"

	# shellcheck disable=SC2016 # ${...} here is dpkg's own format syntax.
	policy_package="$(
		run_as_zabbix "$dpkg_query_path" --show \
			'--showformat=${Package}:${Architecture}\n' dpkg
	)" || fail "the zabbix user cannot resolve an installed package for APT policy preflight"
	[ -n "$policy_package" ] || fail "APT policy preflight found no installed dpkg package"
	run_as_zabbix "$apt_cache_path" policy "$policy_package" >/dev/null ||
		fail "the zabbix user cannot query APT package policy"
	run_as_zabbix "$dpkg_path" --compare-versions 1 eq 1 ||
		fail "the zabbix user cannot compare Debian package versions"
	run_as_zabbix "$dpkg_path" --print-architecture >/dev/null ||
		fail "the zabbix user cannot read the dpkg native architecture"
}

test_agent_item() {
	item_key=$1
	expected_backend=$2

	printf 'Testing %s as the zabbix user...\n' "$item_key"
	test_output="$(
		run_as_zabbix \
			zabbix_agent2 -c "$agent_config" -t "$item_key"
	)"
	printf '%s\n' "$test_output"

	case "$test_output" in
	*"[s|"*'"collection":{"complete":true'*) ;;
	*) fail "$item_key did not return a complete collection" ;;
	esac
	if [ -n "$expected_backend" ]; then
		case "$test_output" in
		*"\"backend\":\"${expected_backend}\""*) ;;
		*) fail "$item_key did not report the $expected_backend backend" ;;
		esac
	fi
}

if [ "$(id -u)" -ne 0 ]; then
	fail "run this installer as root"
fi

if [ "$(uname -s)" != "Linux" ] || [ "$(uname -m)" != "x86_64" ]; then
	fail "only Linux on x86_64 is supported"
fi

package_backend=
check_operating_system

for command_name in curl env getent sha256sum install mktemp runuser zabbix_agent2; do
	command -v "$command_name" >/dev/null 2>&1 || fail "required command not found: ${command_name}"
done

case "$package_backend" in
dnf)
	for command_name in dnf rpm; do
		command -v "$command_name" >/dev/null 2>&1 || fail "required command not found: ${command_name}"
	done
	;;
apt)
	for command_name in apt-get apt-cache dpkg-query dpkg; do
		command -v "$command_name" >/dev/null 2>&1 || fail "required command not found: ${command_name}"
	done
	;;
*) fail "internal error: no package backend selected" ;;
esac

zabbix_account="$(getent passwd zabbix)" || fail "required user not found: zabbix"
zabbix_home="${zabbix_account%:*}"
zabbix_home="${zabbix_home##*:}"

case "$zabbix_home" in
'' | /)
	fail "invalid home directory for zabbix user: ${zabbix_home:-empty}"
	;;
/*) ;;
*) fail "zabbix user home is not an absolute path: $zabbix_home" ;;
esac

if [ ! -e "$zabbix_home" ]; then
	install -d -m 0755 -o zabbix -g zabbix "$zabbix_home"
elif [ ! -d "$zabbix_home" ]; then
	fail "zabbix user home is not a directory: $zabbix_home"
fi

if command -v restorecon >/dev/null 2>&1; then
	restorecon -R "$zabbix_home"
fi

printf '%s\n' 'Checking Zabbix Agent 2 version...'
check_agent_version

tmp_dir="$(mktemp -d)"
install -d -m 0755 "$tmp_dir"

cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup 0

run_as_zabbix() {
	runuser -u zabbix -- env HOME="$zabbix_home" "$@"
}

case "$package_backend" in
dnf) check_dnf_access ;;
apt) check_apt_access ;;
esac

printf '%s\n' 'Downloading release files...'
for file_name in zabbix-agent2-plugin-package-updates zabbix-agent2-plugin-package-updates.sha256 package-updates.conf; do
	curl -fL --retry 3 \
		-o "${tmp_dir}/${file_name}" \
		"${release_url}/${file_name}"
done

printf '%s\n' 'Verifying checksum...'
(
	cd "$tmp_dir"
	sha256sum --check zabbix-agent2-plugin-package-updates.sha256
)

printf '%s\n' 'Installing plugin and configuration...'
install -d -m 0755 "$plugin_dir" "$config_dir"
install -m 0755 "${tmp_dir}/zabbix-agent2-plugin-package-updates" "$plugin_path"
install -m 0644 "${tmp_dir}/package-updates.conf" "$config_path"

if command -v restorecon >/dev/null 2>&1; then
	restorecon -R "$plugin_dir"
	restorecon "$config_path"
fi

printf '%s\n' 'Validating Zabbix Agent 2 configuration...'
zabbix_agent2 -T -c "$agent_config"

if [ "${SKIP_SERVICE_RESTART:-0}" = "1" ]; then
	printf '%s\n' 'Skipping service restart for container image build.'
else
	command -v systemctl >/dev/null 2>&1 || fail "required command not found: systemctl"
	printf '%s\n' 'Restarting Zabbix Agent 2...'
	systemctl restart zabbix-agent2
	systemctl is-active --quiet zabbix-agent2 || fail "zabbix-agent2 did not start"
fi

case "$package_backend" in
dnf)
	test_agent_item packages.get dnf
	;;
apt) test_agent_item packages.get apt ;;
esac

printf '%s\n' 'Installation completed successfully.'
