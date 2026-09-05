package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idempotencyTestRouter wires the middleware against a real migrated schema
// (dbtest — CLAUDE.md backend trap #1; the ON CONFLICT claim depends on the
// real idx_idempotency_keys_user_key unique index, which AutoMigrate would not
// reproduce from the model tags) plus a POST handler that mints one Note row
// and echoes a body, and increments a side-effect counter.
func idempotencyTestRouter(t *testing.T) (*gin.Engine, *gorm.DB, *int64) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "idem", Password: "password123!A", Email: "idem@example.com"}
	require.NoError(t, db.Create(&user).Error)

	var sideEffects int64
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	r.Use(IdempotencyMiddleware())
	r.POST("/api/v1/things", func(c *gin.Context) {
		n := models.Note{UserID: user.ID, Content: "made by handler"}
		if err := db.Create(&n).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		atomic.AddInt64(&sideEffects, 1)
		c.JSON(http.StatusCreated, gin.H{"id": n.ID, "content": n.Content})
	})
	r.POST("/api/v1/always-400", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nope"})
	})
	return r, db, &sideEffects
}

func TestIdempotency_NoKey_PassesThrough(t *testing.T) {
	r, db, se := idempotencyTestRouter(t)

	do := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(`{}`)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusCreated, do().Code)
	require.Equal(t, http.StatusCreated, do().Code)

	// No key -> no dedup: two rows, two side effects, nothing stored.
	var notes, keys int64
	require.NoError(t, db.Model(&models.Note{}).Count(&notes).Error)
	require.NoError(t, db.Model(&models.IdempotencyKey{}).Count(&keys).Error)
	assert.EqualValues(t, 2, notes)
	assert.EqualValues(t, 2, atomic.LoadInt64(se))
	assert.EqualValues(t, 0, keys)
}

func TestIdempotency_SameKey_ReplaysExactlyOnce(t *testing.T) {
	r, db, se := idempotencyTestRouter(t)

	do := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(`{"n":1}`)))
		req.Header.Set("Idempotency-Key", "abc-123")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	first := do()
	require.Equal(t, http.StatusCreated, first.Code)
	require.Empty(t, first.Header().Get("Idempotency-Replayed"))

	second := do()
	require.Equal(t, http.StatusCreated, second.Code)
	assert.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))
	assert.JSONEq(t, first.Body.String(), second.Body.String(), "replay is byte-for-byte the first response")

	third := do()
	assert.Equal(t, "true", third.Header().Get("Idempotency-Replayed"))
	assert.JSONEq(t, first.Body.String(), third.Body.String())

	// Exactly one row, one side effect, one stored key.
	var notes, keys int64
	require.NoError(t, db.Model(&models.Note{}).Count(&notes).Error)
	require.NoError(t, db.Model(&models.IdempotencyKey{}).Count(&keys).Error)
	assert.EqualValues(t, 1, notes, "the retried create produced exactly one row")
	assert.EqualValues(t, 1, atomic.LoadInt64(se), "the handler side effect fired exactly once")
	assert.EqualValues(t, 1, keys)

	var stored models.IdempotencyKey
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, models.IdempotencyStateCompleted, stored.State)
	assert.Equal(t, http.StatusCreated, stored.ResponseStatus)
	assert.Equal(t, "POST", stored.Method)
	assert.Equal(t, "/api/v1/things", stored.Path)
}

func TestIdempotency_SameKey_DifferentBody_422(t *testing.T) {
	r, _, _ := idempotencyTestRouter(t)
	mk := func(body string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(body)))
		req.Header.Set("Idempotency-Key", "reused-key")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusCreated, mk(`{"n":1}`).Code)

	clash := mk(`{"n":2}`)
	require.Equal(t, http.StatusUnprocessableEntity, clash.Code, clash.Body.String())
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(clash.Body.Bytes(), &env))
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", env.Error.Code)
}

func TestIdempotency_Non2xx_NotCached_RetryReruns(t *testing.T) {
	r, db, _ := idempotencyTestRouter(t)
	mk := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/api/v1/always-400", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Idempotency-Key", "k-400")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusBadRequest, mk().Code)
	// The pending row was dropped, so a retry runs the handler again (still 400,
	// not a replayed cached response).
	second := mk()
	require.Equal(t, http.StatusBadRequest, second.Code)
	assert.Empty(t, second.Header().Get("Idempotency-Replayed"))

	var keys int64
	require.NoError(t, db.Model(&models.IdempotencyKey{}).Count(&keys).Error)
	assert.EqualValues(t, 0, keys, "a non-2xx outcome leaves no cached key")
}

