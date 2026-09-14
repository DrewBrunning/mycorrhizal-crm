package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IdempotencyKeyHeader is the request header a client sets to make a
// non-idempotent POST safe to retry (CON-04, issue #459, ADR 0010).
const IdempotencyKeyHeader = "Idempotency-Key"

// idempotencyReplayedHeader is set on a replayed response so a client (or a
// test) can tell a cached replay from a freshly-executed request.
const idempotencyReplayedHeader = "Idempotency-Replayed"

// maxIdempotencyKeyLen bounds the client-supplied token. 255 is generous for a
// UUID/ULID/random string and keeps the row small.
const maxIdempotencyKeyLen = 255

// maxCachedResponseBytes caps what is stored for replay. A create response is a
// few KB; anything larger is not cached (the key is marked
// terminal-but-unreplayable, never re-run) rather than bloating the table.
const maxCachedResponseBytes = 256 << 10 // 256 KiB

// idempotencyPendingTimeout bounds how long a claimed-but-unfinished key may be
// reported as "in progress". No live request can outlive the HTTP server's
// write timeout, and that timeout is validated to at most 300s
// (config.Config.WriteTimeout), so a pending row older than this cannot still
// be in flight: it is one whose response store never landed. Such a row is
// terminal (the handler's write already committed) and must not be re-run
// (issue #995) — see ErrIdempotencyResultUnavailable.
const idempotencyPendingTimeout = 10 * time.Minute

// IdempotencyMiddleware implements the one CON-04 mechanism: a client-supplied
// Idempotency-Key, the stored outcome of the request that first used it, and a
// verbatim replay of that outcome for every later request with the same
// (user, key). It composes with retries the client never told us about — the
// ambiguous failure where the write committed but the response was lost.
//
// It is inert unless the request is a POST carrying an Idempotency-Key, so it
// is safe to install once on the whole authenticated group: GET/PUT/PATCH/
// DELETE and un-keyed POSTs pass straight through (PUT/DELETE are already
// idempotent; ADR 0010 explains why POST is the only method that needs this).
//
// Flow on a keyed POST:
//   - claim the key with INSERT ... ON CONFLICT DO NOTHING (the unique index
//     idx_idempotency_keys_user_key makes this race-safe)
//   - claimed  -> run the handler, capture the response, store it on 2xx
//     (delete the pending row on non-2xx so a corrected retry is allowed)
//   - not claimed -> look at the existing row:
//   - different request fingerprint -> 422 IDEMPOTENCY_KEY_REUSED
//   - still pending                  -> 409 IDEMPOTENCY_IN_PROGRESS
//   - completed                      -> replay stored status + body
//   - completed with no stored response (the response store failed, or the
//     body was too large to cache) -> 409 IDEMPOTENCY_RESULT_UNAVAILABLE:
//     terminal, never re-runs the handler (the write already committed). A
//     pending row older than idempotencyPendingTimeout is treated the same
//     way, so a lost response store can never wedge a key "in progress" until
//     the TTL purge and then double-apply on the first retry after it (#995).
func IdempotencyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(IdempotencyKeyHeader)
		if c.Request.Method != http.MethodPost || key == "" {
			c.Next()
			return
		}
		if len(key) > maxIdempotencyKeyLen {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput(IdempotencyKeyHeader,
				"must be at most 255 characters"))
			return
		}

		userIDVal, ok := c.Get("userID")
		if !ok {
			// Unauthenticated: AuthMiddleware will reject it. Nothing to key on.
			c.Next()
			return
		}
		userID, ok := userIDVal.(uint)
		if !ok {
			c.Next()
			return
		}

		db := c.MustGet("db").(*gorm.DB)

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrValidation("Failed to read request body"))
			return
		}
		_ = c.Request.Body.Close()
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		fingerprint := fingerprintRequest(c.Request.Method, c.FullPath(), body)

		now := time.Now().UTC()
		row := models.IdempotencyKey{
			UserID:             userID,
			Key:                key,
			Method:             c.Request.Method,
			Path:               c.FullPath(),
			RequestFingerprint: fingerprint,
			State:              models.IdempotencyStatePending,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		claim := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if claim.Error != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to record idempotency key").WithError(claim.Error))
			return
		}

		if claim.RowsAffected == 0 {
			// Someone else claimed this key first (an earlier attempt, or a
			// concurrent retry). Serve from the stored outcome.
			var existing models.IdempotencyKey
			if err := db.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&existing).Error; err != nil {
				apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to load idempotency key").WithError(err))
				return
			}
			if existing.RequestFingerprint != fingerprint {
				apperrors.AbortWithError(c, apperrors.ErrIdempotencyKeyReused())
				return
			}
			if existing.State != models.IdempotencyStateCompleted {
				// A pending row is "still being processed" only while it can
				// plausibly be in flight. Past idempotencyPendingTimeout it
				// lost its response store (or the process died after
				// claiming the key); the handler's write may already have
				// committed, so this is terminal — never 409 "retry shortly"
				// forever, and never a re-run (issue #995).
				if existing.State == models.IdempotencyStatePending &&
					time.Since(existing.CreatedAt) <= idempotencyPendingTimeout {
					apperrors.AbortWithError(c, apperrors.ErrIdempotencyInProgress())
					return
				}
				apperrors.AbortWithError(c, apperrors.ErrIdempotencyResultUnavailable())
				return
			}
			if existing.ResponseStatus == 0 {
				// Completed, but no response was stored for replay: the
				// terminal-but-unreplayable marker (issue #995). Re-running
				// the handler would double-apply a write that already
				// committed, so refuse rather than replay or re-run.
				apperrors.AbortWithError(c, apperrors.ErrIdempotencyResultUnavailable())
				return
			}
			replayStoredResponse(c, existing)
			return
		}

		// We own the key. Run the handler with the response captured.
		rc := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = rc
		c.Next()
		c.Writer = rc.ResponseWriter

		status := rc.Status()
		if status >= 200 && status < 300 {
			if rc.body.Len() <= maxCachedResponseBytes {
				upd := db.Model(&models.IdempotencyKey{}).
					Where("id = ?", row.ID).
					Updates(map[string]any{
						"state":           models.IdempotencyStateCompleted,
						"response_status": status,
						"response_body":   rc.body.String(),
						"updated_at":      time.Now().UTC(),
					})
				if upd.Error != nil {
					logger.FromContext(c).Error().Err(upd.Error).Msg("idempotency: failed to store response for replay")
					markTerminalUnreplayable(c, db, row.ID)
				}
				return
			}
			// A 2xx whose body is too large to cache. The handler's write
			// committed; dropping the key here would let a retry re-run the
			// handler and double-apply (issue #995), so mark it terminal
			// instead of caching the oversized body.
			logger.FromContext(c).Warn().Int("bytes", rc.body.Len()).
				Msg("idempotency: response too large to cache; marking key terminal")
			markTerminalUnreplayable(c, db, row.ID)
			return
		}

		// Non-2xx: drop the pending row so a corrected retry re-runs the
		// handler rather than replaying a failure (or getting stuck as
		// permanently "in progress").
		if err := db.Where("id = ?", row.ID).Delete(&models.IdempotencyKey{}).Error; err != nil {
			logger.FromContext(c).Error().Err(err).Msg("idempotency: failed to drop pending row after non-2xx")
		}
	}
}

