package models

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// testJPEGDataURL returns a "data:image/jpeg;base64,..." URI wrapping a
// minimal, valid 2x2 JPEG — usable wherever a test needs
// Contact.PhotoThumbnail (or any decodable photo payload, e.g. for
// photostore.SaveContactPhoto) without caring about actual pixel content.
func testJPEGDataURL() string {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// fullyPopulatedContact builds a *Contact with every field RecordFromContact
// maps populated, so tests can assert each lands in its correct neutral
// home per docs/adrs/0002-correspondence-table-locked-oracle.md.
func fullyPopulatedContact() *Contact {
	return &Contact{
		Firstname:   "Jane",
		Lastname:    "Doe",
		MiddleName:  "Quinn",
		Prefix:      "Dr.",
		Suffix:      "Jr.",
		Nickname:    "Janie",
		Gender:      "female",
		Email:       "jane.legacy@example.com", // overridden by Emails[0] below
		Phone:       "+10000000000",            // overridden by Phones[0] below
		Birthday:    "1990-06-15",
		Anniversary: "--09-20",
		Emails: []ContactEmail{
			{Type: "work", Value: "jane@example.com"},
		},
		Phones: []ContactPhone{
			{Type: "cell", Value: "+15551234567"},
		},
		Addresses: []ContactAddress{
			{Type: "home", Street: "1 Main St", POBox: "PO Box 42", Apartment: "Apt 3B", Floor: "Floor 2", City: "Springfield", Region: "IL", Postal: "62704", Country: "US"},
		},
		URLs: []ContactURL{
			{Type: "personal", Value: "https://jane.example.com"},
		},
		IMPPs: []ContactIMPP{
			{Type: "telegram", Value: "xmpp:jane@example.com"},
		},
		Organization:       "Acme Corp",
		Department:         "Engineering",
		JobTitle:           "Staff Engineer",
		Role:               "Tech Lead",
		HowWeMet:           "Conference",
		WorkInformation:    "Remote",
		ContactInformation: "Prefers email",
		Circles:            []string{"friends", "work"},
		VCardUID:           "uid-1234",
		VCardExtra:         `{"properties":{"X-CUSTOM":[{"Value":"keep","Params":{"TYPE":["home"]},"Group":""}]}}`,
		PhotoThumbnail:     testJPEGDataURL(),
	}
}

// TestRecordFromContact_FullyPopulated asserts every mapped Contact field
// lands in the neutral home documented by docs/adrs/0002-correspondence-table-locked-oracle.md.
func TestRecordFromContact_FullyPopulated(t *testing.T) {
	t.Parallel()
	c := fullyPopulatedContact()
	record := RecordFromContact(c, "")
	card := record.Card

	// "uid" row: Card.UID <- VCardUID
	if card.UID != "uid-1234" {
		t.Errorf("Card.UID = %q, want %q", card.UID, "uid-1234")
	}
	if record.UID != "uid-1234" {
		t.Errorf("Record.UID = %q, want %q", record.UID, "uid-1234")
	}

	// "name.given"/"name.surname"/"name.given2"/"name.title"/"name.credential" rows
	if card.Name == nil {
		t.Fatal("Card.Name is nil, want populated")
	}
	wantComponents := map[string]string{
		"given":      "Jane",
		"surname":    "Doe",
		"given2":     "Quinn",
		"title":      "Dr.",
		"credential": "Jr.",
	}
	gotComponents := map[string]string{}
	for _, comp := range card.Name.Components {
		gotComponents[comp.Kind] = comp.Value
	}
	for kind, want := range wantComponents {
		if got := gotComponents[kind]; got != want {
			t.Errorf("Card.Name.Components[kind=%s] = %q, want %q", kind, got, want)
		}
	}

	// "nickname" row: Card.Nicknames[].Name
	if len(card.Nicknames) != 1 || card.Nicknames[0].Name != "Janie" {
		t.Errorf("Card.Nicknames = %+v, want single entry Name=Janie", card.Nicknames)
	}

	// "org"/"org.unit" rows: Card.Organizations[].Name / .Units[].Name
	if len(card.Organizations) != 1 {
		t.Fatalf("Card.Organizations = %+v, want 1 entry", card.Organizations)
	}
	if card.Organizations[0].Name != "Acme Corp" {
		t.Errorf("Card.Organizations[0].Name = %q, want Acme Corp", card.Organizations[0].Name)
	}
	if len(card.Organizations[0].Units) != 1 || card.Organizations[0].Units[0].Name != "Engineering" {
		t.Errorf("Card.Organizations[0].Units = %+v, want single unit Engineering", card.Organizations[0].Units)
	}

	// "title"/"role" rows: Card.Titles[kind=title/role].Name
	var gotTitle, gotRole string
	for _, ti := range card.Titles {
		switch ti.Kind {
		case "title":
			gotTitle = ti.Name
		case "role":
			gotRole = ti.Name
		}
	}
	if gotTitle != "Staff Engineer" {
		t.Errorf("Card.Titles[kind=title].Name = %q, want Staff Engineer", gotTitle)
	}
	if gotRole != "Tech Lead" {
		t.Errorf("Card.Titles[kind=role].Name = %q, want Tech Lead", gotRole)
	}

	// "email" row: Card.Emails[].Address (Emails[] takes precedence over the
	// legacy Email scalar, which is only a fallback for scalar-only contacts)
	if len(card.Emails) != 1 || card.Emails[0].Address != "jane@example.com" || card.Emails[0].Label != "work" {
		t.Errorf("Card.Emails = %+v, want [{Address:jane@example.com Label:work}]", card.Emails)
	}

	// "phone" row: Card.Phones[].Number
	if len(card.Phones) != 1 || card.Phones[0].Number != "+15551234567" || card.Phones[0].Label != "cell" {
		t.Errorf("Card.Phones = %+v, want [{Number:+15551234567 Label:cell}]", card.Phones)
	}

	// "impp" row: Card.ImppAddresses[].URI; legacy Type -> Service, per this
	// WP's explicit instruction (Type holds a service name, not a category)
	if len(card.ImppAddresses) != 1 || card.ImppAddresses[0].Service != "telegram" || card.ImppAddresses[0].URI != "xmpp:jane@example.com" {
		t.Errorf("Card.ImppAddresses = %+v, want [{Service:telegram URI:xmpp:jane@example.com}]", card.ImppAddresses)
	}

	// "adr" row: Card.Addresses[]
	if len(card.Addresses) != 1 {
		t.Fatalf("Card.Addresses = %+v, want 1 entry", card.Addresses)
	}
	addr := card.Addresses[0]
	wantAddrComponents := map[string]string{
		"name":          "1 Main St",
		"postOfficeBox": "PO Box 42",
		"apartment":     "Apt 3B",
		"floor":         "Floor 2",
		"locality":      "Springfield",
		"region":        "IL",
		"postcode":      "62704",
		"country":       "US",
	}
	gotAddrComponents := map[string]string{}
	for _, comp := range addr.Components {
		gotAddrComponents[comp.Kind] = comp.Value
	}
	for kind, want := range wantAddrComponents {
		if got := gotAddrComponents[kind]; got != want {
			t.Errorf("Card.Addresses[0].Components[kind=%s] = %q, want %q", kind, got, want)
		}
	}
	if addr.Full == "" {
		t.Error("Card.Addresses[0].Full is empty, want the formatted address line")
	}
	if len(addr.Contexts) != 1 || addr.Contexts[0] != "home" {
		t.Errorf("Card.Addresses[0].Contexts = %+v, want [home]", addr.Contexts)
	}

	// "link" row: Card.Links[].URI
	if len(card.Links) != 1 || card.Links[0].URI != "https://jane.example.com" || card.Links[0].Label != "personal" {
		t.Errorf("Card.Links = %+v, want [{URI:https://jane.example.com Label:personal}]", card.Links)
	}

	// "anniversary.birth"/"anniversary.wedding" rows: Card.Anniversaries[kind=X].Date
	var birth, wedding *contactmodel.Anniversary
	for i := range card.Anniversaries {
		switch card.Anniversaries[i].Kind {
		case "birth":
			birth = &card.Anniversaries[i]
		case "wedding":
			wedding = &card.Anniversaries[i]
		}
	}
	if birth == nil || birth.Date.Partial == nil {
		t.Fatal("Card.Anniversaries[kind=birth] missing or has no partial date")
	}
	if birth.Date.Partial.Year == nil || *birth.Date.Partial.Year != 1990 ||
		birth.Date.Partial.Month == nil || *birth.Date.Partial.Month != 6 ||
		birth.Date.Partial.Day == nil || *birth.Date.Partial.Day != 15 {
		t.Errorf("Card.Anniversaries[kind=birth].Date.Partial = %+v, want 1990-06-15", birth.Date.Partial)
	}
	if wedding == nil || wedding.Date.Partial == nil {
		t.Fatal("Card.Anniversaries[kind=wedding] missing or has no partial date")
	}
	if wedding.Date.Partial.Year != nil {
		t.Errorf("Card.Anniversaries[kind=wedding].Date.Partial.Year = %v, want nil (year-less --MM-DD)", *wedding.Date.Partial.Year)
	}
	if wedding.Date.Partial.Month == nil || *wedding.Date.Partial.Month != 9 ||
		wedding.Date.Partial.Day == nil || *wedding.Date.Partial.Day != 20 {
		t.Errorf("Card.Anniversaries[kind=wedding].Date.Partial = %+v, want --09-20", wedding.Date.Partial)
	}

	// CRM-only fields: 1:1 direct copy into Record.Envelope
	env := record.Envelope
	if env.HowWeMet != "Conference" ||
		env.WorkInformation != "Remote" || env.ContactInformation != "Prefers email" {
		t.Errorf("Record.Envelope text fields = %+v, want the values set on the Contact", env)
	}
	if env.Gender != "female" {
		t.Errorf("Record.Envelope.Gender = %q, want female (issue #515: Gender has a CRMEnvelope home)", env.Gender)
	}
	if len(env.Circles) != 2 || env.Circles[0] != "friends" || env.Circles[1] != "work" {
		t.Errorf("Record.Envelope.Circles = %+v, want [friends work]", env.Circles)
	}

	// photo-bridging prerequisite: Contact.PhotoThumbnail (there is no
	// Contact.Photo/on-disk file in this test, so ReadContactPhoto falls back
	// to the thumbnail) bridges into a single Card.Media{Kind:"photo"} entry,
	// encoded as the same "data:<mediaType>;base64,<data>" URI convention the
	// vcard4/vcard3 adapters already use for an embedded PHOTO value.
	if len(card.Media) != 1 {
		t.Fatalf("Card.Media = %+v, want 1 photo entry bridged from PhotoThumbnail", card.Media)
	}
	photo := card.Media[0]
	if photo.Kind != "photo" {
		t.Errorf("Card.Media[0].Kind = %q, want photo", photo.Kind)
	}
	if photo.MediaType != "image/jpeg" {
		t.Errorf("Card.Media[0].MediaType = %q, want image/jpeg", photo.MediaType)
	}
	if !strings.HasPrefix(photo.URI, "data:image/jpeg;base64,") {
		t.Errorf("Card.Media[0].URI = %q, want a data:image/jpeg;base64,... URI", photo.URI)
	}

	// "pt.vcard" row (best-effort): Passthrough.VCard <- VCardExtra
	if len(record.Passthrough.VCard) != 1 {
		t.Fatalf("Record.Passthrough.VCard = %+v, want 1 entry from VCardExtra", record.Passthrough.VCard)
	}
	if record.Passthrough.VCard[0].Name != "X-CUSTOM" {
		t.Errorf("Record.Passthrough.VCard[0].Name = %q, want X-CUSTOM", record.Passthrough.VCard[0].Name)
	}

	// Deliberate, flagged gap: Gender has no neutral home in this WP (see the
	// long comment in RecordFromContact). Contact.Gender itself must remain
	// completely untouched by RecordFromContact (it's read-only here).
	if c.Gender != "female" {
		t.Errorf("c.Gender was mutated to %q, want it untouched (RecordFromContact must not mutate its input)", c.Gender)
	}
}

// TestRecordFromContact_ZeroValue asserts RecordFromContact never panics on
// an empty Contact or a nil receiver, per this WP's explicit requirement
// (needed for BeforeSave on a brand-new, mostly-empty contact being created).
func TestRecordFromContact_ZeroValue(t *testing.T) {
	t.Parallel()
	record := RecordFromContact(&Contact{}, "")
	if record == nil {
		t.Fatal("RecordFromContact(&Contact{}, \"\") returned nil")
	}
	if record.Card.Name != nil {
		t.Errorf("zero-value Contact produced non-nil Card.Name: %+v", record.Card.Name)
	}
	if len(record.Card.Emails) != 0 || len(record.Card.Phones) != 0 || len(record.Card.Addresses) != 0 {
		t.Errorf("zero-value Contact produced non-empty collections: %+v", record.Card)
	}
	if len(record.Card.Media) != 0 {
		t.Errorf("zero-value Contact produced non-empty Card.Media: %+v", record.Card.Media)
	}

	nilRecord := RecordFromContact(nil, "")
	if nilRecord == nil {
		t.Fatal("RecordFromContact(nil, \"\") returned nil, want a safe empty *Record")
	}
}

// TestRecordFromContact_ScalarOnlyFallback asserts that contacts whose
// Email/Phone/Address were only ever set as legacy scalars (no
// Emails/Phones/Addresses array entries — a real pattern in this codebase's
// own existing tests/fixtures) still get that data mapped into Card, so the
// P1 data migration is lossless for them.
func TestRecordFromContact_ScalarOnlyFallback(t *testing.T) {
	t.Parallel()
	c := &Contact{
		Firstname: "Alice",
		Email:     "alice@example.com",
		Phone:     "+15550001111",
		Address:   "42 Wallaby Way, Sydney",
	}
	record := RecordFromContact(c, "")

	if len(record.Card.Emails) != 1 || record.Card.Emails[0].Address != "alice@example.com" {
		t.Errorf("Card.Emails = %+v, want scalar-fallback entry alice@example.com", record.Card.Emails)
	}
	if len(record.Card.Phones) != 1 || record.Card.Phones[0].Number != "+15550001111" {
		t.Errorf("Card.Phones = %+v, want scalar-fallback entry +15550001111", record.Card.Phones)
	}
	if len(record.Card.Addresses) != 1 || record.Card.Addresses[0].Full != "42 Wallaby Way, Sydney" {
		t.Errorf("Card.Addresses = %+v, want scalar-fallback Full=42 Wallaby Way, Sydney", record.Card.Addresses)
	}

	proj := contactmodel.DeriveProjection(record)
	if proj.PrimaryEmail != "alice@example.com" {
		t.Errorf("DeriveProjection PrimaryEmail = %q, want alice@example.com (round-trip of the scalar-only contact)", proj.PrimaryEmail)
	}
	if proj.PrimaryPhone != "+15550001111" {
		t.Errorf("DeriveProjection PrimaryPhone = %q, want +15550001111", proj.PrimaryPhone)
	}
}

// TestRecordForContact_PrefersPersistedCardOverFreshDerivation is the
// regression test for a real, live bug found while auditing work:
// three call sites (CardDAV export, the REST API's detail/write response,
// and VCF/JSContact export) each independently called RecordFromContact a
// second time on a contact whose Card was already persisted, which silently
// discards any data with no flat-field home (SpeakToAs here) — exactly the
// data a nested REST POST/PUT or a CardDAV PUT of a modern vCard would have
// set. RecordForContact must return the persisted Card as-is instead of
// re-deriving it from the (necessarily lossy) flat fields.
func TestRecordForContact_PrefersPersistedCardOverFreshDerivation(t *testing.T) {
	t.Parallel()
	c := &Contact{
		Firstname: "Ada",
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			SpeakToAs: &contactmodel.SpeakToAs{
				Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}},
			},
		},
	}

	record := RecordForContact(c, "", nil)

	if record.Card.SpeakToAs == nil || len(record.Card.SpeakToAs.Pronouns) != 1 || record.Card.SpeakToAs.Pronouns[0].Pronouns != "she/her" {
		t.Errorf("RecordForContact's Card.SpeakToAs = %+v, want the persisted she/her preserved (a fresh RecordFromContact call would drop it — no flat field holds pronouns)", record.Card.SpeakToAs)
	}
}

