//go:build integration

//nolint:testpackage // White-box: the kernel comparison is unexported.
package apt

import (
	"context"
	"io/fs"
	"os/exec"
	"testing"
	"time"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/command"
)

// The unit tests fake the command runner, so the dpkg invocation that decides
// whether an installed kernel is newer than the running one has never actually
// run. A container cannot close that gap by installing a kernel package:
// /proc/sys/kernel/osrelease reports the host's kernel, which no dpkg package
// on the container will ever match, so the comparison is never reached.
//
// Everything except the running-kernel read is therefore real here: the real
// command.Runner, the real dpkg binary, and the real release parsing and
// flavour matching. Only the procfs value and the installed set are supplied,
// and both are plain data.
func TestRebootPendingKernelComparisonAgainstRealDpkg(t *testing.T) {
	dpkgPath, err := exec.LookPath("dpkg")
	if err != nil {
		t.Skipf("dpkg is unavailable: %v", err)
	}

	tests := []struct {
		name     string
		running  string
		packages map[string]string
		want     bool
	}{
		{
			name:    "ubuntu abi bump requires a reboot",
			running: "6.8.0-51-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic": "6.8.0-51.52",
				"linux-image-6.8.0-60-generic": "6.8.0-60.63",
			},
			want: true,
		},
		{
			name:    "running the newest installed kernel needs no reboot",
			running: "6.8.0-60-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic": "6.8.0-51.52",
				"linux-image-6.8.0-60-generic": "6.8.0-60.63",
			},
			want: false,
		},
		{
			name:    "debian point release requires a reboot",
			running: "6.1.0-28-amd64",
			packages: map[string]string{
				"linux-image-6.1.0-28-amd64": "6.1.119-1",
				"linux-image-6.1.0-31-amd64": "6.1.123-1",
			},
			want: true,
		},
		{
			// String ordering puts 6.1.119 above 6.12.5; dpkg does not.
			// This is the case that makes delegating to dpkg necessary.
			name:    "major series bump requires a reboot",
			running: "6.1.0-28-amd64",
			packages: map[string]string{
				"linux-image-6.1.0-28-amd64": "6.1.119-1",
				"linux-image-6.12.9-1-amd64": "6.12.9-1",
			},
			want: true,
		},
		{
			name:    "hwe kernel alongside the running one requires a reboot",
			running: "6.8.0-51-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic":  "6.8.0-51.52",
				"linux-image-6.11.0-17-generic": "6.11.0-17.17~24.04.2",
			},
			want: true,
		},
		{
			name:    "a newer kernel of another flavour is a different line",
			running: "6.8.0-51-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic":    "6.8.0-51.52",
				"linux-image-6.8.0-60-lowlatency": "6.8.0-60.63",
			},
			want: false,
		},
		{
			name:    "an unsigned newer image counts",
			running: "6.8.0-51-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic":          "6.8.0-51.52",
				"linux-image-unsigned-6.8.0-60-generic": "6.8.0-60.63",
			},
			want: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			client, clientErr := newClientWithSystemForTest(testSystem{
				runner: command.Runner{},
				paths: CommandPaths{
					APTGet:    dpkgPath,
					APTCache:  dpkgPath,
					DPKGQuery: dpkgPath,
					DPKG:      dpkgPath,
				},
				// No /run/reboot-required, so the kernel comparison decides.
				stat:              func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
				readFile:          runningKernelFile(test.running),
				now:               time.Now,
				historyFileSystem: &fakeHistoryFileSystem{readDirErr: fs.ErrNotExist},
				historyDirectory:  "/var/log/apt",
				rebootMarker:      "/nonexistent/reboot-required",
				refreshSignals:    []string{"/nonexistent/refresh"},
				location:          time.UTC,
			})
			if clientErr != nil {
				t.Fatalf("construct APT client: %v", clientErr)
			}

			got, pendingErr := client.RebootPending(
				context.Background(),
				installedKernelPackages(t, test.packages),
			)
			if pendingErr != nil {
				t.Fatalf("RebootPending() error = %v", pendingErr)
			}
			if got != test.want {
				t.Fatalf(
					"RebootPending() = %t, want %t for running %s with %v",
					got, test.want, test.running, test.packages,
				)
			}
		})
	}
}
