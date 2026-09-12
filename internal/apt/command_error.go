package apt

import (
	"context"
	"errors"
	"fmt"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/command"
)

// CommandError is a credential-safe APT command failure. It omits raw
// arguments and keeps only a redacted diagnostic line, because repository
// URLs can contain userinfo.
type CommandError struct {
	operation  string
	exitStatus int
	diagnostic string
	err        error
}

func (failure *CommandError) Error() string {
	if failure.exitStatus >= 0 {
		return fmt.Sprintf("APT command failed with exit status %d: %v", failure.exitStatus, failure.err)
	}

	return fmt.Sprintf("APT command failed: %v", failure.err)
}

func (failure *CommandError) Unwrap() error {
	return failure.err
}

// Operation returns a bounded command description without arguments.
func (failure *CommandError) Operation() string {
	return failure.operation
}

// Diagnostic returns the redacted reason the command failed.
func (failure *CommandError) Diagnostic() string {
	return failure.diagnostic
}

// Status returns the process exit status, or -1 when no process exited.
func (failure *CommandError) Status() int {
	return failure.exitStatus
}

// IsTimeout reports whether command execution exceeded its context deadline.
func (failure *CommandError) IsTimeout() bool {
	return errors.Is(failure.err, context.DeadlineExceeded)
}

// IsCanceled reports whether command execution was canceled for another reason.
func (failure *CommandError) IsCanceled() bool {
	return errors.Is(failure.err, context.Canceled)
}

var _ command.Failure = (*CommandError)(nil)