// TestRecordForContact_FallsBackWhenCardIsZeroValue covers the other half of
// RecordForContact: a contact whose Card has never been populated at all
// (e.g. a pre-migration row that predates cmd/backfill-contact-records, or a
// Contact built directly in a test without going through BeforeSave) must
// still fall back to deriving from the flat fields — not return an empty Card.
func TestRecordForContact_FallsBackWhenCardIsZeroValue(t *testing.T) {
	t.Parallel()
	c := &Contact{
		Firstname: "Bob",
		Email:     "bob@example.com",
	}

	record := RecordForContact(c, "", nil)

	if len(record.Card.Emails) != 1 || record.Card.Emails[0].Address != "bob@example.com" {
		t.Errorf("RecordForContact's Card.Emails = %+v, want a fallback-derived entry for bob@example.com (Card was zero-value, should behave like RecordFromContact)", record.Card.Emails)
	}
}

// TestRecordForContact_StampsCardUIDFromVCardUID is the regression test for
// issue #693 ("Contact created by Android has no VCard UID"): a contact
// created via the nested REST shape persists a Card without a UID (the client
// doesn't know one yet — BeforeCreate mints the VCardUID, but the Card was
// already assigned before that hook ran). The read path must stamp
// Card.UID from the VCardUID column so every consumer of RecordForContact
// (REST detail/write response, CardDAV GET, VCF/JSContact export) presents a
// stable identity rather than a UID-less card — the Android app resolved the
// contact's identity from card.uid and failed with that error. Pinned against
// the real migrated schema (dbtest.NewAt) and through the same
// ApplyRecordToContact -> Create -> RecordForContact flow the controller uses.
func TestRecordForContact_StampsCardUIDFromVCardUID(t *testing.T) {
	t.Parallel()
	db := dbtest.NewAt(t, filepath.Join(t.TempDir(), "card-uid.db"))
	user := User{Username: "uidrepro", Password: "password123!A", Email: "uidrepro@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Mimic CreateContact exactly: a nested input whose Card carries no UID.
	input := ContactRecordInput{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
				{Kind: "given", Value: "Alice"},
				{Kind: "surname", Value: "Johnson"},
			}},
		},
	}
	contact := Contact{UserID: user.ID, Gender: input.Gender}
	ApplyRecordToContact(&contact, input.ToRecord(), "")
	require.NoError(t, db.Create(&contact).Error)

	require.NotEmpty(t, contact.VCardUID, "BeforeCreate must have minted a VCardUID")
	require.Empty(t, contact.Card.UID, "the persisted Card must still carry no UID (that is the shape this regression guards)")

	record := RecordForContact(&contact, "", db)
	if record.UID != contact.VCardUID {
		t.Errorf("Record.UID = %q, want %q (the VCardUID column)", record.UID, contact.VCardUID)
	}
	if record.Card.UID != contact.VCardUID {
		t.Errorf("Record.Card.UID = %q, want %q — Card.UID must mirror VCardUID so a client resolving identity via card.uid (Android, issue #693) works", record.Card.UID, contact.VCardUID)
	}
}

