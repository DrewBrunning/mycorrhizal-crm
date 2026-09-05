package models

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
)

// TestApplyRecordToContact_ReDerivesRelativePhotoURL is the regression test
// for M6's write-path round-trip (M6): the read response now exposes Card.Media's photo uri as
// a relative profile-picture URL, and the web client PUTs that card back
// verbatim on the next edit. applyMedia must recognize that relative URL as
// "this contact's own photo pointer" and re-derive the entry from the flat
// Photo/PhotoThumbnail — persisting the dead URL instead would break VCF/
// JSContact export and CardDAV, whose consumers cannot fetch a relative path.
func TestApplyRecordToContact_ReDerivesRelativePhotoURL(t *testing.T) {
	t.Parallel()
	photoDir := t.TempDir()
	writeTestJPEG(t, filepath.Join(photoDir, "disk_photo.jpg"))

	contact := &Contact{
		Photo:          "disk_photo.jpg",
		PhotoThumbnail: testJPEGDataURL(),
	}

	incoming := &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			Media: []contactmodel.Resource{
				{Kind: "photo", URI: "/api/v1/contacts/5/profile_picture"},
			},
		},
	}

	ApplyRecordToContact(contact, incoming, photoDir)

	if len(contact.Card.Media) != 1 {
		t.Fatalf("Card.Media = %+v, want the single photo entry preserved (repaired, not dropped)", contact.Card.Media)
	}
	photo := contact.Card.Media[0]
	if photo.Kind != "photo" {
		t.Fatalf("Card.Media[0].Kind = %q, want photo", photo.Kind)
	}
	if photo.URI == "/api/v1/contacts/5/profile_picture" {
		t.Error("the relative profile-picture URL must NOT be persisted into the Card; the entry must be re-derived from the flat photo")
	}
	if !strings.HasPrefix(photo.URI, "data:image/jpeg;base64,") {
		t.Errorf("Card.Media[0].URI = %q, want a data URI re-derived from the on-disk photo", photo.URI)
	}
	if contact.Photo != "disk_photo.jpg" {
		t.Errorf("contact.Photo = %q, want unchanged (no new photo was applied)", contact.Photo)
	}
}

// TestApplyRecordToContact_RealPhotoRoundTripStillPersists pins that the
// applyMedia repair only fires for non-data, non-URL garbage: a real embedded
// data-URI photo in the incoming card is still decoded and persisted to disk
// (the pre-M6 behavior the round-trip test at the top of this file depends on).
func TestApplyRecordToContact_RealPhotoRoundTripStillPersists(t *testing.T) {
	t.Parallel()
	photoDir := t.TempDir()
	thumb := testJPEGDataURL()

	contact := &Contact{}
	incoming := &contactmodel.Record{
		Card: contactmodel.Card{
			Name:  &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			Media: []contactmodel.Resource{{Kind: "photo", URI: thumb, MediaType: "image/jpeg"}},
		},
	}

	ApplyRecordToContact(contact, incoming, photoDir)

	if contact.Photo == "" {
		t.Error("a real data-URI photo must still be persisted to disk and onto Contact.Photo")
	}
	if !strings.HasPrefix(contact.PhotoThumbnail, "data:image/jpeg;base64,") {
		t.Errorf("Contact.PhotoThumbnail = %q, want a data:image/jpeg;base64,... thumbnail", contact.PhotoThumbnail)
	}
	if len(contact.Card.Media) != 1 || !strings.HasPrefix(contact.Card.Media[0].URI, "data:image/jpeg;base64,") {
		t.Errorf("Card.Media[0] = %+v, want the embedded data-URI photo entry preserved", contact.Card.Media)
	}
}

