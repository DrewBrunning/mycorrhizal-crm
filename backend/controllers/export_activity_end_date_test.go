package controllers

import (
	"encoding/csv"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2006: the flat CSV export's ACTIVITIES section carries an "End Date"
// column (RFC 3339 like "Date", empty for a single-day activity). It is
// appended after the existing columns so positional consumers do not shift.
func TestExportCSVActivitiesCarryEndDate(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.GET("/export", ExportData)

	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.Activity{UserID: user.ID, Title: "Trip", Date: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC), EndDate: &end}).Error)
	require.NoError(t, db.Create(&models.Activity{UserID: user.ID, Title: "Lunch", Date: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)}).Error)

	req, _ := http.NewRequest("GET", "/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	_, after, ok := strings.Cut(w.Body.String(), "=== ACTIVITIES ===\n")
	require.True(t, ok)
	section, _, _ := strings.Cut(after, "\n=== ")
	rows, err := csv.NewReader(strings.NewReader(section)).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 3)

	header := rows[0]
	assert.Equal(t, []string{"ID", "Title", "Description", "Location", "Date", "Contact Names", "Created At", "Updated At", "End Date"}, header)
	byTitle := map[string][]string{}
	for _, r := range rows[1:] {
		byTitle[r[1]] = r
	}
	assert.Equal(t, "2026-05-15T00:00:00Z", byTitle["Trip"][8])
	assert.Equal(t, "", byTitle["Lunch"][8])
}
