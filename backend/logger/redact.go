package logger

import (
	"net/url"
	"strings"
)

// sensitiveQueryKeys are query-string keys whose values must never reach a log
// line. They cover the OAuth/OIDC `code` exchanged on the callback endpoint
// plus the generic credential-shaped key names that should never appear in a
// URL. A value for any of these keys (matched case-insensitively, after
// percent-decoding the key) is replaced with [REDACTED].
var sensitiveQueryKeys = map[string]struct{}{
	"code":         {},
	"token":        {},
	"access_token": {},
	"key":          {},
	"secret":       {},
	"password":     {},
	"signature":    {},
}

// RedactQueryValues scrubs the values of sensitive query-string keys from a raw
// query string before it is logged. It preserves the order, separators, and
// every non-sensitive byte of the input — only the value portion of a sensitive
// key/value pair is replaced — so a legitimately useful query (e.g.
// "page=2&sort=name") is logged intact while "code=SECRET" becomes
// "code=[REDACTED]".
//
// Keys are percent-decoded before the sensitive-key check so a caller cannot
// bypass redaction by encoding the key name (e.g. "c%6Fde"). A key that fails
// to decode is compared verbatim. Values are never decoded, so a value that
// itself contains an '&' or ';' cannot split the pair and hide a secret.
func RedactQueryValues(raw string) string {
	if raw == "" {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); {
		j := i
		for j < len(raw) && raw[j] != '&' && raw[j] != ';' {
			j++
		}
		b.WriteString(redactQueryPair(raw[i:j]))
		if j < len(raw) {
			b.WriteByte(raw[j])
			j++
		}
		i = j
	}
	return b.String()
}

// redactQueryPair redacts the value of a single key/value pair if the key is
// sensitive. seg is the pair without its trailing separator.
func redactQueryPair(seg string) string {
	eq := strings.IndexByte(seg, '=')
	if eq < 0 {
		return seg
	}
	keyRaw := seg[:eq]
	key, err := url.QueryUnescape(keyRaw)
	if err != nil {
		key = keyRaw
	}
	if _, sensitive := sensitiveQueryKeys[strings.ToLower(key)]; sensitive {
		return keyRaw + "=[REDACTED]"
	}
	return seg
}
