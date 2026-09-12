//nolint:testpackage // White-box: CommandError fields are unexported.
package apt

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/command"
)

func TestCommandErrorImplementsSafeFailure(t *testing.T) {
	t.Parallel()

	cause := context.DeadlineExceeded
	failure := &CommandError{
		operation:  "apt-cache policy (512 packages)",
		exitStatus: -1,
		err:        cause,
	}
	if !errors.Is(failure, cause) || !failure.IsTimeout() || failure.IsCanceled() {
		t.Errorf("CommandError flags = timeout:%t canceled:%t", failure.IsTimeout(), failure.IsCanceled())
	}
	if failure.Status() != -1 || failure.Operation() != "apt-cache policy (512 packages)" {
		t.Errorf("CommandError metadata = %d/%q", failure.Status(), failure.Operation())
	}
	if strings.Contains(failure.Operation(), "https://") {
		t.Errorf("CommandError operation exposes arguments: %q", failure.Operation())
	}

	// *CommandError implementing command.Failure is asserted at compile time
	// in command_error.go; comparing a non-nil pointer in an interface to nil
	// is never true and asserted nothing. Exercise the interface instead.
	var structured command.Failure = failure
	if structured.Status() != -1 || !structured.IsTimeout() {
		t.Errorf("Failure view = status %d timeout %t", structured.Status(), structured.IsTimeout())
	}
	if structured.Diagnostic() != "" {
		t.Errorf("Diagnostic() = %q, want empty when the command produced no stderr", structured.Diagnostic())
	}
}

func TestCommandErrorDiagnosticIsRedactedAndBounded(t *testing.T) {
	t.Parallel()

	failure := &CommandError{
		operation:  "apt-get indextargets",
		exitStatus: 100,
		diagnostic: command.Diagnostic([]byte(
			"Reading package lists...\nE: Failed to fetch https://alice:s3cr3t@packages.example/debian?key=abc\n",
		)),
		err: errors.New("exit status 100"),
	}

	got := failure.Diagnostic()
	if !strings.Contains(got, "Failed to fetch") {
		t.Errorf("Diagnostic() = %q, want the error line rather than the progress line", got)
	}
	for _, secret := range []string{"s3cr3t", "alice", "abc", "key="} {
		if strings.Contains(got, secret) {
			t.Errorf("Diagnostic() leaked %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "packages.example") {
		t.Errorf("Diagnostic() = %q, want the host retained", got)
	}
}