// TestRecordForContact_PreservesImportedCardUID pins the "only stamp when
// empty" half of the issue #693 fix: a Card that legitimately carries its own
// imported UID (a vCard/JSContact import preserves the source UID on the
// Card, and ApplyRecordToContact mirrors it into VCardUID) must NOT have it
// clobbered by the stamping logic.
func TestRecordForContact_PreservesImportedCardUID(t *testing.T) {
	t.Parallel()
	c := &Contact{
		VCardUID: "imported-uid-1",
		Card: contactmodel.Card{
			UID:  "imported-uid-1",
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		},
	}

	record := RecordForContact(c, "", nil)

	if record.Card.UID != "imported-uid-1" {
		t.Errorf("Record.Card.UID = %q, want imported-uid-1 (a non-empty Card UID must survive untouched)", record.Card.UID)
	}
}

// TestEnvelopeExportLossDiagnostics covers issue #515's "never a silent drop"
// guarantee: the CRM-only envelope fields (Gender, Circles, HowWeMet, ...)
// round-trip through the neutral Record but have no home in any vCard/JSContact
// FILE export (the format adapters ignore the envelope entirely), so the
// export path must name the drop as a warn Diagnostic rather than silently
// omitting it.
func TestEnvelopeExportLossDiagnostics(t *testing.T) {
	t.Parallel()
	// A nil record or an empty envelope must be silent.
	if diags := EnvelopeExportLossDiagnostics(nil); len(diags) != 0 {
		t.Errorf("EnvelopeExportLossDiagnostics(nil) = %+v, want none", diags)
	}
	if diags := EnvelopeExportLossDiagnostics(&contactmodel.Record{}); len(diags) != 0 {
		t.Errorf("EnvelopeExportLossDiagnostics(empty) = %+v, want none", diags)
	}

	// A fully populated envelope must name every populated field.
	rec := &contactmodel.Record{
		Envelope: contactmodel.CRMEnvelope{
			Gender:             "female",
			Circles:            []string{"friends"},
			HowWeMet:           "Conference",
			WorkInformation:    "Remote",
			ContactInformation: "Prefers email",
		},
	}
	diags := EnvelopeExportLossDiagnostics(rec)
	got := map[string]bool{}
	for _, d := range diags {
		if d.Severity != "warn" {
			t.Errorf("diagnostic %q has severity %q, want warn", d.Concept, d.Severity)
		}
		got[d.Concept] = true
	}
	for _, want := range []string{"crm.gender", "crm.circles", "crm.how_we_met", "crm.work_information", "crm.contact_information"} {
		if !got[want] {
			t.Errorf("missing diagnostic for %s (a populated envelope field must be named, never silently dropped)", want)
		}
	}

	// Kind (human/animal) and unset fields carry no diagnostic: Kind is a
	// classifier the envelope always carries (it is not user data to
	// recover), and empty fields have nothing to report.
	kindOnly := &contactmodel.Record{
		Envelope: contactmodel.CRMEnvelope{Kind: "human"},
	}
	if diags := EnvelopeExportLossDiagnostics(kindOnly); len(diags) != 0 {
		t.Errorf("EnvelopeExportLossDiagnostics(Kind-only) = %+v, want none (Kind is not user data)", diags)
	}
}