// TestApplyRecordToContact_RoundTrip: RecordFromContact
// a fully-populated Contact into a Record, ApplyRecordToContact that Record
// onto a fresh Contact, and assert the result matches the original closely
// enough that nothing was silently lost in a way the doc doesn't already
// call out as an accepted, documented lossy case.
//
// Also  photo-bridging prerequisite end-to-end: original
// (fullyPopulatedContact) carries a PhotoThumbnail, which RecordFromContact
// bridges into record.Card.Media, which ApplyRecordToContact (given a real
// photoDir) then decodes and persists back to disk — proving the photo
// round-trips through Card.Media in both directions, not just one.
func TestApplyRecordToContact_RoundTrip(t *testing.T) {
	t.Parallel()
	original := fullyPopulatedContact()
	photoDir := t.TempDir()
	record := RecordFromContact(original, photoDir)

	got := &Contact{}
	ApplyRecordToContact(got, record, photoDir)

	// Card/CRM/Passthrough are the authoritative full-fidelity copy: exact,
	// byte-for-byte (deep-equal) match required.
	if !reflect.DeepEqual(got.Card, record.Card) {
		t.Errorf("got.Card = %+v, want exact match with record.Card = %+v", got.Card, record.Card)
	}
	if !reflect.DeepEqual(got.CRM, record.Envelope) {
		t.Errorf("got.CRM = %+v, want exact match with record.Envelope = %+v", got.CRM, record.Envelope)
	}
	if !reflect.DeepEqual(got.Passthrough, record.Passthrough) {
		t.Errorf("got.Passthrough = %+v, want exact match with record.Passthrough = %+v", got.Passthrough, record.Passthrough)
	}

	// Flat legacy fields: exact match expected for everything that has an
	// unambiguous neutral home.
	if got.Firstname != original.Firstname {
		t.Errorf("Firstname = %q, want %q", got.Firstname, original.Firstname)
	}
	if got.Lastname != original.Lastname {
		t.Errorf("Lastname = %q, want %q", got.Lastname, original.Lastname)
	}
	if got.MiddleName != original.MiddleName {
		t.Errorf("MiddleName = %q, want %q", got.MiddleName, original.MiddleName)
	}
	if got.Prefix != original.Prefix {
		t.Errorf("Prefix = %q, want %q", got.Prefix, original.Prefix)
	}
	if got.Suffix != original.Suffix {
		t.Errorf("Suffix = %q, want %q", got.Suffix, original.Suffix)
	}
	if got.Nickname != original.Nickname {
		t.Errorf("Nickname = %q, want %q", got.Nickname, original.Nickname)
	}
	if got.Organization != original.Organization {
		t.Errorf("Organization = %q, want %q", got.Organization, original.Organization)
	}
	if got.Department != original.Department {
		t.Errorf("Department = %q, want %q", got.Department, original.Department)
	}
	if got.JobTitle != original.JobTitle {
		t.Errorf("JobTitle = %q, want %q", got.JobTitle, original.JobTitle)
	}
	if got.Role != original.Role {
		t.Errorf("Role = %q, want %q", got.Role, original.Role)
	}
	if got.Birthday != original.Birthday {
		t.Errorf("Birthday = %q, want %q", got.Birthday, original.Birthday)
	}
	if got.Anniversary != original.Anniversary {
		t.Errorf("Anniversary = %q, want %q", got.Anniversary, original.Anniversary)
	}
	if got.HowWeMet != original.HowWeMet {
		t.Errorf("HowWeMet = %q, want %q", got.HowWeMet, original.HowWeMet)
	}
	if got.WorkInformation != original.WorkInformation {
		t.Errorf("WorkInformation = %q, want %q", got.WorkInformation, original.WorkInformation)
	}
	if got.ContactInformation != original.ContactInformation {
		t.Errorf("ContactInformation = %q, want %q", got.ContactInformation, original.ContactInformation)
	}
	// Gender (issue #515): it now has a CRMEnvelope home, so it participates
	// in the Record round trip exactly like the other CRM-only fields above —
	// this assertion used to be the "known accepted gap" block, removed when
	// the hole was closed.
	if got.Gender != original.Gender {
		t.Errorf("Gender = %q, want %q (issue #515: CRMEnvelope.Gender round-trips)", got.Gender, original.Gender)
	}
	if !reflect.DeepEqual(got.Circles, original.Circles) {
		t.Errorf("Circles = %+v, want %+v", got.Circles, original.Circles)
	}
	if got.VCardUID != original.VCardUID {
		t.Errorf("VCardUID = %q, want %q", got.VCardUID, original.VCardUID)
	}

	// Emails/Phones/URLs/IMPPs: exact match (Label<->Type is a lossless 1:1
	// round trip for these).
	if !reflect.DeepEqual(got.Emails, original.Emails) {
		t.Errorf("Emails = %+v, want %+v", got.Emails, original.Emails)
	}
	if !reflect.DeepEqual(got.Phones, original.Phones) {
		t.Errorf("Phones = %+v, want %+v", got.Phones, original.Phones)
	}
	if !reflect.DeepEqual(got.URLs, original.URLs) {
		t.Errorf("URLs = %+v, want %+v", got.URLs, original.URLs)
	}
	if !reflect.DeepEqual(got.IMPPs, original.IMPPs) {
		t.Errorf("IMPPs = %+v, want %+v", got.IMPPs, original.IMPPs)
	}

	// Addresses: exact match expected here too, since fullyPopulatedContact's
	// one address has real structured components including the T79 sub-street
	// parts (PO box / apartment / floor) — not the Full-only fallback case
	// documented as lossy in applyAddresses.
	if !reflect.DeepEqual(got.Addresses, original.Addresses) {
		t.Errorf("Addresses = %+v, want %+v", got.Addresses, original.Addresses)
	}

	// Known, documented, ACCEPTED gaps (not asserted equal):
	//   - VCardExtra: RecordFromContact's buildPassthrough is explicitly
	//     best-effort/lossy converting VCardExtra -> Passthrough.VCard (see
	//     that function's doc comment), and ApplyRecordToContact deliberately
	//     does not attempt to re-serialize Passthrough.VCard back into
	//     VCardExtra's bespoke JSON shape (see ApplyRecordToContact's own doc
	//     comment) — the data lives on, losslessly, in c.Passthrough (already
	//     asserted equal to record.Passthrough above), just not mirrored back
	//     into the legacy VCardExtra column.
	if got.VCardExtra != "" {
		t.Errorf("got.VCardExtra = %q, want \"\" (Passthrough is not re-serialized back into the legacy VCardExtra column)", got.VCardExtra)
	}

	// photo-bridging prerequisite: the photo bridged into
	// record.Card.Media by RecordFromContact (from original.PhotoThumbnail,
	// since original.Photo has no on-disk file) round-trips through
	// ApplyRecordToContact back onto disk and onto got.Photo/PhotoThumbnail.
	if got.Photo == "" {
		t.Error("got.Photo is empty, want a saved photo filename (the Card.Media photo entry should have been persisted to disk)")
	}
	if !strings.HasPrefix(got.PhotoThumbnail, "data:image/jpeg;base64,") {
		t.Errorf("got.PhotoThumbnail = %q, want a data:image/jpeg;base64,... thumbnail", got.PhotoThumbnail)
	}
	if _, err := os.Stat(filepath.Join(photoDir, got.Photo)); err != nil {
		t.Errorf("saved photo file not found on disk at %q: %v", filepath.Join(photoDir, got.Photo), err)
	}

	// The cardSetDirectly guard must be set so a subsequent BeforeSave
	// doesn't clobber the Card/CRM/Passthrough we just asserted above.
	if !got.cardSetDirectly {
		t.Error("got.cardSetDirectly = false, want true after ApplyRecordToContact")
	}
}

