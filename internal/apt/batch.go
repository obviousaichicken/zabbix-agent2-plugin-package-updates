package apt

import (
	"errors"
	"fmt"
)

const (
	// maxPolicyArgumentBytes is Linux's MAX_ARG_STRLEN, the ceiling on a
	// single argument regardless of how much total room an exec has.
	maxPolicyArgumentBytes = 32 * 4096

	// execArgumentPointerBytes is charged per argument on top of its bytes.
	// The kernel copies an argv pointer array onto the new process stack and
	// charges it against the same budget as the strings. Measured on Linux
	// with an 8 MiB stack (ARG_MAX 2 MiB): 1536 KiB of strings across 62914
	// arguments execs, while 1900 KiB across 77824 fails with E2BIG. Neither
	// result fits an accounting that counts only the strings; both fit once
	// eight bytes per argument are included.
	execArgumentPointerBytes = 8

	// fallbackExecArgumentLimit is the POSIX _POSIX_ARG_MAX floor every
	// system must provide, used when the live limit cannot be read.
	fallbackExecArgumentLimit = 128 << 10

	// maxExecArgumentLimit caps the derived budget so an unusually large
	// stack limit cannot produce an argument list far beyond anything a
	// package set needs.
	maxExecArgumentLimit = 8 << 20

	// execArgumentSafetyDivisor keeps the batch well inside the limit. The
	// kernel's own accounting includes the environment and per-exec overhead
	// this package cannot observe exactly, and exceeding the limit fails the
	// whole check, so half the reported limit buys margin that costs nothing:
	// even the conservative floor still admits thousands of packages per
	// call.
	execArgumentSafetyDivisor = 2
)

// policyArgumentBudget reports the bytes available for package arguments in
// one apt-cache policy invocation, after reserving the environment and the
// fixed leading arguments.
//
// The budget is derived from the live exec limit rather than hard-coded. A
// fixed limit is necessarily a guess, and guessing low is not free: every
// extra batch is another apt-cache process that reloads the entire APT cache,
// which dominates collection time.
func policyArgumentBudget(execLimit int, environmentBytes int, fixedArguments []string) int {
	budget := execLimit / execArgumentSafetyDivisor
	budget -= environmentBytes
	for _, argument := range fixedArguments {
		budget -= len(argument) + 1 + execArgumentPointerBytes
	}

	// Always leave room for at least one maximum-length argument so a single
	// pathological package name cannot make batching impossible.
	if budget < maxPolicyArgumentBytes {
		return maxPolicyArgumentBytes
	}

	return budget
}

// BatchPolicyArguments splits installed packages into apt-cache policy
// argument batches that fit within budgetBytes. Each argument is charged its
// bytes, its terminating NUL, and its argv pointer.
func BatchPolicyArguments(packages []InstalledPackage, budgetBytes int) ([][]string, error) {
	if budgetBytes <= 0 {
		return nil, errors.New("apt-cache policy argument budget must be positive")
	}
	if len(packages) == 0 {
		return make([][]string, 0), nil
	}

	batches := make([][]string, 0, 1)
	batch := make([]string, 0, len(packages))
	batchBytes := 0
	seen := make(map[string]struct{}, len(packages))

	for _, pkg := range packages {
		if !validPackageName(pkg.Name) || !validArchitecture(pkg.Architecture) {
			return nil, errors.New("invalid package identity for apt-cache policy")
		}
		key := packageKey(pkg.Name, pkg.Architecture)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("duplicate apt-cache policy package %s:%s", pkg.Name, pkg.Architecture)
		}
		seen[key] = struct{}{}

		argument := pkg.Name + ":" + pkg.Architecture
		if len(argument)+1 > maxPolicyArgumentBytes {
			return nil, errors.New("apt-cache policy argument exceeds byte limit")
		}
		argumentBytes := len(argument) + 1 + execArgumentPointerBytes
		// An argument that cannot fit an empty batch would otherwise be
		// emitted anyway and fail the exec, so refuse it here instead.
		if argumentBytes > budgetBytes {
			return nil, fmt.Errorf(
				"apt-cache policy argument for %s:%s does not fit the argument budget",
				pkg.Name,
				pkg.Architecture,
			)
		}
		if len(batch) != 0 && batchBytes+argumentBytes > budgetBytes {
			batches = append(batches, batch)
			batch = make([]string, 0, len(packages)-len(seen)+1)
			batchBytes = 0
		}
		batch = append(batch, argument)
		batchBytes += argumentBytes
	}
	if len(batch) != 0 {
		batches = append(batches, batch)
	}

	return batches, nil
}
