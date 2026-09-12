package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/packageinfo"
)

// The installer must decide which backend a host uses before the plugin binary
// exists, so the rule is necessarily implemented twice. This test pins the two
// implementations together: they previously disagreed, and the installer
// refused hosts the plugin supports with a misleading missing-command error.
func TestInstallerBackendDetectionMatchesPlugin(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}

	script, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	function := extractShellFunction(t, string(script), "detect_package_backend")

	tests := []struct {
		id     string
		idLike string
	}{
		{id: "debian"},
		{id: "ubuntu"},
		{id: "fedora"},
		{id: "rhel"},
		{id: "centos", idLike: "rhel fedora"},
		{id: "rocky", idLike: "rhel centos fedora"},
		{id: "almalinux", idLike: "rhel centos fedora"},
		{id: "ol", idLike: "fedora"},
		{id: "linuxmint", idLike: "ubuntu debian"},
		{id: "pop", idLike: "ubuntu debian"},
		{id: "raspbian", idLike: "debian"},
		{id: "tuxedo", idLike: "ubuntu debian"},
		{id: "arch"},
		{id: "alpine"},
		{id: "weird", idLike: "debian fedora"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.id, func(t *testing.T) {
			t.Parallel()

			wantBackend, wantErr := detectOSBackend(test.id, test.idLike)
			gotBackend, gotErr := runInstallerDetection(t, function, test.id, test.idLike)

			if (wantErr != nil) != (gotErr != nil) {
				t.Fatalf(
					"installer error = %v, plugin error = %v for ID=%q ID_LIKE=%q",
					gotErr, wantErr, test.id, test.idLike,
				)
			}
			if wantErr != nil {
				return
			}
			if gotBackend != wantBackend {
				t.Fatalf(
					"installer backend = %s, plugin backend = %s for ID=%q ID_LIKE=%q",
					gotBackend, wantBackend, test.id, test.idLike,
				)
			}
		})
	}
}

func runInstallerDetection(
	t *testing.T,
	function string,
	id string,
	idLike string,
) (packageinfo.Backend, error) {
	t.Helper()

	script := "set -eu\n" +
		"fail() { printf 'error: %s\\n' \"$*\" >&2; exit 1; }\n" +
		function + "\ndetect_package_backend \"$1\" \"$2\"\n"

	output, err := exec.Command("sh", "-c", script, "sh", id, idLike).Output()
	if err != nil {
		return packageinfo.BackendUnknown, err
	}

	switch strings.TrimSpace(string(output)) {
	case backendAPT:
		return packageinfo.BackendAPT, nil
	case backendDNF:
		return packageinfo.BackendDNF, nil
	default:
		t.Fatalf("installer printed an unknown backend %q", output)

		return packageinfo.BackendUnknown, nil
	}
}

func extractShellFunction(t *testing.T, script string, name string) string {
	t.Helper()

	start := strings.Index(script, name+"() {")
	if start < 0 {
		t.Fatalf("install.sh no longer defines %s()", name)
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("cannot find the end of %s() in install.sh", name)
	}

	return script[start : start+end+len("\n}\n")]
}
