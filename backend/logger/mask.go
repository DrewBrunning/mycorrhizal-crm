package logger

import "strings"

// MaskEmail reduces an email address to a form safe to write to an
// instance-wide log: the local part is replaced with its first character plus
// a fixed marker, and the domain is kept verbatim. "alice.example@host.tld"
// becomes "a***@host.tld"; a single-character local part becomes "*@host.tld".
//
// The rationale is data minimization (issue #510): delivery-diagnostic logs
// need to distinguish "this domain is failing" from "every domain is failing"
// and to correlate a burst of failures, both of which the domain alone
// supports — the local part is the identifying half and does not need to be
// stored. An input with no "@" is treated as an opaque token and masked
// wholesale (first character + marker) rather than echoed, so a malformed
// address can never leak in full either.
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}

	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		// No usable local@domain split: mask the whole thing.
		return maskLocal(email)
	}

	local := email[:at]
	domain := email[at+1:]
	return maskLocal(local) + "@" + domain
}

// maskLocal returns the first rune of s followed by "***", or "*" when s has
// a single rune. s is assumed non-empty.
func maskLocal(s string) string {
	runes := []rune(s)
	if len(runes) <= 1 {
		return "*"
	}
	return string(runes[0]) + "***"
}
