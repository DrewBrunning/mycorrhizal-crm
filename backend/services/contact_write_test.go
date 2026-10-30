package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SaveContact is the shared REST/CardDAV contact write (issue #2255).

func TestSaveContact_CreateAndUpdateFireTheirWebhooksAfterCommit(t *testing.T) {
	db, user := selfApplyFixture(t)
	var created, updated atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-Mycorrhizal-Event") {
		case "contact.created":
			created.Add(1)
		case "contact.updated":
			updated.Add(1)
		}
	}))
	defer srv.Close()
	require.NoError(t, db.Create(&models.Webhook{UserID: user.ID, Name: "h", URL: srv.URL,
		Events: []string{"contact.created", "contact.updated"}, IsActive: true}).Error)

	c := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, SaveContact(context.Background(), db, config.Config{}, user.ID, &c, false))
	require.NotZero(t, c.ID, "created")
	c.Firstname = "Ada2"
	require.NoError(t, SaveContact(context.Background(), db, config.Config{}, user.ID, &c, false))
	WaitForWebhookGoroutines()

	assert.EqualValues(t, 1, created.Load())
	assert.EqualValues(t, 1, updated.Load())
}

func TestSaveContact_ErrorIsReturnedAndFiresNothing(t *testing.T) {
	db, user := selfApplyFixture(t)
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	require.NoError(t, db.Create(&models.Webhook{UserID: user.ID, Name: "h", URL: srv.URL,
		Events: []string{"contact.created"}, IsActive: true}).Error)
	dbtest.HideTable(t, db, "contacts")

	c := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.Error(t, SaveContact(context.Background(), db, config.Config{}, user.ID, &c, false))
	WaitForWebhookGoroutines()
	assert.Zero(t, hits.Load())
}

func TestSaveContact_AmbientChangeOnMeClearsProvenance(t *testing.T) {
	db, user := selfApplyFixture(t)
	me := selfApplyContact(t, db, user.ID, "Me", true)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{"self_contact_vcard_uid": me.VCardUID, "self_contact_ambient_set": true}).Error)

	me.Ambient = false
	require.NoError(t, SaveContact(context.Background(), db, config.Config{}, user.ID, &me, true))

	var got models.User
	require.NoError(t, db.First(&got, user.ID).Error)
	assert.False(t, got.SelfContactAmbientSet, "unmarking Me takes ownership of the flag")
}

func TestSaveContact_WeddingSyncFailureIsLoggedNotReturned(t *testing.T) {
	db, user := selfApplyFixture(t)
	rec := logtest.Capture(t)
	rec2 := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Anniversaries: []contactmodel.Anniversary{{Kind: "wedding", Date: contactmodel.AnniversaryDate{
			Partial: &contactmodel.PartialDate{Year: intp(2009), Month: intp(8), Day: intp(16)}}}},
	}}
	c := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&c, rec2, "")
	dbtest.HideTable(t, db, "life_events")

	require.NoError(t, SaveContact(context.Background(), db, config.Config{}, user.ID, &c, false),
		"the contact write committed; the mirror failure must not fail it")
	assert.Contains(t, rec.String(), "Error syncing wedding anniversary")
}

func intp(n int) *int { return &n }
