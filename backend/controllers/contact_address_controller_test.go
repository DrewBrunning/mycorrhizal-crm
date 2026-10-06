package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
)

// fakeAddressGeocoder stands in for the provider: it records every address it
// is asked about and answers with canned values.
type fakeAddressGeocoder struct {
	disabled bool
	uri      string
	cached   bool
	err      error
	onCall   func() // runs inside the lookup, e.g. to simulate a concurrent edit
	calls    []models.ContactAddress
}

func (f *fakeAddressGeocoder) Enabled() bool { return !f.disabled }

func (f *fakeAddressGeocoder) GeocodeAddress(_ context.Context, a models.ContactAddress) (string, bool, error) {
	f.calls = append(f.calls, a)
	if f.onCall != nil {
		f.onCall()
	}
	return f.uri, f.cached, f.err
}

// seedGeocodeContact creates a contact (owned by the router's seeded user) with
// two addresses through the nested path, returning it as stored. address[0] is
// "1 Main St" with a Card-only 'building' component (so a lossy re-derivation
// would be visible) and the given sensitivity.
func seedGeocodeContact(t *testing.T, db *gorm.DB, userID uint, sensitivity string) models.Contact {
	t.Helper()
	record := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Geo"}}},
		Addresses: []contactmodel.Address{
			{
				Components:  []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}, {Kind: "building", Value: "Tower B"}, {Kind: "locality", Value: "Springfield"}},
				Full:        "1 Main St",
				Sensitivity: sensitivity,
			},
			{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "2 Side St"}}, Full: "2 Side St", Coordinates: "geo:1,1"},
		},
	}}
	c := models.Contact{UserID: userID}
	models.ApplyRecordToContact(&c, record, "")
	require.NoError(t, db.Create(&c).Error)
	var loaded models.Contact
	require.NoError(t, db.First(&loaded, c.ID).Error)
	require.Len(t, loaded.Addresses, 2)
	require.NotEmpty(t, loaded.Addresses[0].ID, "the create minted an ID")
	return loaded
}

func geocodeRouter(t *testing.T, g addressGeocoder) (*gorm.DB, *gin.Engine, uint) {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.POST("/contacts/:id/addresses/:addressId/geocode", GeocodeContactAddress(g))
	return db, router, user.ID
}

func postGeocode(router *gin.Engine, contactID, addressID, query string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/contacts/%s/addresses/%s/geocode%s", contactID, addressID, query), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGeocodeContactAddress_StoresCoordinatesOnBothCopies(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:51.5,-0.12"}
	db, router, uid := geocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")
	addrID := c.Addresses[0].ID

	w := postGeocode(router, fmt.Sprint(c.ID), addrID, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, map[string]any{"address_id": addrID, "coordinates": "geo:51.5,-0.12", "cached": false}, body)

	require.Len(t, fake.calls, 1, "exactly one lookup")
	assert.Equal(t, "1 Main St", fake.calls[0].Street)
	assert.Equal(t, addrID, fake.calls[0].ID)

	var stored models.Contact
	require.NoError(t, db.First(&stored, c.ID).Error)
	assert.Equal(t, "geo:51.5,-0.12", stored.Addresses[0].Coordinates)
	assert.Equal(t, "geo:51.5,-0.12", stored.Card.Addresses[0].Coordinates)
	assert.Equal(t, "geo:1,1", stored.Addresses[1].Coordinates, "the other address is untouched")
	assert.Equal(t, addrID, stored.Addresses[0].ID, "the ID is stable across the write")
	assert.Contains(t, stored.Card.Addresses[0].Components, contactmodel.AddressComponent{Kind: "building", Value: "Tower B"},
		"the write must not flatten the Card entry (trap #3 / T75)")
	assert.Greater(t, stored.Revision, c.Revision, "an ordinary revision-bearing write")
}

func TestGeocodeContactAddress_OverwritesExistingCoordinatesAndReportsCached(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:9,9", cached: true}
	db, router, uid := geocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[1].ID, "") // had geo:1,1
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["cached"])

	var stored models.Contact
	require.NoError(t, db.First(&stored, c.ID).Error)
	assert.Equal(t, "geo:9,9", stored.Addresses[1].Coordinates)
}

