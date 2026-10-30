package carddav

// Per-contact field visibility (issue #1997) is cosmetic display state kept in
// contact_field_overrides, outside the neutral Card. This pins, on the CardDAV
// surface and on the neutral Record projection every exporter shares, that
//
//   - no override data reaches a served vCard or the neutral Card, and
//   - an override of "show" on every field cannot widen what leaves the
//     instance: sensitivity is still filtered in the projection query.

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCardDAV_FieldOverridesNeverReachCardsAndNeverWidenSensitivity(t *testing.T) {
	db := dbtest.New(t)
	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	backend := NewBackend(db, t.TempDir())
	ctx := ContextWithUser(context.Background(), ds.User.ID, "tester", db, "", "")
	adaV, hugoV, idaV, bobV := ds.Contacts["ada"], ds.Contacts["hugo"], ds.Contacts["ida"], ds.Contacts["bob"]
	ada, hugo, ida, bob := &adaV, &hugoV, &idaV, &bobV

	serve := func(c *models.Contact) (vcardText, etag, record string) {
		obj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+c.VCardUID+".vcf", nil)
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, vcard.NewEncoder(&buf).Encode(obj.Card))
		var fresh models.Contact
		require.NoError(t, db.First(&fresh, c.ID).Error)
		raw, err := json.Marshal(models.RecordForContact(&fresh, t.TempDir(), db))
		require.NoError(t, err)
		return buf.String(), obj.ETag, string(raw)
	}

	type served struct{ card, etag, record string }
	baseline := map[uint]served{}
	for _, c := range []*models.Contact{ada, hugo, ida, bob} {
		card, etag, rec := serve(c)
		baseline[c.ID] = served{card, etag, rec}
	}

	// Every field forced visible on every contact, plus hide-empty: the
	// widest override a user can express.
	for _, c := range []*models.Contact{ada, hugo, ida, bob} {
		for _, k := range models.ContactFieldKeys {
			require.NoError(t, db.Create(&models.ContactFieldOverride{UserID: ds.User.ID, ContactVCardUID: c.VCardUID, FieldKey: k, State: models.FieldStateShow}).Error)
		}
		require.NoError(t, db.Create(&models.ContactFieldOverride{UserID: ds.User.ID, ContactVCardUID: c.VCardUID, FieldKey: models.HideEmptyOverrideKey, State: models.FieldStateHide}).Error)
	}

	for _, c := range []*models.Contact{ada, hugo, ida, bob} {
		card, etag, rec := serve(c)
		want := baseline[c.ID]
		assert.Equal(t, want.card, card, "the served vCard of %s is byte-identical with overrides present", c.VCardUID)
		assert.Equal(t, want.etag, etag, "the ETag is unchanged: overrides are not card data")
		assert.JSONEq(t, want.record, rec, "the neutral Record of %s is unchanged", c.VCardUID)
		for _, needle := range []string{"__hide_empty", "field_key", "field_override", "hide_empty"} {
			assert.NotContains(t, card, needle)
			assert.NotContains(t, rec, needle)
		}
	}

	// Sensitivity is unaffected by visibility: the secret/private surfaces are
	// still absent even though every field is "shown".
	obj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+ada.VCardUID+".vcf", nil)
	require.NoError(t, err)
	assertNoCardProp(t, obj.Card, "X-PRIVATE-NICK", "a show-all override must not expose ada's secret custom field")
	hugoObj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+hugo.VCardUID+".vcf", nil)
	require.NoError(t, err)
	assertNoRelation(t, hugoObj.Card, "urn:uuid:"+ida.VCardUID, "a show-all override must not expose hugo's private edge")
	bobObj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+bob.VCardUID+".vcf", nil)
	require.NoError(t, err)
	assertNoHobby(t, bobObj.Card, "chess", "a show-all override must not expose bob's private hobby")
}
