//go:build linux

//nolint:testpackage // runner tests exercise process cancellation directly.
package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	helperProcessDescendantEnv = "GO_WANT_DESCENDANT_COMMAND_HELPER_PROCESS"
	helperProcessDetachedEnv   = "GO_WANT_DETACHED_COMMAND_HELPER_PROCESS"
	helperProcessPIDFileEnv    = "GO_COMMAND_HELPER_PID_FILE"
)

func TestRunnerStopsDescendantsAtContextDeadline(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()

	_, err = (Runner{}).Run(ctx, Request{
		Name: executable,
		Args: []string{"-test.run=^TestRunnerDescendantHelper$"},
		Env: map[string]string{
			helperProcessDescendantEnv: "1",
			helperProcessPIDFileEnv:    pidFile,
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline exceeded", err)
	}
	if duration := time.Since(started); duration >= 2*time.Second {
		t.Fatalf("Run() returned after %s, want prompt process-tree cancellation", duration)
	}

	descendantPID, err := readHelperPID(pidFile)
	if err != nil {
		t.Fatalf("read descendant PID: %v", err)
	}

	state, err := waitForProcessStop(descendantPID, time.Second)
	if err != nil {
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)

		t.Fatalf("descendant process %d remains in state %q: %v", descendantPID, state, err)
	}
}

func TestRunnerStopsDescendantsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := (Runner{}).Run(ctx, Request{
			Name: executable,
			Args: []string{"-test.run=^TestRunnerDescendantHelper$"},
			Env: map[string]string{
				helperProcessDescendantEnv: "1",
				helperProcessPIDFileEnv:    pidFile,
			},
		})
		result <- err
	}()

	descendantPID, err := waitForHelperPID(pidFile, time.Second)
	if err != nil {
		cancel()
		t.Fatalf("wait for descendant PID: %v", err)
	}
	cancel()

	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)
		t.Fatal("Run() did not return after context cancellation")
	}

	state, err := waitForProcessStop(descendantPID, time.Second)
	if err != nil {
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)
		t.Fatalf("descendant process %d remains in state %q: %v", descendantPID, state, err)
	}
}

func TestRunnerBoundsWaitForDetachedDescendant(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()

	_, err = (Runner{}).Run(ctx, Request{
		Name: executable,
		Args: []string{"-test.run=^TestRunnerDescendantHelper$"},
		Env: map[string]string{
			helperProcessDescendantEnv: "1",
			helperProcessDetachedEnv:   "1",
			helperProcessPIDFileEnv:    pidFile,
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline exceeded", err)
	}
	if duration := time.Since(started); duration >= 2*time.Second {
		t.Fatalf("Run() returned after %s, want bounded pipe wait", duration)
	}

	descendantPID, err := readHelperPID(pidFile)
	if err != nil {
		t.Fatalf("read descendant PID: %v", err)
	}

	state, err := linuxProcessState(descendantPID)
	if err != nil {
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)

		t.Fatalf("read descendant process %d state: %v", descendantPID, err)
	}
	if state == 0 || state == 'Z' || state == 'X' {
		t.Fatalf("detached descendant process %d unexpectedly stopped", descendantPID)
	}

	if err := syscall.Kill(descendantPID, syscall.SIGKILL); err != nil {
		t.Fatalf("stop detached descendant process %d: %v", descendantPID, err)
	}
	if state, err = waitForProcessStop(descendantPID, time.Second); err != nil {
		t.Fatalf("detached descendant process %d remains in state %q: %v", descendantPID, state, err)
	}
}

//nolint:paralleltest // this helper runs only in a subprocess and creates a descendant.
func TestRunnerDescendantHelper(_ *testing.T) {
	if os.Getenv(helperProcessDescendantEnv) != "1" {
		return
	}

	executable, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}

	cmd := exec.Command(executable, "-test.run=^TestRunnerBlockingHelper$")
	cmd.Env = append(os.Environ(), helperProcessBlockEnv+"=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if os.Getenv(helperProcessDetachedEnv) == "1" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}

	err = cmd.Start()
	if err != nil {
		os.Exit(2)
	}

	if err = writeHelperPID(os.Getenv(helperProcessPIDFileEnv), cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()

		os.Exit(2)
	}

	if cmd.Wait() != nil {
		os.Exit(2)
	}
}

// writeHelperPID publishes the descendant's PID atomically. os.WriteFile
// creates the file and then writes it, so a reader polling for the file can
// open it in between and read nothing. waitForHelperPID then fails on a PID it
// cannot parse instead of retrying, which is a race it can only lose under
// load. Renaming into place leaves no state where the file exists but is not
// yet complete, and keeps an unparseable PID meaning what it should: a bug in
// this helper, not a poll that arrived early.
func writeHelperPID(path string, pid int) error {
	temporary := path + ".tmp"
	//nolint:gosec // The helper writes to the PID path this test itself set.
	if err := os.WriteFile(temporary, []byte(strconv.Itoa(pid)), 0o600); err != nil {
		return err
	}

	//nolint:gosec // The helper writes to the PID path this test itself set.
	return os.Rename(temporary, path)
}

func readHelperPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse PID: %w", err)
	}

	return pid, nil
}

func waitForHelperPID(path string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for {
		pid, err := readHelperPID(path)
		if err == nil {
			return pid, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		if time.Now().After(deadline) {
			return 0, errors.New("PID file was not created")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func waitForProcessStop(pid int, timeout time.Duration) (byte, error) {
	deadline := time.Now().Add(timeout)

	for {
		state, err := linuxProcessState(pid)
		if err != nil {
			return 0, err
		}
		if state == 0 || state == 'Z' || state == 'X' {
			return state, nil
		}
		if time.Now().After(deadline) {
			return state, errors.New("process did not stop")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func linuxProcessState(pid int) (byte, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	closingParenthesis := bytes.LastIndexByte(data, ')')
	if closingParenthesis < 0 || closingParenthesis+2 >= len(data) {
		return 0, errors.New("malformed process status")
	}

	return data[closingParenthesis+2], nil
}