func TestGeocodeContactAddress_SensitivityGate(t *testing.T) {
	for _, sensitivity := range []string{models.RelationshipSensitivityPrivate, models.RelationshipSensitivitySecret} {
		t.Run(sensitivity+" refused without opt-in", func(t *testing.T) {
			fake := &fakeAddressGeocoder{uri: "geo:5,5"}
			db, router, uid := geocodeRouter(t, fake)
			c := seedGeocodeContact(t, db, uid, sensitivity)

			for _, q := range []string{"", "?include_sensitive=false", "?include_sensitive=yes", "?include_sensitive=", "?include_sensitive=TRUE"} {
				w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[0].ID, q)
				assert.Equal(t, http.StatusBadRequest, w.Code, "query %q: %s", q, w.Body.String())
				assert.Contains(t, w.Body.String(), sensitivity)
			}
			assert.Empty(t, fake.calls, "a refused address never reaches the geocoder")

			var stored models.Contact
			require.NoError(t, db.First(&stored, c.ID).Error)
			assert.Empty(t, stored.Addresses[0].Coordinates, "nothing was written")
		})

		t.Run(sensitivity+" allowed with the explicit opt-in", func(t *testing.T) {
			for _, q := range []string{"?include_sensitive=true", "?include_sensitive=1"} {
				fake := &fakeAddressGeocoder{uri: "geo:5,5"}
				db, router, uid := geocodeRouter(t, fake)
				c := seedGeocodeContact(t, db, uid, sensitivity)

				w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[0].ID, q)
				assert.Equal(t, http.StatusOK, w.Code, "query %q: %s", q, w.Body.String())
				assert.Len(t, fake.calls, 1)
			}
		})
	}
}

func TestGeocodeContactAddress_NormalAndExplicitNormalAreNotGated(t *testing.T) {
	for _, sensitivity := range []string{"", models.RelationshipSensitivityNormal} {
		fake := &fakeAddressGeocoder{uri: "geo:5,5"}
		db, router, uid := geocodeRouter(t, fake)
		c := seedGeocodeContact(t, db, uid, sensitivity)
		w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[0].ID, "")
		assert.Equal(t, http.StatusOK, w.Code, "sensitivity %q: %s", sensitivity, w.Body.String())
	}
}

