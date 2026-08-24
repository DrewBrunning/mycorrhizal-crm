package logger

import (
	"strings"
	"unicode/utf8"
)

// maxLogFieldLen caps how many runes of a user-controlled value are echoed
// into a log line, so a single oversized input (e.g. a megabyte query string)
// cannot flood the log stream.
const maxLogFieldLen = 1024

// SanitizeLogField makes a user-controlled value safe to embed in a log field
// or message. Control characters — newlines, carriage returns, tabs, ANSI
// escape sequences (ESC), and the rest of C0/C1 — are escaped to printable
// \xNN sequences so a crafted value cannot forge or corrupt log lines, and
// over-long values are truncated to maxLogFieldLen runes. Anything derived
// from a request, a database row, or an external system should pass through
// this before it reaches the logger: zerolog JSON-escapes structured fields,
// but the console writer prints the message verbatim, so a raw newline in the
// message position is a real log-line injection in pretty mode.
func SanitizeLogField(s string) string {
	if s == "" {
		return s
	}
	if len(s) <= maxLogFieldLen && !hasLogControl(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	kept := 0
	for len(s) > 0 {
		if kept >= maxLogFieldLen {
			b.WriteString("...")
			break
		}
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte: escape it by its raw value so a stray C0/C1
			// byte (e.g. a lone 0x85) cannot reach the terminal as a control.
			b.WriteString(escapeLogByte(s[0]))
		} else {
			switch {
			case r == '\n':
				b.WriteString(`\n`)
			case r == '\r':
				b.WriteString(`\r`)
			case r == '\t':
				b.WriteString(`\t`)
			case isLogControl(r):
				b.WriteString(escapeLogByte(byte(r)))
			default:
				// Preserve valid UTF-8 (including continuation bytes that fall
				// in the C1 range, which are not control characters here).
				b.WriteString(s[:size])
			}
		}
		s = s[size:]
		kept++
	}
	return b.String()
}

// hasLogControl is the fast-path scan. It works on bytes because a lone C1
// byte (0x80–0x9F) is invalid UTF-8 and never surfaces as a rune. Being
// conservative on valid multi-byte UTF-8 is fine: the full pass then
// preserves it untouched.
func hasLogControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if isLogControl(rune(s[i])) {
			return true
		}
	}
	return false
}

// isLogControl reports whether r is a control character: C0 (0x00–0x1F),
// DEL (0x7F), or C1 (0x80–0x9F). C1 covers stray bytes that some terminals
// interpret as escape sequences even without a preceding ESC.
func isLogControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0)
}

// escapeLogByte renders a control byte as a printable \xNN sequence.
func escapeLogByte(b byte) string {
	const hex = "0123456789abcdef"
	return `\x` + string([]byte{hex[b>>4], hex[b&0xf]})
}
