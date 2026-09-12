//go:build linux

package apt

import "syscall"

// execArgumentLimit reports the total bytes of argv plus envp one exec may
// carry, which Linux derives from the stack limit as RLIMIT_STACK/4 and
// reports through sysconf(_SC_ARG_MAX). Reading the live limit matters
// because a service unit may lower LimitSTACK well below the 8 MiB default,
// which would make any hard-coded budget unsafe on exactly the hosts that
// can least afford a failed check.
func execArgumentLimit() int {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_STACK, &limit); err != nil {
		return fallbackExecArgumentLimit
	}
	// RLIM_INFINITY means the kernel applies its own ceiling rather than a
	// stack-derived one, so the conservative fallback is the honest answer.
	if limit.Cur == ^uint64(0) || limit.Cur == 0 {
		return fallbackExecArgumentLimit
	}

	quarter := limit.Cur / 4
	if quarter > uint64(maxExecArgumentLimit) {
		return maxExecArgumentLimit
	}
	if quarter < uint64(fallbackExecArgumentLimit) {
		return fallbackExecArgumentLimit
	}

	return int(quarter)
}
