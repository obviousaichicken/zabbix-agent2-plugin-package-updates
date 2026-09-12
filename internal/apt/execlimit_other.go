//go:build !linux

package apt

// execArgumentLimit returns the conservative floor on platforms that do not
// expose a stack-derived exec limit. The APT collector only ever runs on
// Linux; this exists so the package still builds elsewhere.
func execArgumentLimit() int {
	return fallbackExecArgumentLimit
}