// TestAddressMapping_RoundTripsSubStreetFields pins T79
// (T79):
// the flat ContactAddress gained PO box / apartment / floor slots, so both
// directions of the flat<->neutral mapping must carry them. A vCard-imported
// address holding those components must land on the flat struct (and back
// onto the card on the next derivation) instead of being silently dropped by
// the "five fields the flat projection can store" narrowing this ticket
// closes.
func TestAddressMapping_RoundTripsSubStreetFields(t *testing.T) {
	t.Parallel()
	neutral := contactmodel.Address{
		Components: []contactmodel.AddressComponent{
			{Kind: "postOfficeBox", Value: "PO Box 42"},
			{Kind: "apartment", Value: "Apt 3B"},
			{Kind: "floor", Value: "Floor 2"},
			{Kind: "name", Value: "742 Clark St"},
			{Kind: "locality", Value: "Springfield"},
			{Kind: "region", Value: "IL"},
			{Kind: "postcode", Value: "62701"},
			{Kind: "country", Value: "USA"},
		},
		Contexts: []string{"home"},
	}

	flat := contactAddressFromNeutral(neutral)
	if flat.POBox != "PO Box 42" || flat.Apartment != "Apt 3B" || flat.Floor != "Floor 2" {
		t.Errorf("contactAddressFromNeutral dropped sub-street parts: %+v", flat)
	}
	if flat.Street != "742 Clark St" || flat.City != "Springfield" {
		t.Errorf("contactAddressFromNeutral lost an existing projected field: %+v", flat)
	}
	if flat.Type != "home" {
		t.Errorf("contactAddressFromNeutral type = %q, want contexts[0]", flat.Type)
	}

	// The forward direction re-emits all three kinds (postOfficeBox /
	// apartment / floor) as components.
	roundTripped := AddressFromContactAddress(flat)
	kinds := map[string]string{}
	for _, c := range roundTripped.Components {
		kinds[c.Kind] = c.Value
	}
	for _, tc := range []struct{ kind, want string }{
		{"postOfficeBox", "PO Box 42"},
		{"apartment", "Apt 3B"},
		{"floor", "Floor 2"},
		{"name", "742 Clark St"},
		{"locality", "Springfield"},
	} {
		if kinds[tc.kind] != tc.want {
			t.Errorf("AddressFromContactAddress component %q = %q, want %q (components=%v)", tc.kind, kinds[tc.kind], tc.want, kinds)
		}
	}
}

