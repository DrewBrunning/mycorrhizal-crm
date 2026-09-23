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

func contactPeriodInput(entryID string, rangeIn contactmodel.TemporalRange) models.ContactRecordInput {
	return models.ContactRecordInput{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
				{Kind: "given", Value: "Alice"},
			}},
			Addresses: []contactmodel.Address{{ID: entryID, Full: "1 Main St"}},
		},
		CRM: contactmodel.CRMEnvelope{
			Periods: []contactmodel.EntryPeriod{{Kind: contactmodel.PeriodKindAddress, EntryID: entryID, Range: rangeIn}},
		},
	}
}

// ADR 0025: a period attached to a Card entry by its ID round-trips through
// the nested write path and is persisted on the envelope.
func TestCreateContactWithAddressPeriod(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	payload := contactPeriodInput("addr-1", contactmodel.TemporalRange{
		Start: &contactmodel.PartialDate{Year: intPtr(2019), Month: intPtr(4)},
		End:   &contactmodel.PartialDate{Year: intPtr(2024)},
	})
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var saved models.Contact
	require.NoError(t, db.First(&saved).Error)
	require.Len(t, saved.CRM.Periods, 1, "the period must persist on the envelope")
	got := saved.CRM.Periods[0]
	assert.Equal(t, "addr-1", got.EntryID)
	require.NotNil(t, got.Range.Start)
	assert.Equal(t, 2019, *got.Range.Start.Year)
	require.NotNil(t, got.Range.End)
	assert.Equal(t, 2024, *got.Range.End.Year)
}

func TestCreateContactRejectsUnresolvedPeriodEntry(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	payload := contactPeriodInput("addr-1", contactmodel.TemporalRange{
		Start: &contactmodel.PartialDate{Year: intPtr(2019)},
	})
	payload.CRM.Periods[0].EntryID = "does-not-exist"
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var count int64
	db.Model(&models.Contact{}).Count(&count)
	assert.Zero(t, count, "an unresolved period reference must not create a contact")
}

func TestCreateContactRejectsEmptyPeriod(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	payload := contactPeriodInput("addr-1", contactmodel.TemporalRange{})
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var count int64
	db.Model(&models.Contact{}).Count(&count)
	assert.Zero(t, count, "an empty period must not create a contact")
}

// A flat-only write (no periods) must remain valid — periods are optional.
func TestCreateContactWithoutPeriodsIsValid(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	payload := models.ContactRecordInput{Card: contactmodel.Card{
		Name:      &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Bob"}}},
		Addresses: []contactmodel.Address{{ID: "addr-1", Full: "2 Elm St"}},
	}}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var saved models.Contact
	require.NoError(t, db.First(&saved).Error)
	assert.Empty(t, saved.CRM.Periods)
}
