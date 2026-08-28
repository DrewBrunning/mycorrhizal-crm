package middleware

import (
	"strconv"
	"time"

	"mycorrhizal/metrics"

	"github.com/gin-gonic/gin"
)

// MetricsMiddleware records per-request Prometheus metrics: request count and
// latency by method + matched route template + status, plus an in-flight
// gauge. The route label is c.FullPath() — the registered template
// ("/api/v1/contacts/:id"), never the concrete path — so a request against an
// unbounded ID space cannot explode label cardinality. An unmatched request
// (404 with no route) is labelled "unmatched" for the same reason.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		metrics.HTTPInFlightInc()
		defer metrics.HTTPInFlightDec()

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		metrics.HTTPRequest(method, route, strconv.Itoa(c.Writer.Status()))
		metrics.HTTPObserve(method, route, time.Since(start).Seconds())
	}
}