// The gate answers before the "geocoding is off" check, so what a user learns
// about a private address does not depend on the operator's provider setting,
// and a disabled instance never looks at an address at all.
func TestGeocodeContactAddress_DisabledInstance(t *testing.T) {
	fake := &fakeAddressGeocoder{disabled: true}
	db, router, uid := geocodeRouter(t, fake)
	normal := seedGeocodeContact(t, db, uid, "")
	private := seedGeocodeContact(t, db, uid, models.RelationshipSensitivityPrivate)

	w := postGeocode(router, fmt.Sprint(normal.ID), normal.Addresses[0].ID, "")
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not enabled")

	w = postGeocode(router, fmt.Sprint(private.ID), private.Addresses[0].ID, "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "the sensitivity gate comes first")

	w = postGeocode(router, fmt.Sprint(private.ID), private.Addresses[0].ID, "?include_sensitive=true")
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Empty(t, fake.calls)
}

func TestGeocodeContactAddress_NotFoundCases(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := geocodeRouter(t, fake)
	mine := seedGeocodeContact(t, db, uid, "")
	other := seedGeocodeContact(t, db, uid, "")

	// Another user's contact: never found, never leaked as 403.
	stranger := models.User{Username: "stranger", Password: "password123", Email: "stranger@example.com"}
	require.NoError(t, db.Create(&stranger).Error)
	theirs := seedGeocodeContact(t, db, stranger.ID, "")

	deleted := seedGeocodeContact(t, db, uid, "")
	require.NoError(t, db.Delete(&models.Contact{}, deleted.ID).Error)

	for name, tc := range map[string]struct{ contact, address string }{
		"unknown address id":           {fmt.Sprint(mine.ID), "no-such-address"},
		"another contact's address id": {fmt.Sprint(mine.ID), other.Addresses[0].ID},
		"another user's contact":       {fmt.Sprint(theirs.ID), theirs.Addresses[0].ID},
		"nonexistent contact":          {"999999", mine.Addresses[0].ID},
		"soft-deleted contact":         {fmt.Sprint(deleted.ID), deleted.Addresses[0].ID},
	} {
		t.Run(name, func(t *testing.T) {
			w := postGeocode(router, tc.contact, tc.address, "")
			assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		})
	}
	assert.Empty(t, fake.calls, "nothing was looked up for any of them")

	var untouched models.Contact
	require.NoError(t, db.First(&untouched, theirs.ID).Error)
	assert.Empty(t, untouched.Addresses[0].Coordinates, "the other user's data was not written")
}

func TestGeocodeContactAddress_RejectsNonNumericContactID(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	_, router, _ := geocodeRouter(t, fake)
	w := postGeocode(router, "abc", "x", "")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Empty(t, fake.calls)
}

func TestGeocodeContactAddress_ErrorMapping(t *testing.T) {
	const providerSecret = "key=SUPER-SECRET"
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"no match", services.ErrGeocoderNoResult, http.StatusUnprocessableEntity},
		{"disabled", services.ErrGeocoderDisabled, http.StatusUnprocessableEntity},
		{"rate limited", services.ErrGeocoderRateLimited, http.StatusServiceUnavailable},
		{"unauthorized", services.ErrGeocoderUnauthorized, http.StatusServiceUnavailable},
		{"private address", services.ErrGeocoderPrivateAddr, http.StatusServiceUnavailable},
		{"invalid data", fmt.Errorf("%w: %s", services.ErrGeocoderInvalidData, providerSecret), http.StatusServiceUnavailable},
		{"unreachable", fmt.Errorf("%w: dial tcp: %s", services.ErrGeocoderUnreachable, providerSecret), http.StatusServiceUnavailable},
		{"endpoint gone", services.ErrGeocoderNotFound, http.StatusServiceUnavailable},
		{"unexpected status", &services.GeocoderRequestError{StatusCode: 500, Status: "500 Internal Server Error"}, http.StatusServiceUnavailable},
		{"unknown error", errors.New("boom " + providerSecret), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAddressGeocoder{err: tc.err}
			db, router, uid := geocodeRouter(t, fake)
			c := seedGeocodeContact(t, db, uid, "")

			w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[1].ID, "")
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.NotContains(t, w.Body.String(), providerSecret, "wrapped provider text never reaches the response")

			var stored models.Contact
			require.NoError(t, db.First(&stored, c.ID).Error)
			assert.Equal(t, "geo:1,1", stored.Addresses[1].Coordinates, "a failed lookup leaves the existing coordinate alone")
			assert.Equal(t, c.Revision, stored.Revision, "and does not write at all")
		})
	}
}

func TestGeocodeContactAddress_ConcurrentEditIsAPreconditionFailure(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := geocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	// While the (slow) lookup runs, someone else edits the contact.
	fake.onCall = func() {
		var racing models.Contact
		require.NoError(t, db.First(&racing, c.ID).Error)
		racing.Nickname = "edited meanwhile"
		require.NoError(t, db.Save(&racing).Error)
	}

	w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[0].ID, "")
	assert.Equal(t, http.StatusPreconditionFailed, w.Code, w.Body.String())

	var stored models.Contact
	require.NoError(t, db.First(&stored, c.ID).Error)
	assert.Equal(t, "edited meanwhile", stored.Nickname, "the concurrent edit is not clobbered")
	assert.Empty(t, stored.Addresses[0].Coordinates)
}

