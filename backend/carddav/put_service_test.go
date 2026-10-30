package carddav

// Issue #2255: CardDAV PUT create/update must run the same post-write hooks as
// the REST contact write path (contact.created / contact.updated webhooks and
// the wedding-anniversary -> married LifeEvent sync), after the write commits.
// Real migrated schema (backend trap #1).

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type putFixture struct {
	b        *Backend
	f        davDeleteFixture
	received atomic.Int64
}

// newPutFixture seeds alice (+bob), a webhook subscribed to both contact
// events, and returns a backend wired to a live receiver.
func newPutFixture(t *testing.T) *putFixture {
	t.Helper()
	f := newDavDeleteFixture(t)
	pf := &putFixture{b: f.backend, f: f}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pf.received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, f.db.Create(&models.Webhook{UserID: f.user.ID, Name: "h", URL: srv.URL,
		Events: []string{"contact.created", "contact.updated"}, IsActive: true}).Error)
	return pf
}

func weddingCard(uid, anniversary string) vcard.Card {
	card := clientCard(uid)
	if anniversary != "" {
		card.SetValue("ANNIVERSARY", anniversary)
	}
	return card
}

func (p *putFixture) marriedEvents(t *testing.T, uid string) []models.LifeEvent {
	t.Helper()
	var evs []models.LifeEvent
	require.NoError(t, p.f.db.Where("user_id = ? AND entity_id = ? AND type = ?",
		p.f.user.ID, uid, models.LifeEventTypeMarried).Find(&evs).Error)
	return evs
}

func TestPutAddressObject_CreateFiresContactCreatedWebhook(t *testing.T) {
	p := newPutFixture(t)
	uid := "new-card-uid"
	path := "/carddav/addressbooks/alice/contacts/" + uid + ".vcf"

	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), path, clientCard(uid), nil)
	require.NoError(t, err)
	services.WaitForWebhookGoroutines()

	assert.EqualValues(t, 1, p.received.Load(), "exactly one webhook for the create")
	var created models.Contact
	require.NoError(t, p.f.db.Where("user_id = ? AND vcard_uid = ?", p.f.user.ID, uid).First(&created).Error)
}

func TestPutAddressObject_UpdateFiresContactUpdatedWebhook(t *testing.T) {
	p := newPutFixture(t)

	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), p.f.path, clientCard(p.f.contact.VCardUID), nil)
	require.NoError(t, err)
	services.WaitForWebhookGoroutines()

	assert.EqualValues(t, 1, p.received.Load(), "exactly one webhook for the update")
}

func TestPutAddressObject_FailedWriteFiresNoWebhook(t *testing.T) {
	p := newPutFixture(t)
	require.NoError(t, p.f.db.Exec(`CREATE TRIGGER fail_contact_write BEFORE UPDATE ON contacts
		BEGIN SELECT RAISE(ABORT, 'forced write failure'); END`).Error)

	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), p.f.path, clientCard(p.f.contact.VCardUID), nil)
	require.Error(t, err)
	services.WaitForWebhookGoroutines()

	assert.Zero(t, p.received.Load(), "a failed write must fire no webhook")
}

func TestPutAddressObject_IfMatchConflictFiresNothing(t *testing.T) {
	p := newPutFixture(t)

	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), p.f.path, clientCard(p.f.contact.VCardUID), &carddav.PutAddressObjectOptions{
		IfMatch: webdav.ConditionalMatch(`"stale"`),
	})
	require.ErrorContains(t, err, "ETag mismatch", "the 412 precondition failure is unchanged")
	services.WaitForWebhookGoroutines()
	assert.Zero(t, p.received.Load())
}

func TestPutAddressObject_AnniversaryCreatesMarriedLifeEvent(t *testing.T) {
	p := newPutFixture(t)
	uid := p.f.contact.VCardUID

	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), p.f.path, weddingCard(uid, "20090816"), nil)
	require.NoError(t, err)

	evs := p.marriedEvents(t, uid)
	require.Len(t, evs, 1, "a wedding date PUT from a phone mirrors into the married life event")
	require.NotNil(t, evs[0].Date)
	assert.Equal(t, 2009, *evs[0].Date.Year)
	assert.Equal(t, 8, *evs[0].Date.Month)
	assert.Equal(t, 16, *evs[0].Date.Day)
}

func TestPutAddressObject_CreateWithAnniversaryCreatesMarriedLifeEvent(t *testing.T) {
	p := newPutFixture(t)
	uid := "fresh-wedding-uid"
	path := "/carddav/addressbooks/alice/contacts/" + uid + ".vcf"

	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), path, weddingCard(uid, "20090816"), nil)
	require.NoError(t, err)

	require.Len(t, p.marriedEvents(t, uid), 1)
}

func TestPutAddressObject_RemovedAnniversaryClearsMarriedLifeEvent(t *testing.T) {
	p := newPutFixture(t)
	uid := p.f.contact.VCardUID
	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), p.f.path, weddingCard(uid, "20090816"), nil)
	require.NoError(t, err)
	require.Len(t, p.marriedEvents(t, uid), 1)

	_, err = p.b.PutAddressObject(p.f.ctx(p.f.user), p.f.path, weddingCard(uid, ""), nil)
	require.NoError(t, err)

	assert.Empty(t, p.marriedEvents(t, uid), "dropping the date removes the date-only married event, as REST does")
}

func TestPutAddressObject_OtherUsersUIDDoesNotTouchOwnersContact(t *testing.T) {
	p := newPutFixture(t)
	uid := p.f.contact.VCardUID

	// bob PUTs alice's UID: scoped by user_id, so it is a brand new contact for
	// bob and alice's row is untouched.
	_, err := p.b.PutAddressObject(p.f.ctx(p.f.other), "/carddav/addressbooks/bob/contacts/"+uid+".vcf",
		weddingCard(uid, "20090816"), nil)
	require.NoError(t, err)

	var mine models.Contact
	require.NoError(t, p.f.db.Where("user_id = ?", p.f.user.ID).First(&mine, p.f.contact.ID).Error)
	assert.Equal(t, "Del", mine.Firstname, "alice's contact is untouched")
	assert.Empty(t, p.marriedEvents(t, uid), "no life event for alice")
	services.WaitForWebhookGoroutines()
	assert.Zero(t, p.received.Load(), "alice's webhook does not see bob's write")
}

func TestPutAddressObject_PreservesSensitiveAddressAndStillFiresHooks(t *testing.T) {
	p := newPutFixture(t)
	c := p.f.contact
	c.Addresses = []models.ContactAddress{{ID: "a-secret", Street: "2 Secret St", Sensitivity: models.RelationshipSensitivitySecret}}
	require.NoError(t, p.f.db.Save(&c).Error)

	_, err := p.b.PutAddressObject(p.f.ctx(p.f.user), p.f.path, clientCard(c.VCardUID, "1 Normal St"), nil)
	require.NoError(t, err)
	services.WaitForWebhookGoroutines()

	var stored models.Contact
	require.NoError(t, p.f.db.First(&stored, c.ID).Error)
	_, ok := findStoredAddress(stored.Addresses, "2 Secret St")
	assert.True(t, ok, "the secret address survives a DAV PUT that never saw it")
	assert.EqualValues(t, 1, p.received.Load())
}