// TestBeforeSave_DerivesProjection asserts BeforeSave populates
// Firstname/Lastname/Email/Phone/Birthday/FN/Org (and the Card/CRM/
// Passthrough columns) via the new DeriveProjection-based path.
func TestBeforeSave_DerivesProjection(t *testing.T) {
	t.Parallel()
	c := fullyPopulatedContact()

	if err := c.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}

	if c.Firstname != "Jane" {
		t.Errorf("c.Firstname = %q, want Jane", c.Firstname)
	}
	if c.Lastname != "Doe" {
		t.Errorf("c.Lastname = %q, want Doe", c.Lastname)
	}
	if c.Email != "jane@example.com" {
		t.Errorf("c.Email = %q, want jane@example.com (from Emails[0], via DeriveProjection)", c.Email)
	}
	if c.Phone != "+15551234567" {
		t.Errorf("c.Phone = %q, want +15551234567 (from Phones[0], via DeriveProjection)", c.Phone)
	}
	if c.Birthday != "1990-06-15" {
		t.Errorf("c.Birthday = %q, want 1990-06-15 (round-tripped through Card.Anniversaries)", c.Birthday)
	}
	if c.FN != "Jane Doe" {
		t.Errorf("c.FN = %q, want \"Jane Doe\" (Card.Name.Full is unset, so FN falls back to given+surname)", c.FN)
	}
	if c.Org != "Acme Corp" {
		t.Errorf("c.Org = %q, want Acme Corp", c.Org)
	}

	if c.Card.UID != "uid-1234" {
		t.Errorf("c.Card.UID = %q, want uid-1234 (BeforeSave must populate the Card column too)", c.Card.UID)
	}
	if c.CRM.HowWeMet != "Conference" {
		t.Errorf("c.CRM.HowWeMet = %q, want Conference (BeforeSave must populate the CRM column too)", c.CRM.HowWeMet)
	}
	if len(c.Passthrough.VCard) != 1 {
		t.Errorf("c.Passthrough.VCard = %+v, want 1 entry (BeforeSave must populate the Passthrough column too)", c.Passthrough.VCard)
	}
}

