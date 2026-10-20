package controllers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/clock"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issues #1733 (ambient contacts) and #1736 (given gifts as interactions) at
// the HTTP boundary. The service-level behaviour is in services/
// ambient_gift_scoring_test.go; this pins the wire shape and the write rules.

func putContactRaw(t *testing.T, router http.Handler, id uint, body string) map[string]json.RawMessage {
	t.Helper()
	req, _ := http.NewRequest("PUT", "/contacts/"+strconv.Itoa(int(id)), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	return raw
}

func crmOf(t *testing.T, raw map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var crm map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["crm"], &crm))
	return crm
}

// crm.ambient is preserve-when-absent: an older client that rebuilds crm on
// every save (and so never mentions the field) must not clear the flag, while
// an explicit true/false is authoritative.
func TestUpdateContact_AmbientIsPreservedWhenAbsentAndSetWhenExplicit(t *testing.T) {
	db, router := setupRouter(t)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	named := `"card":{"name":{"components":[{"kind":"given","value":"Alice"}]}}`

	// Explicit true sets it, and the response says so (raw JSON, not a decoded struct).
	raw := putContactRaw(t, router, contact.ID, `{`+named+`,"crm":{"ambient":true}}`)
	assert.JSONEq(t, `true`, string(crmOf(t, raw)["ambient"]))
	var stored models.Contact
	require.NoError(t, db.First(&stored, contact.ID).Error)
	assert.True(t, stored.Ambient, "the flat column is the store")

	// An older client's save: crm rebuilt without the field. The flag survives.
	raw = putContactRaw(t, router, contact.ID, `{`+named+`,"crm":{"how_we_met":"edited by an old client"}}`)
	assert.JSONEq(t, `true`, string(crmOf(t, raw)["ambient"]), "absent must preserve, not clear")
	require.NoError(t, db.First(&stored, contact.ID).Error)
	assert.True(t, stored.Ambient)
	assert.Equal(t, "edited by an old client", stored.HowWeMet)

	// Same for a body with no crm object at all.
	raw = putContactRaw(t, router, contact.ID, `{`+named+`}`)
	assert.JSONEq(t, `true`, string(crmOf(t, raw)["ambient"]))

	// Explicit false clears it; a non-ambient contact omits the key.
	raw = putContactRaw(t, router, contact.ID, `{`+named+`,"crm":{"ambient":false}}`)
	_, present := crmOf(t, raw)["ambient"]
	assert.False(t, present, "a non-ambient contact's envelope is unchanged from before the flag existed")
	require.NoError(t, db.First(&stored, contact.ID).Error)
	assert.False(t, stored.Ambient)
}

func TestGetContactScore_AmbientFieldsOnTheWire(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/contacts/:id/score", GetContactScore)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	flagged := models.Contact{UserID: user.ID, Firstname: "Flagged", Ambient: true}
	require.NoError(t, db.Create(&flagged).Error)
	plain := models.Contact{UserID: user.ID, Firstname: "Plain"}
	require.NoError(t, db.Create(&plain).Error)

	get := func(id uint) map[string]json.RawMessage {
		req, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(id))+"/score", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
		return raw
	}

	raw := get(flagged.ID)
	assert.JSONEq(t, `true`, string(raw["ambient"]))
	assert.JSONEq(t, `"flag"`, string(raw["ambient_reason"]))
	var recency models.ContactScoreFacet
	require.NoError(t, json.Unmarshal(raw["recency"], &recency))
	assert.Equal(t, 100.0, recency.Value)
	assert.Equal(t, "Ambient contact (marked)", recency.Reason)

	raw = get(plain.ID)
	assert.JSONEq(t, `false`, string(raw["ambient"]), "ambient is never omitted")
	_, hasReason := raw["ambient_reason"]
	assert.False(t, hasReason)
}

