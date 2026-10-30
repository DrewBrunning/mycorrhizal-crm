package controllers

// Per-contact field visibility API (issue #1997). Real migrated schema
// (trap #1); the handlers are driven through a gin router with the caller's
// userID injected, exactly like the other contact sub-resource tests.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fieldVisibilityEnv struct {
	db      *gorm.DB
	router  *gin.Engine
	user    models.User
	contact models.Contact
}

func newFieldVisibilityEnv(t *testing.T) fieldVisibilityEnv {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Vis", Lastname: "Ible", Email: "vis@example.com"}
	require.NoError(t, db.Create(&contact).Error)

	router.GET("/contacts/:id/field-visibility", GetContactFieldVisibility)
	router.PUT("/contacts/:id/field-visibility", withValidated(func() any { return &models.ContactFieldVisibilityInput{} }), PutContactFieldVisibility)
	router.DELETE("/contacts/:id/field-visibility", ResetContactFieldVisibility)
	router.PATCH("/users/hide-empty-fields", withValidated(func() any { return &models.HideEmptyFieldsInput{} }), UpdateHideEmptyFields)
	router.GET("/users/enabled-contact-fields", GetEnabledContactFields)
	return fieldVisibilityEnv{db: db, router: router, user: user, contact: contact}
}

func (e fieldVisibilityEnv) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		switch b := body.(type) {
		case string:
			buf.WriteString(b)
		default:
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func (e fieldVisibilityEnv) path() string {
	return "/contacts/" + strconv.Itoa(int(e.contact.ID)) + "/field-visibility"
}

func decodeVisibility(t *testing.T, w *httptest.ResponseRecorder) models.ContactFieldVisibilityResponse {
	t.Helper()
	var out models.ContactFieldVisibilityResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	return out
}

func TestFieldVisibility_GetDefaultsToInheritedBuiltIn(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	w := e.do(t, "GET", e.path(), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := decodeVisibility(t, w)

	assert.Equal(t, map[string]string{}, got.Overrides, "overrides serialize as {} (never null)")
	assert.Equal(t, models.FieldStateInherit, got.HideEmpty)
	assert.True(t, got.Resolved.Fields["emails"])
	assert.False(t, got.Resolved.Fields["suffix"])
	assert.False(t, got.Resolved.HideEmpty)
	assert.Len(t, got.Resolved.Fields, len(models.ContactFieldKeys))

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `{}`, string(raw["overrides"]))
}

func TestFieldVisibility_PutThenGetRoundTripAndResolutionOrder(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	require.NoError(t, e.db.Model(&models.User{}).Where("id = ?", e.user.ID).
		Updates(map[string]any{"enabled_contact_fields": `["emails","suffix"]`}).Error)

	w := e.do(t, "PUT", e.path(), models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide", "phones": "show", "suffix": "inherit"},
		HideEmpty: "hide",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	put := decodeVisibility(t, w)
	assert.Equal(t, map[string]string{"emails": "hide", "phones": "show"}, put.Overrides, "inherit is not stored")
	assert.Equal(t, "hide", put.HideEmpty)

	got := decodeVisibility(t, e.do(t, "GET", e.path(), nil))
	assert.Equal(t, put, got, "GET returns what PUT returned")
	assert.False(t, got.Resolved.Fields["emails"], "per-contact hide beats the account default")
	assert.True(t, got.Resolved.Fields["phones"], "per-contact show beats the account default")
	assert.True(t, got.Resolved.Fields["suffix"], "an untouched field inherits the account default")
	assert.True(t, got.Defaults.Fields["emails"], "defaults are the account-level decision")
	assert.True(t, got.Resolved.HideEmpty)
	assert.False(t, got.Defaults.HideEmpty)
}

func TestFieldVisibility_PutIsAFullReplace(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	require.Equal(t, http.StatusOK, e.do(t, "PUT", e.path(), models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide", "phones": "hide"}, HideEmpty: "hide",
	}).Code)
	w := e.do(t, "PUT", e.path(), `{"overrides":{"emails":"show"}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := decodeVisibility(t, w)
	assert.Equal(t, map[string]string{"emails": "show"}, got.Overrides)
	assert.Equal(t, models.FieldStateInherit, got.HideEmpty, "an absent hide_empty means inherit")
}

func TestFieldVisibility_RejectsUnknownKeyWithReasonCode(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	w := e.do(t, "PUT", e.path(), `{"overrides":{"emails":"hide","firstname":"hide"}}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var env struct {
		Error struct {
			Code         string         `json:"code"`
			Message      string         `json:"message"`
			ReasonCode   string         `json:"reason_code"`
			ReasonParams map[string]any `json:"reason_params"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), w.Body.String())
	assert.Equal(t, "contact_field_override_unknown_key", env.Error.ReasonCode)
	assert.Equal(t, map[string]any{"field": "firstname"}, env.Error.ReasonParams)
	assert.Equal(t, "firstname is not a field that can be shown or hidden", env.Error.Message)

	assert.Zero(t, countRows(t, e.db, "contact_field_overrides", "user_id = ?", e.user.ID), "a rejected PUT writes nothing, not even the valid entries")
}

func TestFieldVisibility_RejectsReservedKeyAndBadStates(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	cases := []struct {
		name, body, reason, field string
	}{
		{"reserved key", `{"overrides":{"__hide_empty":"hide"}}`, "contact_field_override_unknown_key", "__hide_empty"},
		{"bad field state", `{"overrides":{"emails":"maybe"}}`, "contact_field_override_state_invalid", "emails"},
		{"bad hide_empty state", `{"hide_empty":"on"}`, "contact_field_override_state_invalid", "hide_empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := e.do(t, "PUT", e.path(), tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var env struct {
				Error struct {
					ReasonCode   string         `json:"reason_code"`
					ReasonParams map[string]any `json:"reason_params"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
			assert.Equal(t, tc.reason, env.Error.ReasonCode)
			assert.Equal(t, tc.field, env.Error.ReasonParams["field"])
		})
	}
}

func TestFieldVisibility_DeleteResetsToDefault(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	require.Equal(t, http.StatusOK, e.do(t, "PUT", e.path(), models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"}, HideEmpty: "show",
	}).Code)

	w := e.do(t, "DELETE", e.path(), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := decodeVisibility(t, w)
	assert.Empty(t, got.Overrides)
	assert.Equal(t, models.FieldStateInherit, got.HideEmpty)
	assert.True(t, got.Resolved.Fields["emails"])
	assert.Zero(t, countRows(t, e.db, "contact_field_overrides", "user_id = ?", e.user.ID))
}

func TestFieldVisibility_OtherUsersContactIs404AndUntouched(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Not", Lastname: "Yours"}
	require.NoError(t, e.db.Create(&foreign).Error)
	require.NoError(t, e.db.Create(&models.ContactFieldOverride{UserID: other.ID, ContactVCardUID: foreign.VCardUID, FieldKey: "emails", State: "hide"}).Error)
	fpath := "/contacts/" + strconv.Itoa(int(foreign.ID)) + "/field-visibility"

	for _, tc := range []struct {
		method string
		body   any
	}{
		{"GET", nil},
		{"PUT", `{"overrides":{"emails":"show"}}`},
		{"DELETE", nil},
	} {
		w := e.do(t, tc.method, fpath, tc.body)
		assert.Equal(t, http.StatusNotFound, w.Code, "%s: %s", tc.method, w.Body.String())
		assert.NotContains(t, w.Body.String(), "hide", "no oracle for the other user's overrides")
	}

	var row models.ContactFieldOverride
	require.NoError(t, e.db.Where("user_id = ?", other.ID).First(&row).Error)
	assert.Equal(t, "hide", row.State, "the owner's row is untouched")
	assert.EqualValues(t, 1, countRows(t, e.db, "contact_field_overrides", "1 = 1"), "nothing was written for the caller")

	// A trashed contact is also a 404 (not live).
	require.NoError(t, e.db.Delete(&models.Contact{}, e.contact.ID).Error)
	assert.Equal(t, http.StatusNotFound, e.do(t, "GET", e.path(), nil).Code)
}

func TestFieldVisibility_MalformedIDIs400(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	assert.Equal(t, http.StatusBadRequest, e.do(t, "GET", "/contacts/abc/field-visibility", nil).Code)
}

// Display preferences are not card data: writing them must not bump the
// contact's revision/ETag (which CardDAV clients and the ?since= feed read) or
// change the contact record the API serves.
func TestFieldVisibility_WritesDoNotTouchTheContact(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	e.router.GET("/contacts/:id", GetContact)
	contactPath := "/contacts/" + strconv.Itoa(int(e.contact.ID))
	before := e.do(t, "GET", contactPath, nil)
	require.Equal(t, http.StatusOK, before.Code, before.Body.String())
	var rowBefore models.Contact
	require.NoError(t, e.db.First(&rowBefore, e.contact.ID).Error)

	require.Equal(t, http.StatusOK, e.do(t, "PUT", e.path(), models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide", "phones": "show"}, HideEmpty: "hide",
	}).Code)
	require.Equal(t, http.StatusOK, e.do(t, "DELETE", e.path(), nil).Code)
	require.Equal(t, http.StatusOK, e.do(t, "PUT", e.path(), models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"},
	}).Code)

	var rowAfter models.Contact
	require.NoError(t, e.db.First(&rowAfter, e.contact.ID).Error)
	assert.Equal(t, rowBefore.Revision, rowAfter.Revision, "no revision bump")
	assert.Equal(t, rowBefore.ETag, rowAfter.ETag, "no ETag change")

	after := e.do(t, "GET", contactPath, nil)
	assert.JSONEq(t, before.Body.String(), after.Body.String(), "the served contact record is byte-for-value identical")
	assert.NotContains(t, after.Body.String(), "field_visibility")
	assert.NotContains(t, after.Body.String(), "hide_empty")
}

