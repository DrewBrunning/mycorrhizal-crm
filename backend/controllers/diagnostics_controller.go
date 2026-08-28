package controllers

import (
	"context"
	"net/http"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
)

// diagnosticsSweepBudget is the total wall-clock budget for one diagnostics
// sweep. Every individual probe is already time-bounded by the service
// (integrationProbeTimeout); this is the backstop so the whole endpoint — the
// DB reads plus the bounded fan-out over configured integration endpoints —
// cannot exceed a fixed time even on a large instance.
const diagnosticsSweepBudget = 45 * time.Second

// RunDiagnostics handles GET /admin/diagnostics (issue #423) — the
// admin-gated "is this install healthy?" sweep an operator runs after an
// install, an upgrade, a migration, or a config change. It folds the same
// single-check paths the continuous surfaces use into one ok / warning / error
// checklist with a summary ("2 warnings, 0 errors"). Read-only and secret-free
// (config values are never echoed; integration base URLs and transport errors
// go to the log only). Like the other /admin/* observability endpoints it is
// instance-wide and not user-scoped, so it is deliberately outside the
// user_id-scoped route group.
func RunDiagnostics(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), diagnosticsSweepBudget)
	defer cancel()

	db, _ := dbFromContext(c)
	cfg := currentConfig(c)

	out := services.RunDiagnostics(ctx, db, cfg)
	if err := ctx.Err(); err != nil {
		// The sweep exceeded its total budget. It already reports every
		// time-bounded probe's verdict; surface the truncation rather than
		// pretending the sweep fully completed.
		logger.FromContext(c).Warn().Err(err).Msg("diagnostics: sweep exceeded its time budget")
	}

	c.JSON(http.StatusOK, out)
}
