package apt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/packageinfo"
)

// APT stores each index under /var/lib/apt/lists with the modification time
// sent by the mirror, so an index file's mtime is when the archive published
// that index, not when this host last refreshed it. Immutable release pockets
// such as Debian's "trixie/main" or Ubuntu's "noble/main" are published once
// and keep their release-day timestamp forever, which makes index mtimes
// useless as a refresh clock.
//
// The signals below are paths APT itself writes while refreshing, so they
// answer the question the metadata age is actually monitored for: does this
// host still run apt-get update?
const (
	// refreshSignalPartial is APT's download staging directory. Every
	// apt-get update creates and removes work files in it, including runs
	// where every index comes back "304 Not Modified" and nothing is
	// rewritten under lists/. Only the directory's own metadata is read;
	// its contents are never listed, so the 0700 ownership APT gives it
	// does not matter.
	refreshSignalPartial = "/var/lib/apt/lists/partial"

	// refreshSignalPeriodicStamp is touched by APT's periodic maintenance
	// after a successful unattended refresh. It is absent unless
	// APT::Periodic::Update-Package-Lists is enabled, and it never moves for
	// a refresh an operator runs by hand.
	refreshSignalPeriodicStamp = "/var/lib/apt/periodic/update-success-stamp"
)

// defaultRefreshSignals returns the production refresh signals. Each signal is
// independent evidence that a refresh happened, and each can be absent on a
// legitimate host, so the newest one that exists wins rather than the first.
func defaultRefreshSignals() []string {
	return []string{refreshSignalPartial, refreshSignalPeriodicStamp}
}

// validateIndexTargets fails collection when an enabled binary index that
// apt-cache policy depends on is missing or is not a regular file. Index
// filenames encode repository URLs, so errors identify a target by position
// only.
func (client *Client) validateIndexTargets(ctx context.Context, targets []IndexTarget) error {
	if len(targets) == 0 {
		return errors.New("no enabled APT binary package indexes")
	}

	seen := make(map[string]struct{}, len(targets))
	for targetIndex, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, duplicate := seen[target.Filename]; duplicate {
			continue
		}
		seen[target.Filename] = struct{}{}

		info, err := client.stat(target.Filename)
		if err != nil {
			return &indexStatError{index: targetIndex + 1, err: err}
		}
		if info == nil {
			return fmt.Errorf("APT package index %d returned no file metadata", targetIndex+1)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("APT package index %d is not a regular file", targetIndex+1)
		}
	}

	return nil
}

// refreshMetadata reports when APT last refreshed this host's package indexes,
// and how long ago that was.
func (client *Client) refreshMetadata(ctx context.Context) (packageinfo.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return packageinfo.Metadata{}, err
	}
	if len(client.refreshSignals) == 0 {
		return packageinfo.Metadata{}, errors.New("no APT refresh signals are configured")
	}

	var (
		refreshed time.Time
		failures  []error
	)
	for _, signal := range client.refreshSignals {
		info, err := client.stat(signal)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				failures = append(failures, &refreshSignalError{signal: signal, err: err})
			}

			continue
		}
		if info == nil {
			failures = append(failures, &refreshSignalError{
				signal: signal,
				err:    errors.New("stat returned no file metadata"),
			})

			continue
		}
		modified := info.ModTime().UTC()
		if modified.IsZero() {
			failures = append(failures, &refreshSignalError{
				signal: signal,
				err:    errors.New("file has no modification time"),
			})

			continue
		}
		if modified.After(refreshed) {
			refreshed = modified
		}
	}
	if refreshed.IsZero() {
		if len(failures) != 0 {
			return packageinfo.Metadata{}, fmt.Errorf(
				"read APT refresh signals: %w",
				errors.Join(failures...),
			)
		}

		return packageinfo.Metadata{}, errors.New(
			"APT has no record of a package index refresh; run apt-get update",
		)
	}

	now := client.now().UTC()
	age := int64(0)
	if now.After(refreshed) {
		age = int64(now.Sub(refreshed) / time.Second)
	}

	return packageinfo.Metadata{RefreshedAt: &refreshed, AgeSeconds: &age}, nil
}

// refreshSignalError names the signal it failed on. Unlike index filenames,
// refresh signal paths are fixed constants and carry no repository detail, so
// naming them is safe and makes a permission problem actionable.
type refreshSignalError struct {
	signal string
	err    error
}

func (failure *refreshSignalError) Error() string {
	return fmt.Sprintf("%s: %v", failure.signal, failure.err)
}

func (failure *refreshSignalError) Unwrap() error {
	return failure.err
}

type indexStatError struct {
	index int
	err   error
}

func (err *indexStatError) Error() string {
	return fmt.Sprintf("failed to stat APT package index %d", err.index)
}

func (err *indexStatError) Unwrap() error {
	return err.err
}