func TestIdempotency_KeyScopedPerRequestShape(t *testing.T) {
	r, _, _ := idempotencyTestRouter(t)
	// A GET is never keyed; a PUT/PATCH/DELETE passes straight through even
	// with the header. Only the POST path caches.
	req, _ := http.NewRequest("DELETE", "/api/v1/things", nil)
	req.Header.Set("Idempotency-Key", "ignored")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// No DELETE route registered -> 404, and crucially not a 5xx from the
	// middleware trying to key a non-POST.
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestIdempotency_ConcurrentRetries_OneWins(t *testing.T) {
	r, db, se := idempotencyTestRouter(t)

	const n = 8
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(`{"x":1}`)))
			req.Header.Set("Idempotency-Key", "race-key")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			codes[idx] = w.Code
		}(i)
	}
	wg.Wait()

	// Every caller gets a usable answer: the winner a 201, the losers a 201
	// replay or a 409 "in progress" (never a 5xx, never a duplicate 201 that
	// ran the handler).
	created := 0
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			// acceptable: retry-shortly
		default:
			t.Fatalf("unexpected status under concurrent retry: %d", code)
		}
	}
	require.GreaterOrEqual(t, created, 1)

	var notes int64
	require.NoError(t, db.Model(&models.Note{}).Count(&notes).Error)
	assert.EqualValues(t, 1, notes, "concurrent retries of one keyed create produced exactly one row")
	assert.EqualValues(t, 1, atomic.LoadInt64(se), "and exactly one side effect")
}

// --- addressable-coverage-gap tests (issue #809) ---------------------------

// assertIdempotencyErrorCode decodes the standard AppError envelope written by
// RespondWithError (mycorrhizal/errors) and asserts the error code.
func assertIdempotencyErrorCode(t *testing.T, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, code, env.Error.Code)
}

// TestIdempotency_KeyTooLong_Rejected pins the length guard: an
// Idempotency-Key longer than 255 chars is a client error, rejected before any
// DB work or handler run. The guard responds via apperrors.ErrInvalidInput,
// whose real status is 400 (not 422 — the issue text said 422, the constructor
// yields http.StatusBadRequest).
func TestIdempotency_KeyTooLong_Rejected(t *testing.T) {
	r, db, se := idempotencyTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Idempotency-Key", strings.Repeat("x", maxIdempotencyKeyLen+1))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assertIdempotencyErrorCode(t, w, "INVALID_INPUT")
	assert.EqualValues(t, 0, atomic.LoadInt64(se), "an over-long key must not reach the handler")

	var keys int64
	require.NoError(t, db.Model(&models.IdempotencyKey{}).Count(&keys).Error)
	assert.EqualValues(t, 0, keys, "an over-long key must not be stored")
}

// idempotencyKeyedPOSTRouter builds a bare engine running the middleware over a
// POST route whose handler flips *ran and answers 204. It deliberately wires no
// "userID"/"db" context values — used to prove the middleware's defensive
// pass-through guards fire before any DB access.
func idempotencyKeyedPOSTRouter(t *testing.T, pre func(c *gin.Context), ran *bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	if pre != nil {
		r.Use(func(c *gin.Context) { pre(c); c.Next() })
	}
	r.Use(IdempotencyMiddleware())
	r.POST("/api/v1/passthrough", func(c *gin.Context) {
		*ran = true
		c.Status(http.StatusNoContent)
	})
	return r
}

