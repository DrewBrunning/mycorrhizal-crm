package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoteLabelFromContent(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"first line", "Title\nbody", "Title"},
		{"skips blank lines", "\n \n  Title  \nbody", "Title"},
		{"crlf", "Title\r\nbody", "Title"},
		{"collapses whitespace", "A \t  B", "A B"},
		{"drops control characters", "A\x00B\x07C", "ABC"},
		{"control-only first line is skipped", "\x00\x01\nReal", "Real"},
		{"empty", "", "Note"},
		{"whitespace only", " \n\t\n", "Note"},
		{"exactly the cap", strings.Repeat("a", ShareNoteLabelMaxRunes), strings.Repeat("a", ShareNoteLabelMaxRunes)},
		{"over the cap is cut with an ellipsis", strings.Repeat("a", ShareNoteLabelMaxRunes+1), strings.Repeat("a", ShareNoteLabelMaxRunes-1) + "…"},
		{"cap counts runes not bytes", strings.Repeat("日", 200), strings.Repeat("日", ShareNoteLabelMaxRunes-1) + "…"},
		{"cut trims trailing space before the ellipsis", strings.Repeat("a", ShareNoteLabelMaxRunes-2) + " bcd", strings.Repeat("a", ShareNoteLabelMaxRunes-2) + "…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := noteLabelFromContent(tc.in)
			assert.Equal(t, tc.want, got)
			assert.LessOrEqual(t, len([]rune(got)), ShareNoteLabelMaxRunes)
		})
	}
}
