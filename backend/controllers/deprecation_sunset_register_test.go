package controllers

// The RFC 8594 Sunset header a body-keyed deprecation sends is a promise to
// API clients, and docs/deprecations.md's not-before column is the same
// promise to operators. Both are hand-written constants, so moving one (for
// example when a release date slips and the 90-day window has to move) must
// fail until the other moves with it.

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// registerNotBefore returns the not-before column of the docs/deprecations.md
// register row whose id is id.
func registerNotBefore(t *testing.T, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "deprecations.md"))
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		cells := strings.Split(line, "|")
		// | id | surface | summary | deprecated-in | deprecated-on | replacement | earliest-removal | not-before | status | tracking |
		if len(cells) < 12 || strings.TrimSpace(cells[1]) != id {
			continue
		}
		return strings.TrimSpace(cells[8])
	}
	t.Fatalf("docs/deprecations.md has no register row %q", id)
	return ""
}

func TestDeprecationSunsetHeadersMatchRegister(t *testing.T) {
	for _, tc := range []struct {
		id     string
		sunset string
	}{
		{"flat-lockout-429-body", flatLockoutDeprecationSunset},
		{"preference-category-hobby", hobbyPreferenceDeprecationSunset},
	} {
		t.Run(tc.id, func(t *testing.T) {
			sunset, err := http.ParseTime(tc.sunset)
			require.NoError(t, err, "Sunset must be an HTTP-date")
			require.Equal(t, registerNotBefore(t, tc.id), sunset.UTC().Format("2006-01-02"),
				"the Sunset header and the register's not-before date must name the same day")
		})
	}
}
