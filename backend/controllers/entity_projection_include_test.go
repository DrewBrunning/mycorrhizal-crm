package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v1.7.0 gate (#2335, #2298): one regression test per leaving surface for the
// entity-projection sensitivity rule -- an above-normal primary organisation or
// place is filtered in the query by default and comes back only through that
// surface's explicit include flag. The default-deny halves for REST/export/
// share/MCP/CardDAV live in entity_projection_surfaces_test.go and
// carddav/entity_projection_test.go; these cover the opt-in halves and the
// surfaces that had no test (standing share content, bulk-style export).

// The VCF export brings a private/secret entity's ORG/ADR back only with
// include_sensitive=true, for the 4.0 and 3.0 writers alike.
func TestEntityProjection_VCFExportIncludeFlag(t *testing.T) {
	for _, sens := range []string{"private", "secret"} {
		db, router := setupRouter(t)
		registerVCFRoute(router, "")
		var user models.User
		require.NoError(t, db.First(&user).Error)
		contact := seedProjectedContact(t, db, user.ID, sens)
		get := func(path string) string {
			req, _ := http.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			return w.Body.String()
		}
		base := "/export/vcf?vcard_uid=" + contact.VCardUID
		for _, q := range []string{"", "&version=3"} {
			assert.NotContains(t, get(base+q), "Acme Corp", "%s default%s", sens, q)
		}
		opt := get(base + "&include_sensitive=true")
		assert.Contains(t, opt, "Acme Corp", "%s opt-in", sens)
		assertNoEntityLeak(t, "vcf opt-in "+sens, opt)
	}
}

// The unscoped export (no vcard_uid: every contact) takes the same path.
func TestEntityProjection_ExportAllContactsFiltersAndOptsIn(t *testing.T) {
	db, router := setupRouter(t)
	registerVCFRoute(router, "")
	registerJSContactRoute(router, "")
	var user models.User
	require.NoError(t, db.First(&user).Error)
	seedProjectedContact(t, db, user.ID, "secret")
	get := func(path string) string {
		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return w.Body.String()
	}
	for _, p := range []string{"/export/vcf", "/export/jscontact"} {
		assert.NotContains(t, get(p), "Acme Corp", p)
		assert.NotContains(t, get(p), "Springfield", p)
		assert.Contains(t, get(p+"?include_sensitive=true"), "Acme Corp", p+" opt-in")
	}
}

// A one-time contact share carries an above-normal entity's value only when the
// sender sets include_sensitive; the entity's other fields never travel.
func TestEntityProjection_ContactShareIncludeFlag(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	contact := seedProjectedContact(t, db, sender.ID, "secret")
	router := contactShareRouterFor(db, cfg, sender.ID)
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID, VCardUID: contact.VCardUID, IncludeSensitive: true,
		Sections: []string{models.SectionOrganizations, models.SectionAddresses},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var share models.ContactShare
	require.NoError(t, db.Where("from_user_id = ?", sender.ID).First(&share).Error)
	assert.Contains(t, share.Payload, "Acme Corp")
	assertNoEntityLeak(t, "share opt-in", share.Payload)
	assert.NotContains(t, share.Payload, "ent-")
}

// MCP get_contact: private/secret entity withheld by default, returned with
// include_sensitive, still without the entity's notes/url/role label.
func TestEntityProjection_MCPGetContactIncludeFlag(t *testing.T) {
	f := newMCPFixture(t)
	for _, sens := range []string{"private", "secret"} {
		contact := seedProjectedContact(t, f.db, f.alice.ID, sens)
		def := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": contact.ID})
		require.False(t, def.isError, def.text)
		assert.NotContains(t, def.text, "Acme Corp", sens)
		assert.NotContains(t, def.text, "Springfield", sens)

		opt := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": contact.ID, "include_sensitive": true})
		require.False(t, opt.isError, opt.text)
		assert.Contains(t, opt.text, "Acme Corp", sens)
		assertNoEntityLeak(t, "mcp opt-in "+sens, opt.text)
	}
}

// Standing share content has no include flag at all (grantSelection forces it
// off): an above-normal entity never reaches the recipient, and the owner's
// re-confirmation cannot unlock it.
func TestEntityProjection_StandingShareNeverCarriesAboveNormalEntity(t *testing.T) {
	for _, sens := range []string{"private", "secret"} {
		f := newEntityGrantFixture(t)
		require.Contains(t, f.body(), "Acme Corp", "precondition: a normal entity is shared")
		require.NoError(t, f.e.db.Model(&f.org).Update("sensitivity", sens).Error)
		require.NoError(t, f.e.db.Model(&f.place).Update("sensitivity", sens).Error)

		assert.NotContains(t, f.body(), "Acme Corp", sens)
		assert.NotContains(t, f.body(), "1 Main St", sens)
		require.Equal(t, 200, f.e.do(f.e.A, http.MethodPost, "/share-grants/"+f.grantID+"/reconfirm", nil).Code)
		body := f.body()
		assert.NotContains(t, body, "Acme Corp", "%s: re-confirming does not widen sensitivity", sens)
		assert.NotContains(t, body, "1 Main St", sens)
		assertNoEntityLeak(t, "standing share "+sens, body)
	}
}

// The link carries its own sensitivity: a private link to a normal entity is
// withheld from every leaving surface by default and returned by the flag.
func TestEntityProjection_LinkSensitivityIsFilteredToo(t *testing.T) {
	f := newMCPFixture(t)
	contact := seedProjectedContact(t, f.db, f.alice.ID, "normal")
	require.NoError(t, f.db.Model(&models.ContactEntityLink{}).Where("contact_vcard_uid = ?", contact.VCardUID).
		Update("sensitivity", "private").Error)

	def := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": contact.ID})
	require.False(t, def.isError, def.text)
	assert.NotContains(t, def.text, "Acme Corp")
	assert.NotContains(t, def.text, "Springfield")
	opt := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": contact.ID, "include_sensitive": true})
	assert.Contains(t, opt.text, "Acme Corp")
	assert.Contains(t, opt.text, "Springfield")

	db, router := setupRouter(t)
	registerVCFRoute(router, "")
	var user models.User
	require.NoError(t, db.First(&user).Error)
	c2 := seedProjectedContact(t, db, user.ID, "normal")
	require.NoError(t, db.Model(&models.ContactEntityLink{}).Where("contact_vcard_uid = ?", c2.VCardUID).
		Update("sensitivity", "secret").Error)
	get := func(q string) string {
		req, _ := http.NewRequest("GET", "/export/vcf?vcard_uid="+c2.VCardUID+q, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		return w.Body.String()
	}
	assert.NotContains(t, get(""), "Acme Corp")
	assert.Contains(t, get("&include_sensitive=true"), "Acme Corp")
}
