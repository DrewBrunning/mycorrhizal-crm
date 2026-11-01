package carddav

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func (f *scopeReadFixture) put(t *testing.T, uid, vcard string, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewHandler(f.backend.db, f.backend.photoDir).GinHandler()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut,
		"/carddav/addressbooks/"+f.user.Username+"/contacts/"+uid+".vcf", strings.NewReader(vcard))
	req.Header.Set("Content-Type", "text/vcard")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("userID", f.user.ID)
	c.Set("username", f.user.Username)
	h(c)
	return w
}

func (f *scopeReadFixture) deleteViaHandler(t *testing.T, uid string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewHandler(f.backend.db, f.backend.photoDir).GinHandler()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete,
		"/carddav/addressbooks/"+f.user.Username+"/contacts/"+uid+".vcf", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("userID", f.user.ID)
	c.Set("username", f.user.Username)
	h(c)
	return w
}

func (f *scopeReadFixture) memberCircleIDs(t *testing.T, uid string) []string {
	t.Helper()
	var ids []string
	require.NoError(t, f.backend.db.Model(&models.CircleMember{}).
		Where("user_id = ? AND member_vcard_uid = ?", f.user.ID, uid).
		Pluck("circle_id", &ids).Error)
	return ids
}

func TestSyncScope_PutExistingOutOfScopeForbidden(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	var before models.Contact
	require.NoError(t, f.backend.db.First(&before, f.c3.ID).Error)

	w := f.put(t, f.c3.VCardUID, simpleVCard4(f.c3.VCardUID, "Changed"), "")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// A stale If-Match must still be 403: scope precedes the ETag check, so a
	// 412 cannot be used as an existence oracle.
	w = f.put(t, f.c3.VCardUID, simpleVCard4(f.c3.VCardUID, "Changed"), `"stale-etag"`)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var after models.Contact
	require.NoError(t, f.backend.db.First(&after, f.c3.ID).Error)
	assert.Equal(t, before.Firstname, after.Firstname, "an out-of-scope PUT writes nothing")
	assert.Equal(t, before.Revision, after.Revision)
}

func TestSyncScope_PutExistingInScopeSucceeds(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	w := f.put(t, f.c1.VCardUID, simpleVCard4(f.c1.VCardUID, "Changed"), "")
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())

	var stored models.Contact
	require.NoError(t, f.backend.db.First(&stored, f.c1.ID).Error)
	assert.Equal(t, "Changed", stored.Lastname)
}

func TestSyncScope_PutNewIncludeJoinsDefaultCircle(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	const uid = "phone-created-include"
	w := f.put(t, uid, simpleVCard4(uid, "Phone Created"), "")
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())
	assert.Equal(t, []string{f.a.ID}, f.memberCircleIDs(t, uid))
	assert.Contains(t, f.list(t), uid, "a newly filed contact is immediately in the listing")
}

func TestSyncScope_PutNewIncludeWithoutDefaultForbidden(t *testing.T) {
	f := newScopeReadFixture(t)
	_, err := services.ReplaceCardDAVSyncScope(f.backend.db, f.user.ID, services.CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID},
	})
	require.NoError(t, err)

	const uid = "phone-created-no-default"
	w := f.put(t, uid, simpleVCard4(uid, "No Default"), "")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var count int64
	require.NoError(t, f.backend.db.Model(&models.Contact{}).Where("user_id = ? AND vcard_uid = ?", f.user.ID, uid).Count(&count).Error)
	assert.Zero(t, count, "a refused create writes no contact")
}

func TestSyncScope_PutNewExcludeOptionalDefault(t *testing.T) {
	f := newScopeReadFixture(t)

	// Exclude B, default A: the contact is already in scope but is also
	// tagged into A.
	_, err := services.ReplaceCardDAVSyncScope(f.backend.db, f.user.ID, services.CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.b.ID}, DefaultCircleID: &f.a.ID,
	})
	require.NoError(t, err)
	const uid = "phone-created-exclude-default"
	w := f.put(t, uid, simpleVCard4(uid, "Exclude Default"), "")
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())
	assert.Equal(t, []string{f.a.ID}, f.memberCircleIDs(t, uid))

	// Exclude B, no default: still in scope, no circle added.
	_, err = services.ReplaceCardDAVSyncScope(f.backend.db, f.user.ID, services.CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.b.ID},
	})
	require.NoError(t, err)
	const uid2 = "phone-created-exclude-none"
	w = f.put(t, uid2, simpleVCard4(uid2, "Exclude None"), "")
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())
	assert.Empty(t, f.memberCircleIDs(t, uid2))
}

