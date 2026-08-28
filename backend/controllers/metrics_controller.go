package controllers

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/metrics"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MetricsHandler serves the Prometheus text exposition at GET /metrics.
//
// The route is registered only when METRICS_TOKEN is configured (see
// routes.go) — the endpoint is opt-in and off by default for a self-hosted
// deployment. Every request must present `Authorization: Bearer <METRICS_TOKEN>`;
// the token is compared in constant time. There is no session/JWT path here on
// purpose: a Prometheus scraper authenticates with a static bearer credential,
// not a login cookie, and the metrics carry no per-user data.
//
// crypto/subtle is used only as a comparison helper (constant-time), not to
// select a cryptographic primitive — see docs/security/crypto-surface.ignore.
func MetricsHandler(cfg *config.Config, db *gorm.DB) gin.HandlerFunc {
	want := []byte(cfg.MetricsToken)
	return func(c *gin.Context) {
		got := bearerToken(c.GetHeader("Authorization"))
		if len(want) == 0 || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			// No body: nothing to learn from a rejected scrape.
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		metrics.SetRuntimeGauges()
		if sqlDB, err := db.DB(); err == nil {
			metrics.SetDBGauges(sqlDB.Stats())
		}
		metrics.SetStorageGauges(cfg.DBPath)

		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.Status(http.StatusOK)
		if err := metrics.Default().WritePrometheus(c.Writer); err != nil {
			logger.Ctx(c.Request.Context()).Error().Err(err).Msg("failed to write metrics exposition")
		}
	}
}

// bearerToken extracts the credential from an `Authorization: Bearer <token>`
// header value, tolerating scheme casing and surrounding whitespace. Returns
// "" when the header is absent or not a bearer credential.
func bearerToken(header string) string {
	const scheme = "bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return ""
	}
	return strings.TrimSpace(header[len(scheme):])
}
