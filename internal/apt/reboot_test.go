package apt

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func rebootClient(
	t *testing.T,
	runner Runner,
	stat func(string) (fs.FileInfo, error),
	readFile func(string) ([]byte, error),
) *Client {
	t.Helper()

	client, err := newClientWithSystemForTest(testSystem{
		runner:            runner,
		paths:             testAPTPaths(),
		stat:              stat,
		readFile:          readFile,
		now:               time.Now,
		historyFileSystem: &fakeHistoryFileSystem{readDirErr: fs.ErrNotExist},
		historyDirectory:  "/virtual/apt",
		rebootMarker:      "/private/alice:s3cr3t/reboot-required",
		refreshSignals:    []string{"/virtual/apt-refresh"},
		location:          time.UTC,
	})
	if err != nil {
		t.Fatalf("construct APT client: %v", err)
	}

	return client
}

func runningKernelFile(release string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		if path != defaultKernelReleasePath {
			return nil, fs.ErrNotExist
		}

		return []byte(release + "\n"), nil
	}
}

func installedKernelPackages(t *testing.T, versions map[string]string) []InstalledPackage {
	t.Helper()

	packages := make([]InstalledPackage, 0, len(versions))
	for name, version := range versions {
		packages = append(packages, InstalledPackage{
			Name:         name,
			Architecture: "amd64",
			Version:      mustDebianVersion(t, version),
		})
	}

	return packages
}

func TestRebootPendingMarkerSemantics(t *testing.T) {
	t.Parallel()

	permissionErr := errors.New("permission denied")
	tests := []struct {
		name    string
		statErr error
		want    bool
		wantErr error
	}{
		{name: "exists", want: true},
		{name: "missing", statErr: fs.ErrNotExist},
		{name: "permission", statErr: permissionErr, wantErr: permissionErr},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := rebootClient(
				t,
				&fakeAPTRunner{},
				func(string) (fs.FileInfo, error) { return nil, test.statErr },
				runningKernelFile("6.1.0-28-amd64"),
			)

			got, err := client.RebootPending(context.Background(), nil)
			if got != test.want {
				t.Errorf("RebootPending() = %t, want %t", got, test.want)
			}
			if test.wantErr == nil && err != nil {
				t.Fatalf("RebootPending() error = %v", err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("RebootPending() error = %v, want %v", err, test.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "s3cr3t") {
				t.Errorf("RebootPending() error exposes marker path: %v", err)
			}
		})
	}
}

// /run/reboot-required is written by optional packages, so a host without them
// must still detect a newer installed kernel rather than reporting a confident
// "no reboot needed".
func TestRebootPendingDetectsNewerKernelWithoutMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		running  string
		packages map[string]string
		exitCode int
		want     bool
		wantRuns int
	}{
		{
			name:    "newer kernel of the running flavour",
			running: "6.8.0-51-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic": "6.8.0-51.52",
				"linux-image-6.8.0-60-generic": "6.8.0-60.63",
			},
			want:     true,
			wantRuns: 1,
		},
		{
			name:    "only an older kernel alongside",
			running: "6.8.0-60-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic": "6.8.0-51.52",
				"linux-image-6.8.0-60-generic": "6.8.0-60.63",
			},
			exitCode: 1,
			want:     false,
			wantRuns: 1,
		},
		{
			name:    "newer kernel of a different flavour is not the running line",
			running: "6.8.0-51-generic",
			packages: map[string]string{
				"linux-image-6.8.0-51-generic":    "6.8.0-51.52",
				"linux-image-6.8.0-60-lowlatency": "6.8.0-60.63",
			},
			want:     false,
			wantRuns: 0,
		},
		{
			name:    "meta packages carry no release and are ignored",
			running: "6.1.0-28-amd64",
			packages: map[string]string{
				"linux-image-6.1.0-28-amd64": "6.1.119-1",
				"linux-image-amd64":          "6.1.119-1",
			},
			want:     false,
			wantRuns: 0,
		},
		{
			name:    "running kernel has no installed package",
			running: "6.18.44-fc-v24",
			packages: map[string]string{
				"linux-image-6.8.0-60-generic": "6.8.0-60.63",
			},
			want:     false,
			wantRuns: 0,
		},
		{
			name:     "no kernel packages at all",
			running:  "6.8.0-51-generic",
			packages: map[string]string{},
			want:     false,
			wantRuns: 0,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeAPTRunner{responses: []fakeAPTResponse{
				{exitCode: test.exitCode},
			}}
			client := rebootClient(
				t,
				runner,
				func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
				runningKernelFile(test.running),
			)

			got, err := client.RebootPending(
				context.Background(),
				installedKernelPackages(t, test.packages),
			)
			if err != nil {
				t.Fatalf("RebootPending() error = %v", err)
			}
			if got != test.want {
				t.Errorf("RebootPending() = %t, want %t", got, test.want)
			}
			if runs := len(runner.Requests()); runs != test.wantRuns {
				t.Errorf("dpkg comparisons = %d, want %d", runs, test.wantRuns)
			}
		})
	}
}

