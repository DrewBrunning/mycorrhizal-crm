package perfbench

import (
	"errors"
	"net/http"
	"testing"

	"mycorrhizal/internal/largedata"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportViaDiscard_SurfacesNon200 drives an export "handler" that aborts
// with a 4xx so the status-check branch is exercised.
func TestExportViaDiscard_SurfacesNon200(t *testing.T) {
	_, err := exportViaDiscard(&Env{}, func(c *gin.Context) {
		c.String(http.StatusForbidden, "nope")
	}, "/export/boom")
	assert.ErrorContains(t, err, "status 403")
}

func TestExportViaDiscard_HappyPathCountsBytes(t *testing.T) {
	n, err := exportViaDiscard(&Env{}, func(c *gin.Context) {
		c.String(http.StatusOK, "hello world")
	}, "/export/ok")
	require.NoError(t, err)
	assert.Equal(t, int64(11), n)
}

// TestMeasureDataMovement_ErrorBranches covers the prepare-error and run-error
// paths — both return before the result is built, so a fixture-less Env is
// enough.
func TestMeasureDataMovement_ErrorBranches(t *testing.T) {
	env := &Env{Profile: largedata.Profile{Name: "boom"}}

	t.Run("prepare error", func(t *testing.T) {
		_, err := MeasureDataMovement(env, DataMovementOperation{
			Name:    "boom",
			Prepare: func(*Env, string) error { return errors.New("prep failed") },
			Run:     func(*Env, string) (int, int64, error) { return 0, 0, nil },
		}, t.TempDir())
		assert.ErrorContains(t, err, "prepare boom: prep failed")
	})

	t.Run("run error", func(t *testing.T) {
		_, err := MeasureDataMovement(env, DataMovementOperation{
			Name: "boom",
			Run:  func(*Env, string) (int, int64, error) { return 0, 0, errors.New("run failed") },
		}, t.TempDir())
		assert.ErrorContains(t, err, "run boom: run failed")
	})
}

// TestMeasureDataMovement_ProbeOpenError points the probe at a path under a
// directory that does not exist so newWriteProbe's CREATE TABLE fails.
func TestMeasureDataMovement_ProbeOpenError(t *testing.T) {
	env := &Env{Profile: largedata.Profile{Name: "boom"}, dbPath: "/nonexistent-dir-xyz/does/not/exist.db"}
	_, err := MeasureDataMovement(env, DataMovementOperation{
		Name:  "boom",
		Probe: true,
		Run:   func(*Env, string) (int, int64, error) { return 0, 0, nil },
	}, t.TempDir())
	assert.ErrorContains(t, err, "write probe for boom")
}
