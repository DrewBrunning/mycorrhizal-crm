package controllers

import (
	"context"
	"net/http"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
)

// integrityCheckBudget bounds one on-demand integrity sweep. The data pass is
// a bounded set of aggregate queries plus one paged scan of the Card column;
// this is the backstop so the endpoint cannot run unbounded on a very large
// instance.
const integrityCheckBudget = 60 * time.Second

// integrityCheckResponse is the combined payload: the storage-level PRAGMA
// pass and the application-level data-invariant pass, reported distinctly so
// an operator can tell "the disk is failing" from "the data has a logical
// hole" (issue #460).
type integrityCheckResponse struct {
	Timestamp string                          `json:"timestamp"`
	OK        bool                            `json:"ok"`
	Storage   services.StorageIntegrityReport `json:"storage"`
	Data      services.DataIntegrityReport    `json:"data"`
}

// RunIntegrityCheck handles GET /admin/integrity-check (DB-01, issue #460) —
// the admin-gated "is the data meaningful?" sweep an operator runs after a
// restore, a migration, or a bulk import. It folds the two SQLite structural
// pragmas and the per-invariant data checks (ADR 0012) into one report.
//
// Read-only and secret-free: findings carry table/column names and row counts
// only, never a value, an id, or a file path. Detection only — there is no
// repair over HTTP; that path is the `doctor` CLI behind an explicit
// -confirm. A broken database still returns 200 with `ok: false` — the
// diagnosis IS the payload. Instance-wide, not user-scoped, so it lives
// outside the user_id-scoped route group like the other /admin/* surfaces.
func RunIntegrityCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), integrityCheckBudget)
	defer cancel()

	db, _ := dbFromContext(c)
	cfg := currentConfig(c)

	out := integrityCheckResponse{Timestamp: time.Now().UTC().Format(time.RFC3339)}

	if db == nil {
		out.OK = false
		c.JSON(http.StatusOK, out)
		return
	}

	storage, sErr := services.RunStorageIntegrityChecks(db)
	out.Storage = storage
	if sErr != nil {
		logger.FromContext(c).Error().Err(sErr).Msg("integrity check: storage pass could not run")
	}

	data, dErr := services.RunDataIntegrityChecks(ctx, db, cfg)
	out.Data = data
	if dErr != nil {
		// A probe could not complete (not the same as a clean run that found
		// violations). data.OK is already false; log the cause, do not echo it
		// into the body beyond the per-finding details the service produced.
		logger.FromContext(c).Warn().Err(dErr).Msg("integrity check: a data-invariant probe could not complete")
	}

	out.OK = sErr == nil && dErr == nil && storage.OK && data.OK
	c.JSON(http.StatusOK, out)
}
