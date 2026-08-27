package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddressHouseholdSuggestions_RealMigratedSchema is T40's end-to-end round
// trip against the real migrated schema: detection, dismissal persistence
// across passes, cross-user isolation, the unique-index guard on duplicate
// dismissals, and the accept flow that materializes a Household. Runs against
// database.InitDB so the dismissed_household_suggestions table and its unique
// index come from the real migration SQL, not AutoMigrate.
func TestAddressHouseholdSuggestions_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "addr-suggest-real.db")
	db := dbtest.NewAt(t, dbPath)

	user := models.User{Username: "addrsuggest", Password: "password123!A", Email: "addrsuggest@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/households/suggest-addresses", SuggestAddressHouseholds)
	router.POST("/households/suggestions/accept", middleware.ValidateJSONMiddleware(&models.AcceptHouseholdSuggestionInput{}), AcceptAddressHouseholdSuggestion)
	router.POST("/households/suggestions/dismiss", middleware.ValidateJSONMiddleware(&models.DismissHouseholdSuggestionInput{}), DismissAddressHouseholdSuggestion)
	router.GET("/households", ListHouseholds)

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

	detect := func() []servicesAddressSuggestion {
		w := doJSON("POST", "/households/suggest-addresses", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Suggestions []servicesAddressSuggestion `json:"suggestions"`
			Total       int                         `json:"total"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body.Suggestions
	}

	// Helper to create a contact with one structured address.
	addr := func(street, city, postal string) models.ContactAddress {
		return models.ContactAddress{Street: street, City: city, Region: "IL", Postal: postal}
	}
	mkContact := func(firstname string, addresses []models.ContactAddress) models.Contact {
		c := models.Contact{UserID: user.ID, Firstname: firstname, Addresses: addresses}
		require.NoError(t, db.Create(&c).Error)
		return c
	}

	// Three contacts sharing one normalized address (with casing/punctuation
	// differences to prove normalization), plus one contact with a different
	// address.
	alice := mkContact("Alice", []models.ContactAddress{addr("123 Main St.", "Springfield", "62704")})
	bob := mkContact("Bob", []models.ContactAddress{addr("123 main st", "Springfield", "62704")})
	carol := mkContact("Carol", []models.ContactAddress{addr("123 MAIN ST", "Springfield", "62704")})
	isolated := mkContact("Dave", []models.ContactAddress{addr("999 Oak Ave", "Chicago", "60614")})

	// Pass 1: one suggestion for {alice,bob,carol}, none for Dave.
	s1 := detect()
	require.Len(t, s1, 1, "exactly one shared-address group should be suggested")
	assert.ElementsMatch(t, []string{alice.VCardUID, bob.VCardUID, carol.VCardUID}, s1[0].MemberVCardUIDs)
	assert.NotEmpty(t, s1[0].AddressHash)
	assert.NotEmpty(t, s1[0].MemberHash)
	// The shared street is whichever member sorts first — assert it is one of
	// the three known spellings of the same physical address.
	assert.Contains(t,
		[]string{"123 Main St.", "123 main st", "123 MAIN ST"},
		s1[0].Address.Components[0].Value,
		"street component survives normalization round-trip",
	)

	// Dismiss the group, then confirm it is not re-offered.
	dismiss := doJSON("POST", "/households/suggestions/dismiss", models.DismissHouseholdSuggestionInput{
		MemberVCardUIDs: []string{bob.VCardUID, carol.VCardUID, alice.VCardUID},
	})
	require.Equal(t, http.StatusOK, dismiss.Code, dismiss.Body.String())

	s2 := detect()
	assert.Len(t, s2, 0, "a dismissed group must not be re-offered")

	// Dismissing the same group again must hit the unique index -> 409.
	dismiss2 := doJSON("POST", "/households/suggestions/dismiss", models.DismissHouseholdSuggestionInput{
		MemberVCardUIDs: []string{alice.VCardUID, bob.VCardUID, carol.VCardUID},
	})
	assert.Equal(t, http.StatusConflict, dismiss2.Code, "duplicate dismissal must be rejected by the unique index")

	// A dismissal referencing a contact that isn't the user's must 404, not
	// silently record a partial group.
	dismissPhantom := doJSON("POST", "/households/suggestions/dismiss", models.DismissHouseholdSuggestionInput{
		MemberVCardUIDs: []string{alice.VCardUID, "00000000-0000-4000-8000-000000000000"},
	})
	assert.Equal(t, http.StatusNotFound, dismissPhantom.Code, "a dismissal referencing an unknown contact must 404")

	// Accepting a dismissed group must also be rejected (409), not silently
	// create the household.
	acceptDismissed := doJSON("POST", "/households/suggestions/accept", models.AcceptHouseholdSuggestionInput{
		MemberVCardUIDs: []string{alice.VCardUID, bob.VCardUID, carol.VCardUID},
	})
	assert.Equal(t, http.StatusConflict, acceptDismissed.Code, "accepting a dismissed group must be rejected")

	// Cross-user isolation: another user's contact at the same address must
	// never join this user's suggestions.
	otherUser := models.User{Username: "other-addr", Password: "password123!A", Email: "other-addr@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	otherContact := models.Contact{UserID: otherUser.ID, Firstname: "Eve", Addresses: []models.ContactAddress{addr("123 Main St", "Springfield", "62704")}}
	require.NoError(t, db.Create(&otherContact).Error)

	// A new group is now detectable: {alice,bob} (carol's contact still shares
	// it too, but the whole three-again group is dismissed; a fresh pair from
	// within it is a different member_hash and re-offers). Assert only that no
	// suggestion ever contains another user's VCardUID and that cross-user
	// data does not leak into the count.
	s3 := detect()
	for _, s := range s3 {
		assert.NotContains(t, s.MemberVCardUIDs, otherContact.VCardUID, "cross-user contacts must never match")
	}

	// Accept flow: create a household for the fresh pair {alice, isolated} —
	// not a real shared address, so first give them one, then accept.
	newPairAddr := addr("77 Lake Shore Dr", "Chicago", "60611")
	var aliceReload models.Contact
	require.NoError(t, db.First(&aliceReload, "vcard_uid = ?", alice.VCardUID).Error)
	aliceReload.Addresses = append(aliceReload.Addresses, newPairAddr)
	require.NoError(t, db.Save(&aliceReload).Error)

	var isolatedReload models.Contact
	require.NoError(t, db.First(&isolatedReload, "vcard_uid = ?", isolated.VCardUID).Error)
	isolatedReload.Addresses = append(isolatedReload.Addresses, newPairAddr)
	require.NoError(t, db.Save(&isolatedReload).Error)

	accept := doJSON("POST", "/households/suggestions/accept", models.AcceptHouseholdSuggestionInput{
		MemberVCardUIDs: []string{alice.VCardUID, isolated.VCardUID},
	})
	require.Equal(t, http.StatusCreated, accept.Code, accept.Body.String())
	var acceptBody struct {
		Household models.Household `json:"household"`
	}
	require.NoError(t, json.Unmarshal(accept.Body.Bytes(), &acceptBody))
	household := acceptBody.Household
	assert.NotEmpty(t, household.ID)
	assert.NotNil(t, household.Address)

	var members []models.HouseholdMember
	require.NoError(t, db.Where("household_id = ?", household.ID).Find(&members).Error)
	require.Len(t, members, 2)
	assert.Equal(t, models.HouseholdRoleAdult, members[0].Role)

	// After acceptance the pair are co-members, so no further suggestion.
	s4 := detect()
	for _, s := range s4 {
		assert.False(t, containsAll(s.MemberVCardUIDs, []string{alice.VCardUID, isolated.VCardUID}),
			"contacts now co-members of a household must not be re-suggested")
	}

	// And accepting the same pair again is rejected (already co-members).
	acceptAgain := doJSON("POST", "/households/suggestions/accept", models.AcceptHouseholdSuggestionInput{
		MemberVCardUIDs: []string{alice.VCardUID, isolated.VCardUID},
	})
	assert.Equal(t, http.StatusConflict, acceptAgain.Code, "accepting an already-co-member group must be rejected")
}

// TestAddressHouseholdSuggestions_EmptyResultIsJSONArrayNotNull is T64
// (T64): an
// account with zero qualifying groups must get back literal `"suggestions":[]`
// in the raw response body, not `"suggestions":null`. Decoding into a Go
// struct would make `null` and `[]` indistinguishable (both unmarshal into a
// nil/empty slice depending on the target's zero value) — that's exactly why
// this shipped unnoticed — so this test asserts against the raw bytes.
func TestAddressHouseholdSuggestions_EmptyResultIsJSONArrayNotNull(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "addr-suggest-empty.db")
	db := dbtest.NewAt(t, dbPath)

	user := models.User{Username: "addrsuggestempty", Password: "password123!A", Email: "addrsuggestempty@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/households/suggest-addresses", SuggestAddressHouseholds)

	req, _ := http.NewRequest("POST", "/households/suggest-addresses", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"suggestions":[],"total":0}`, w.Body.String())
	assert.Contains(t, w.Body.String(), `"suggestions":[]`, "must serialize as an empty array, not null")
	assert.NotContains(t, w.Body.String(), `"suggestions":null`)
}

// servicesAddressSuggestion mirrors services.AddressSuggestion for decoding.
type servicesAddressSuggestion struct {
	AddressHash     string   `json:"address_hash"`
	MemberHash      string   `json:"member_hash"`
	MemberVCardUIDs []string `json:"member_vcard_uids"`
	Address         struct {
		Components []struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"components"`
		Full string `json:"full"`
	} `json:"address"`
}

func containsAll(haystack, needles []string) bool {
	set := map[string]bool{}
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}
