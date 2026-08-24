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
		{name: "no query kept intact", in: "page=2&sort=name", want: "page=2&sort=name"},
		{name: "single sensitive key", in: "code=SECRET", want: "code=[REDACTED]"},
		{name: "sensitive key among others", in: "page=2&code=SECRET&sort=name", want: "page=2&code=[REDACTED]&sort=name"},
		{name: "first key sensitive", in: "token=abc&page=2", want: "token=[REDACTED]&page=2"},
		{name: "last key sensitive", in: "page=2&password=hunter2", want: "page=2&password=[REDACTED]"},
		{name: "all listed keys", in: "code=a&token=b&access_token=c&key=d&secret=e&password=f&signature=g",
			want: "code=[REDACTED]&token=[REDACTED]&access_token=[REDACTED]&key=[REDACTED]&secret=[REDACTED]&password=[REDACTED]&signature=[REDACTED]"},
		{name: "case-insensitive key", in: "Code=SECRET", want: "Code=[REDACTED]"},
		{name: "percent-encoded key", in: "c%6Fde=SECRET", want: "c%6Fde=[REDACTED]"},
		{name: "sensitive key with empty value", in: "code=&page=2", want: "code=[REDACTED]&page=2"},
		{name: "sensitive bare key has no value", in: "code&page=2", want: "code&page=2"},
		{name: "non-sensitive key named similarly", in: "coder=value", want: "coder=value"},
		{name: "semicolon separator", in: "page=2;code=SECRET", want: "page=2;code=[REDACTED]"},
		{name: "value with ampersand not split", in: "url=a%26b&code=SECRET", want: "url=a%26b&code=[REDACTED]"},
		{name: "raw ampersand in value belongs to pair", in: "next=a&b&code=SECRET", want: "next=a&b&code=[REDACTED]"},
		{name: "value with equals", in: "code=a=b&page=2", want: "code=[REDACTED]&page=2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, RedactQueryValues(tt.in))
		})
	}
}

func TestRedactQueryValuesDoesNotLeakSensitiveValues(t *testing.T) {
	// Every sensitive key must be scrubbed regardless of position or encoding.
	raw := "state=ok&code=TOP-SECRET&access_token=TOKEN_VALUE_XYZ&redirect_uri=https%3A%2F%2Fx"
	out := RedactQueryValues(raw)
	require.NotContains(t, out, "TOP-SECRET")
	require.NotContains(t, out, "TOKEN_VALUE_XYZ")
	require.Contains(t, out, "code=[REDACTED]")
	require.Contains(t, out, "access_token=[REDACTED]")
	require.Contains(t, out, "state=ok", "non-sensitive pairs must be preserved")
}
