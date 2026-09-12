package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Version support is a list, and a list is exactly the kind of thing that
// drifts from the distributions the project actually builds and tests. The
// installer is also the only place the rule exists - the plugin binary does
// not version gate at all - so nothing else would catch a release being
// dropped from it by accident.
func TestInstallerVersionGate(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}

	script, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	function := extractShellFunction(t, string(script), "supported_version_check")

	const (
		supported   = 0
		unsupported = 1
		notGated    = 2
	)

	tests := []struct {
		name    string
		id      string
		version string
		status  int
	}{
		{name: "debian 12", id: "debian", version: "12", status: supported},
		{name: "debian 13", id: "debian", version: "13", status: supported},
		{name: "debian 11", id: "debian", version: "11", status: unsupported},
		{name: "debian sid has no version", id: "debian", version: "", status: unsupported},
		{name: "ubuntu 22.04", id: "ubuntu", version: "22.04", status: supported},
		{name: "ubuntu 24.04", id: "ubuntu", version: "24.04", status: supported},
		{name: "ubuntu 25.04", id: "ubuntu", version: "25.04", status: supported},
		{name: "ubuntu 25.10", id: "ubuntu", version: "25.10", status: supported},
		{name: "ubuntu 26.04", id: "ubuntu", version: "26.04", status: supported},
		{name: "ubuntu 20.04", id: "ubuntu", version: "20.04", status: unsupported},
		// Mint point releases keep their Ubuntu base, so the major decides.
		{name: "mint 21", id: "linuxmint", version: "21", status: supported},
		{name: "mint 21.3", id: "linuxmint", version: "21.3", status: supported},
		{name: "mint 22.2", id: "linuxmint", version: "22.2", status: supported},
		{name: "mint 20.3", id: "linuxmint", version: "20.3", status: unsupported},
		{name: "fedora 44", id: "fedora", version: "44", status: supported},
		{name: "rhel 8.10", id: "rhel", version: "8.10", status: supported},
		{name: "centos 10", id: "centos", version: "10", status: supported},
		{name: "rocky 9.5", id: "rocky", version: "9.5", status: supported},
		{name: "almalinux 10", id: "almalinux", version: "10", status: supported},
		{name: "oracle linux 9", id: "ol", version: "9", status: supported},
		{name: "centos 7", id: "centos", version: "7", status: unsupported},
		{name: "fedora rawhide is not numeric", id: "fedora", version: "rawhide", status: unsupported},
		// Reached through ID_LIKE. These number their releases on their own
		// schedule, so there is nothing to check the version against.
		{name: "kali", id: "kali", version: "2026.1", status: notGated},
		{name: "pop", id: "pop", version: "24.04", status: notGated},
		{name: "amazon linux 2023", id: "amzn", version: "2023", status: notGated},
		{name: "devuan", id: "devuan", version: "5", status: notGated},
		{name: "lmde", id: "lmde", version: "6", status: notGated},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			requirement, status := runInstallerVersionCheck(t, function, test.id, test.version)
			if status != test.status {
				t.Fatalf(
					"supported_version_check(%q, %q) = %d (%q), want %d",
					test.id, test.version, status, requirement, test.status,
				)
			}
			// An unsupported version must say what would be supported: the
			// installer's message is built from this text, and an empty one
			// would read "require ".
			if test.status == unsupported && requirement == "" {
				t.Fatalf(
					"supported_version_check(%q, %q) rejected the version without naming a requirement",
					test.id, test.version,
				)
			}
			if test.status != unsupported && requirement != "" {
				t.Fatalf(
					"supported_version_check(%q, %q) printed %q for a non-rejection",
					test.id, test.version, requirement,
				)
			}
		})
	}
}

func runInstallerVersionCheck(
	t *testing.T,
	function string,
	id string,
	version string,
) (string, int) {
	t.Helper()

	script := "set -eu\n" + function + "\nsupported_version_check \"$1\" \"$2\"\n"

	output, err := exec.Command("sh", "-c", script, "sh", id, version).Output()
	status := 0
	if err != nil {
		exitErr := &exec.ExitError{}
		if !errors.As(err, &exitErr) {
			t.Fatalf("run supported_version_check: %v", err)
		}
		status = exitErr.ExitCode()
	}

	return strings.TrimSpace(string(output)), status
}
