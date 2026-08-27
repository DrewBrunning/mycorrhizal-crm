package models

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVCardImportSaveExportRoundTrip_SubStreetParts is T79's
// (T79)
// item-5 round trip, against a real migrated schema (CLAUDE.md backend trap
// #1): a vCard carrying a populated PO box, apartment and floor imports,
// survives a save, and exports back to the same vCard fields.
//
// Before T79 the flat projection dropped postOfficeBox/apartment/floor, so an
// imported address lost those parts on the way into the flat Addresses[]
// (and a plain save would then have destroyed them for good — the T75 bug).
// Now they ride the flat struct, so the whole lifecycle must round-trip.
func TestVCardImportSaveExportRoundTrip_SubStreetParts(t *testing.T) {
	raw := "BEGIN:VCARD\r\n" +
		"VERSION:4.0\r\n" +
		"UID:t79-roundtrip-example\r\n" +
		"FN:Sub Street Person\r\n" +
		"N:Person;Sub Street;;;\r\n" +
		"ADR;TYPE=HOME:PO Box 42;;123 Main Street;Any Town;CA;91921;U.S.A;;Apt 3B;Floor 2;;;;;;;\r\n" +
		"END:VCARD\r\n"

	record, _, err := vcard4.Adapter{}.Import([]byte(raw))
	require.NoError(t, err)
	require.Len(t, record.Card.Addresses, 1)

	kinds := map[string]string{}
	for _, c := range record.Card.Addresses[0].Components {
		kinds[c.Kind] = c.Value
	}
	require.Equal(t, "PO Box 42", kinds["postOfficeBox"])
	require.Equal(t, "Apt 3B", kinds["apartment"])
	require.Equal(t, "Floor 2", kinds["floor"])

	db := dbtest.New(t)
	user := User{Username: "t79-roundtrip", Password: "password123!A", Email: "t79-roundtrip@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, record, "")
	require.NoError(t, db.Create(contact).Error)

	// The flat projection carried the parts across the save.
	var persisted Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	require.Len(t, persisted.Addresses, 1)
	if persisted.Addresses[0].POBox != "PO Box 42" || persisted.Addresses[0].Apartment != "Apt 3B" || persisted.Addresses[0].Floor != "Floor 2" {
		t.Errorf("persisted flat address = %+v, want the imported sub-street parts", persisted.Addresses[0])
	}
	// The searchable flattening carries them too.
	flat := FlattenAddresses(persisted.Addresses)
	assert.Contains(t, flat, "PO Box 42")
	assert.Contains(t, flat, "Apt 3B")
	assert.Contains(t, flat, "Floor 2")

	// A plain save (the T75 merge path) must not destroy them.
	var loaded Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	loaded.Nickname = "Roundtripped"
	require.NoError(t, db.Save(&loaded).Error)

	// Export the real read path (RecordForContact reads what is persisted)
	// and re-import the export: the sub-street parts must be back.
	got := RecordForContact(&loaded, "", nil)
	require.NotNil(t, got)
	out, diags, err := vcard4.Adapter{}.Export(got)
	require.NoError(t, err)
	for _, d := range diags {
		if d.Severity == "warn" {
			t.Logf("export diagnostic: %s", d.Message)
		}
	}
	reimported, _, err := vcard4.Adapter{}.Import(out)
	require.NoError(t, err)
	require.Len(t, reimported.Card.Addresses, 1)
	gotKinds := map[string]string{}
	for _, c := range reimported.Card.Addresses[0].Components {
		gotKinds[c.Kind] = c.Value
	}
	for _, tc := range []struct{ kind, want string }{
		{"postOfficeBox", "PO Box 42"},
		{"apartment", "Apt 3B"},
		{"floor", "Floor 2"},
		{"name", "123 Main Street"},
		{"locality", "Any Town"},
	} {
		if gotKinds[tc.kind] != tc.want {
			t.Errorf("re-imported component %q = %q, want %q (components=%v)", tc.kind, gotKinds[tc.kind], tc.want, gotKinds)
		}
	}
}

// TestVCardImportAddressType is T91
// (T91): the whole
// path, end to end, because the bug was only visible across a package
// boundary. vcard4's importer maps ADR;TYPE=HOME to the neutral Contexts token
// "private" (correct, RFC 9553), and the flat projection then used to copy
// that token across untranslated -- so every VCF-imported home address
// persisted with Type "private", which the web address editor renders as raw
// text because there is no i18n key for it. Android's device import emitted
// the same neutral token deliberately and hit the same projection.
//
// Neither half was wrong on its own, which is why unit tests on either side
// passed throughout. This test spans both.
func TestVCardImportAddressType(t *testing.T) {
	for _, tc := range []struct{ name, typeParam, want string }{
		{"home imports as the legacy home token", "HOME", "home"},
		{"work is unaffected", "WORK", "work"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := "BEGIN:VCARD\r\n" +
				"VERSION:4.0\r\n" +
				"UID:t91-" + tc.typeParam + "\r\n" +
				"FN:Address Type Person\r\n" +
				"N:Person;Address Type;;;\r\n" +
				"ADR;TYPE=" + tc.typeParam + ":;;123 Fake St;Townesville;MO;55555;USA\r\n" +
				"END:VCARD\r\n"

			record, _, err := vcard4.Adapter{}.Import([]byte(raw))
			require.NoError(t, err)
			require.Len(t, record.Card.Addresses, 1)

			contact := &Contact{}
			ApplyRecordToContact(contact, record, "")
			require.Len(t, contact.Addresses, 1)
			assert.Equal(t, tc.want, contact.Addresses[0].Type,
				"a TYPE=%s vCard address must land on the flat model as %q, not as the neutral context token",
				tc.typeParam, tc.want)
		})
	}
}

// TestAddressFromContactAddress_OrderingPinsFormatAddressSymmetry guards the
// "don't add parsing" trap from a different angle: the neutral components
// AddressFromContactAddress emits must flatten back to exactly the flat
// FormatAddress line the ticket's "new parts appear in the formatted display
// line" promise makes — i.e. nothing is re-ordered or lost in the round trip
// through the card.
func TestAddressFromContactAddress_OrderingPinsFormatAddressSymmetry(t *testing.T) {
	flat := ContactAddress{
		Type: "home", Street: "742 Clark St", POBox: "PO Box 42", Apartment: "Apt 3B",
		Floor: "Floor 2", City: "Springfield", Region: "IL", Postal: "62701", Country: "USA",
	}
	neutral := AddressFromContactAddress(flat)
	back := contactAddressFromNeutral(neutral)
	if back != flat {
		t.Errorf("round trip through the card = %+v, want %+v", back, flat)
	}
	if FormatAddress(flat) != neutral.Full {
		t.Errorf("neutral.Full = %q, want FormatAddress %q (the display line must match)", neutral.Full, FormatAddress(flat))
	}
}
