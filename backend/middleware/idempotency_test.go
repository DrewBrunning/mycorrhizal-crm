package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

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
