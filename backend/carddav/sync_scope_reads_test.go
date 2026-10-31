package carddav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scopeReadFixture struct {
	backend *Backend
	user    models.User
	a, b    models.Circle
	c1      models.Contact
	c2      models.Contact
	c3      models.Contact
}

func newScopeReadFixture(t *testing.T) *scopeReadFixture {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "scope-reader", Password: "password123!A", Email: "scope-reader@example.com"}
	require.NoError(t, db.Create(&user).Error)

	a := models.Circle{UserID: user.ID, Name: "A"}
	b := models.Circle{UserID: user.ID, Name: "B"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)

	c1 := models.Contact{UserID: user.ID, Firstname: "One"}
	c2 := models.Contact{UserID: user.ID, Firstname: "Two"}
	c3 := models.Contact{UserID: user.ID, Firstname: "Three"}
	require.NoError(t, db.Create(&c1).Error)
	require.NoError(t, db.Create(&c2).Error)
	require.NoError(t, db.Create(&c3).Error)

	require.NoError(t, db.Create(&models.CircleMember{CircleID: a.ID, UserID: user.ID, MemberVCardUID: c1.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: a.ID, UserID: user.ID, MemberVCardUID: c2.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: b.ID, UserID: user.ID, MemberVCardUID: c2.VCardUID}).Error)

	return &scopeReadFixture{backend: NewBackend(db, t.TempDir()), user: user, a: a, b: b, c1: c1, c2: c2, c3: c3}
}

func (f *scopeReadFixture) ctx() context.Context {
	return ContextWithUser(context.Background(), f.user.ID, f.user.Username, f.backend.db, f.backend.photoDir, "")
}

func (f *scopeReadFixture) setScope(t *testing.T, mode string, circleIDs []string) {
	t.Helper()
	sel := services.CardDAVSyncScopeSelection{Mode: mode, CircleIDs: circleIDs}
	if mode == models.CardDAVSyncScopeModeInclude && len(circleIDs) > 0 {
		def := circleIDs[0]
		sel.DefaultCircleID = &def
	}
	_, err := services.ReplaceCardDAVSyncScope(f.backend.db, f.user.ID, sel)
	require.NoError(t, err)
}

func (f *scopeReadFixture) list(t *testing.T) []string {
	t.Helper()
	objects, err := f.backend.ListAddressObjects(f.ctx(), "/carddav/addressbooks/"+f.user.Username+"/contacts/", nil)
	require.NoError(t, err)
	uids := make([]string, 0, len(objects))
	for _, object := range objects {
		uids = append(uids, strings.TrimSuffix(path.Base(object.Path), ".vcf"))
	}
	return uids
}

// ADR 0041 decision 4: a listing contains exactly the in-scope contacts for
// each mode, a contact in two circles appears once, and a contact with no
// circle is out for include but in for exclude.
func TestSyncScope_ListModes(t *testing.T) {
	f := newScopeReadFixture(t)

	require.ElementsMatch(t, []string{f.c1.VCardUID, f.c2.VCardUID, f.c3.VCardUID}, f.list(t), "no row is all")

	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})
	require.ElementsMatch(t, []string{f.c1.VCardUID, f.c2.VCardUID}, f.list(t))

	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID, f.b.ID})
	require.ElementsMatch(t, []string{f.c1.VCardUID, f.c2.VCardUID}, f.list(t), "c2 is in two circles but appears once")

	f.setScope(t, models.CardDAVSyncScopeModeExclude, []string{f.b.ID})
	require.ElementsMatch(t, []string{f.c1.VCardUID, f.c3.VCardUID}, f.list(t), "no-circle c3 is in scope for exclude")

	// Reachable through circle deletion (ADR 0041 decision 3): include with
	// no circles syncs nothing and never falls back to all; exclude with no
	// circles is equivalent to all.
	_, err := services.ReplaceCardDAVSyncScope(f.backend.db, f.user.ID, services.CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude})
	require.NoError(t, err)
	require.Empty(t, f.list(t))

	_, err = services.ReplaceCardDAVSyncScope(f.backend.db, f.user.ID, services.CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude})
	require.NoError(t, err)
	require.Len(t, f.list(t), 3)
}

// A scope change only changes presence in the listing: no row is deleted and
// no contact's revision/ETag is bumped (ADR 0041 decision 4). Narrowing and
// widening therefore look like a client-side delete/re-download.
func TestSyncScope_NarrowingLeavesDataAndETagsAlone(t *testing.T) {
	f := newScopeReadFixture(t)
	all, err := f.backend.ListAddressObjects(f.ctx(), "/carddav/addressbooks/"+f.user.Username+"/contacts/", nil)
	require.NoError(t, err)
	etags := map[string]string{}
	for _, object := range all {
		etags[strings.TrimSuffix(path.Base(object.Path), ".vcf")] = object.ETag
	}

	var before models.Contact
	require.NoError(t, f.backend.db.First(&before, f.c3.ID).Error)

	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})
	narrowed, err := f.backend.ListAddressObjects(f.ctx(), "/carddav/addressbooks/"+f.user.Username+"/contacts/", nil)
	require.NoError(t, err)
	require.Len(t, narrowed, 2)
	for _, object := range narrowed {
		uid := strings.TrimSuffix(path.Base(object.Path), ".vcf")
		assert.Equal(t, etags[uid], object.ETag, "widening/narrowing must not change an ETag")
	}

	var after models.Contact
	require.NoError(t, f.backend.db.First(&after, f.c3.ID).Error, "the out-of-scope contact must still exist")
	assert.Equal(t, before.Revision, after.Revision)
	assert.Equal(t, before.ETag, after.ETag)
}

