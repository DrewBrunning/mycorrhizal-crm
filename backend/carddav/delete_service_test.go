package carddav

// Issue #2249: CardDAV DELETE must go through services.DeleteContact so the
// registry cascade, the ADR 0037 trash snapshot, the audit trail and the
// contact.deleted webhook match a REST delete. Real migrated schema (trap #1).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type davDeleteFixture struct {
	backend *Backend
	db      *gorm.DB
	user    models.User
	other   models.User
	contact models.Contact
	path    string
}

func newDavDeleteFixture(t *testing.T) davDeleteFixture {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "alice", Password: "password123!A", Email: "alice@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "bob", Password: "password123!A", Email: "bob@example.com"}
	require.NoError(t, db.Create(&other).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Del", Lastname: "Me"}
	require.NoError(t, db.Create(&contact).Error)
	return davDeleteFixture{
		backend: NewBackend(db, t.TempDir()), db: db, user: user, other: other, contact: contact,
		path: "/carddav/addressbooks/alice/contacts/" + contact.VCardUID + ".vcf",
	}
}

func (f davDeleteFixture) ctx(u models.User) context.Context {
	return ContextWithUser(context.Background(), u.ID, u.Username, f.db, f.backend.photoDir, "")
}

// seedDependants adds a soft-cascaded note and a hard-cascaded edge.
func (f davDeleteFixture) seedDependants(t *testing.T) uint {
	t.Helper()
	note := models.Note{UserID: f.user.ID, ContactID: &f.contact.ID, Content: "n", Date: time.Now()}
	require.NoError(t, f.db.Create(&note).Error)
	other := models.Contact{UserID: f.user.ID, Firstname: "Keep"}
	require.NoError(t, f.db.Create(&other).Error)
	require.NoError(t, f.db.Create(&models.RelationshipEdge{UserID: f.user.ID, SourceID: f.contact.VCardUID, TargetID: other.VCardUID, Type: "related_to"}).Error)
	return note.ID
}

// httpDelete drives a real DAV DELETE through the handler as user u and
// returns the response status (go-webdav maps the backend result to it).
func (f davDeleteFixture) httpDelete(t *testing.T, u models.User, path string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &Handler{backend: f.backend, db: f.db, photoDir: f.backend.photoDir}
	h.handler = NewHandler(f.db, f.backend.photoDir).handler
	h.handler.Backend = f.backend
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, path, nil)
	c.Set("userID", u.ID)
	c.Set("username", u.Username)
	h.GinHandler()(c)
	return w.Code
}

func TestDeleteAddressObject_WritesTrashSnapshotAndRestores(t *testing.T) {
	f := newDavDeleteFixture(t)
	noteID := f.seedDependants(t)

	require.NoError(t, f.backend.DeleteAddressObject(f.ctx(f.user), f.path))

	entries, total, err := services.ListTrash(f.db, f.user.ID, 0, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "a CardDAV delete must be listed in the trash")
	require.Len(t, entries, 1)
	assert.Equal(t, f.contact.ID, entries[0].Contact.ID)

	_, err = services.RestoreContact(f.db, f.user.ID, f.contact.ID, time.Now())
	require.NoError(t, err)

	var restored models.Contact
	require.NoError(t, f.db.First(&restored, f.contact.ID).Error, "contact is live again")
	var note models.Note
	require.NoError(t, f.db.First(&note, noteID).Error, "restore brings the note back")
	var edges int64
	require.NoError(t, f.db.Model(&models.RelationshipEdge{}).Where("source_id = ?", f.contact.VCardUID).Count(&edges).Error)
	assert.EqualValues(t, 1, edges, "restore brings the hard-deleted edge back")
}

func TestDeleteAddressObject_RunsCascade(t *testing.T) {
	f := newDavDeleteFixture(t)
	noteID := f.seedDependants(t)

	require.NoError(t, f.backend.DeleteAddressObject(f.ctx(f.user), f.path))

	var live int64
	require.NoError(t, f.db.Model(&models.Note{}).Where("id = ?", noteID).Count(&live).Error)
	assert.Zero(t, live, "note is soft-deleted with the contact")
	var tomb int64
	require.NoError(t, f.db.Unscoped().Model(&models.Note{}).Where("id = ? AND deleted_at IS NOT NULL", noteID).Count(&tomb).Error)
	assert.EqualValues(t, 1, tomb)
	var edges int64
	require.NoError(t, f.db.Model(&models.RelationshipEdge{}).Where("source_id = ?", f.contact.VCardUID).Count(&edges).Error)
	assert.Zero(t, edges, "edge removed by the cascade")
}

func TestDeleteAddressObject_AuditMatchesRESTDelete(t *testing.T) {
	f := newDavDeleteFixture(t)
	f.seedDependants(t)

	require.NoError(t, f.backend.DeleteAddressObject(f.ctx(f.user), f.path))

	var n int64
	require.NoError(t, f.db.Model(&models.AuditEvent{}).Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, f.contact.VCardUID, models.AuditOpDelete).Count(&n).Error)
	assert.EqualValues(t, 1, n, "contact delete audit event")
	require.NoError(t, f.db.Model(&models.AuditEvent{}).Where("entity_type = ? AND operation = ?",
		models.AuditEntityNote, models.AuditOpDelete).Count(&n).Error)
	assert.EqualValues(t, 1, n, "cascaded note delete audit event")
}

func TestDeleteAddressObject_FiresContactDeletedWebhook(t *testing.T) {
	f := newDavDeleteFixture(t)
	var received atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	require.NoError(t, f.db.Create(&models.Webhook{UserID: f.user.ID, Name: "h", URL: srv.URL,
		Events: []string{"contact.deleted"}, IsActive: true}).Error)

	require.NoError(t, f.backend.DeleteAddressObject(f.ctx(f.user), f.path))
	services.WaitForWebhookGoroutines()

	assert.EqualValues(t, 1, received.Load(), "contact.deleted webhook delivered")
}

func TestDeleteAddressObject_OtherUsersPathIs404AndUntouched(t *testing.T) {
	f := newDavDeleteFixture(t)
	noteID := f.seedDependants(t)

	assert.Equal(t, http.StatusNotFound, f.httpDelete(t, f.other, f.path))

	var c models.Contact
	require.NoError(t, f.db.First(&c, f.contact.ID).Error, "contact untouched")
	var note models.Note
	require.NoError(t, f.db.First(&note, noteID).Error, "dependants untouched")
	var snaps int64
	require.NoError(t, f.db.Model(&models.ContactDeletionSnapshot{}).Count(&snaps).Error)
	assert.Zero(t, snaps)
}

func TestDeleteAddressObject_AlreadyDeletedIs404(t *testing.T) {
	f := newDavDeleteFixture(t)
	require.Equal(t, http.StatusOK, f.httpDelete(t, f.user, f.path), "go-webdav answers a successful DELETE 200")

	assert.Equal(t, http.StatusNotFound, f.httpDelete(t, f.user, f.path))

	var snaps int64
	require.NoError(t, f.db.Model(&models.ContactDeletionSnapshot{}).Count(&snaps).Error)
	assert.EqualValues(t, 1, snaps, "second delete adds no snapshot")
}