// TestBeforeSave_DoesNotBlankScalarOnlyContact is a regression guard: many
// existing contacts (and existing tests/fixtures elsewhere in this
// codebase) set only the legacy Email/Phone scalars directly, without ever
// populating the Emails/Phones arrays. BeforeSave must not blank those
// scalars out just because DeriveProjection is now in the loop — the
// guarded ("only overwrite when non-empty") assignment plus
// RecordFromContact's scalar fallback together must make this a no-op.
func TestBeforeSave_DoesNotBlankScalarOnlyContact(t *testing.T) {
	t.Parallel()
	c := &Contact{
		Firstname: "Bob",
		Email:     "bob@example.com",
		Phone:     "+15559998888",
	}

	if err := c.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}

	if c.Email != "bob@example.com" {
		t.Errorf("c.Email = %q, want unchanged bob@example.com", c.Email)
	}
	if c.Phone != "+15559998888" {
		t.Errorf("c.Phone = %q, want unchanged +15559998888", c.Phone)
	}
}

// TestRoundTrip_ProjectionStable is the round-trip check called for by
// for a sample of contacts, re-deriving the projection from a
// contact that has already gone through BeforeSave (simulating an
// already-migrated row) must reproduce an equal projection, i.e.
// RecordFromContact -> DeriveProjection is a stable fixed point, not a
// moving target that would drift on repeated saves/backfill runs.
func TestRoundTrip_ProjectionStable(t *testing.T) {
	t.Parallel()
	samples := []*Contact{
		fullyPopulatedContact(),
		{Firstname: "Alice", Email: "alice@example.com", Phone: "+15550001111", Address: "42 Wallaby Way"},
		{Firstname: "Zero"},
	}

	for _, c := range samples {
		if err := c.BeforeSave(nil); err != nil {
			t.Fatalf("BeforeSave returned error for %q: %v", c.Firstname, err)
		}
		firstProj := contactmodel.DeriveProjection(RecordFromContact(c, ""))

		// Simulate a second migration/save pass over the now-migrated row.
		if err := c.BeforeSave(nil); err != nil {
			t.Fatalf("second BeforeSave returned error for %q: %v", c.Firstname, err)
		}
		secondProj := contactmodel.DeriveProjection(RecordFromContact(c, ""))

		if firstProj != secondProj {
			t.Errorf("projection for %q drifted across a second pass: first=%+v second=%+v", c.Firstname, firstProj, secondProj)
		}
	}
}

