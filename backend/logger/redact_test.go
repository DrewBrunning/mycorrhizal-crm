package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactQueryValues(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "allow-listed pair kept intact", in: "page=2&sort=name", want: "page=2&sort=name"},
		{name: "search term redacted", in: "search=Ada%20Lovelace", want: "search=[REDACTED]"},
		{name: "q redacted", in: "q=words+from+a+private+note", want: "q=[REDACTED]"},
		{name: "email redacted", in: "email=someone@example.com", want: "email=[REDACTED]"},
		{name: "mixed safe and unsafe", in: "page=2&search=Bob&sort=name", want: "page=2&search=[REDACTED]&sort=name"},
		{name: "credential-shaped keys still redacted", in: "code=a&token=b&access_token=c&key=d&secret=e&password=f&signature=g",
			want: "code=[REDACTED]&token=[REDACTED]&access_token=[REDACTED]&key=[REDACTED]&secret=[REDACTED]&password=[REDACTED]&signature=[REDACTED]"},
		{name: "internal ids redacted", in: "contact_id=42&vcard_uid=abc-123&entity_id=x", want: "contact_id=[REDACTED]&vcard_uid=[REDACTED]&entity_id=[REDACTED]"},
		{name: "oidc state redacted", in: "state=csrf-token-value&code=authcode", want: "state=[REDACTED]&code=[REDACTED]"},
		{name: "case-insensitive allow-list", in: "Page=2", want: "Page=2"},
		{name: "case-insensitive redaction", in: "Search=Bob", want: "Search=[REDACTED]"},
		{name: "percent-encoded key cannot smuggle", in: "s%65arch=Bob", want: "s%65arch=[REDACTED]"},
		{name: "bare key untouched", in: "search&page=2", want: "search&page=2"},
		{name: "empty value on unsafe key", in: "search=&page=2", want: "search=[REDACTED]&page=2"},
		{name: "semicolon separator", in: "page=2;search=Bob", want: "page=2;search=[REDACTED]"},
		{name: "value with ampersand not split", in: "search=a%26b&page=2", want: "search=[REDACTED]&page=2"},
		{name: "raw ampersand in value belongs to pair", in: "next=a&b&search=Bob", want: "next=[REDACTED]&b&search=[REDACTED]"},
		{name: "value with equals", in: "search=a=b&page=2", want: "search=[REDACTED]&page=2"},
		{name: "boolean filters kept", in: "archived=true&include_sensitive=false&favorites=1", want: "archived=true&include_sensitive=false&favorites=1"},
		{name: "cursor and since kept", in: "cursor=eyJpZCI6MTB9&since=2026-01-01", want: "cursor=eyJpZCI6MTB9&since=2026-01-01"},
		{name: "undecodable key still redacted", in: "s%zzarch=Bob&page=2", want: "s%zzarch=[REDACTED]&page=2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RedactQueryValues(tt.in))
		})
	}
}

func TestRedactQueryValuesDoesNotLeakUnsafeValues(t *testing.T) {
	// Every non-allow-listed value must be scrubbed regardless of position.
	raw := "page=1&search=Ada+Lovelace&email=ada@example.com&code=TOP-SECRET&sort=name"
	out := RedactQueryValues(raw)
	require.NotContains(t, out, "Ada")
	require.NotContains(t, out, "Lovelace")
	require.NotContains(t, out, "ada@example.com")
	require.NotContains(t, out, "TOP-SECRET")
	require.Contains(t, out, "page=1", "allow-listed pairs must be preserved")
	require.Contains(t, out, "sort=name", "allow-listed pairs must be preserved")
	require.Contains(t, out, "search=[REDACTED]")
	require.Contains(t, out, "email=[REDACTED]")
}