// markTerminalUnreplayable flips a claimed key to the terminal-but-unreplayable
// outcome: state=completed with response_status=0 and no stored body. A later
// retry is answered IDEMPOTENCY_RESULT_UNAVAILABLE instead of re-running the
// handler (whose write already committed) or being told "in progress" until the
// TTL purge, after which a retry would double-apply (issue #995).
//
// It is a raw UPDATE, not a model save: it must repair the row even when the
// failure was in the GORM response-store path, and it deliberately touches only
// the state columns. The `response_status = 0` predicate means it can never
// clobber a response the primary store actually wrote (the ambiguous case where
// the UPDATE committed but the driver still returned an error). Best-effort: if
// it also fails the row stays pending, and the bounded pending-timeout read path
// above still refuses to re-run it.
func markTerminalUnreplayable(c *gin.Context, db *gorm.DB, id uint) {
	res := db.Exec(
		"UPDATE idempotency_keys SET state = ?, response_status = 0, response_body = '', updated_at = ? WHERE id = ? AND response_status = 0",
		models.IdempotencyStateCompleted, time.Now().UTC(), id)
	if res.Error != nil {
		logger.FromContext(c).Error().Err(res.Error).Msg("idempotency: failed to mark key terminal after response-store failure")
	}
}

// fingerprintRequest hashes the parts of a request that must match for a
// replay to be the right answer: method, the route template (not the concrete
// path — they are the same for a keyed POST, which has no path params), and
// the raw body.
func fingerprintRequest(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{'\n'})
	h.Write([]byte(path))
	h.Write([]byte{'\n'})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func replayStoredResponse(c *gin.Context, row models.IdempotencyKey) {
	c.Header(idempotencyReplayedHeader, "true")
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.String(row.ResponseStatus, "%s", row.ResponseBody)
	c.Abort()
}

// responseCapture tees the handler's response into a buffer so a 2xx body can
// be stored for replay, while still writing through to the real client.
type responseCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseCapture) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// WriteString is an interface-conformance shim: it exists only to satisfy
// gin.ResponseWriter's WriteString method (gin-gonic/gin@v1.12.0/response_writer.go:37).
// Gin's rendering never calls it — render.WriteString funnels through
// fmt.Fprintf(w, ...), which ends in Write — and no handler in this codebase
// calls c.Writer.WriteString directly. Provably dead, kept only for the
// interface.
func (w *responseCapture) WriteString(s string) (int, error) {
	w.body.WriteString(s)                  // # pragma: no cover — interface-conformance shim gin never invokes; see the method comment
	return w.ResponseWriter.WriteString(s) // # pragma: no cover — interface-conformance shim gin never invokes; see the method comment
}
