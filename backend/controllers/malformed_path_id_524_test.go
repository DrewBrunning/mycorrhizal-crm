package controllers

import (
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMalformedPathID_Returns400NotServerError is a regression test for
// issue #524's "malformed path IDs -> GORM error -> 500" bug class: every
// handler below used to pass c.Param("id") straight into db.First(&x, id),
// so a non-integer id (Schemathesis fuzzed "null,null", huge ints, plain
// strings) produced a non-ErrRecordNotFound GORM/driver error that the
// existing ErrRecordNotFound-vs-else branch turned into a 500 instead of a
// 400. requirePathUintID (helpers.go) — used directly, or indirectly via
// resolveOwnedContactByID for the field-definition-controller-owned
// routes — now rejects the malformed id before any query runs.
//
// Hand-verified: reverting a handler's requirePathUintID call back to a bare
// `id := c.Param("id")` turns its subtest's 400 into a 500 here.
func TestMalformedPathID_Returns400NotServerError(t *testing.T) {
	_, router := setupRouter()

	router.GET("/contacts/:id", GetContact)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
	router.DELETE("/contacts/:id", DeleteContact)
	router.POST("/contacts/:id/archive", ArchiveContact)
	router.POST("/contacts/:id/unarchive", UnarchiveContact)
	router.POST("/contacts/:id/favorite", FavoriteContact)
	router.POST("/contacts/:id/unfavorite", UnfavoriteContact)
	router.POST("/contacts/:id/notes", withValidated(func() any { return &models.NoteInput{} }), CreateNote)
	router.GET("/contacts/:id/notes", GetNotesForContact)
	router.GET("/contacts/:id/activities", GetActivitiesForContact)
	router.GET("/contacts/:id/reminders", GetRemindersForContact)
	router.GET("/contacts/:id/reminder-completions", GetCompletionsForContact)
	router.GET("/contacts/:id/briefing", GetContactBriefing)
	router.GET("/contacts/:id/detail", GetContactDetail)
	router.GET("/contacts/:id/timeline", GetContactTimeline)
	router.GET("/contacts/:id/field-values", ListContactFieldValues)
	router.PUT("/contacts/:id/field-values", withValidated(func() any { return &models.ContactFieldValuesInput{} }), ReplaceContactFieldValues)

	router.GET("/activities/:id", GetActivity)
	router.PUT("/activities/:id", withValidated(func() any { return &models.ActivityInput{} }), UpdateActivity)
	router.DELETE("/activities/:id", DeleteActivity)

	router.GET("/notes/:id", GetNote)
	router.PUT("/notes/:id", withValidated(func() any { return &models.NoteInput{} }), UpdateNote)
	router.DELETE("/notes/:id", DeleteNote)

	router.GET("/reminders/:id", GetReminder)
	router.PUT("/reminders/:id", withValidated(func() any { return &models.Reminder{} }), UpdateReminder)
	router.DELETE("/reminders/:id", DeleteReminder)
	router.POST("/reminders/:id/complete", CompleteReminder)
	router.DELETE("/reminder-completions/:id", DeleteCompletion)

	cases := []struct {
		method string
		path   string
		body   bool // send an (empty-but-valid-JSON) body
	}{
		{"GET", "/contacts/not-a-number", false},
		{"PUT", "/contacts/not-a-number", true},
		{"DELETE", "/contacts/not-a-number", false},
		{"POST", "/contacts/not-a-number/archive", false},
		{"POST", "/contacts/not-a-number/unarchive", false},
		{"POST", "/contacts/not-a-number/favorite", false},
		{"POST", "/contacts/not-a-number/unfavorite", false},
		{"POST", "/contacts/not-a-number/notes", true},
		{"GET", "/contacts/not-a-number/notes", false},
		{"GET", "/contacts/not-a-number/activities", false},
		{"GET", "/contacts/not-a-number/reminders", false},
		{"GET", "/contacts/not-a-number/reminder-completions", false},
		{"GET", "/contacts/not-a-number/briefing", false},
		{"GET", "/contacts/not-a-number/detail", false},
		{"GET", "/contacts/not-a-number/timeline", false},
		{"GET", "/contacts/not-a-number/field-values", false},
		{"PUT", "/contacts/not-a-number/field-values", true},

		{"GET", "/activities/not-a-number", false},
		{"PUT", "/activities/not-a-number", true},
		{"DELETE", "/activities/not-a-number", false},

		{"GET", "/notes/not-a-number", false},
		{"PUT", "/notes/not-a-number", true},
		{"DELETE", "/notes/not-a-number", false},

		{"GET", "/reminders/not-a-number", false},
		{"PUT", "/reminders/not-a-number", true},
		{"DELETE", "/reminders/not-a-number", false},
		{"POST", "/reminders/not-a-number/complete", false},
		{"DELETE", "/reminder-completions/not-a-number", false},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var req *http.Request
			if tc.body {
				req, _ = http.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, _ = http.NewRequest(tc.method, tc.path, nil)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "%s %s: got %d, body %s", tc.method, tc.path, w.Code, w.Body.String())
		})
	}
}