// TestAddressMapping_TranslatesContextsToLegacyType is T91: the neutral model
// uses private/work for Contexts, the flat model uses home/work, and this
// projection used to copy Contexts[0] across untranslated. Every address that
// arrived through an adapter therefore came out typed "private" -- a token no
// other part of the app understands, so the web address editor rendered it as
// raw text.
//
// The unmapped case is asserted deliberately: RecordFromContact puts the
// legacy free-text Type straight into Contexts (Address has no Label field),
// so an unrecognized context is user data that must survive, not be blanked.
func TestAddressMapping_TranslatesContextsToLegacyType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, context, want string }{
		{"private becomes home", "private", "home"},
		{"work is identical both ways", "work", "work"},
		{"billing is identical both ways", "billing", "billing"},
		{"an already-legacy token passes through", "home", "home"},
		{"an arbitrary context is preserved verbatim", "cabin", "cabin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flat := contactAddressFromNeutral(contactmodel.Address{
				Components: []contactmodel.AddressComponent{{Kind: "name", Value: "123 Fake St"}},
				Contexts:   []string{tc.context},
			})
			if flat.Type != tc.want {
				t.Errorf("contexts[0]=%q projected to Type %q, want %q", tc.context, flat.Type, tc.want)
			}
		})
	}

	// A flat-entered address must still survive a full round trip unchanged --
	// the forward direction (contact_record.go) puts the flat Type into
	// Contexts verbatim, so "home" -> ["home"] -> "home" must not become
	// "private" or blank on the way back.
	roundTripped := contactAddressFromNeutral(AddressFromContactAddress(ContactAddress{
		Street: "123 Fake St", City: "Townesville", Type: "home",
	}))
	if roundTripped.Type != "home" {
		t.Errorf("flat->Card->flat Type = %q, want %q", roundTripped.Type, "home")
	}
}

// TestApplyRecordToContact_AddressFullOnlyFallback documents the one known
// lossy case called out in applyAddresses's doc comment: an Address with no
// structured Components, only Full, reconstructs to an all-blank
// ContactAddress (nothing structured to recover it from) — but the data is
// not gone, it remains verbatim in c.Card.Addresses[].Full.
func TestApplyRecordToContact_AddressFullOnlyFallback(t *testing.T) {
	t.Parallel()
	record := &contactmodel.Record{
		Card: contactmodel.Card{
			Addresses: []contactmodel.Address{{Full: "42 Wallaby Way, Sydney"}},
		},
	}

	got := &Contact{}
	ApplyRecordToContact(got, record, "")

	if len(got.Addresses) != 1 {
		t.Fatalf("Addresses = %+v, want 1 (blank-but-present) entry", got.Addresses)
	}
	if (got.Addresses[0] != ContactAddress{}) {
		t.Errorf("Addresses[0] = %+v, want all-blank (no structured components to recover)", got.Addresses[0])
	}
	// The raw string is not lost: it is still on c.Card.
	if got.Card.Addresses[0].Full != "42 Wallaby Way, Sydney" {
		t.Errorf("Card.Addresses[0].Full = %q, want the original Full string preserved", got.Card.Addresses[0].Full)
	}
}

// TestApplyRecordToContact_NilSafety mirrors RecordFromContact's own
// nil-safety test: ApplyRecordToContact must not panic on a nil Contact or a
// nil Record.
func TestApplyRecordToContact_NilSafety(t *testing.T) {
	t.Parallel()
	ApplyRecordToContact(nil, &contactmodel.Record{}, "")
	ApplyRecordToContact(&Contact{}, nil, "")
}

