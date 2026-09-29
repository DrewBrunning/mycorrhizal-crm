package controllers

import (
	"net/http"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
)

// AssetLinksHandler serves GET /.well-known/assetlinks.json, the Digital Asset
// Links document Google fetches to associate the Android app with this
// instance's domain so Credential Manager can create and use passkeys for the
// RP ID (ADR 0034, issue #1293).
//
// The effective state is resolved once, when the route is built. When native
// Android passkeys are not effective the route answers a bare 404 — never an
// empty or placeholder statement. When effective it is a plain 200 JSON body:
// unauthenticated, no redirect, and it sets no cookie.
func AssetLinksHandler(cfg *config.Config) gin.HandlerFunc {
	body := cfg.AndroidPasskeys().AssetLinksJSON()
	return func(c *gin.Context) {
		if body == nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "application/json", body)
	}
}
