package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/clock"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2762 / ADR 0015 Rule 4 (viewer-relative "today" for display): the
// display endpoints accept an optional ?tz=<IANA zone>. The tests pin the
// request clock and leave the server on its default (UTC) reminder zone, so
// the viewer's zone is the only variable.

func viewerTZRouter(t *testing.T, at time.Time) (*gin.Engine, uint, func(model interface{})) {
	t.Helper()
	db, router := setupRouter(t)
	withClock(router, clock.NewFake(at))
	registerOccasionRoutes(router)
	router.GET("/dashboard", GetDashboard)
	router.GET("/contacts/birthdays", GetUpcomingBirthdays)
	router.GET("/reminders/upcoming", GetUpcomingReminders)
	router.GET("/occasion-obligations/gift-shopping-list", GetGiftShoppingList)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	return router, user.ID, func(model interface{}) { require.NoError(t, db.Create(model).Error) }
}

type daysResp struct {
	Occasions []struct {
		DaysUntil int `json:"days_until"`
	} `json:"occasions"`
	GiftList []struct {
		DaysUntil int `json:"days_until"`
	} `json:"gift_shopping_list"`
	Birthdays []struct {
		Birthday string `json:"birthday"`
	} `json:"birthdays"`
}

func getViewerTZ(t *testing.T, router *gin.Engine, path string) daysResp {
	t.Helper()
	w := doOccasionGET(router, path)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp daysResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func occasionDays(r daysResp) []int {
	out := []int{}
	for _, o := range r.Occasions {
		out = append(out, o.DaysUntil)
	}
	return out
}

func birthdayDates(r daysResp) []string {
	out := []string{}
	for _, b := range r.Birthdays {
		out = append(out, b.Birthday)
	}
	return out
}

// 01:30Z on 17 Nov is 19:30 on 16 Nov in Chicago (CST): a 16 Nov birthday is
// today there but already past on the server's UTC day (#2679).
func TestViewerTZ_BehindServerZoneKeepsLocalToday(t *testing.T) {
	router, uid, create := viewerTZRouter(t, time.Date(2026, 11, 17, 1, 30, 0, 0, time.UTC))
	create(&models.Contact{UserID: uid, Firstname: "Soren", Birthday: "1979-11-16"})

	assert.Empty(t, occasionDays(getViewerTZ(t, router, "/occasions/upcoming?days=30")), "without tz the UTC day is unchanged")
	assert.Equal(t, []int{0}, occasionDays(getViewerTZ(t, router, "/occasions/upcoming?days=30&tz=America/Chicago")))

	assert.NotContains(t, birthdayDates(getViewerTZ(t, router, "/dashboard")), "1979-11-16")
	assert.Contains(t, birthdayDates(getViewerTZ(t, router, "/dashboard?tz=America/Chicago")), "1979-11-16")

	assert.NotContains(t, birthdayDates(getViewerTZ(t, router, "/contacts/birthdays")), "1979-11-16")
	assert.Contains(t, birthdayDates(getViewerTZ(t, router, "/contacts/birthdays?tz=America/Chicago")), "1979-11-16")
}

// 12:30Z on 16 Nov is 01:30 on 17 Nov in Auckland (NZDT): a 17 Nov birthday
// is today there (days_until 0) and tomorrow on the UTC day.
func TestViewerTZ_AheadOfServerZoneCountsDownFromLocalToday(t *testing.T) {
	router, uid, create := viewerTZRouter(t, time.Date(2026, 11, 16, 12, 30, 0, 0, time.UTC))
	eve := models.Contact{UserID: uid, Firstname: "Eve", Birthday: "1990-11-17"}
	create(&eve)
	create(&models.OccasionObligation{
		UserID: uid, EntityID: eve.VCardUID, Kind: "gift", Label: "Birthday gift",
		AnchorMonth: intPtr(11), AnchorDay: intPtr(17), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	})

	assert.Equal(t, 1, occasionDays(getViewerTZ(t, router, "/occasions/upcoming?days=30"))[0])
	assert.Equal(t, 0, occasionDays(getViewerTZ(t, router, "/occasions/upcoming?days=30&tz=Pacific/Auckland"))[0])

	gifts := getViewerTZ(t, router, "/occasion-obligations/gift-shopping-list?days=30")
	require.NotEmpty(t, gifts.GiftList)
	assert.Equal(t, 1, gifts.GiftList[0].DaysUntil)
	gifts = getViewerTZ(t, router, "/occasion-obligations/gift-shopping-list?days=30&tz=Pacific/Auckland")
	assert.Equal(t, 0, gifts.GiftList[0].DaysUntil)
}

// The viewer zone follows its own offset on a DST-change day: 06:30Z on
// 8 Mar 2026 (the US spring-forward day) is 01:30 EST in New York, but still
// 22:30 PST on 7 Mar in Los Angeles.
func TestViewerTZ_DSTBoundary(t *testing.T) {
	router, uid, create := viewerTZRouter(t, time.Date(2026, 3, 8, 6, 30, 0, 0, time.UTC))
	create(&models.Contact{UserID: uid, Firstname: "Dst", Birthday: "1980-03-08"})

	// 06:30Z on 8 Mar is 01:30 EST on 8 Mar (the clocks jump at 02:00 local).
	assert.Equal(t, []int{0}, occasionDays(getViewerTZ(t, router, "/occasions/upcoming?days=30&tz=America/New_York")))
	// Los Angeles is still on 7 Mar (22:30 PST), so the birthday is tomorrow.
	assert.Equal(t, []int{1}, occasionDays(getViewerTZ(t, router, "/occasions/upcoming?days=30&tz=America/Los_Angeles")))
}

func TestViewerTZ_EmptyTZIsUnchangedDefault(t *testing.T) {
	router, uid, create := viewerTZRouter(t, time.Date(2026, 11, 17, 1, 30, 0, 0, time.UTC))
	create(&models.Contact{UserID: uid, Firstname: "Eve", Birthday: "1990-11-17"})
	assert.Equal(t, []int{0}, occasionDays(getViewerTZ(t, router, "/occasions/upcoming?days=30&tz=")))
}

func TestViewerTZ_InvalidZoneRejectedEverywhere(t *testing.T) {
	router, _, _ := viewerTZRouter(t, time.Date(2026, 11, 17, 1, 30, 0, 0, time.UTC))
	for _, route := range []string{
		"/dashboard", "/occasions/upcoming", "/contacts/birthdays",
		"/reminders/upcoming", "/occasion-obligations/gift-shopping-list",
	} {
		for _, tz := range []string{"Mars/Olympus", "  ", "Not A Zone", "../etc/passwd", "Local", strings.Repeat("A", 80), "America/Chicago%00"} {
			w := doOccasionGET(router, route+"?tz="+strings.ReplaceAll(tz, " ", "%20"))
			env := mzErrorCode(t, w, http.StatusBadRequest, "INVALID_INPUT")
			assert.Contains(t, w.Body.String(), `"reason_code":"validation_timezone_invalid"`, "%s tz=%q", route, tz)
			assert.Equal(t, "tz", env.Error.Details["field"], "%s tz=%q", route, tz)
		}
		w := doOccasionGET(router, route+"?tz=Europe/Berlin")
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", route, w.Body.String())
	}
}