// The marker is authoritative for library-only reboots, so it short-circuits
// before any kernel comparison runs.
func TestRebootPendingPrefersMarkerOverKernelComparison(t *testing.T) {
	t.Parallel()

	runner := &fakeAPTRunner{}
	client := rebootClient(
		t,
		runner,
		func(string) (fs.FileInfo, error) { return fakeFileInfo{mode: 0o644}, nil },
		func(string) ([]byte, error) {
			t.Error("running kernel release was read despite the marker being present")

			return nil, fs.ErrNotExist
		},
	)

	got, err := client.RebootPending(context.Background(), nil)
	if err != nil || !got {
		t.Fatalf("RebootPending() = %t, %v, want true", got, err)
	}
	if len(runner.Requests()) != 0 {
		t.Errorf("dpkg ran despite the marker being present")
	}
}

func TestRebootPendingSignedAndUnsignedImagesAgree(t *testing.T) {
	t.Parallel()

	runner := &fakeAPTRunner{responses: []fakeAPTResponse{{exitCode: 0}}}
	client := rebootClient(
		t,
		runner,
		func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
		runningKernelFile("6.8.0-51-generic"),
	)
	installed := installedKernelPackages(t, map[string]string{
		"linux-image-6.8.0-51-generic":          "6.8.0-51.52",
		"linux-image-unsigned-6.8.0-60-generic": "6.8.0-60.63",
	})

	got, err := client.RebootPending(context.Background(), installed)
	if err != nil || !got {
		t.Fatalf("RebootPending() = %t, %v, want true for an unsigned newer image", got, err)
	}
}

func TestRebootPendingReportsUnreadableKernelRelease(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("permission denied")
	client := rebootClient(
		t,
		&fakeAPTRunner{},
		func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
		func(string) ([]byte, error) { return nil, sentinel },
	)

	_, err := client.RebootPending(context.Background(), nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("RebootPending() error = %v, want sentinel", err)
	}
	if !strings.Contains(err.Error(), defaultKernelReleasePath) {
		t.Errorf("error should name the fixed procfs path: %v", err)
	}
}

func TestRebootPendingHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	statCalled := false
	client := rebootClient(
		t,
		&fakeAPTRunner{},
		func(string) (fs.FileInfo, error) {
			statCalled = true

			return nil, nil
		},
		runningKernelFile("6.1.0-28-amd64"),
	)

	_, err := client.RebootPending(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RebootPending() error = %v, want context canceled", err)
	}
	if statCalled {
		t.Error("reboot marker was statted after cancellation")
	}
}

func TestParseKernelRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		release string
		flavour string
		valid   bool
	}{
		{release: "6.8.0-51-generic", flavour: "generic", valid: true},
		{release: "6.1.0-28-amd64", flavour: "amd64", valid: true},
		{release: "6.1.0-28-rt-amd64", flavour: "rt-amd64", valid: true},
		{release: "6.1.0-28-cloud-amd64", flavour: "cloud-amd64", valid: true},
		{release: "6.18.44-fc-v24"},
		{release: "6.8.0-51"},
		{release: "generic"},
		{release: ""},
	}

	for _, test := range tests {
		test := test
		t.Run(test.release, func(t *testing.T) {
			t.Parallel()

			parsed, ok := parseKernelRelease(test.release)
			if ok != test.valid {
				t.Fatalf("parseKernelRelease(%q) ok = %t, want %t", test.release, ok, test.valid)
			}
			if ok && parsed.flavour != test.flavour {
				t.Errorf("flavour = %q, want %q", parsed.flavour, test.flavour)
			}
		})
	}
}