func keyedPOST(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("POST", path, bytes.NewReader([]byte(`{"n":1}`)))
	req.Header.Set("Idempotency-Key", "some-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestIdempotency_NoUserID_PassesThrough pins the unauthenticated-passthrough
// guard: with no userID in the gin context there is nothing to key on, so the
// middleware must let the request through untouched (AuthMiddleware downstream
// is what actually rejects an unauthenticated caller).
func TestIdempotency_NoUserID_PassesThrough(t *testing.T) {
	var ran bool
	r := idempotencyKeyedPOSTRouter(t, nil, &ran)

	w := keyedPOST(t, r, "/api/v1/passthrough")

	assert.True(t, ran, "without a userID the middleware must not swallow a keyed POST")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Header().Get("Idempotency-Replayed"))
}

// TestIdempotency_NonUintUserID_PassesThrough pins the defensive type-assertion
// guard: a userID that is present but not a uint is equally unusable as a key,
// so the request passes through rather than panicking on the type assertion.
func TestIdempotency_NonUintUserID_PassesThrough(t *testing.T) {
	var ran bool
	r := idempotencyKeyedPOSTRouter(t, func(c *gin.Context) {
		c.Set("userID", "not-a-uint")
	}, &ran)

	w := keyedPOST(t, r, "/api/v1/passthrough")

	assert.True(t, ran, "a non-uint userID must not crash or swallow the request")
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// errBodyReader is an io.Reader whose Read always fails, standing in for a
// request body that errors mid-read (a truncated or broken upload).
type errBodyReader struct{}

func (errBodyReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated body read failure")
}

// TestIdempotency_BodyReadError_Rejected pins the io.ReadAll guard: a body that
// fails to read is a 400 (apperrors.ErrValidation) and the handler never runs.
func TestIdempotency_BodyReadError_Rejected(t *testing.T) {
	r, _, se := idempotencyTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/v1/things", io.NopCloser(errBodyReader{}))
	req.Header.Set("Idempotency-Key", "body-read-fail")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assertIdempotencyErrorCode(t, w, "VALIDATION_ERROR")
	assert.EqualValues(t, 0, atomic.LoadInt64(se), "an unreadable body must not reach the handler")
}

// TestIdempotency_ClaimInsertError_500 pins the claim-INSERT error branch: when
// the row cannot be recorded the request is aborted 500 with DATABASE_ERROR,
// never run, and never silently replayed.
func TestIdempotency_ClaimInsertError_500(t *testing.T) {
	r, db, se := idempotencyTestRouter(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Idempotency-Key", "claim-fail")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assertIdempotencyErrorCode(t, w, "DATABASE_ERROR")
	assert.EqualValues(t, 0, atomic.LoadInt64(se), "a failed claim must not reach the handler")
}

// TestIdempotency_ExistingKeyReadError_500 pins the existing-row read branch:
// when the claim loses the race (RowsAffected == 0) and the subsequent load of
// the stored row fails, the request is aborted 500 rather than panicking.
//
// The claim INSERT must succeed (reporting "no row inserted" because the key is
// already taken) while the follow-up SELECT fails — closing the whole DB would
// fail the claim first and land in TestIdempotency_ClaimInsertError_500's
// branch instead. So the read is poisoned with a GORM query callback that
// errors on every query, registered after the conflicting row is seeded (the
// claim is an INSERT and is unaffected).
func TestIdempotency_ExistingKeyReadError_500(t *testing.T) {
	r, db, se := idempotencyTestRouter(t)
	var user models.User
	require.NoError(t, db.Where("username = ?", "idem").First(&user).Error)

	key := "read-fail"
	require.NoError(t, db.Create(&models.IdempotencyKey{
		UserID: user.ID, Key: key, Method: "POST", Path: "/api/v1/things",
		RequestFingerprint: "fingerprint-of-something", State: models.IdempotencyStateCompleted,
		ResponseStatus: http.StatusCreated, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error)

	require.NoError(t, db.Callback().Query().Before("gorm:query").
		Register("idempotency_test_fail_existing_read", func(tx *gorm.DB) {
			tx.AddError(errors.New("simulated read failure"))
		}))

	req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assertIdempotencyErrorCode(t, w, "DATABASE_ERROR")
	assert.EqualValues(t, 0, atomic.LoadInt64(se), "a failed existing-row read must not reach the handler")
}

// TestIdempotency_ResponseStoreUpdateError_Logged pins the response-store
// UPDATE branch: a 2xx whose outcome cannot be persisted (the underlying DB is
// closed by the handler mid-request, after the claim succeeded) is logged, not
// surfaced to the client as an error — the handler already produced its
// answer.
func TestIdempotency_ResponseStoreUpdateError_Logged(t *testing.T) {
	r, db, _ := idempotencyTestRouter(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	buf := &bytes.Buffer{}
	captureLogger(t, buf, false)

	r.POST("/api/v1/close-db-201", func(c *gin.Context) {
		_ = sqlDB.Close() // the claim INSERT already ran; break the store-UPDATE
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("POST", "/api/v1/close-db-201", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Idempotency-Key", "store-fail")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "the handler's own 2xx must still reach the client")
	assert.Contains(t, buf.String(), "idempotency: failed to store response for replay")
}

// TestIdempotency_PendingRowDeleteError_Logged pins the pending-row DELETE
// branch: a non-2xx whose cleanup delete fails is logged and the corrected
// retry is left to re-run the handler next time.
func TestIdempotency_PendingRowDeleteError_Logged(t *testing.T) {
	r, db, _ := idempotencyTestRouter(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	buf := &bytes.Buffer{}
	captureLogger(t, buf, false)

	r.POST("/api/v1/close-db-400", func(c *gin.Context) {
		_ = sqlDB.Close() // the claim INSERT already ran; break the cleanup DELETE
		c.JSON(http.StatusBadRequest, gin.H{"error": "nope"})
	})

	req, _ := http.NewRequest("POST", "/api/v1/close-db-400", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Idempotency-Key", "delete-fail")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, buf.String(), "idempotency: failed to drop pending row after non-2xx")
}

// TestIdempotency_PendingRow_409InProgress pins the in-progress branch: a key
// whose stored row is still pending means an earlier attempt with the same
// fingerprint is running, so the retry is answered 409 (never re-runs the
// handler, never replays a half-written outcome).
func TestIdempotency_PendingRow_409InProgress(t *testing.T) {
	r, db, se := idempotencyTestRouter(t)
	var user models.User
	require.NoError(t, db.Where("username = ?", "idem").First(&user).Error)

	body := []byte(`{"n":1}`)
	key := "in-progress-key"
	require.NoError(t, db.Create(&models.IdempotencyKey{
		UserID: user.ID, Key: key, Method: "POST", Path: "/api/v1/things",
		RequestFingerprint: fingerprintRequest("POST", "/api/v1/things", body),
		State:              models.IdempotencyStatePending,
		CreatedAt:          time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error)

	req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	assertIdempotencyErrorCode(t, w, "IDEMPOTENCY_IN_PROGRESS")
	assert.EqualValues(t, 0, atomic.LoadInt64(se), "an in-progress key must not re-run the handler")
	assert.Empty(t, w.Header().Get("Idempotency-Replayed"))
}
