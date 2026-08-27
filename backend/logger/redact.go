package logger

import (
	"net/url"
	"strings"
)

// safeQueryKeys is the allow-list of query-string keys whose values may be
// written verbatim to the request log. Everything else is redacted.
//
// This is an allow-list, not a deny-list, on purpose (issue #510, data
// minimization): the request log is instance-wide and attributes each line to
// a user, so a free-text value that rode along in the query string —
// `?search=<a contact's name>`, `?q=<words from a private note>`,
// `?email=<someone's address>` — is a cross-user disclosure of personal data
// even though every API handler is correctly `user_id`-scoped. A deny-list
// only redacts the key names someone thought to add; an allow-list redacts
// every key that has not been positively judged non-identifying. The keys here
// are pagination, sorting, and low-cardinality boolean/enum filters — the
// parts of a query string that are actually useful in a log and carry no PII.
// Internal record IDs (contact_id, vcard_uid, entity_id, …) are deliberately
// NOT on the list: they are linkable to a person and high-cardinality.
//
// Matched case-insensitively after percent-decoding the key. Keep this in sync
// with the query-parameter surface in controllers/ when a new non-PII operational
// parameter is added.
var safeQueryKeys = map[string]struct{}{
	// pagination / cursors
	"page": {}, "page_size": {}, "per_page": {}, "limit": {}, "offset": {},
	"skip": {}, "cursor": {}, "since": {},
	// ordering
	"sort": {}, "order": {}, "dir": {}, "direction": {},
	// low-cardinality filters / toggles
	"archived": {}, "include_archived": {}, "favorites": {}, "favorite": {},
	"has_contact_info": {}, "include": {}, "includes": {}, "include_contacts": {},
	"include_members": {}, "include_sensitive": {}, "include_snapshots": {},
	"sections": {}, "status": {}, "type": {}, "relation": {}, "kind": {},
	"bucket": {}, "depth": {}, "thumbnail": {}, "format": {},
	// client/protocol metadata
	"client": {}, "version": {}, "legacy": {}, "circle_legacy": {},
}

const redactedMarker = "[REDACTED]"

// RedactQueryValues rewrites a raw query string so it is safe to log: the value
// of every key NOT in safeQueryKeys is replaced with [REDACTED]. It preserves
// the order, separators, and every byte of a safe key's pair, so a useful query
// ("page=2&sort=name") is logged intact while "search=Ada%20Lovelace" becomes
// "search=[REDACTED]" and "code=SECRET" becomes "code=[REDACTED]".
//
// Keys are percent-decoded before the allow-list check so a caller cannot smuggle
// a value through by encoding the key name (e.g. "s%65arch"). A key that fails to
// decode is compared verbatim. Values are never decoded, so a value that itself
// contains '&' or ';' cannot split the pair.
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

// redactQueryPair redacts the value of a single key/value pair unless the key
// is allow-listed. seg is the pair without its trailing separator.
func redactQueryPair(seg string) string {
	eq := strings.IndexByte(seg, '=')
	if eq < 0 {
		// A bare key with no value: nothing to redact.
		return seg
	}
	keyRaw := seg[:eq]
	key, err := url.QueryUnescape(keyRaw)
	if err != nil {
		key = keyRaw
	}
	if _, safe := safeQueryKeys[strings.ToLower(key)]; safe {
		return seg
	}
	return keyRaw + "=" + redactedMarker
}
