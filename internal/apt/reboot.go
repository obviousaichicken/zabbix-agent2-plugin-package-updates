package apt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

const (
	defaultRebootMarker = "/run/reboot-required"

	// defaultKernelReleasePath is the running kernel release. Reading the
	// procfs entry avoids adding uname to the collector's required
	// executables, and needs no subprocess.
	defaultKernelReleasePath = "/proc/sys/kernel/osrelease"

	kernelImagePrefix         = "linux-image-"
	kernelImageUnsignedPrefix = "linux-image-unsigned-"
)

// RebootPending reports whether the host needs a reboot to finish applying
// updates.
//
// Two independent signals are combined, because neither is sufficient alone:
//
//  1. /run/reboot-required. Written by update-notifier-common's APT hook and
//     by unattended-upgrades' kernel hook. When present it is authoritative
//     and is the only signal that covers library-only reboots such as libc,
//     systemd or dbus. Both writers are optional packages, absent from
//     minimal Debian installs and from most container and cloud images, so
//     relying on it alone reports a confident "no reboot needed" on hosts
//     that simply have nobody to write the file.
//
//  2. A kernel image newer than the running one being installed, derived from
//     the package set already collected. This needs nothing beyond dpkg and
//     therefore works on every host.
//
// Library-only reboots still go undetected where signal 1 is absent, which is
// why the APT backend reports reboot detection as best effort rather than
// supported.
func (client *Client) RebootPending(
	ctx context.Context,
	installed []InstalledPackage,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	marked, err := client.rebootMarkerPresent()
	if err != nil {
		return false, err
	}
	if marked {
		return true, nil
	}

	return client.newerKernelInstalled(ctx, installed)
}

func (client *Client) rebootMarkerPresent() (bool, error) {
	_, err := client.stat(client.rebootMarker)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, &markerStatError{err: err}
}

// newerKernelInstalled reports whether an installed kernel image of the
// running kernel's flavour is newer than the running one.
func (client *Client) newerKernelInstalled(
	ctx context.Context,
	installed []InstalledPackage,
) (bool, error) {
	runningRelease, err := client.runningKernelRelease()
	if err != nil {
		return false, err
	}
	running, parsed := parseKernelRelease(runningRelease)
	if !parsed {
		// A kernel that does not use Debian's version-ABI-flavour release
		// form was not installed by dpkg, so no installed package
		// describes it. Custom builds and containers running the host's
		// kernel land here.
		return false, nil
	}

	kernels := installedKernels(installed)
	runningPackage, found := kernels[runningRelease]
	if !found {
		// The running kernel's package is not installed: it was removed,
		// or this is a container sharing the host kernel. Nothing can be
		// concluded by comparison.
		return false, nil
	}

	for release, pkg := range kernels {
		if release == runningRelease {
			continue
		}
		candidate, ok := parseKernelRelease(release)
		if !ok || candidate.flavour != running.flavour {
			// A different flavour is a separate kernel line. Having
			// linux-image-*-lowlatency installed does not mean the
			// running -generic kernel is out of date.
			continue
		}

		newer, compareErr := client.kernelIsNewer(ctx, pkg, runningPackage)
		if compareErr != nil {
			return false, compareErr
		}
		if newer {
			return true, nil
		}
	}

	return false, nil
}

func (client *Client) runningKernelRelease() (string, error) {
	data, err := client.readFile(defaultKernelReleasePath)
	if err != nil {
		return "", &kernelReleaseError{err: err}
	}

	release := strings.TrimSpace(string(data))
	if release == "" {
		return "", &kernelReleaseError{err: errors.New("running kernel release is empty")}
	}

	return release, nil
}

func (client *Client) kernelIsNewer(
	ctx context.Context,
	candidate InstalledPackage,
	running InstalledPackage,
) (bool, error) {
	result, err := client.run(
		ctx,
		"dpkg compare kernel versions",
		client.paths.DPKG,
		[]string{"--compare-versions", candidate.Version.Full, "gt", running.Version.Full},
		[]int{1},
	)
	if err != nil {
		return false, fmt.Errorf("compare kernel versions: %w", err)
	}
	switch result.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, errors.New("dpkg --compare-versions returned an unexpected exit status")
	}
}

// installedKernels indexes installed kernel image packages by the kernel
// release embedded in their name. Meta packages such as linux-image-amd64 and
// linux-image-generic carry no release and are skipped.
func installedKernels(installed []InstalledPackage) map[string]InstalledPackage {
	kernels := make(map[string]InstalledPackage)
	for _, pkg := range installed {
		release, ok := kernelReleaseFromPackage(pkg.Name)
		if !ok {
			continue
		}
		// Signed and unsigned images of one release carry the same
		// version, so either answers the comparison.
		if _, exists := kernels[release]; !exists {
			kernels[release] = pkg
		}
	}

	return kernels
}

func kernelReleaseFromPackage(name string) (string, bool) {
	release := ""
	switch {
	case strings.HasPrefix(name, kernelImageUnsignedPrefix):
		release = strings.TrimPrefix(name, kernelImageUnsignedPrefix)
	case strings.HasPrefix(name, kernelImagePrefix):
		release = strings.TrimPrefix(name, kernelImagePrefix)
	default:
		return "", false
	}
	if _, ok := parseKernelRelease(release); !ok {
		return "", false
	}

	return release, true
}

// kernelRelease is a Debian kernel release, "version-abi-flavour", such as
// 6.8.0-51-generic, 6.1.0-28-amd64 or 6.1.0-28-rt-amd64.
type kernelRelease struct {
	flavour string
}

func parseKernelRelease(release string) (kernelRelease, bool) {
	fields := strings.Split(release, "-")
	if len(fields) < 3 {
		return kernelRelease{}, false
	}
	if fields[0] == "" || !startsWithDigit(fields[0]) || !allDigits(fields[1]) {
		return kernelRelease{}, false
	}
	flavour := strings.Join(fields[2:], "-")
	if flavour == "" {
		return kernelRelease{}, false
	}

	return kernelRelease{flavour: flavour}, true
}

func startsWithDigit(value string) bool {
	return value != "" && value[0] >= '0' && value[0] <= '9'
}

type markerStatError struct {
	err error
}

func (*markerStatError) Error() string {
	return fmt.Sprintf("failed to stat APT reboot marker %q", defaultRebootMarker)
}

func (failure *markerStatError) Unwrap() error {
	return failure.err
}

type kernelReleaseError struct {
	err error
}

func (*kernelReleaseError) Error() string {
	return fmt.Sprintf("failed to read running kernel release from %q", defaultKernelReleasePath)
}

func (failure *kernelReleaseError) Unwrap() error {
	return failure.err
}
