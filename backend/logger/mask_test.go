package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "whitespace only", in: "   ", want: ""},
		{name: "typical address", in: "alice.example@host.tld", want: "a***@host.tld"},
		{name: "single-char local part", in: "a@host.tld", want: "*@host.tld"},
		{name: "surrounding whitespace trimmed", in: "  bob@example.com  ", want: "b***@example.com"},
		{name: "plus addressing kept in local mask", in: "carol+news@example.com", want: "c***@example.com"},
		{name: "multiple @ uses last", in: "weird@name@example.com", want: "w***@example.com"},
		{name: "no domain part", in: "notanemail", want: "n***"},
		{name: "trailing @ has no domain", in: "user@", want: "u***"},
		{name: "leading @ has no local", in: "@example.com", want: "@***"},
		{name: "unicode local part", in: "élodie@example.fr", want: "é***@example.fr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, MaskEmail(tt.in))
		})
	}
}

func TestMaskEmailNeverEchoesLocalPart(t *testing.T) {
	// The identifying half of the address must never survive masking.
	for _, in := range []string{
		"sensitive.person@example.test",
		"first.last.2024@sub.domain.example",
		"reallylonglocalpartthatisunique@x.io",
	} {
		out := MaskEmail(in)
		at := len(out)
		for i, r := range out {
			if r == '@' {
				at = i
				break
			}
		}
		require.LessOrEqual(t, at, 4, "masked local part must be at most one rune plus the marker: %q", out)
		require.NotContains(t, out, "sensitive.person")
		require.NotContains(t, out, "first.last.2024")
		require.NotContains(t, out, "reallylonglocalpart")
	}
}
