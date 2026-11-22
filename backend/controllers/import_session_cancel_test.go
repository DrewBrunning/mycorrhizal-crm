package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"io"
	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2664: closing/cancelling the import dialog left server sessions open,
// and the sixth upload then got "429 Too many in-progress imports" for 15
// minutes. The server half: an explicit idempotent cancel endpoint, and the
// cap evicting the user's oldest session instead of locking them out.

func uploadCSVSession(t *testing.T, router http.Handler) string {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "contacts.csv", csvFixture([3]string{"Alice", "Smith", "alice@example.com"})))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.ImportUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.SessionID
}

func cancelSession(router http.Handler, sessionID string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	body := map[string]string{"session_id": sessionID}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/contacts/import/cancel", jsonBody(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

// sessionIsLive probes a session through the preview endpoint: 200 while it
// exists, 404 once it is gone.
func sessionIsLive(t *testing.T, router http.Handler, sessionID string) bool {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/preview", models.ImportPreviewRequest{
		SessionID: sessionID,
		Mappings:  []models.ColumnMapping{{CSVColumn: "First Name", ContactField: "firstname"}},
	}))
	switch w.Code {
	case http.StatusOK:
		return true
	case http.StatusNotFound:
		return false
	}
	t.Fatalf("unexpected preview status %d: %s", w.Code, w.Body.String())
	return false
}

func TestCancelImportSession_SixSequentialUploadsWithCancelsAllSucceed(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	for i := 0; i < services.MaxImportSessionsPerUser+3; i++ {
		sid := uploadCSVSession(t, router)
		w := cancelSession(router, sid)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.False(t, sessionIsLive(t, router, sid), "a cancelled session is gone")
	}
}

func TestCancelImportSession_IsIdempotentAndQuietForUnknownSessions(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	sid := uploadCSVSession(t, router)

	for i := 0; i < 3; i++ {
		w := cancelSession(router, sid)
		require.Equal(t, http.StatusOK, w.Code, "repeat %d: %s", i, w.Body.String())
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "cancelled", resp["status"])
	}
	assert.Equal(t, http.StatusOK, cancelSession(router, "no-such-session").Code)
}

func TestCancelImportSession_RequiresSessionID(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	w := cancelSession(router, "")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCancelImportSession_CannotTouchAnotherUsersSession(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	victim := uploadCSVSession(t, router)

	other := models.User{Username: "cancel-other", Password: "password123!A", Email: "cancel-other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	otherRouter := gin.New()
	otherRouter.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", other.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	// registerImportRoutes would reset the shared manager and drop the victim's
	// session, so register only the two routes the other user needs.
	otherRouter.POST("/contacts/import/cancel", middleware.ValidateJSONMiddleware(&models.ImportCancelRequest{}), CancelImportSession)
	w := httptest.NewRecorder()
	raw, _ := json.Marshal(map[string]string{"session_id": victim})
	req := httptest.NewRequest(http.MethodPost, "/contacts/import/cancel", jsonBody(raw))
	req.Header.Set("Content-Type", "application/json")
	otherRouter.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "answers like an unknown session: no existence leak")

	assert.True(t, sessionIsLive(t, router, victim), "another user's cancel must not discard my session")
}

func TestUploadCSV_AtTheCapEvictsTheOldestInsteadOfLockingTheUserOut(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	sessions := make([]string, 0, services.MaxImportSessionsPerUser+1)
	for i := 0; i < services.MaxImportSessionsPerUser; i++ {
		sessions = append(sessions, uploadCSVSession(t, router))
	}
	// The sixth upload without any cancel used to be a 429 for 15 minutes.
	sessions = append(sessions, uploadCSVSession(t, router))

	assert.False(t, sessionIsLive(t, router, sessions[0]), "the oldest unconfirmed session is evicted")
	for _, sid := range sessions[1:] {
		assert.True(t, sessionIsLive(t, router, sid), "newer sessions survive")
	}
	assert.LessOrEqual(t, importSessions.CountActive(currentTestUserID(t, db)), services.MaxImportSessionsPerUser, "the memory bound of issue #415 still holds")
}

func jsonBody(raw []byte) io.Reader { return bytes.NewReader(raw) }

func currentTestUserID(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	var u models.User
	require.NoError(t, db.First(&u).Error)
	return u.ID
}