// setupContactETagTestDB creates an in-memory SQLite DB with the tables the
// Contact ETag/revision tests need. AutoMigrate derives the schema from the
// same Go struct tags the application code uses, so these tests verify hook
// behavior (stamp, bump, no-loop, zero-value guard) but cannot catch a GORM
// column-tag mismatch against the real migration SQL — etag_real_db_test.go
// covers that with a database.InitDB-migrated DB.
// NowFunc is pinned to a single instant: the token is now a monotonic
// revision counter (ADR 0006) with no wall-clock input, but pinning the clock
// keeps the tests' UpdatedAt assertions (and any future timestamp-dependent
// behavior) deterministic under CI load.
func setupContactETagTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	frozen := time.Now()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		NowFunc: func() time.Time { return frozen },
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&User{}, &Contact{}))
	return db
}

func TestContactETagGeneratedOnCreateAndPersists(t *testing.T) {
	t.Parallel()
	db := setupContactETagTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}
	require.NoError(t, db.Create(&contact).Error)

	// ADR 0006: the token is the revision counter, stamped at 1 on create and
	// derived into the ETag string (e-{id}-{revision}).
	require.NotEmpty(t, contact.ETag)
	assert.Regexp(t, regexp.MustCompile(`^e-\d+-\d+$`), contact.ETag)
	assert.Equal(t, int64(1), contact.Revision, "a new Contact starts at revision 1")
	assert.Equal(t, fmt.Sprintf("e-%d-%d", contact.ID, contact.Revision), contact.ETag)

	var reloaded Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Equal(t, contact.ETag, reloaded.ETag, "ETag must be persisted, not just set in memory")
	assert.Equal(t, int64(1), reloaded.Revision, "revision must be persisted, not just set in memory")
}

