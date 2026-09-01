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
// few KB; anything larger is not cached (the row is dropped and the handler
// re-runs on retry) rather than bloating the table.
const maxCachedResponseBytes = 256 << 10 // 256 KiB

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
				apperrors.AbortWithError(c, apperrors.ErrIdempotencyInProgress())
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
		if status >= 200 && status < 300 && rc.body.Len() <= maxCachedResponseBytes {
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
			}
			return
		}

		// Non-2xx, or a response too large to cache: drop the pending row so a
		// corrected retry re-runs the handler rather than replaying a failure
		// (or getting stuck as permanently "in progress").
		if err := db.Where("id = ?", row.ID).Delete(&models.IdempotencyKey{}).Error; err != nil {
			logger.FromContext(c).Error().Err(err).Msg("idempotency: failed to drop pending row after non-2xx")
		}
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

func (w *responseCapture) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
