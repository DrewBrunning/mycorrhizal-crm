package controllers

import (
	"encoding/json"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dateOnlyWire = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// TestCalendarOnlyFields_SerializeDateOnly pins issue #1778: every
// calendar-only derived field (cadence `next_due` on the dashboard and the
// contact briefing, data-decay `next_due` on the dashboard) goes over the wire
// as a date-only `YYYY-MM-DD`, never an RFC 3339 instant. A `…T00:00:00Z`
// value renders a day early on web for any user west of UTC. Reads the RAW
// JSON: decoding into the Go struct would round-trip either shape.
func TestCalendarOnlyFields_SerializeDateOnly(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/dashboard", GetDashboard)
	router.GET("/contacts/:id/briefing", GetContactBriefing)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Soren", Lastname: "Dahl"}
	require.NoError(t, db.Create(&contact).Error)

	activity := models.Activity{
		UserID: user.ID, Title: "Last call", Date: time.Now().AddDate(0, 0, -40),
		Type: models.InteractionTypeCall, Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&activity).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}).Error)

	decay := models.DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 30, Active: true}
	require.NoError(t, db.Create(&decay).Error)
	require.NoError(t, db.Model(&decay).UpdateColumn("created_at", time.Now().AddDate(0, 0, -40)).Error)

	get := func(path string) []byte {
		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		return w.Body.Bytes()
	}

	type healthRow struct {
		Health map[string]json.RawMessage `json:"health"`
	}
	nextDue := func(m map[string]json.RawMessage) string {
		var s string
		require.NoError(t, json.Unmarshal(m["next_due"], &s), "next_due must be a JSON string")
		return s
	}

	var dash struct {
		Overdue          []healthRow `json:"overdue"`
		DataDecayOverdue []healthRow `json:"data_decay_overdue"`
	}
	require.NoError(t, json.Unmarshal(get("/dashboard"), &dash))
	require.Len(t, dash.Overdue, 1)
	require.Len(t, dash.DataDecayOverdue, 1)
	assert.Regexp(t, dateOnlyWire, nextDue(dash.Overdue[0].Health), "dashboard overdue[].health.next_due")
	assert.Regexp(t, dateOnlyWire, nextDue(dash.DataDecayOverdue[0].Health), "dashboard data_decay_overdue[].health.next_due")

	var briefing struct {
		Cadence healthRow `json:"cadence"`
	}
	require.NoError(t, json.Unmarshal(get("/contacts/"+idString(contact.ID)+"/briefing"), &briefing))
	assert.Regexp(t, dateOnlyWire, nextDue(briefing.Cadence.Health), "briefing cadence.health.next_due")
}