// Resource GET and addressbook-multiget return 404 for an out-of-scope href,
// including the legacy numeric-id fallback. The in-scope resource still serves.
func TestSyncScope_GetAndMultigetMaskOutOfScope(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})
	gin.SetMode(gin.TestMode)
	h := NewHandler(f.backend.db, f.backend.photoDir).GinHandler()

	get := func(href string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, href, nil)
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("userID", f.user.ID)
		c.Set("username", f.user.Username)
		h(c)
		return w
	}

	base := "/carddav/addressbooks/" + f.user.Username + "/contacts/"
	require.Equal(t, http.StatusOK, get(base+f.c1.VCardUID+".vcf").Code)
	require.Equal(t, http.StatusNotFound, get(base+f.c3.VCardUID+".vcf").Code)
	require.Equal(t, http.StatusNotFound, get(base+itoa(f.c3.ID)+".vcf").Code, "numeric-id fallback must be scoped too")

	body := `<?xml version="1.0" encoding="utf-8"?>
<card:addressbook-multiget xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop><d:getetag/><card:address-data/></d:prop>
  <d:href>` + base + f.c1.VCardUID + `.vcf</d:href>
  <d:href>` + base + f.c3.VCardUID + `.vcf</d:href>
</card:addressbook-multiget>`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("REPORT", base, strings.NewReader(body))
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("userID", f.user.ID)
	c.Set("username", f.user.Username)
	h(c)

	require.Equal(t, http.StatusMultiStatus, w.Code)
	assert.Contains(t, w.Body.String(), f.c1.VCardUID+".vcf")
	assert.Contains(t, w.Body.String(), f.c3.VCardUID+".vcf")
	assert.Contains(t, w.Body.String(), "404 Not Found", "the out-of-scope href gets a per-href 404, never data")
	assert.NotContains(t, w.Body.String(), "Three", "the out-of-scope card body must not leak")
}

// Filter-less addressbook-query (the real-client enumeration path) is scoped
// over the wire too.
func TestSyncScope_ReportFilters(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})
	gin.SetMode(gin.TestMode)
	h := NewHandler(f.backend.db, f.backend.photoDir).GinHandler()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("REPORT", "/carddav/addressbooks/"+f.user.Username+"/contacts/", strings.NewReader(
		`<?xml version="1.0" encoding="utf-8" ?>
<card:addressbook-query xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop><d:getetag/><card:address-data/></d:prop>
</card:addressbook-query>`))
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("userID", f.user.ID)
	c.Set("username", f.user.Username)
	h(c)

	require.Equal(t, http.StatusMultiStatus, w.Code)
	assert.Contains(t, w.Body.String(), f.c1.VCardUID+".vcf")
	assert.Contains(t, w.Body.String(), f.c2.VCardUID+".vcf")
	assert.NotContains(t, w.Body.String(), f.c3.VCardUID+".vcf")
}

// Scope decides which contacts are served; sensitivity still decides what a
// served contact contains (ADR 0041 decision 4). An in-scope contact is never
// hidden because of sensitivity, and its private address stays withheld.
func TestSyncScope_SensitivityRemainsIndependent(t *testing.T) {
	f := newScopeReadFixture(t)
	record := &contactmodel.Record{Card: contactmodel.Card{
		UID:  "scope-secret",
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Addresses: []contactmodel.Address{
			{ID: "normal", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Normal St"}}, Full: "1 Normal St"},
			{ID: "secret", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "2 Secret St"}}, Full: "2 Secret St", Sensitivity: models.RelationshipSensitivitySecret},
		},
	}}
	contact := models.Contact{UserID: f.user.ID}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, f.backend.db.Create(&contact).Error)
	require.NoError(t, f.backend.db.Create(&models.CircleMember{CircleID: f.a.ID, UserID: f.user.ID, MemberVCardUID: contact.VCardUID}).Error)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	objects, err := f.backend.ListAddressObjects(f.ctx(), "/carddav/addressbooks/"+f.user.Username+"/contacts/", nil)
	require.NoError(t, err)
	found := false
	for _, object := range objects {
		if strings.Contains(object.Path, contact.VCardUID) {
			found = true
		}
	}
	require.True(t, found, "a sensitive contact is still in scope; sensitivity is a content filter")

	object, err := f.backend.GetAddressObject(f.ctx(), "/carddav/addressbooks/"+f.user.Username+"/contacts/"+contact.VCardUID+".vcf", nil)
	require.NoError(t, err)
	assert.NotContains(t, encodeCard(t, object.Card), "2 Secret St")
	assert.Contains(t, encodeCard(t, object.Card), "1 Normal St")
}

func itoa(n uint) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