func TestGeocodeContactAddress_DBFailureIs500(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := geocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	fake.onCall = func() { require.NoError(t, db.Exec("DROP TABLE contacts_fts").Error) }
	w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[0].ID, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

// --- POST /contacts/:id/addresses/geocode (stateless draft lookup) ----------

func draftGeocodeRouter(t *testing.T, g addressGeocoder) (*gorm.DB, *gin.Engine, uint) {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.POST("/contacts/:id/addresses/geocode", withValidated(func() any { return &models.GeocodeDraftInput{} }), GeocodeContactAddressDraft(g))
	return db, router, user.ID
}

func TestGeocodeContactAddressDraft_ReturnsCoordinatesWithoutWriting(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:51.5,-0.12"}
	db, router, uid := draftGeocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	w := sendJSON(router, http.MethodPost, fmt.Sprintf("/contacts/%d/addresses/geocode", c.ID), models.GeocodeDraftInput{
		Street: "1 Main St", City: "Springfield", Region: "IL", Postal: "62701", Country: "US",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, map[string]any{"coordinates": "geo:51.5,-0.12", "cached": false}, body,
		"the draft response carries no address_id")

	require.Len(t, fake.calls, 1, "exactly one lookup")
	assert.Equal(t, "1 Main St", fake.calls[0].Street)
	assert.Equal(t, "Springfield", fake.calls[0].City)
	assert.Empty(t, fake.calls[0].ID, "the body has no id and none is invented")

	var stored models.Contact
	require.NoError(t, db.First(&stored, c.ID).Error)
	assert.Empty(t, stored.Addresses[0].Coordinates, "the draft lookup writes nothing")
	assert.Equal(t, c.Revision, stored.Revision, "and does not touch the revision")
}

func TestGeocodeContactAddressDraft_SensitivityGate(t *testing.T) {
	for _, sensitivity := range []string{models.RelationshipSensitivityPrivate, models.RelationshipSensitivitySecret} {
		t.Run(sensitivity+" refused without opt-in", func(t *testing.T) {
			fake := &fakeAddressGeocoder{uri: "geo:5,5"}
			db, router, uid := draftGeocodeRouter(t, fake)
			c := seedGeocodeContact(t, db, uid, "")
			body := models.GeocodeDraftInput{Street: "1 Main St", Sensitivity: sensitivity}

			for _, q := range []string{"", "?include_sensitive=false", "?include_sensitive=yes"} {
				w := sendJSON(router, http.MethodPost, fmt.Sprintf("/contacts/%d/addresses/geocode%s", c.ID, q), body)
				assert.Equal(t, http.StatusBadRequest, w.Code, "query %q: %s", q, w.Body.String())
				assert.Contains(t, w.Body.String(), sensitivity)
			}
			assert.Empty(t, fake.calls, "a refused address never reaches the geocoder")
		})

		t.Run(sensitivity+" allowed with the explicit opt-in", func(t *testing.T) {
			fake := &fakeAddressGeocoder{uri: "geo:5,5"}
			db, router, uid := draftGeocodeRouter(t, fake)
			c := seedGeocodeContact(t, db, uid, "")
			body := models.GeocodeDraftInput{Street: "1 Main St", Sensitivity: sensitivity}

			w := sendJSON(router, http.MethodPost, fmt.Sprintf("/contacts/%d/addresses/geocode?include_sensitive=true", c.ID), body)
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Len(t, fake.calls, 1)
		})
	}
}

func TestGeocodeContactAddressDraft_NotFoundCases(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := draftGeocodeRouter(t, fake)

	stranger := models.User{Username: "draft-stranger", Password: "password123", Email: "draft-stranger@example.com"}
	require.NoError(t, db.Create(&stranger).Error)
	theirs := seedGeocodeContact(t, db, stranger.ID, "")

	deleted := seedGeocodeContact(t, db, uid, "")
	require.NoError(t, db.Delete(&models.Contact{}, deleted.ID).Error)

	for name, contact := range map[string]string{
		"another user's contact": fmt.Sprint(theirs.ID),
		"nonexistent contact":    "999999",
		"soft-deleted contact":   fmt.Sprint(deleted.ID),
	} {
		t.Run(name, func(t *testing.T) {
			w := sendJSON(router, http.MethodPost, fmt.Sprintf("/contacts/%s/addresses/geocode", contact), models.GeocodeDraftInput{Street: "1 Main St"})
			assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		})
	}
	assert.Empty(t, fake.calls, "nothing was looked up for any of them")
}

func TestGeocodeContactAddressDraft_RejectsNonNumericContactID(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	_, router, _ := draftGeocodeRouter(t, fake)
	w := sendJSON(router, http.MethodPost, "/contacts/abc/addresses/geocode", models.GeocodeDraftInput{Street: "1 Main St"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Empty(t, fake.calls)
}

func TestGeocodeContactAddressDraft_RejectsInvalidSensitivity(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := draftGeocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")
	// The real middleware, so the oneof tag is what rejects the value (the
	// withValidated test helper only binds JSON, it does not run validators).
	router.POST("/validated/contacts/:id/addresses/geocode", middleware.ValidateJSONMiddleware(&models.GeocodeDraftInput{}), GeocodeContactAddressDraft(fake))
	w := sendJSON(router, http.MethodPost, fmt.Sprintf("/validated/contacts/%d/addresses/geocode", c.ID), map[string]string{"sensitivity": "top-secret"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Empty(t, fake.calls)
}

// The GetValidated guard is defensive — the route is always registered behind
// ValidateJSONMiddleware — but a route accidentally wired without it must fail
// closed rather than act on a nil body.
func TestGeocodeContactAddressDraft_MissingValidatedBodyIs400(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := draftGeocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")
	router.POST("/raw/contacts/:id/addresses/geocode", GeocodeContactAddressDraft(fake))
	w := sendJSON(router, http.MethodPost, fmt.Sprintf("/raw/contacts/%d/addresses/geocode", c.ID), models.GeocodeDraftInput{Street: "1 Main St"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Empty(t, fake.calls)
}

func TestGeocodeContactAddressDraft_ContactLookupFailureIs500(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := draftGeocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	dbtest.HideTable(t, db, "contacts")
	w := sendJSON(router, http.MethodPost, fmt.Sprintf("/contacts/%d/addresses/geocode", c.ID), models.GeocodeDraftInput{Street: "1 Main St"})
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Empty(t, fake.calls)
}

func TestGeocodeContactAddressDraft_DisabledInstance(t *testing.T) {
	fake := &fakeAddressGeocoder{disabled: true}
	db, router, uid := draftGeocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	w := sendJSON(router, http.MethodPost, fmt.Sprintf("/contacts/%d/addresses/geocode", c.ID), models.GeocodeDraftInput{Street: "1 Main St"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not enabled")
	assert.Empty(t, fake.calls)
}

func TestGeocodeContactAddressDraft_DoesNotLeakProviderText(t *testing.T) {
	const providerSecret = "key=SUPER-SECRET"
	fake := &fakeAddressGeocoder{err: fmt.Errorf("%w: %s", services.ErrGeocoderUnreachable, providerSecret)}
	db, router, uid := draftGeocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	w := sendJSON(router, http.MethodPost, fmt.Sprintf("/contacts/%d/addresses/geocode", c.ID), models.GeocodeDraftInput{Street: "1 Main St"})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), providerSecret)
}

func TestGeocodeContactAddressDraft_RequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db, _ := setupRouter(t)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // a db, but no userID: as if AuthMiddleware were bypassed
	router.POST("/contacts/:id/addresses/geocode", withValidated(func() any { return &models.GeocodeDraftInput{} }), GeocodeContactAddressDraft(&fakeAddressGeocoder{uri: "geo:1,1"}))
	w := sendJSON(router, http.MethodPost, "/contacts/1/addresses/geocode", models.GeocodeDraftInput{Street: "1 Main St"})
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

// --- GET /api/v1/config/map -------------------------------------------------

func TestMapConfigHandler(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	get := func(cfg *config.Config) (*httptest.ResponseRecorder, map[string]any) {
		router := gin.New()
		router.GET("/config/map", MapConfigHandler(cfg))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config/map", nil))
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return w, body
	}

	w, body := get(&config.Config{})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, map[string]any{"tile_style_url": config.DefaultMapTileStyleURL}, body, "unset falls back to OpenFreeMap")

	_, body = get(&config.Config{MapTileStyleURL: " https://tiles.example.org/style.json "})
	assert.Equal(t, map[string]any{"tile_style_url": "https://tiles.example.org/style.json"}, body)

	// Deliberately narrow: nothing else about the instance rides this endpoint.
	_, body = get(&config.Config{
		MapTileStyleURL: "https://t.example/s.json", GeocoderProvider: config.GeocoderProviderMapTiler,
		GeocoderAPIKey: "do-not-leak", JWTSecretKey: "also-secret",
	})
	assert.Len(t, body, 1)
	assert.NotContains(t, fmt.Sprint(body), "do-not-leak")
}

// --- address validation on create / update ----------------------------------

func addressCreateRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	db, router := setupRouter(t)
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
	return db, router
}

func recordInputWith(addrs ...contactmodel.Address) models.ContactRecordInput {
	return models.ContactRecordInput{Card: contactmodel.Card{
		Name:      &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Val"}}},
		Addresses: addrs,
	}}
}

func sendJSON(router *gin.Engine, method, path string, v any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(v)
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCreateAndUpdateContact_RejectInvalidAddressMapFields(t *testing.T) {
	db, router := addressCreateRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	existing := seedGeocodeContact(t, db, user.ID, "")

	bad := map[string]contactmodel.Address{
		"card.addresses[0].coordinates": {Full: "x", Coordinates: "51.5,-0.12"},
		"coordinates out of range":      {Full: "x", Coordinates: "geo:91,0"},
		"card.addresses[0].sensitivity": {Full: "x", Sensitivity: "top-secret"},
	}
	for name, addr := range bad {
		t.Run("create "+name, func(t *testing.T) {
			w := sendJSON(router, http.MethodPost, "/contacts", recordInputWith(addr))
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "card.addresses[0].")
		})
		t.Run("update "+name, func(t *testing.T) {
			w := sendJSON(router, http.MethodPut, fmt.Sprintf("/contacts/%d", existing.ID), recordInputWith(addr))
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}

	var untouched models.Contact
	require.NoError(t, db.First(&untouched, existing.ID).Error)
	assert.Equal(t, existing.Revision, untouched.Revision, "a rejected update writes nothing")

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("firstname = ?", "Val").Count(&count).Error)
	assert.Zero(t, count, "a rejected create stores nothing")
}

func TestCreateContact_MintsAddressIDsAndKeepsCoordinatesAndSensitivity(t *testing.T) {
	_, router := addressCreateRouter(t)
	w := sendJSON(router, http.MethodPost, "/contacts", recordInputWith(
		contactmodel.Address{Full: "1 Main St", Coordinates: "geo:48.2,16.3", Sensitivity: "private"},
		contactmodel.Address{ID: "client-chosen", Full: "2 Side St"},
	))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		Contact struct {
			Card struct {
				Addresses []contactmodel.Address `json:"addresses"`
			} `json:"card"`
		} `json:"contact"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	addrs := resp.Contact.Card.Addresses
	require.Len(t, addrs, 2)
	assert.NotEmpty(t, addrs[0].ID, "the server minted an ID the client can now use for the geocode route")
	assert.Equal(t, "geo:48.2,16.3", addrs[0].Coordinates)
	assert.Equal(t, "private", addrs[0].Sensitivity)
	assert.Equal(t, "client-chosen", addrs[1].ID)
}

func TestUpdateContact_RoundTrippedAddressKeepsIDAndCoordinates(t *testing.T) {
	db, router := addressCreateRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	c := seedGeocodeContact(t, db, user.ID, models.RelationshipSensitivityPrivate)

	// A client echoes back what it read (the web editor's behaviour), with a name change.
	in := recordInputWith(
		contactmodel.Address{ID: c.Addresses[0].ID, Components: c.Card.Addresses[0].Components, Full: "1 Main St", Sensitivity: "private", Coordinates: "geo:3,4"},
		contactmodel.Address{ID: c.Addresses[1].ID, Components: c.Card.Addresses[1].Components, Full: "2 Side St", Coordinates: "geo:1,1"},
	)
	w := sendJSON(router, http.MethodPut, fmt.Sprintf("/contacts/%d", c.ID), in)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.Contact
	require.NoError(t, db.First(&stored, c.ID).Error)
	assert.Equal(t, c.Addresses[0].ID, stored.Addresses[0].ID)
	assert.Equal(t, c.Addresses[1].ID, stored.Addresses[1].ID)
	assert.Equal(t, "geo:3,4", stored.Addresses[0].Coordinates)
	assert.Equal(t, "private", stored.Addresses[0].Sensitivity)
}

func TestGeocodeContactAddress_RequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db, _ := setupRouter(t)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // a db, but no userID: as if AuthMiddleware were bypassed
	router.POST("/contacts/:id/addresses/:addressId/geocode", GeocodeContactAddress(&fakeAddressGeocoder{uri: "geo:1,1"}))
	w := postGeocode(router, "1", "x", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestGeocodeContactAddress_ContactLookupFailureIs500(t *testing.T) {
	fake := &fakeAddressGeocoder{uri: "geo:5,5"}
	db, router, uid := geocodeRouter(t, fake)
	c := seedGeocodeContact(t, db, uid, "")

	dbtest.HideTable(t, db, "contacts")
	w := postGeocode(router, fmt.Sprint(c.ID), c.Addresses[0].ID, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Empty(t, fake.calls)
}