func TestContactETagChangesOnUpdate(t *testing.T) {
	t.Parallel()
	db := setupContactETagTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}
	require.NoError(t, db.Create(&contact).Error)
	firstETag := contact.ETag

	// updated_at in the map is a red herring now: the token no longer depends
	// on the clock at all, so this update bumps the revision regardless.
	require.NoError(t, db.Model(&contact).Updates(map[string]any{"firstname": "Alicia", "updated_at": time.Now().Add(10 * time.Second)}).Error)

	assert.NotEqual(t, firstETag, contact.ETag, "updating a Contact must change its ETag")
	assert.Equal(t, int64(2), contact.Revision, "an update bumps the revision to 2")
	assert.Equal(t, fmt.Sprintf("e-%d-%d", contact.ID, contact.Revision), contact.ETag)

	var reloaded Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Equal(t, int64(2), reloaded.Revision, "the bumped revision must persist")
	assert.Equal(t, contact.ETag, reloaded.ETag, "the re-derived ETag must persist")
}

// TestContactRevisionBumpsPerSaveNoLoop replaces the old
// TestContactETagSaveDoesNotLoop: under ADR 0006 every persisted write IS a
// new revision (that is the point — a no-op save is still a write), so two
// back-to-back Save() calls must bump revision exactly twice and never loop
// (UpdateColumns bypasses hooks).
func TestContactRevisionBumpsPerSaveNoLoop(t *testing.T) {
	t.Parallel()
	db := setupContactETagTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}
	require.NoError(t, db.Create(&contact).Error)
	require.Equal(t, int64(1), contact.Revision)

	require.NoError(t, db.Save(&contact).Error)
	require.NoError(t, db.Save(&contact).Error)

	assert.Equal(t, int64(3), contact.Revision, "each plain Save bumps the revision (1 create + 2 saves)")
	assert.Equal(t, fmt.Sprintf("e-%d-%d", contact.ID, contact.Revision), contact.ETag)

	var reloaded Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Equal(t, int64(3), reloaded.Revision, "in-memory and persisted revisions must agree (no loop drift)")
	assert.Equal(t, contact.ETag, reloaded.ETag)
}

func TestContactETagBulkUpdateOnZeroValueReceiverDoesNotCorrupt(t *testing.T) {
	t.Parallel()
	db := setupContactETagTestDB(t)
	user := User{Username: "tester", Password: "x", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	one := Contact{UserID: user.ID, Firstname: "Alice", Email: "alice@example.com"}
	two := Contact{UserID: user.ID, Firstname: "Bob", Email: "bob@example.com"}
	require.NoError(t, db.Create(&one).Error)
	require.NoError(t, db.Create(&two).Error)

	require.NoError(t, db.Model(&Contact{}).Where("user_id = ?", user.ID).
		Update("firstname", "Renamed").Error)

	var rows []Contact
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.NotEmpty(t, r.ETag)
		assert.Regexp(t, regexp.MustCompile(`^e-\d+-\d+$`), r.ETag, "bulk update must not rewrite ETags from an empty ID")
		assert.Equal(t, int64(1), r.Revision, "bulk update on a zero-value receiver must not bump revisions")
		assert.Equal(t, fmt.Sprintf("e-%d-%d", r.ID, r.Revision), r.ETag, "ETag must survive the bulk update unchanged")
	}
}

// TestFormatAddress pins the human-readable display line used to keep the
// legacy Address scalar (and, through FlattenAddresses, the searchable
// AddressesFlat column) in sync with the structured Addresses[] JSON. T79
// (T79)
// widened the projection with the sub-street parts a vCard ADR can carry, and
// the conventional display ordering puts them between street and city.
func TestFormatAddress(t *testing.T) {
	t.Parallel()
	full := ContactAddress{Type: "home", Street: "742 Clark St", POBox: "PO Box 42", Apartment: "Apt 3B", Floor: "Floor 2", City: "Springfield", Region: "IL", Postal: "62701", Country: "USA"}
	if got := FormatAddress(full); got != "742 Clark St, PO Box 42, Apt 3B, Floor 2, Springfield, IL, 62701, USA" {
		t.Errorf("FormatAddress(full) = %q", got)
	}

	// Sub-street parts without a street still render (a PO-box-only address
	// has no street to precede them).
	poboxOnly := ContactAddress{POBox: "PO Box 42", City: "Springfield", Region: "IL"}
	if got := FormatAddress(poboxOnly); got != "PO Box 42, Springfield, IL" {
		t.Errorf("FormatAddress(pobox-only) = %q", got)
	}

	if blank := FormatAddress(ContactAddress{}); blank != "" {
		t.Errorf("FormatAddress(blank) = %q, want empty", blank)
	}
}

