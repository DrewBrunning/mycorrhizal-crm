package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pd(y, m, d int) *contactmodel.PartialDate {
	p := &contactmodel.PartialDate{}
	if y != 0 {
		p.Year = intPtr(y)
	}
	if m != 0 {
		p.Month = intPtr(m)
	}
	if d != 0 {
		p.Day = intPtr(d)
	}
	return p
}

// Issue #2001: relationship start/end dates round-trip through create, get and
// update, validate end >= start, and stay owner-scoped.
func TestRelationshipEdgeDates(t *testing.T) {
	db, router := setupRouter(t)
	router.POST("/relationship-edges", withValidated(func() any { return &models.RelationshipEdgeInput{} }), CreateRelationshipEdge)
	router.PUT("/relationship-edges/:id", withValidated(func() any { return &models.RelationshipEdgeInput{} }), UpdateRelationshipEdge)
	router.GET("/relationship-edges/:id", GetRelationshipEdge)

	var user models.User
	db.First(&user)
	a := models.Contact{UserID: user.ID, Firstname: "A"}
	b := models.Contact{UserID: user.ID, Firstname: "B"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)

	do := func(method, url string, in models.RelationshipEdgeInput) *httptest.ResponseRecorder {
		body, _ := json.Marshal(in)
		req, _ := http.NewRequest(method, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	base := models.RelationshipEdgeInput{SourceID: a.VCardUID, TargetID: b.VCardUID, Type: "spouse_of"}

	t.Run("reversed range is rejected", func(t *testing.T) {
		in := base
		in.StartDate, in.EndDate = pd(2018, 1, 1), pd(2010, 5, 17)
		w := do("POST", "/relationship-edges", in)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "end_date")
		var n int64
		db.Model(&models.RelationshipEdge{}).Count(&n)
		assert.Zero(t, n, "a rejected create must not persist")
	})
	t.Run("out-of-range month and empty date are rejected", func(t *testing.T) {
		in := base
		in.StartDate = pd(2010, 13, 0)
		assert.Equal(t, http.StatusBadRequest, do("POST", "/relationship-edges", in).Code)
		in.StartDate, in.EndDate = nil, &contactmodel.PartialDate{}
		assert.Equal(t, http.StatusBadRequest, do("POST", "/relationship-edges", in).Code)
	})
	t.Run("different precision is not an error", func(t *testing.T) {
		in := base
		in.StartDate, in.EndDate = pd(2010, 5, 17), pd(2010, 0, 0)
		require.Equal(t, http.StatusCreated, do("POST", "/relationship-edges", in).Code)
		require.NoError(t, db.Where("1=1").Delete(&models.RelationshipEdge{}).Error)
	})

	t.Run("round trip and update", func(t *testing.T) {
		in := base
		in.StartDate, in.EndDate = pd(2010, 5, 17), pd(2018, 0, 0)
		w := do("POST", "/relationship-edges", in)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var created struct {
			RelationshipEdge models.RelationshipEdge `json:"relationship_edge"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		id := created.RelationshipEdge.ID

		req, _ := http.NewRequest("GET", "/relationship-edges/"+id, nil)
		gw := httptest.NewRecorder()
		router.ServeHTTP(gw, req)
		var got models.RelationshipEdge
		require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &got))
		require.NotNil(t, got.StartDate)
		require.NotNil(t, got.EndDate)
		assert.Equal(t, 2010, *got.StartDate.Year)
		assert.Equal(t, 17, *got.StartDate.Day)
		assert.Equal(t, 2018, *got.EndDate.Year)
		assert.Nil(t, got.EndDate.Month)

		// Update that reverses the range is rejected and leaves the row alone.
		bad := base
		bad.StartDate, bad.EndDate = pd(2019, 0, 0), pd(2018, 0, 0)
		require.Equal(t, http.StatusBadRequest, do("PUT", "/relationship-edges/"+id, bad).Code)
		var stored models.RelationshipEdge
		require.NoError(t, db.First(&stored, "id = ?", id).Error)
		assert.Equal(t, 2010, *stored.StartDate.Year)

		// Full-replace: omitting the dates clears them (reopens the edge).
		require.Equal(t, http.StatusOK, do("PUT", "/relationship-edges/"+id, base).Code)
		require.NoError(t, db.First(&stored, "id = ?", id).Error)
		assert.Nil(t, stored.StartDate)
		assert.Nil(t, stored.EndDate)
	})
}
