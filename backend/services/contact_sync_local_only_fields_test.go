package services

// Issue #2055: the contact-subscription reconcile is a full remote overwrite,
// but vCards cannot carry the CRM-only fields (circles, how-we-met,
// work/contact information, CRM.Kind, CRM.Periods). A remote update must
// preserve them while still applying the remote's own fields.

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcileContactSync_PreservesLocalOnlyCRMFields(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	href := "/addressbooks/test/local-only-crm.vcf"
	first := carddav.AddressObject{
		Path: href,
		ETag: "\"etag-1\"",
		Card: testCard(t, "local-only-crm-uid", "Ada", "Lovelace", "ada@example.com"),
	}
	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{first}, nil, false, "")
	require.NoError(t, err)
	require.Equal(t, 1, stats.Created)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "local-only-crm-uid").First(&contact).Error)

	year := 2019
	record := models.RecordForContact(&contact, "", db)
	record.Card.Addresses = append(record.Card.Addresses, contactmodel.Address{
		ID:         "addr-1",
		Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}},
		Full:       "1 Main St",
	})
	record.Envelope.Kind = "animal"
	record.Envelope.Circles = []string{"Friends"}
	record.Envelope.HowWeMet = "At a conference"
	record.Envelope.WorkInformation = "Analytical Engines Ltd"
	record.Envelope.ContactInformation = "Prefers email"
	record.Envelope.Periods = []contactmodel.EntryPeriod{{
		Kind:    contactmodel.PeriodKindAddress,
		EntryID: "addr-1",
		Range:   contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: &year}},
	}}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, db.Save(&contact).Error)

	// A remote change to an ordinary field, carrying none of the CRM-only data.
	second := carddav.AddressObject{
		Path: href,
		ETag: "\"etag-2\"",
		Card: testCard(t, "local-only-crm-uid", "Ada", "Lovelace", "ada.new@example.com"),
	}
	stats, err = reconcileContactSync(db, sub, []carddav.AddressObject{second}, nil, false, "")
	require.NoError(t, err)
	require.Equal(t, 1, stats.Updated)

	var stored models.Contact
	require.NoError(t, db.First(&stored, contact.ID).Error)
	assert.Equal(t, "ada.new@example.com", stored.Email, "control: an ordinary remote change is applied")
	assert.Equal(t, []string{"Friends"}, stored.Circles)
	assert.Equal(t, "At a conference", stored.HowWeMet)
	assert.Equal(t, "Analytical Engines Ltd", stored.WorkInformation)
	assert.Equal(t, "Prefers email", stored.ContactInformation)
	assert.Equal(t, "animal", stored.CRM.Kind)
	require.Len(t, stored.CRM.Periods, 1)
	assert.Equal(t, "addr-1", stored.CRM.Periods[0].EntryID)
	require.NotNil(t, stored.CRM.Periods[0].Range.Start)
	assert.Equal(t, year, *stored.CRM.Periods[0].Range.Start.Year)
}