func TestSyncScope_PutNewAllAddsNoCircle(t *testing.T) {
	f := newScopeReadFixture(t)
	const uid = "phone-created-all"
	w := f.put(t, uid, simpleVCard4(uid, "All Mode"), "")
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())
	assert.Empty(t, f.memberCircleIDs(t, uid))
}

func TestSyncScope_DeleteOutOfScopeNotFound(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	w := f.deleteViaHandler(t, f.c3.VCardUID)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var stored models.Contact
	require.NoError(t, f.backend.db.Unscoped().First(&stored, f.c3.ID).Error)
	assert.False(t, stored.DeletedAt.Valid, "an out-of-scope DELETE must not soft-delete")
	var snapshots int64
	require.NoError(t, f.backend.db.Model(&models.ContactDeletionSnapshot{}).Where("user_id = ? AND contact_id = ?", f.user.ID, f.c3.ID).Count(&snapshots).Error)
	assert.Zero(t, snapshots)
}

func TestSyncScope_DeleteInScopeCreatesTrashSnapshot(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	w := f.deleteViaHandler(t, f.c1.VCardUID)
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())

	var stored models.Contact
	require.NoError(t, f.backend.db.Unscoped().First(&stored, f.c1.ID).Error)
	assert.True(t, stored.DeletedAt.Valid)
	var snapshots int64
	require.NoError(t, f.backend.db.Model(&models.ContactDeletionSnapshot{}).Where("user_id = ? AND contact_id = ?", f.user.ID, f.c1.ID).Count(&snapshots).Error)
	assert.EqualValues(t, 1, snapshots)
}

func TestSyncScope_PutNewCircleFailureRollsBackContact(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})
	dbtest.HideTable(t, f.backend.db, "circle_members")

	const uid = "phone-created-atomic"
	w := f.put(t, uid, simpleVCard4(uid, "Atomic"), "")
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	var count int64
	require.NoError(t, f.backend.db.Model(&models.Contact{}).Where("user_id = ? AND vcard_uid = ?", f.user.ID, uid).Count(&count).Error)
	assert.Zero(t, count, "the contact insert must roll back with the circle membership")
}

func TestSyncScope_PutReusingTrashedUID(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	trashed := models.Contact{UserID: f.user.ID, Firstname: "Trashed"}
	require.NoError(t, f.backend.db.Create(&trashed).Error)
	require.NoError(t, f.backend.db.Delete(&trashed).Error)

	w := f.put(t, trashed.VCardUID, simpleVCard4(trashed.VCardUID, "Reborn"), "")
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())

	var live models.Contact
	require.NoError(t, f.backend.db.Where("user_id = ? AND vcard_uid = ?", f.user.ID, trashed.VCardUID).First(&live).Error)
	assert.NotEqual(t, trashed.ID, live.ID)
	assert.Equal(t, []string{f.a.ID}, f.memberCircleIDs(t, trashed.VCardUID))
}

var _ = gorm.ErrRecordNotFound

// A vCard UID that merely begins with a number must not be retried as a
// numeric primary key on DELETE: the prefix can resolve to a different,
// in-scope contact and delete it. Only a UID that is entirely decimal is the
// legacy numeric form.
func TestSyncScope_DeleteNumericPrefixUIDDoesNotHitAnotherContact(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	uid := itoa(f.c2.ID) + "-out-of-scope"
	require.NoError(t, f.backend.db.Model(&models.Contact{}).Where("id = ?", f.c3.ID).Update("vcard_uid", uid).Error)
	f.c3.VCardUID = uid

	w := f.deleteViaHandler(t, uid)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	for _, id := range []uint{f.c2.ID, f.c3.ID} {
		var stored models.Contact
		require.NoError(t, f.backend.db.Unscoped().First(&stored, id).Error)
		assert.False(t, stored.DeletedAt.Valid, "contact %d must survive", id)
	}
}

// The legacy numeric-id fallback still deletes a bare decimal UID.
func TestSyncScope_DeleteNumericUIDStillWorks(t *testing.T) {
	f := newScopeReadFixture(t)
	f.setScope(t, models.CardDAVSyncScopeModeInclude, []string{f.a.ID})

	w := f.deleteViaHandler(t, itoa(f.c1.ID))
	require.True(t, w.Code >= 200 && w.Code < 300, "%d %s", w.Code, w.Body.String())

	var stored models.Contact
	require.NoError(t, f.backend.db.Unscoped().First(&stored, f.c1.ID).Error)
	assert.True(t, stored.DeletedAt.Valid)
}
