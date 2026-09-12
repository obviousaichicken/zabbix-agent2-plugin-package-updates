"""Run the complete installer with real downloads/checksums/file installation.

Only absolute host paths and host-facing commands are sandboxed. No root needed.
"""

import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
BINARY = "zabbix-agent2-plugin-package-updates"
MOCK = r'''#!/usr/bin/env python3
import json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ["CALLS"], "a") as log:
    log.write(json.dumps([name, args]) + "\n")
failure = os.environ.get("FAILURE", "")
if name == "id":
    print("0")
elif name == "uname":
    print("Linux" if args == ["-s"] else "x86_64")
elif name == "getent":
    print("zabbix:x:123:123::" + os.environ["ZABBIX_HOME"] + ":/bin/sh")
elif name == "runuser":
    assert args[:3] == ["-u", "zabbix", "--"], args
    os.execvp(args[3], args[3:])
elif name == "apt-get":
    print("Identifier: Packages")
elif name == "dpkg-query":
    print("dpkg:amd64")
elif name == "zabbix_agent2":
    if args == ["--version"]:
        print("zabbix_agent2 (Zabbix) 7.0.20")
    elif "-T" in args:
        if failure == "validation":
            print("invalid configuration", file=sys.stderr)
            sys.exit(42)
    elif "-t" in args:
        print('packages.get [s|{"backend":"apt","collection":{"complete":true}}]')
    else:
        sys.exit(99)
elif name == "systemctl":
    if (failure == "restart" and args[0] == "restart") or (failure == "inactive" and args[0] == "is-active"):
        print("injected service failure", file=sys.stderr)
        sys.exit(43)
elif name not in ("apt-cache", "dpkg", "restorecon"):
    sys.exit(99)
'''


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="installer-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.release = self.root / "release"
        self.release.mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.home = self.root / "home"
        self.home.mkdir()
        self.downloads = self.root / "downloads"
        self.downloads.mkdir()
        self.calls = self.root / "calls.jsonl"
        self.calls.touch()
        self.plugin = self.root / "plugin" / BINARY
        self.config = self.root / "config" / "package-updates.conf"
        self.agent_config = self.root / "agent.conf"
        self.agent_config.write_text("# operator-owned main config\n")
        os_release = self.root / "os-release"
        os_release.write_text('ID=ubuntu\nVERSION_ID="24.04"\n')
        script = (ROOT / "install.sh").read_text()
        for old, new in {
            "/usr/sbin/zabbix-agent2-plugin": str(self.plugin.parent),
            "/etc/zabbix/zabbix_agent2.d/plugins.d": str(self.config.parent),
            "/etc/zabbix/zabbix_agent2.conf": str(self.agent_config),
            "/etc/os-release": str(os_release),
        }.items():
            self.assertIn(old, script)
            script = script.replace(old, new)
        self.script = self.root / "install.sh"
        self.script.write_text(script)
        for name in ("id", "uname", "getent", "runuser", "apt-get", "apt-cache",
                     "dpkg-query", "dpkg", "zabbix_agent2", "systemctl", "restorecon"):
            mock = self.bin / name
            mock.write_text(MOCK)
            mock.chmod(0o755)
        self.publish("v1")

    def publish(self, version):
        data = ("plugin " + version + "\n").encode()
        (self.release / BINARY).write_bytes(data)
        (self.release / (BINARY + ".sha256")).write_text(
            hashlib.sha256(data).hexdigest() + "  " + BINARY + "\n")
        (self.release / "package-updates.conf").write_text("# config " + version + "\n")

    def run_installer(self, failure="", success=True):
        self.calls.write_text("")
        env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                   RELEASE_URL=self.release.as_uri(), SKIP_SERVICE_RESTART="0",
                   CALLS=str(self.calls), ZABBIX_HOME=str(self.home),
                   TMPDIR=str(self.downloads), FAILURE=failure)
        result = subprocess.run(["sh", str(self.script)], env=env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=30)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertIn("Installation completed successfully.", result.stdout)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertNotIn("Installation completed successfully.", result.stdout)
        self.assertEqual(list(self.downloads.iterdir()), [], "temporary downloads leaked")
        self.assertEqual(self.agent_config.read_text(), "# operator-owned main config\n")
        import json
        self.events = [json.loads(line) for line in self.calls.read_text().splitlines()]
        return result.stdout

    def assert_installed(self):
        for installed, source, mode in (
            (self.plugin, self.release / BINARY, 0o755),
            (self.config, self.release / "package-updates.conf", 0o644),
        ):
            self.assertEqual(installed.read_bytes(), source.read_bytes())
            self.assertEqual(installed.stat().st_mode & 0o777, mode)

    def assert_no_service_or_item(self):
        self.assertFalse(any(name == "systemctl" or "-t" in args for name, args in self.events))

    def test_fresh_install_reinstall_and_upgrade(self):
        for version in ("v1", "v1", "v2"):
            with self.subTest(version=version):
                if self.config.exists():
                    self.config.write_text("# operator customization, replaced on reinstall\n")
                self.publish(version)
                self.run_installer()
                self.assert_installed()
                validation = ["zabbix_agent2", ["-T", "-c", str(self.agent_config)]]
                restart = ["systemctl", ["restart", "zabbix-agent2"]]
                active = ["systemctl", ["is-active", "--quiet", "zabbix-agent2"]]
                item = ["runuser", ["-u", "zabbix", "--", "env", "HOME=" + str(self.home),
                                   "zabbix_agent2", "-c", str(self.agent_config), "-t", "packages.get"]]
                indices = [self.events.index(event) for event in (validation, restart, active, item)]
                self.assertEqual(indices, sorted(indices))

    def test_checksum_mismatch_leaves_existing_install_untouched(self):
        self.run_installer()
        old = (self.plugin.read_bytes(), self.config.read_bytes())
        self.publish("v2")
        (self.release / BINARY).write_text("tampered binary\n")
        output = self.run_installer(success=False)
        self.assertIn("FAILED", output)
        self.assertEqual((self.plugin.read_bytes(), self.config.read_bytes()), old)
        self.assert_no_service_or_item()

    def test_each_missing_download_leaves_existing_install_untouched(self):
        self.run_installer()
        old = (self.plugin.read_bytes(), self.config.read_bytes())
        for name in (BINARY, BINARY + ".sha256", "package-updates.conf"):
            with self.subTest(missing=name):
                self.publish("v2")
                (self.release / name).unlink()
                output = self.run_installer(success=False)
                self.assertIn("curl:", output)
                self.assertNotIn("Installing plugin", output)
                self.assertEqual((self.plugin.read_bytes(), self.config.read_bytes()), old)
                self.assert_no_service_or_item()

    def test_configuration_validation_failure_stops_before_restart(self):
        output = self.run_installer("validation", success=False)
        self.assertIn("invalid configuration", output)
        self.assert_installed()  # Current installer does not roll back installed files.
        self.assert_no_service_or_item()

    def test_restart_failure_stops_before_health_check_and_item(self):
        self.run_installer("restart", success=False)
        self.assertIn(["systemctl", ["restart", "zabbix-agent2"]], self.events)
        self.assertFalse(any("is-active" in args or "-t" in args for _, args in self.events))
        self.assert_installed()

    def test_inactive_service_stops_before_item(self):
        output = self.run_installer("inactive", success=False)
        self.assertIn("zabbix-agent2 did not start", output)
        self.assertIn(["systemctl", ["is-active", "--quiet", "zabbix-agent2"]], self.events)
        self.assertFalse(any("-t" in args for _, args in self.events))


if __name__ == "__main__":
    unittest.main(verbosity=2)
