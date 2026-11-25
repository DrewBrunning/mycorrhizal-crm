package middleware

import (
	"os"
	"testing"

	"mycorrhizal/internal/lint/reasonprose"
)

// TestMain runs this package's tests under the reason-param prose guard
// (issue #2920 item 2): every reason any test here attaches is checked, so a
// prose param on a path no assertion inspects still fails the package.
func TestMain(m *testing.M) { os.Exit(reasonprose.RunGuarded(m)) }