func TestGiftController_GivenWithoutDateIsStampedNow(t *testing.T) {
	db, router := setupRouter(t)
	fixed := time.Date(2030, 6, 15, 9, 30, 0, 0, time.UTC)
	withClock(router, clock.NewFake(fixed))
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)
	router.PUT("/gifts/:id", withValidated(func() any { return &models.GiftInput{} }), UpdateGift)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	create := func(status string, date *time.Time) models.Gift {
		body, _ := json.Marshal(models.GiftInput{EntityID: contact.VCardUID, Status: status, Description: "x", Date: date})
		req, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp struct {
			Gift models.Gift `json:"gift"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Gift
	}
	update := func(id string, status string, date *time.Time) models.Gift {
		body, _ := json.Marshal(models.GiftInput{EntityID: contact.VCardUID, Status: status, Description: "x", Date: date})
		req, _ := http.NewRequest("PUT", "/gifts/"+id, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var g models.Gift
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &g))
		return g
	}

	// Create as given with no date: stamped with the request clock.
	g := create(models.GiftStatusGiven, nil)
	require.NotNil(t, g.Date)
	assert.True(t, fixed.Equal(*g.Date), "got %s", g.Date)

	// An explicit date is kept as is.
	explicit := time.Date(2029, 12, 24, 0, 0, 0, 0, time.UTC)
	g2 := create(models.GiftStatusGiven, &explicit)
	require.NotNil(t, g2.Date)
	assert.True(t, explicit.Equal(*g2.Date))

	// An idea stays dateless.
	idea := create(models.GiftStatusIdea, nil)
	assert.Nil(t, idea.Date)

	// idea -> given with no date on update: stamped.
	moved := update(idea.ID, models.GiftStatusGiven, nil)
	require.NotNil(t, moved.Date)
	assert.True(t, fixed.Equal(*moved.Date))

	// An already-given gift saved again with the date omitted keeps its date
	// rather than being re-stamped to now.
	kept := update(g2.ID, models.GiftStatusGiven, nil)
	require.NotNil(t, kept.Date)
	assert.True(t, explicit.Equal(*kept.Date), "a full-replace body that omits the date must not rewrite history")

	// given -> received keeps whatever date it had; nothing is stamped on a non-given status.
	recv := create(models.GiftStatusReceived, nil)
	assert.Nil(t, recv.Date)
}

// The flat CSV export carries the flag as its own "Ambient" column (issue
// #1733), true only for the contact that has it.
func TestExportCSV_CarriesAmbientColumn(t *testing.T) {
	f := seedCSVFullFidelity(t)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", f.ada.ID).Update("ambient", true).Error)

	body := csvFidelityGet(t, f.router, "/export")
	start := strings.Index(body, "=== CONTACTS ===\n")
	require.GreaterOrEqual(t, start, 0)
	section := body[start+len("=== CONTACTS ===\n"):]
	if end := strings.Index(section, "\n=== "); end >= 0 {
		section = section[:end]
	}
	rows, err := csv.NewReader(strings.NewReader(section)).ReadAll()
	require.NoError(t, err)
	col := -1
	for i, h := range rows[0] {
		if h == "Ambient" {
			col = i
		}
	}
	require.GreaterOrEqual(t, col, 0, "the contacts section must have an Ambient column: %v", rows[0])
	got := map[string]string{}
	for _, r := range rows[1:] {
		got[r[1]] = r[col] // Firstname -> Ambient
	}
	assert.Equal(t, "true", got["Ada"])
	assert.Equal(t, "false", got["Bob"])
}

// A merge keeps the ambient flag if EITHER record had it (issue #1733).
func TestContactMerge_AmbientFlagIsUnioned(t *testing.T) {
	db, router := setupRouter(t)
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	merge := func(keeperAmbient, loserAmbient bool) models.Contact {
		keeper := models.Contact{UserID: user.ID, Firstname: "Same"}
		loser := models.Contact{UserID: user.ID, Firstname: "Same"}
		require.NoError(t, db.Create(&keeper).Error)
		require.NoError(t, db.Create(&loser).Error)
		require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", keeper.ID).Update("ambient", keeperAmbient).Error)
		require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", loser.ID).Update("ambient", loserAmbient).Error)

		body, _ := json.Marshal(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID})
		req, _ := http.NewRequest("POST", "/contacts/merge", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var merged models.Contact
		require.NoError(t, db.First(&merged, keeper.ID).Error)
		return merged
	}

	assert.True(t, merge(false, true).Ambient, "the loser's flag survives into the keeper")
	assert.True(t, merge(true, false).Ambient, "the keeper's own flag is not cleared")
	assert.False(t, merge(false, false).Ambient)
}
