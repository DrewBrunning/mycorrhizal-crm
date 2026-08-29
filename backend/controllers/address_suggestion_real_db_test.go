package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContactAddressSuggestions_RealMigratedSchema exercises the generate +
// apply endpoints for contact-address suggestions against a
// database.InitDB-migrated real file database, including ownership scoping
// and the server-side re-derivation contract.
func TestContactAddressSuggestions_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "addruser", Password: "password123!A", Email: "addruser@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "addrother", Password: "password123!A", Email: "addrother@example.com"}
	require.NoError(t, db.Create(&other).Error)

	clarkSt := models.ContactAddress{Street: "742 Clark St", City: "Springfield", Region: "IL", Postal: "62701", Country: "USA"}
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob", Addresses: []models.ContactAddress{clarkSt}}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	spouse := models.RelationshipEdge{
		UserID: user.ID, SourceID: alice.VCardUID, TargetID: bob.VCardUID, Type: "spouse_of",
		Directional: false, Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&spouse).Error)

	// A foreign user's data must never feed this user's suggestions.
	foreign := models.Contact{UserID: other.ID, Firstname: "Foreign", Addresses: []models.ContactAddress{clarkSt}}
	require.NoError(t, db.Create(&foreign).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/address-suggestions", SuggestContactAddresses)
	router.POST("/contacts/address-suggestions/apply", withValidated(func() any { return &models.ApplyContactAddressSuggestionInput{} }), ApplyContactAddressSuggestion)

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Generate.
	genResp := doJSON("POST", "/contacts/address-suggestions", nil)
	require.Equal(t, http.StatusOK, genResp.Code, genResp.Body.String())
	var gen struct {
		Suggestions []servicesContactAddressSuggestion `json:"suggestions"`
		Total       int                                `json:"total"`
	}
	require.NoError(t, json.Unmarshal(genResp.Body.Bytes(), &gen))
	require.Equal(t, 1, gen.Total, "only Alice lacks an address; Foreign's edge belongs to another user")
	require.Len(t, gen.Suggestions, 1)
	s := gen.Suggestions[0]
	assert.Equal(t, alice.VCardUID, s.ContactVCardUID)
	assert.Equal(t, bob.VCardUID, s.SourceID)
	assert.Equal(t, "relationship", s.SourceKind)
	assert.Equal(t, "spouse_of", s.RelationType)
	require.NotEmpty(t, s.AddressKey)

	// Apply.
	applyResp := doJSON("POST", "/contacts/address-suggestions/apply", models.ApplyContactAddressSuggestionInput{
		ContactVCardUID: alice.VCardUID,
		SourceKind:      "relationship",
		SourceID:        bob.VCardUID,
		AddressKey:      s.AddressKey,
	})
	require.Equal(t, http.StatusOK, applyResp.Code, applyResp.Body.String())

	var reloaded models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", alice.VCardUID).First(&reloaded).Error)
	require.Len(t, reloaded.Addresses, 1)
	assert.Equal(t, clarkSt.Street, reloaded.Addresses[0].Street)

	// A second apply is a checked 409 (already has it).
	againResp := doJSON("POST", "/contacts/address-suggestions/apply", models.ApplyContactAddressSuggestionInput{
		ContactVCardUID: alice.VCardUID,
		SourceKind:      "relationship",
		SourceID:        bob.VCardUID,
		AddressKey:      s.AddressKey,
	})
	require.Equal(t, http.StatusConflict, againResp.Code, againResp.Body.String())

	// The regenerate pass now offers nothing new.
	gen2Resp := doJSON("POST", "/contacts/address-suggestions", nil)
	require.Equal(t, http.StatusOK, gen2Resp.Code)
	var gen2 struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(gen2Resp.Body.Bytes(), &gen2))
	assert.Equal(t, 0, gen2.Total)

	// Applying to a foreign contact is a 404 (ownership scoping).
	foreignApply := doJSON("POST", "/contacts/address-suggestions/apply", models.ApplyContactAddressSuggestionInput{
		ContactVCardUID: foreign.VCardUID,
		SourceKind:      "relationship",
		SourceID:        bob.VCardUID,
		AddressKey:      s.AddressKey,
	})
	require.Equal(t, http.StatusNotFound, foreignApply.Code, foreignApply.Body.String())

	// A stale suggestion (relationship deleted) is a 409.
	require.NoError(t, db.Delete(&spouse).Error)
	staleApply := doJSON("POST", "/contacts/address-suggestions/apply", models.ApplyContactAddressSuggestionInput{
		ContactVCardUID: bob.VCardUID,
		SourceKind:      "relationship",
		SourceID:        alice.VCardUID,
		AddressKey:      s.AddressKey,
	})
	require.Equal(t, http.StatusConflict, staleApply.Code, staleApply.Body.String())
}

// servicesContactAddressSuggestion mirrors the wire shape of
// services.ContactAddressSuggestion for test decoding without importing the
// services package into the controllers tests.
type servicesContactAddressSuggestion struct {
	ContactVCardUID string `json:"contact_vcard_uid"`
	ContactName     string `json:"contact_name"`
	SourceKind      string `json:"source_kind"`
	SourceID        string `json:"source_id"`
	SourceName      string `json:"source_name"`
	RelationType    string `json:"relation_type"`
	AddressKey      string `json:"address_key"`
}

// --- handler error branches the happy-path test above does not reach ---

func TestSuggestContactAddresses_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	router.POST("/suggest", SuggestContactAddresses)

	req, _ := http.NewRequest(http.MethodPost, "/suggest", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestSuggestContactAddresses_ServiceError(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "suggesterr", Password: "password123!A", Email: "suggesterr@example.com"}
	require.NoError(t, db.Create(&user).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close()) // subsequent service queries fail

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.POST("/suggest", SuggestContactAddresses)

	req, _ := http.NewRequest(http.MethodPost, "/suggest", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestApplyContactAddressSuggestion_InvalidInput(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "applyinvalid", Password: "password123!A", Email: "applyinvalid@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.POST("/apply", middleware.ValidateJSONMiddleware(&models.ApplyContactAddressSuggestionInput{}), ApplyContactAddressSuggestion)

	// Missing required fields fails validation before the handler's body.
	req, _ := http.NewRequest(http.MethodPost, "/apply", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestApplyContactAddressSuggestion_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	router.POST("/apply", withValidated(func() any { return &models.ApplyContactAddressSuggestionInput{} }), ApplyContactAddressSuggestion)

	req, _ := http.NewRequest(http.MethodPost, "/apply", bytes.NewBufferString(`{"contact_vcard_uid":"x","source_kind":"relationship","source_id":"y","address_key":"k"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestApplyContactAddressSuggestion_ServiceDatabaseError(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "applydberr", Password: "password123!A", Email: "applydberr@example.com"}
	require.NoError(t, db.Create(&user).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close()) // service's first query fails

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.POST("/apply", withValidated(func() any { return &models.ApplyContactAddressSuggestionInput{} }), ApplyContactAddressSuggestion)

	req, _ := http.NewRequest(http.MethodPost, "/apply", bytes.NewBufferString(`{"contact_vcard_uid":"x","source_kind":"relationship","source_id":"y","address_key":"k"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}
