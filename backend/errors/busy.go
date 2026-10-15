package errors

import (
	"errors"
	"net/http"
	"strconv"

	sqlitedriver "github.com/glebarez/go-sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrCodeDatabaseBusy signals that the request's write could not get
// SQLite's write lock within busy_timeout (5s, database.openDSN) because
// other writers held it — the server is overloaded, not broken. Issue #1720:
// this used to surface as an opaque 500 DATABASE_ERROR indistinguishable from
// a real bug, so neither a client nor the soak harness could tell "retry
// shortly" from "something is wrong". RFC 9110's 503 Service Unavailable,
// with a Retry-After hint.
const ErrCodeDatabaseBusy = "DATABASE_BUSY"

// DatabaseBusyRetryAfterSeconds is the Retry-After hint on a DATABASE_BUSY
// response. The server has already waited the full busy_timeout before
// giving up, and lock holders are short transactions, so the lock is almost
// always free again within a second; repeated failure is the client's own
// backoff to govern, not something a longer fixed hint fixes.
const DatabaseBusyRetryAfterSeconds = 1

// ErrDatabaseBusy returns the 503 a lock-timeout write maps to.
func ErrDatabaseBusy() *AppError {
	return NewError(ErrCodeDatabaseBusy, "The database is busy; retry the request shortly", http.StatusServiceUnavailable)
}

// IsDatabaseBusy reports whether err (or anything it wraps) is SQLite's
// SQLITE_BUSY — the write lock was still held when busy_timeout expired.
// Extended codes (SQLITE_BUSY_SNAPSHOT, SQLITE_BUSY_RECOVERY, …) carry the
// primary code in their low byte and count too. The concrete driver type is
// matched rather than an `interface{ Code() int }` so an unrelated error that
// happens to expose a Code method can never be mistaken for a lock timeout.
func IsDatabaseBusy(err error) bool {
	var se *sqlitedriver.Error
	if !errors.As(err, &se) {
		return false
	}
	return se.Code()&0xff == sqlite3.SQLITE_BUSY
}

// busyOverride turns a 500 whose underlying cause is a lock timeout into the
// retryable 503. Only a 500 is rewritten: a handler that already chose a
// specific status (404, 412, 409 …) keeps it. Details (e.g. "operation") and
// the wrapped error carry over, so the log line still names what failed and
// why.
func busyOverride(err *AppError) *AppError {
	if err.HTTPStatus != http.StatusInternalServerError || !IsDatabaseBusy(err.Err) {
		return err
	}
	busy := ErrDatabaseBusy().WithError(err.Err)
	for k, v := range err.Details {
		busy.WithDetails(k, v)
	}
	return busy
}

// retryAfterHeader is the header value RespondWithError sets on a
// DATABASE_BUSY response.
var retryAfterHeader = strconv.Itoa(DatabaseBusyRetryAfterSeconds)
