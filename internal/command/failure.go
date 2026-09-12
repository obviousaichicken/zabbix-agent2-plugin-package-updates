package command

import (
	"net/url"
	"strings"
	"unicode"
)

// maxDiagnosticBytes bounds one diagnostic line. Long enough for a package
// manager's error sentence, short enough that it cannot carry a payload into
// the agent log.
const maxDiagnosticBytes = 200

// Failure is implemented by safe, structured external-command errors.
// Implementations must not expose credentials, unbounded arguments, or raw
// stderr through Operation or Diagnostic.
type Failure interface {
	error
	Operation() string
	Status() int
	IsTimeout() bool
	IsCanceled() bool

	// Diagnostic returns one bounded, credential-redacted line explaining
	// why the command failed, or an empty string when the command produced
	// nothing usable. Without it an operator only learns that some command
	// exited non-zero, which is never enough to act on.
	Diagnostic() string
}

// Diagnostic reduces command stderr to a single line that is safe to log.
//
// Package manager stderr routinely contains repository URLs, and those URLs
// can carry credentials in their userinfo or query string, so redaction is
// implemented once here rather than left to each backend. The alternative
// each backend reached for independently - keeping everything, or dropping
// everything - is either unsafe or useless.
func Diagnostic(stderr []byte) string {
	line := selectDiagnosticLine(string(stderr))
	if line == "" {
		return ""
	}

	line = redactURLs(line)
	line = strings.TrimSpace(stripControl(line))
	if len(line) > maxDiagnosticBytes {
		line = strings.TrimSpace(line[:maxDiagnosticBytes]) + "..."
	}

	return line
}

// selectDiagnosticLine prefers a line the tool itself marked as an error,
// because package managers emit warnings and progress before the failure that
// actually matters. It falls back to the first non-empty line.
func selectDiagnosticLine(stderr string) string {
	fallback := ""
	for _, raw := range strings.Split(stderr, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		if isErrorMarked(line) {
			return line
		}
		if fallback == "" {
			fallback = line
		}
	}

	return fallback
}

func isErrorMarked(line string) bool {
	lowered := strings.ToLower(line)
	for _, marker := range []string{"e:", "error:", "error ", "fatal:", "failed"} {
		if strings.HasPrefix(lowered, marker) {
			return true
		}
	}

	return false
}

// redactURLs strips userinfo and query strings from every URL-shaped token,
// which is where repository credentials and access tokens live.
func redactURLs(line string) string {
	fields := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return line
	}

	replaced := line
	for _, field := range fields {
		trimmed := strings.Trim(field, "'\"()[]<>,;")
		if !strings.Contains(trimmed, "://") {
			continue
		}
		safe := redactURL(trimmed)
		if safe != trimmed {
			replaced = strings.ReplaceAll(replaced, trimmed, safe)
		}
	}

	return replaced
}

func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		// Unparseable but URL-shaped: drop everything after the authority
		// rather than risk emitting an embedded secret.
		if index := strings.Index(raw, "://"); index >= 0 {
			return raw[:index+3] + "redacted"
		}

		return raw
	}
	if parsed.User != nil {
		parsed.User = url.User("redacted")
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""

	return parsed.String()
}

func stripControl(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}

		return r
	}, value)
}
