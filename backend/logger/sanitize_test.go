package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestSanitizeLogField(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "plain string unchanged", in: "/api/v1/contacts?page=2", want: "/api/v1/contacts?page=2"},
		{name: "non-ASCII printable unchanged", in: "grüße/東京", want: "grüße/東京"},
		{name: "newline escaped", in: "/\nFAKE", want: `/\nFAKE`},
		{name: "carriage return escaped", in: "/\rFAKE", want: `/\rFAKE`},
		{name: "tab escaped", in: "a\tb", want: `a\tb`},
		{name: "ANSI escape escaped", in: "\x1b[31mred", want: `\x1b[31mred`},
		{name: "c0 control escaped", in: "a\x00b", want: `a\x00b`},
		{name: "DEL escaped", in: "a\x7fb", want: `a\x7fb`},
		{name: "c1 control escaped", in: "a\x85b", want: `a\x85b`},
		{name: "all control classes", in: "\n\r\t\x00\x1b\x7f\x85", want: `\n\r\t\x00\x1b\x7f\x85`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SanitizeLogField(tt.in))
		})
	}
}

func TestSanitizeLogFieldOutputHasNoControlCharacters(t *testing.T) {
	inputs := []string{
		"/\nFAKE",
		"/\rFAKE",
		"\x1b[31mANSI\x1b[0m",
		strings.Repeat("\x01", 2000),
	}
	for _, in := range inputs {
		for _, r := range SanitizeLogField(in) {
			require.Falsef(t, isLogControl(r), "output must not contain control rune %q from input %q", r, in)
		}
	}
}

func TestSanitizeLogFieldTruncatesLongInput(t *testing.T) {
	in := strings.Repeat("x", maxLogFieldLen*2)
	out := SanitizeLogField(in)
	require.LessOrEqual(t, len([]rune(out)), maxLogFieldLen+3, "output must be capped at maxLogFieldLen runes plus the truncation marker")
	require.True(t, strings.HasSuffix(out, "..."), "truncated output should carry a visible marker")
	require.Equal(t, strings.Repeat("x", maxLogFieldLen)+"...", out)
}

func TestSanitizeLogFieldLongInputWithControls(t *testing.T) {
	// A long input must still have all its control characters escaped even
	// though it is truncated.
	in := strings.Repeat("y", maxLogFieldLen) + "\n" + strings.Repeat("z", 100)
	out := SanitizeLogField(in)
	require.False(t, strings.Contains(out, "\n"), "truncation must not leave a raw newline behind")
	require.NotContains(t, out, "\n")
}

func TestSanitizeLogFieldKeepsPrintablesUpToCap(t *testing.T) {
	in := strings.Repeat("a", maxLogFieldLen)
	require.Equal(t, in, SanitizeLogField(in), "exactly-cap-length printable input is unchanged")
}

func TestConsoleMessageInjection(t *testing.T) {
	// The console (pretty) writer prints the message verbatim — unlike JSON
	// fields, which are escaped — so a user-controlled value placed in the
	// message position is a real log-line injection in pretty mode. This is
	// why call sites (CardDAV/export diagnostics, request errors) sanitize
	// before Msg(). Prove the attack exists and that SanitizeLogField at the
	// call site neutralizes it.
	oldLevel := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	defer zerolog.SetGlobalLevel(oldLevel)

	unsanitized := &bytes.Buffer{}
	Logger = zerolog.New(zerolog.ConsoleWriter{Out: unsanitized, NoColor: true})
	Logger.Info().Msg("CardDAV PUT: forged\nFAKE")
	require.Equal(t, 2, strings.Count(unsanitized.String(), "\n"),
		"precondition: an unsanitized message must be able to forge a log line")

	sanitized := &bytes.Buffer{}
	Logger = zerolog.New(zerolog.ConsoleWriter{Out: sanitized, NoColor: true})
	Logger.Info().Msg("CardDAV PUT: " + SanitizeLogField("forged\nFAKE"))
	require.Equal(t, 1, strings.Count(sanitized.String(), "\n"),
		"sanitizing the user-controlled value must keep the log on a single line")

	jsonOut := &bytes.Buffer{}
	Logger = zerolog.New(jsonOut)
	Logger.Info().Str("path", "/\nFAKE").Msg("HTTP request")
	require.Equal(t, 1, strings.Count(jsonOut.String(), "\n"),
		"JSON output must remain single-line")
}
