package routes

// Part of issue #2060: the six-persona matrix in authorization_matrix_test.go
// probes non-GET item routes with an empty body, so a handler that validated
// the body before enforcing ownership could hide a BOLA behind a 400. These
// probes send a body that is valid for the *intruder* (their own contact, so
// no unrelated ownership check can mask the path-id one) and assert the
// owner's row is untouched. Each probe then repeats the request as the owner
// with an owner-valid body, proving the body shape really is accepted.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

type mutationProbe struct {
	name      string
	path      string
	intruder  any
	owner     any
	unchanged func(t *testing.T)
}

func TestAuthorizationMatrix_BOLAValidBodyMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := dbtest.New(t)
	db.Logger = logger.Default.LogMode(logger.Silent)

	cfg := testConfig()
	// Every request carries its own source IP, but this test is cheap enough
	// that raising the burst just keeps the global limiter out of the verdict.
	middleware.ConfigureAPIRateLimiter(time.Microsecond, 1_000_000)

	owner := models.User{Username: "mutation-owner", Email: "mutation-owner@example.com", Password: "password123"}
	require.NoError(t, db.Create(&owner).Error)
	intruder := models.User{Username: "mutation-intruder", Email: "mutation-intruder@example.com", Password: "password123"}
	require.NoError(t, db.Create(&intruder).Error)

	ownerToken, err := services.IssueSession(db, owner, cfg, "", "")
	require.NoError(t, err)
	intruderToken, err := services.IssueSession(db, intruder, cfg, "", "")
	require.NoError(t, err)

	// The intruder's own contact makes each body valid for the caller without
	// accidentally protecting the owner's row via a second, unrelated
	// ownership check inside the handler.
	intruderContact := models.Contact{UserID: intruder.ID, Firstname: "Intruder"}
	require.NoError(t, db.Create(&intruderContact).Error)
	ownerContact := models.Contact{UserID: owner.ID, Firstname: "Owner"}
	require.NoError(t, db.Create(&ownerContact).Error)

	res := seedResources(t, db, owner.ID)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	RegisterRoutes(router, cfg, db, nil)

	var noteBefore models.Note
	require.NoError(t, db.First(&noteBefore, "id = ?", res.note).Error)
	var activityBefore models.Activity
	require.NoError(t, db.First(&activityBefore, "id = ?", res.activity).Error)
	var lifeEventBefore models.LifeEvent
	require.NoError(t, db.First(&lifeEventBefore, "id = ?", res.lifeEvent).Error)
	var reminderBefore models.Reminder
	require.NoError(t, db.First(&reminderBefore, "id = ?", res.reminder).Error)

	probes := []mutationProbe{
		{
			name: "note",
			path: "/api/v1/notes/" + res.note,
			intruder: models.NoteInput{
				Content:   "intruder overwrite",
				Date:      time.Now(),
				ContactID: &intruderContact.ID,
			},
			owner: models.NoteInput{
				Content:   "owner edit",
				Date:      time.Now(),
				ContactID: &ownerContact.ID,
			},
			unchanged: func(t *testing.T) {
				var after models.Note
				require.NoError(t, db.First(&after, "id = ?", res.note).Error)
				assert.Equal(t, noteBefore.Content, after.Content)
				assert.Equal(t, noteBefore.Date.Unix(), after.Date.Unix())
				assert.Equal(t, noteBefore.ContactID, after.ContactID)
				assert.Equal(t, noteBefore.Revision, after.Revision)
			},
		},
		{
			name: "activity",
			path: "/api/v1/activities/" + res.activity,
			intruder: models.ActivityInput{
				Title:      "intruder overwrite",
				Date:       time.Now(),
				ContactIDs: []uint{intruderContact.ID},
			},
			owner: models.ActivityInput{
				Title:      "owner edit",
				Date:       time.Now(),
				ContactIDs: []uint{ownerContact.ID},
			},
			unchanged: func(t *testing.T) {
				var after models.Activity
				require.NoError(t, db.First(&after, "id = ?", res.activity).Error)
				assert.Equal(t, activityBefore.Title, after.Title)
				assert.Equal(t, activityBefore.Date.Unix(), after.Date.Unix())
				assert.Equal(t, activityBefore.Revision, after.Revision)
			},
		},
		{
			name: "life-event",
			path: "/api/v1/life-events/" + res.lifeEvent,
			intruder: models.LifeEventInput{
				EntityID: intruderContact.VCardUID,
				Type:     models.LifeEventTypeMoved,
			},
			owner: models.LifeEventInput{
				EntityID: ownerContact.VCardUID,
				Type:     models.LifeEventTypeMoved,
			},
			unchanged: func(t *testing.T) {
				var after models.LifeEvent
				require.NoError(t, db.First(&after, "id = ?", res.lifeEvent).Error)
				assert.Equal(t, lifeEventBefore.Type, after.Type)
				assert.Equal(t, lifeEventBefore.EntityID, after.EntityID)
				assert.Equal(t, lifeEventBefore.Revision, after.Revision)
			},
		},
		{
			name: "reminder",
			path: "/api/v1/reminders/" + res.reminder,
			intruder: models.Reminder{
				Message:    "intruder overwrite",
				RemindAt:   time.Now().Add(48 * time.Hour),
				Recurrence: "once",
				ContactID:  &intruderContact.ID,
			},
			owner: models.Reminder{
				Message:    "owner edit",
				RemindAt:   time.Now().Add(72 * time.Hour),
				Recurrence: "once",
				ContactID:  &ownerContact.ID,
			},
			unchanged: func(t *testing.T) {
				var after models.Reminder
				require.NoError(t, db.First(&after, "id = ?", res.reminder).Error)
				assert.Equal(t, reminderBefore.Message, after.Message)
				assert.Equal(t, reminderBefore.RemindAt.Unix(), after.RemindAt.Unix())
				assert.Equal(t, reminderBefore.Recurrence, after.Recurrence)
				assert.Equal(t, reminderBefore.ContactID, after.ContactID)
				assert.Equal(t, reminderBefore.Revision, after.Revision)
			},
		},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			w := putJSON(t, router, probe.path, intruderToken, probe.intruder)
			require.True(t,
				w.Code == http.StatusNotFound || w.Code == http.StatusForbidden,
				"intruder PUT with a valid body must be 404/403, got %d: %s", w.Code, w.Body.String())
			probe.unchanged(t)

			// Positive control: the same route/body shape succeeds for the row
			// owner, so the intruder's rejection above cannot be a validation
			// 400 dressed up as an authorization verdict.
			w = putJSON(t, router, probe.path, ownerToken, probe.owner)
			require.Equal(t, http.StatusOK, w.Code, "owner PUT: %s", w.Body.String())
		})
	}
}

func putJSON(t *testing.T, router http.Handler, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = uniqueTestClientIP() + ":1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
