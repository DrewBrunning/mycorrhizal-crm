package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issues #2193/#2192: choosing "This is me" marks the contact ambient (never
// overdue), and "This isn't me" undoes that only when marking set it. The
// provenance lives on users.self_contact_ambient_set.

type selfAmbientEnv struct {
	db     *gorm.DB
	router *gin.Engine
	user   models.User
}

func newSelfAmbientEnv(t *testing.T) selfAmbientEnv {
	t.Helper()
	db := dbtest.New(t)
	closeTestDBAtTeardown(t, db)
	user := models.User{Username: "ambientme", Password: "password123!A", Email: "ambientme@example.com"}
	require.NoError(t, db.Create(&user).Error)
	router := selfContactTestRouter(t, db, user.ID)
	router.PATCH("/users/me/self-contact", middleware.ValidateJSONMiddleware(&models.SelfContactInput{}), UpdateSelfContact)
	router.PUT("/contacts/:id", middleware.ValidateJSONMiddleware(&models.ContactRecordInput{}), UpdateContact)
	return selfAmbientEnv{db: db, router: router, user: user}
}

func (e selfAmbientEnv) contact(t *testing.T, name string, ambient bool) models.Contact {
	t.Helper()
	c := models.Contact{UserID: e.user.ID, Firstname: name, Ambient: ambient}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e selfAmbientEnv) setMe(t *testing.T, uid string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"vcard_uid": uid})
	w := patchSelfContact(t, e.router, string(body))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func (e selfAmbientEnv) ambient(t *testing.T, id uint) bool {
	t.Helper()
	var c models.Contact
	require.NoError(t, e.db.First(&c, id).Error)
	return c.Ambient
}

func (e selfAmbientEnv) provenance(t *testing.T) bool {
	t.Helper()
	var u models.User
	require.NoError(t, e.db.First(&u, e.user.ID).Error)
	return u.SelfContactAmbientSet
}

func (e selfAmbientEnv) putAmbient(t *testing.T, c models.Contact, v bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"card": map[string]any{"uid": c.VCardUID, "name": map[string]any{"components": []map[string]string{{"kind": "given", "value": c.Firstname}}}},
		"crm":  map[string]any{"ambient": v},
	})
	req, err := http.NewRequest("PUT", "/contacts/"+strconv.Itoa(int(c.ID)), bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestUpdateSelfContact_MarkSetsAmbient(t *testing.T) {
	e := newSelfAmbientEnv(t)
	c := e.contact(t, "Me", false)

	e.setMe(t, c.VCardUID)

	assert.True(t, e.ambient(t, c.ID), "marking sets ambient")
	assert.True(t, e.provenance(t), "marking recorded that it set ambient")
}

func TestUpdateSelfContact_MarkAlreadyAmbientRecordsNoProvenance(t *testing.T) {
	e := newSelfAmbientEnv(t)
	c := e.contact(t, "Me", true)

	e.setMe(t, c.VCardUID)

	assert.True(t, e.ambient(t, c.ID))
	assert.False(t, e.provenance(t), "marking did not set ambient, so unmark must not clear it")
}

func TestUpdateSelfContact_UnmarkClearsAmbientWhenMarkingSetIt(t *testing.T) {
	e := newSelfAmbientEnv(t)
	c := e.contact(t, "Me", false)
	e.setMe(t, c.VCardUID)

	e.setMe(t, "")

	assert.False(t, e.ambient(t, c.ID))
	assert.False(t, e.provenance(t))
}

func TestUpdateSelfContact_UnmarkKeepsAmbientThatWasAlreadyOn(t *testing.T) {
	e := newSelfAmbientEnv(t)
	c := e.contact(t, "Me", true)
	e.setMe(t, c.VCardUID)

	e.setMe(t, "")

	assert.True(t, e.ambient(t, c.ID))
	assert.False(t, e.provenance(t))
}

// A manual ambient edit while marked is the user taking ownership: unmark must
// not override it, whichever way they edited.
func TestUpdateSelfContact_ManualAmbientEditWhileMarkedSurvivesUnmark(t *testing.T) {
	for _, tc := range []struct {
		name string
		back bool
		want bool
	}{
		{"turned off by hand stays off", false, false},
		{"off then on again by hand stays on", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSelfAmbientEnv(t)
			c := e.contact(t, "Me", false)
			e.setMe(t, c.VCardUID)

			e.putAmbient(t, c, false)
			if tc.back {
				e.putAmbient(t, c, true)
			}
			assert.False(t, e.provenance(t), "a manual change clears the provenance")

			e.setMe(t, "")

			assert.Equal(t, tc.want, e.ambient(t, c.ID))
		})
	}
}

