package carddav

// Issue #2055: a vCard has no home for the CRM-only fields (circles,
// how-we-met, work/contact information, CRM.Kind, CRM.Periods), so a CardDAV
// PUT — a full resource overwrite by design — must re-attach the stored values
// when the incoming card does not carry them. Ordinary remote fields remain
// client-controlled.

import (
	"context"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCardDAV_PutPreservesLocalOnlyCRMFields(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	backend, db := newTestBackend(t)
	seedCardDAVUser(t, db)

	year := 2019
	const uid = "local-only-crm-put"
	record := &contactmodel.Record{
		Card: contactmodel.Card{
			UID:  uid,
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			Addresses: []contactmodel.Address{{
				ID:         "addr-1",
				Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}},
				Full:       "1 Main St",
			}},
		},
		Envelope: contactmodel.CRMEnvelope{
			Kind:               "animal",
			Circles:            []string{"Friends", "Work"},
			HowWeMet:           "At a conference",
			WorkInformation:    "Analytical Engines Ltd",
			ContactInformation: "Prefers email",
			Periods: []contactmodel.EntryPeriod{{
				Kind:    contactmodel.PeriodKindAddress,
				EntryID: "addr-1",
				Range:   contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: &year}},
			}},
		},
	}
	contact := models.Contact{UserID: 1}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, db.Create(&contact).Error)

	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	// The client edits an ordinary field and sends no CRM-only data (a vCard
	// cannot express it).
	_, err := backend.PutAddressObject(ctx, urlPath, clientCard(contact.VCardUID, "1 Main St"), nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)
	assert.Equal(t, "Ada Edited", stored.FN, "control: an ordinary remote edit is applied")
	assert.Equal(t, []string{"Friends", "Work"}, stored.Circles)
	assert.Equal(t, "At a conference", stored.HowWeMet)
	assert.Equal(t, "Analytical Engines Ltd", stored.WorkInformation)
	assert.Equal(t, "Prefers email", stored.ContactInformation)
	assert.Equal(t, "animal", stored.CRM.Kind)
	require.Len(t, stored.CRM.Periods, 1)
	assert.Equal(t, "addr-1", stored.CRM.Periods[0].EntryID)
	require.NotNil(t, stored.CRM.Periods[0].Range.Start)
	assert.Equal(t, year, *stored.CRM.Periods[0].Range.Start.Year)
}