// Overrides never reach an export surface: the vCard, JSContact and CSV
// exports are identical with and without overrides, and carry no trace of them.
func TestFieldVisibility_NeverAppearsInExports(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	registerVCFRoute(e.router, "")
	registerJSContactRoute(e.router, "")
	e.router.GET("/export", ExportData)
	paths := []string{"/export/vcf", "/export/vcf?include_sensitive=true", "/export/jscontact", "/export/jscontact?include_sensitive=true", "/export"}

	fetch := func() map[string]string {
		out := map[string]string{}
		for _, p := range paths {
			w := e.do(t, "GET", p, nil)
			require.Equal(t, http.StatusOK, w.Code, "%s: %s", p, w.Body.String())
			out[p] = w.Body.String()
		}
		return out
	}
	// Exports embed no per-request timestamps for a fixed contact set, but
	// normalise the CSV header-less rows defensively by comparing as-is first.
	baseline := fetch()

	all := map[string]string{}
	for _, k := range models.ContactFieldKeys {
		all[k] = "show"
	}
	require.Equal(t, http.StatusOK, e.do(t, "PUT", e.path(), models.ContactFieldVisibilityInput{Overrides: all, HideEmpty: "hide"}).Code)
	withOverrides := fetch()
	for _, p := range paths {
		assert.Equal(t, baseline[p], withOverrides[p], "%s changed once overrides existed", p)
		assert.NotContains(t, withOverrides[p], "field_key")
		assert.NotContains(t, withOverrides[p], "__hide_empty")
		assert.NotContains(t, withOverrides[p], "contact_field_overrides")
	}
}