func TestUpdateContact_UnchangedAmbientWhileMarkedKeepsProvenance(t *testing.T) {
	e := newSelfAmbientEnv(t)
	c := e.contact(t, "Me", false)
	e.setMe(t, c.VCardUID)

	// An unrelated save that echoes the current ambient value is not a manual
	// ambient edit.
	e.putAmbient(t, c, true)

	assert.True(t, e.provenance(t))
	e.setMe(t, "")
	assert.False(t, e.ambient(t, c.ID))
}

func TestUpdateSelfContact_SwitchingMeAppliesRulesToBoth(t *testing.T) {
	e := newSelfAmbientEnv(t)
	a := e.contact(t, "A", false)
	b := e.contact(t, "B", false)
	e.setMe(t, a.VCardUID)
	require.True(t, e.ambient(t, a.ID))

	e.setMe(t, b.VCardUID)

	assert.False(t, e.ambient(t, a.ID), "A was marked ambient by marking, so un-meing A clears it")
	assert.True(t, e.ambient(t, b.ID))
	assert.True(t, e.provenance(t))
}

func TestUpdateSelfContact_SwitchingMeKeepsPreexistingAmbientOnOld(t *testing.T) {
	e := newSelfAmbientEnv(t)
	a := e.contact(t, "A", true)
	b := e.contact(t, "B", true)
	e.setMe(t, a.VCardUID)

	e.setMe(t, b.VCardUID)

	assert.True(t, e.ambient(t, a.ID))
	assert.True(t, e.ambient(t, b.ID))
	assert.False(t, e.provenance(t))
}

func TestUpdateSelfContact_ReMarkingSameContactKeepsProvenance(t *testing.T) {
	e := newSelfAmbientEnv(t)
	c := e.contact(t, "Me", false)
	e.setMe(t, c.VCardUID)

	e.setMe(t, c.VCardUID)

	assert.True(t, e.ambient(t, c.ID))
	assert.True(t, e.provenance(t))
}

func TestUpdateSelfContact_ForeignContactIsNotAmbientMarked(t *testing.T) {
	e := newSelfAmbientEnv(t)
	other := models.User{Username: "someoneelse", Password: "password123!A", Email: "someoneelse@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, e.db.Create(&theirs).Error)

	body, _ := json.Marshal(map[string]string{"vcard_uid": theirs.VCardUID})
	w := patchSelfContact(t, e.router, string(body))

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, e.ambient(t, theirs.ID))
	assert.False(t, e.provenance(t))
}

// The flat column and crm.ambient in the stored card must agree, and the write
// must bump the revision so sync sees the change.
func TestUpdateSelfContact_AmbientWriteReachesCardAndRevision(t *testing.T) {
	e := newSelfAmbientEnv(t)
	c := e.contact(t, "Me", false)
	e.setMe(t, c.VCardUID)

	var got models.Contact
	require.NoError(t, e.db.First(&got, c.ID).Error)
	rec := models.RecordFromContact(&got, "")
	require.NotNil(t, rec.Envelope.Ambient)
	assert.True(t, *rec.Envelope.Ambient)
	assert.Greater(t, got.Revision, c.Revision)
}
