package controllers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestGetCircleTimeline_MergesMemberTimelinesOnly pins the Part 2 decision
// (issue #1995, aggregate): the circle timeline is the merged timeline of its
// members, excluding non-members, archived members and other users' data.
func TestGetCircleTimeline_MergesMemberTimelinesOnly(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createCircleTestUsers(t, db)

	member := models.Contact{UserID: owner.ID, Firstname: "Member"}
	member2 := models.Contact{UserID: owner.ID, Firstname: "Member2"}
	archived := models.Contact{UserID: owner.ID, Firstname: "Archived"}
	outsider := models.Contact{UserID: owner.ID, Firstname: "Outsider"}
	for _, c := range []*models.Contact{&member, &member2, &archived, &outsider} {
		require.NoError(t, db.Create(c).Error)
	}
	require.NoError(t, db.Model(&archived).Update("archived", true).Error)

	circle := models.Circle{UserID: owner.ID, Name: "Climbing"}
	require.NoError(t, db.Create(&circle).Error)
	for _, c := range []models.Contact{member, member2, archived} {
		require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: owner.ID, MemberVCardUID: c.VCardUID}).Error)
	}

	now := time.Now().UTC()
	notes := map[string]models.Contact{"member note": member, "archived note": archived, "outsider note": outsider}
	for content, c := range notes {
		cid := c.ID
		require.NoError(t, db.Create(&models.Note{UserID: owner.ID, ContactID: &cid, Content: content, Date: now}).Error)
	}
	// One activity shared by both members must appear once.
	shared := models.Activity{UserID: owner.ID, Title: "Bouldering", Date: now.Add(-time.Hour), Contacts: []models.Contact{member, member2}}
	require.NoError(t, db.Create(&shared).Error)
	outsiderAct := models.Activity{UserID: owner.ID, Title: "Unrelated", Date: now, Contacts: []models.Contact{outsider}}
	require.NoError(t, db.Create(&outsiderAct).Error)

	router := newCircleRealDBRouter(db, owner.ID)
	router.GET("/circles/:id/timeline", GetCircleTimeline)

	resp := circleDoJSON(t, router, "GET", "/circles/"+circle.ID+"/timeline", nil)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var page struct {
		Items []struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		} `json:"items"`
		MemberCount int64 `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &page))

	var titles []string
	for _, it := range page.Items {
		var d struct {
			Content string `json:"content"`
			Title   string `json:"title"`
		}
		require.NoError(t, json.Unmarshal(it.Data, &d))
		titles = append(titles, it.Type+":"+d.Content+d.Title)
	}
	require.ElementsMatch(t, []string{"note:member note", "activity:Bouldering"}, titles)
	require.EqualValues(t, 3, page.MemberCount)

	// Other users get a 404, never the data.
	otherRouter := newCircleRealDBRouter(db, other.ID)
	otherRouter.GET("/circles/:id/timeline", GetCircleTimeline)
	require.Equal(t, http.StatusNotFound, circleDoJSON(t, otherRouter, "GET", "/circles/"+circle.ID+"/timeline", nil).Code)
}

// TestGetCircleTimeline_EmptyCircle returns an empty (non-null) item list.
func TestGetCircleTimeline_EmptyCircle(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	circle := models.Circle{UserID: owner.ID, Name: "Empty"}
	require.NoError(t, db.Create(&circle).Error)

	router := newCircleRealDBRouter(db, owner.ID)
	router.GET("/circles/:id/timeline", GetCircleTimeline)
	resp := circleDoJSON(t, router, "GET", "/circles/"+circle.ID+"/timeline", nil)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &raw))
	require.JSONEq(t, "[]", string(raw["items"]))
	require.JSONEq(t, "0", string(raw["member_count"]))
}

func circleTimelineSetup(t *testing.T) (*gin.Engine, *gorm.DB, models.Circle) {
	t.Helper()
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	circle := models.Circle{UserID: owner.ID, Name: "C"}
	require.NoError(t, db.Create(&circle).Error)
	router := newCircleRealDBRouter(db, owner.ID)
	router.GET("/circles/:id/timeline", GetCircleTimeline)
	return router, db, circle
}

// TestGetCircleTimeline_BadParams: unknown type/bucket tokens and malformed
// cursors are 400s, as on the contact timeline.
func TestGetCircleTimeline_BadParams(t *testing.T) {
	router, _, circle := circleTimelineSetup(t)
	base := "/circles/" + circle.ID + "/timeline?"
	for name, q := range map[string]string{
		"unknown type":     "type=bogus",
		"unknown bucket":   "bucket=bogus",
		"malformed cursor": "cursor=!!!",
		"bad cursor id":    "cursor=" + services.EncodeTimelineCursor(time.Now(), "note", "not-a-number"),
	} {
		t.Run(name, func(t *testing.T) {
			resp := circleDoJSON(t, router, "GET", base+q, nil)
			require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
		})
	}
	ok := circleDoJSON(t, router, "GET", base+"bucket=last_7_days&type=note&order=asc", nil)
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
}

// TestGetCircleTimeline_DBFailures: storage errors surface as 500s, not empty pages.
func TestGetCircleTimeline_DBFailures(t *testing.T) {
	for _, table := range []string{"circles", "circle_members", "notes"} {
		t.Run(table, func(t *testing.T) {
			router, db, circle := circleTimelineSetup(t)
			dbtest.HideTable(t, db, table)
			resp := circleDoJSON(t, router, "GET", "/circles/"+circle.ID+"/timeline", nil)
			require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
		})
	}
}

// TestUpdateCircle_RejectsUnknownColor: PUT with a non-palette colour is a 400
// and leaves the stored colour alone.
func TestUpdateCircle_RejectsUnknownColor(t *testing.T) {
	router, db, circle := circleTimelineSetup(t)
	require.NoError(t, db.Model(&circle).Update("color", "red").Error)
	resp := circleDoJSON(t, router, "PUT", "/circles/"+circle.ID, map[string]any{"name": "C", "color": "mauve"})
	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
	var reloaded models.Circle
	require.NoError(t, db.First(&reloaded, "id = ?", circle.ID).Error)
	require.Equal(t, "red", reloaded.Color)
}
