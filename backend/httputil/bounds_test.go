package httputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Issue #1491: mutation testing found these bounds survived arithmetic
// mutants. The behavioural tests only prove "a body of maxImageSize passes /
// +1 fails" relative to the constant itself, and nothing observes the client
// timeout without a hanging server. Pinning the literal values makes a
// changed bound a deliberate, reviewed edit.

func TestMaxImageSizeIsTenMiB(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 10*1024*1024, maxImageSize)
}

func TestBuildImageClientHasBoundedTimeout(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 15*time.Second, buildImageClient().Timeout,
		"an SSRF-guarded fetch must never run unbounded")
}

func TestDialTimeoutIsTenSeconds(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 10*time.Second, DialTimeout)
}