// TestApplyRecordToContact_PreservesUnmappedCardData asserts the central
// claim of the resolution: Card-only data with no flat-field home
// (SpeakToAs here) is preserved on c.Card even though nothing on the flat
// side reflects it, and — critically — survives a subsequent BeforeSave
// untouched (this is what the cardSetDirectly guard exists to protect;
// without it, BeforeSave's flat->Card derivation would silently wipe
// SpeakToAs out on the very next save).
func TestApplyRecordToContact_PreservesUnmappedCardData(t *testing.T) {
	t.Parallel()
	record := &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			SpeakToAs: &contactmodel.SpeakToAs{
				Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}},
			},
		},
	}

	c := &Contact{}
	ApplyRecordToContact(c, record, "")

	if c.Card.SpeakToAs == nil || len(c.Card.SpeakToAs.Pronouns) != 1 || c.Card.SpeakToAs.Pronouns[0].Pronouns != "she/her" {
		t.Fatalf("c.Card.SpeakToAs = %+v, want the SpeakToAs from the input Record preserved", c.Card.SpeakToAs)
	}

	if err := c.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}

	if c.Card.SpeakToAs == nil || len(c.Card.SpeakToAs.Pronouns) != 1 || c.Card.SpeakToAs.Pronouns[0].Pronouns != "she/her" {
		t.Errorf("c.Card.SpeakToAs after BeforeSave = %+v, want it still preserved (BeforeSave must not re-derive Card when cardSetDirectly was set)", c.Card.SpeakToAs)
	}
}

// TestApplyRecordToContact_PreservesCRMKind pins T27's contract: the CRM
// envelope-side Kind (individual|pet|animal, contactmodel/envelope.go) is
// set via ApplyRecordToContact exactly like every other CRMEnvelope field,
// survives a save (it lives only in the crm JSON column — there is no flat
// scalar home for it, so the cardSetDirectly guard is what keeps it), and
// round-trips back out through RecordForContact (the read path that reads
// what is actually persisted). The suggestion engine (services/
// household_service.go's classifyMember) depends on Contact.CRM.Kind, so a
// regression here silently reclassifies every pet as an adult.
func TestApplyRecordToContact_PreservesCRMKind(t *testing.T) {
	t.Parallel()
	record := &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Fluffy"}}},
		},
		Envelope: contactmodel.CRMEnvelope{Kind: "pet"},
	}

	c := &Contact{}
	ApplyRecordToContact(c, record, "")

	if c.CRM.Kind != "pet" {
		t.Fatalf("c.CRM.Kind = %q, want %q after ApplyRecordToContact", c.CRM.Kind, "pet")
	}

	// BeforeSave must not re-derive CRM from the (Kind-less) flat fields and
	// wipe it out — the cardSetDirectly guard's whole job.
	if err := c.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}
	if c.CRM.Kind != "pet" {
		t.Errorf("c.CRM.Kind after BeforeSave = %q, want %q preserved", c.CRM.Kind, "pet")
	}

	// RecordForContact (the real read path) must surface it back.
	got := RecordForContact(c, "", nil)
	if got == nil || got.Envelope.Kind != "pet" {
		t.Errorf("RecordForContact.Envelope.Kind = %+v, want %q", got.Envelope.Kind, "pet")
	}
}

// TestApplyRecordToContact_ClearsPhoneScalarWhenPhonesRemoved is the
// regression guard for a real bug found by audit:
// applyPhones cleared
// c.Phones but left the c.Phone scalar untouched when the incoming Record
// had no phones at all, unlike its sibling applyEmails (which always
// resets c.Email, falling back to proj.PrimaryEmail). A contact whose last
// phone number was removed via REST PUT, CardDAV sync, or VCF import kept a
// stale c.Phone value even though c.Phones correctly went empty.
func TestApplyRecordToContact_ClearsPhoneScalarWhenPhonesRemoved(t *testing.T) {
	t.Parallel()
	c := &Contact{Phone: "555-0100", Phones: []ContactPhone{{Type: "home", Value: "555-0100"}}}

	ApplyRecordToContact(c, &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
	}}, "")

	if len(c.Phones) != 0 {
		t.Errorf("c.Phones = %+v, want empty (incoming Record has no phones)", c.Phones)
	}
	if c.Phone != "" {
		t.Errorf("c.Phone = %q, want empty — must not retain a stale value once Phones is cleared", c.Phone)
	}
}
