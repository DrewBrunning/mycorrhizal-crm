package carddav

// Issue #2261: CardDAV PUT of a new contact must enforce the same opt-in
// per-user contact quota as the REST create. The check lives in the shared
// services.SaveContact path, so a DAV create cannot drift from REST again.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/emersion/go-vcard"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type davQuotaFixture struct {
	db       *gorm.DB
	backend  *Backend
	user     models.User
	contact  models.Contact
	path     string
	received atomic.Int64
}

func newDAVQuotaFixture(t *testing.T, cfg config.Config) *davQuotaFixture {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "alice", Password: "password123!A", Email: "alice@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Existing"}
	require.NoError(t, db.Create(&contact).Error)

	backend := NewBackend(db, t.TempDir())
	backend.cfg = cfg

	f := &davQuotaFixture{
		db: db, backend: backend, user: user, contact: contact,
		path: "/carddav/addressbooks/alice/contacts/" + contact.VCardUID + ".vcf",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, db.Create(&models.Webhook{UserID: user.ID, Name: "h", URL: srv.URL,
		Events: []string{"contact.created", "contact.updated"}, IsActive: true}).Error)
	return f
}

// httpPut drives a real CardDAV PUT through the handler, so the assertion is
// the HTTP status a DAV client actually sees (go-webdav maps the backend
// error to it).
func (f *davQuotaFixture) httpPut(t *testing.T, path string, card vcard.Card) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewHandler(f.db, f.backend.photoDir)
	h.SetConfig(f.backend.cfg)

	var body bytes.Buffer
	require.NoError(t, vcard.NewEncoder(&body).Encode(card))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, path, &body)
	c.Request.Header.Set("Content-Type", vcard.MIMEType)
	c.Set("userID", f.user.ID)
	c.Set("username", f.user.Username)
	h.GinHandler()(c)
	c.Writer.WriteHeaderNow()
	return w
}

func TestPutAddressObject_QuotaRefusesCreateAtLimit(t *testing.T) {
	f := newDAVQuotaFixture(t, config.Config{PerUserContactLimit: 1})
	uid := "quota-refused"
	path := "/carddav/addressbooks/alice/contacts/" + uid + ".vcf"

	w := f.httpPut(t, path, clientCard(uid))

	require.Equal(t, http.StatusInsufficientStorage, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "PER_USER_CONTACT_LIMIT",
		"the refusal names the config variable an operator would raise")
	assert.Contains(t, w.Body.String(), "quota",
		"the refusal explains it is a quota, not an IO failure")

	var count int64
	require.NoError(t, f.db.Model(&models.Contact{}).Where("user_id = ?", f.user.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "the refused create must not add a contact")

	services.WaitForWebhookGoroutines()
	assert.Zero(t, f.received.Load(), "a refused create fires no contact.created webhook")
}

func TestPutAddressObject_QuotaAllowsUpdateAtLimit(t *testing.T) {
	f := newDAVQuotaFixture(t, config.Config{PerUserContactLimit: 1})
	card := clientCard(f.contact.VCardUID)
	card.SetValue(vcard.FieldFormattedName, "Updated Person")
	card.SetValue(vcard.FieldName, "Person;Updated;;;")

	w := f.httpPut(t, f.path, card)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var got models.Contact
	require.NoError(t, f.db.First(&got, f.contact.ID).Error)
	assert.Equal(t, "Updated", got.Firstname, "the update at the limit still writes")

	services.WaitForWebhookGoroutines()
	assert.EqualValues(t, 1, f.received.Load(), "the update still fires contact.updated")
}

func TestPutAddressObject_QuotaDisabledAllowsCreate(t *testing.T) {
	f := newDAVQuotaFixture(t, config.Config{})
	uid := "quota-unlimited"
	path := "/carddav/addressbooks/alice/contacts/" + uid + ".vcf"

	w := f.httpPut(t, path, clientCard(uid))

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created models.Contact
	require.NoError(t, f.db.Where("user_id = ? AND vcard_uid = ?", f.user.ID, uid).First(&created).Error)

	services.WaitForWebhookGoroutines()
	assert.EqualValues(t, 1, f.received.Load(), "an unlimited create behaves exactly as before")
}