func TestFieldVisibility_HideEmptyAccountDefault(t *testing.T) {
	e := newFieldVisibilityEnv(t)

	w := e.do(t, "GET", "/users/enabled-contact-fields", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var settings struct {
		HideEmptyFields bool `json:"hide_empty_fields"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &settings))
	assert.False(t, settings.HideEmptyFields, "off for an existing/new account")

	w = e.do(t, "PATCH", "/users/hide-empty-fields", `{"hide_empty_fields":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, decodeVisibility(t, e.do(t, "GET", e.path(), nil)).Resolved.HideEmpty, "the account default flows into every contact")

	// A per-contact "show" beats it.
	require.Equal(t, http.StatusOK, e.do(t, "PUT", e.path(), models.ContactFieldVisibilityInput{HideEmpty: "show"}).Code)
	got := decodeVisibility(t, e.do(t, "GET", e.path(), nil))
	assert.False(t, got.Resolved.HideEmpty)
	assert.True(t, got.Defaults.HideEmpty)

	// An absent value is rejected, not read as "off".
	w = e.do(t, "PATCH", "/users/hide-empty-fields", `{}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var user models.User
	require.NoError(t, e.db.First(&user, e.user.ID).Error)
	assert.True(t, user.HideEmptyFields)
	assert.Nil(t, user.EnabledContactFields, "the account field list is untouched")
}

func countRows(t *testing.T, db *gorm.DB, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table(table).Where(where, args...).Count(&n).Error)
	return n
}
