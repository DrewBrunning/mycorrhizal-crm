package controllers

import (
	"context"
	"errors"

	apperrors "mycorrhizal/errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// requestDB returns the request-scoped *gorm.DB bound to the request's
// context (issue #1791). Every query (and every Go-side loop that checks
// services.statementContextErr) run through it stops when the client
// disconnects or the server's request deadline passes, instead of finishing
// minutes of work for a response nobody will read. Use it in the handlers
// whose work scales with the account (graph, dashboard).
func requestDB(c *gin.Context) *gorm.DB {
	return c.MustGet("db").(*gorm.DB).WithContext(c.Request.Context())
}

// abortQueryFailure aborts the request for a failed query. When the failure
// is the request's own context being cancelled -- the client went away -- it
// aborts without writing a response body or logging a 500 (there is no one to
// answer and it is not a server fault); anything else is the usual database
// error.
func abortQueryFailure(c *gin.Context, message string, err error) {
	if errors.Is(err, context.Canceled) {
		c.Abort()
		return
	}
	apperrors.AbortWithError(c, apperrors.ErrDatabase(message).WithError(err))
}