// TestFlattenAddresses pins the Go-side derivation that feeds the searchable
// AddressesFlat column (T38): each address's non-empty components joined with
// ", ", addresses joined with a space. Migration 000010's SQL backfill
// deliberately mirrors this exact shape for pre-existing rows, so this test
// is what keeps the two from silently diverging (which would make new-contact
// and pre-existing-contact search behavior differ). The sub-street parts the
// projection gained in T79 must appear in the flattened string (that is what
// makes an imported PO box / apartment findable by search).
func TestFlattenAddresses(t *testing.T) {
	t.Parallel()
	addresses := []ContactAddress{
		{Type: "home", Street: "742 Clark St", POBox: "PO Box 42", Apartment: "Apt 3B", Floor: "Floor 2", City: "Springfield", Region: "IL", Postal: "62701", Country: "USA"},
		{Type: "work", Street: "", City: "Chicago"},
	}
	got := FlattenAddresses(addresses)
	want := "742 Clark St, PO Box 42, Apt 3B, Floor 2, Springfield, IL, 62701, USA Chicago"
	if got != want {
		t.Errorf("FlattenAddresses = %q, want %q", got, want)
	}

	if empty := FlattenAddresses(nil); empty != "" {
		t.Errorf("FlattenAddresses(nil) = %q, want empty", empty)
	}
	if blank := FlattenAddresses([]ContactAddress{{Type: "home", Street: "  ", City: "", Region: "", Postal: "", Country: ""}}); blank != "" {
		t.Errorf("FlattenAddresses(all-blank) = %q, want empty", blank)
	}
}

// TestFlattenPhones pins the Go-side derivation that feeds the searchable
// PhonesNormalized column (T69): per entry, the full digit string plus the
// PhoneKey (last-10-digits) when it differs, joined with a space. Migration
// 000020's SQL backfill deliberately mirrors this exact shape for pre-existing
// rows, so this test is what keeps the two from silently diverging (which
// would make new-contact and pre-existing-contact search behavior differ).
func TestFlattenPhones(t *testing.T) {
	t.Parallel()
	phones := []ContactPhone{
		{Type: "cell", Value: "+18005551234"},   // 11 digits → full + key
		{Type: "home", Value: "(800) 555-1234"}, // 10 digits → key == full, one token
	}
	got := FlattenPhones(phones)
	want := "18005551234 8005551234 8005551234"
	if got != want {
		t.Errorf("FlattenPhones = %q, want %q", got, want)
	}

	// A short number keys to nothing beyond its digits.
	if got := FlattenPhones([]ContactPhone{{Type: "cell", Value: "5551"}}); got != "5551" {
		t.Errorf("FlattenPhones(short) = %q, want %q", got, "5551")
	}

	// UK trunk-prefix pair collapses to the same key.
	if got := FlattenPhones([]ContactPhone{{Type: "work", Value: "020 7946 0958"}}); got != "02079460958 2079460958" {
		t.Errorf("FlattenPhones(uk) = %q, want %q", got, "02079460958 2079460958")
	}

	if empty := FlattenPhones(nil); empty != "" {
		t.Errorf("FlattenPhones(nil) = %q, want empty", empty)
	}
	if blank := FlattenPhones([]ContactPhone{{Type: "cell", Value: "  "}}); blank != "" {
		t.Errorf("FlattenPhones(no-digits) = %q, want empty", blank)
	}
}

// TestDeriveSortName pins the Go-side derivation that feeds the denormalized
// sort_name column (T73): lower(trim(lastname)) when non-empty, else
// lower(trim(firstname)). Migration 000021's SQL backfill deliberately
// mirrors this exact shape for pre-existing rows, so this test is what keeps
// the two from silently diverging (which would make new-contact and
// pre-existing-contact sort behavior differ).
func TestDeriveSortName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                string
		lastname, firstname string
		want                string
	}{
		{"lastname wins and is lowercased", "Johnson", "Alice", "johnson"},
		{"firstname fallback is lowercased", "", "Alice", "alice"},
		{"mixed-case lastname lowercases", "De La Cruz", "Ana", "de la cruz"},
		{"lastname whitespace is trimmed", "  Smith  ", "Bob", "smith"},
		{"firstname whitespace is trimmed on fallback", "", "  Eve  ", "eve"},
		{"blank lastname falls back", "   ", "Eve", "eve"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveSortName(tc.lastname, tc.firstname); got != tc.want {
				t.Errorf("DeriveSortName(%q, %q) = %q, want %q", tc.lastname, tc.firstname, got, tc.want)
			}
		})
	}
}

// TestBeforeSave_DerivesSortName asserts BeforeSave populates SortName from
// the final Firstname/Lastname (after the projection block), mirroring how
// it keeps the AddressesFlat/PhonesNormalized derived columns in sync.
func TestBeforeSave_DerivesSortName(t *testing.T) {
	t.Parallel()
	c := &Contact{Firstname: "Jane", Lastname: " Doe "}
	if err := c.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}
	if c.SortName != "doe" {
		t.Errorf("c.SortName = %q, want %q", c.SortName, "doe")
	}

	// No lastname: falls back to the (lowercased, trimmed) firstname.
	firstOnly := &Contact{Firstname: "  Madonna "}
	if err := firstOnly.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave returned error: %v", err)
	}
	if firstOnly.SortName != "madonna" {
		t.Errorf("first-only c.SortName = %q, want %q", firstOnly.SortName, "madonna")
	}
}
