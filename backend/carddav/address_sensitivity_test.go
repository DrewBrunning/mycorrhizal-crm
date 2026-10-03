package carddav

// Issue #1433 — address sensitivity on the CardDAV server surface.
//
// Two halves that must hold together:
//   - GET withholds a private/secret address (and its geo: coordinate), like
//     every other outward copy, via models.RecordForContactForSync.
//   - PUT preserves an above-normal address the server already holds, because
//     a WebDAV PUT replaces the whole resource and the client was never shown
//     the address, so a routine edit would otherwise silently delete it
//     (models.PreserveSensitiveAddresses).
//
// The second is the regression a naive "just filter GET" would introduce. The
// GET test is hand-verified to fail with RecordForContactForSync reverted to
// RecordForContact; the PUT tests to fail with PreserveSensitiveAddresses
// stubbed to return incoming unchanged.
//
// The PUT tests build the client card directly rather than echoing the GET
// card back. A separate, pre-existing CardDAV defect mangles a geo: URI when
// the adapter's byte output is decoded into a vcard.Card and re-encoded by
// go-webdav (the comma-bearer geo: value is not re-quoted); that is unrelated
// to sensitivity and is tracked separately, so these tests isolate the
// sensitivity logic from it.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSensitiveAddressContact seeds a contact (owned by the context user 1)
// with a normal address and an above-normal one, both carrying a geo:
// coordinate.
func newSensitiveAddressContact(t *testing.T, uid, sensitivity string) (*Backend, models.Contact) {
	t.Helper()
	backend, db := newTestBackend(t)

	record := &contactmodel.Record{Card: contactmodel.Card{
		UID:  uid,
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Addresses: []contactmodel.Address{
			{
				ID:          "addr-normal",
				Components:  []contactmodel.AddressComponent{{Kind: "name", Value: "1 Normal St"}},
				Full:        "1 Normal St",
				Coordinates: "geo:1,1",
			},
			{
				ID:          "addr-secret",
				Components:  []contactmodel.AddressComponent{{Kind: "name", Value: "2 Secret St"}},
				Full:        "2 Secret St",
				Coordinates: "geo:2,2",
				Sensitivity: sensitivity,
			},
		},
	}}
	contact := models.Contact{UserID: 1}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, db.Create(&contact).Error)
	return backend, contact
}

// clientCard builds a vCard as a client would PUT it: the addresses it knows
// about, as ADR values (PO;Extended;Street;Locality;Region;PostalCode;Country),
// with no PROP-ID so the content-match path is exercised.
func clientCard(uid string, streets ...string) vcard.Card {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	card.SetValue(vcard.FieldUID, uid)
	card.SetValue(vcard.FieldFormattedName, "Ada Edited")
	card.SetValue(vcard.FieldName, "St;Ada;;;")
	for _, s := range streets {
		card["ADR"] = append(card["ADR"], &vcard.Field{
			Value:  ";;" + s + ";;;;",
			Params: vcard.Params{"LABEL": []string{s}},
		})
	}
	return card
}

func encodeCard(t *testing.T, card vcard.Card) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, vcard.NewEncoder(&buf).Encode(card))
	return buf.String()
}

func TestCardDAV_GetAddressObject_ExcludesSensitiveAddress(t *testing.T) {
	backend, contact := newSensitiveAddressContact(t, "addr-secret-get", models.RelationshipSensitivitySecret)
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")

	obj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+contact.VCardUID+".vcf", nil)
	require.NoError(t, err)

	adrs := obj.Card["ADR"]
	require.Len(t, adrs, 1, "only the normal address may be served; the secret one is withheld")

	// Control: the normal address is served, GEO and all, so the above is the
	// sensitivity filter and not a blank card.
	normal := adrs[0]
	assert.Equal(t, "1 Normal St", normal.Params.Get("LABEL"))
	assert.NotEmpty(t, normal.Params["GEO"], "the normal address's geo: coordinate must be served")
	assert.Contains(t, obj.Card.Value(vcard.FieldFormattedName), "Ada", "identity data must still be served")

	// And the address that is gone really is the secret one.
	body := encodeCard(t, obj.Card)
	assert.NotContains(t, body, "2 Secret St")
	assert.NotContains(t, body, "geo:2,2")
}

// TestCardDAV_PutPreservesOmittedSensitiveAddress is the leak's inverse and the
// regression the fix must not introduce: the client PUTs a card that omits the
// secret address it was never shown. The address must survive.
func TestCardDAV_PutPreservesOmittedSensitiveAddress(t *testing.T) {
	backend, contact := newSensitiveAddressContact(t, "addr-secret-put", models.RelationshipSensitivitySecret)
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	// The client knows only the normal address (it was never served the secret
	// one) and edits an unrelated field.
	_, err := backend.PutAddressObject(ctx, urlPath, clientCard(contact.VCardUID, "1 Normal St"), nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)

	secret, ok := findStoredAddress(stored.Addresses, "2 Secret St")
	require.True(t, ok, "the secret address must survive the client's full-overwrite PUT")
	assert.Equal(t, models.RelationshipSensitivitySecret, secret.Sensitivity, "the classification must survive too")
	assert.Equal(t, "geo:2,2", secret.Coordinates, "and so must the geo: coordinate")
	assert.Equal(t, 1, countStoredAddress(stored.Addresses, "2 Secret St"), "no duplicate may be created")

	normal, ok := findStoredAddress(stored.Addresses, "1 Normal St")
	require.True(t, ok, "the normal address survives unchanged")
	assert.Empty(t, normal.Sensitivity)

	assert.Equal(t, "Ada Edited", stored.FN, "the client's ordinary edit is still applied")
}

// TestCardDAV_PutContentMatchedSensitiveAddressStaysSecret covers the vCard 3.0
// case: no PROP-ID, and a stale client copy matched only by its normalized
// address line. The server's classification/coordinates win and no duplicate
// is created.
func TestCardDAV_PutContentMatchedSensitiveAddressStaysSecret(t *testing.T) {
	backend, contact := newSensitiveAddressContact(t, "addr-secret-content", models.RelationshipSensitivityPrivate)
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	// The stale copy differs only in casing and spacing.
	_, err := backend.PutAddressObject(ctx, urlPath, clientCard(contact.VCardUID, "1 Normal St", "2 secret  st"), nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)

	assert.Equal(t, 1, countStoredAddress(stored.Addresses, "2 Secret St"),
		"the content match must not duplicate the address")
	secret, ok := findStoredAddress(stored.Addresses, "2 Secret St")
	require.True(t, ok)
	assert.Equal(t, models.RelationshipSensitivityPrivate, secret.Sensitivity,
		"the server's classification must survive a casing/spacing-only echo")
	assert.Equal(t, "geo:2,2", secret.Coordinates, "the server's coordinates must survive")
}

func findStoredAddress(addrs []models.ContactAddress, street string) (models.ContactAddress, bool) {
	for _, a := range addrs {
		if strings.EqualFold(a.Street, street) {
			return a, true
		}
	}
	return models.ContactAddress{}, false
}

func countStoredAddress(addrs []models.ContactAddress, street string) int {
	n := 0
	for _, a := range addrs {
		if strings.EqualFold(a.Street, street) {
			n++
		}
	}
	return n
}
