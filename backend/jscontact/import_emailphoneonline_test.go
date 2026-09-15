package jscontact

import (
	"testing"

	"mycorrhizal/internal/rfctest"
)

// Concepts: email, phone, impp, social, email.label, phone.label, onlineservice.other.
// Rows: email  Card.Emails[].Address              /emails/{id}/address        identity
//
//	phone  Card.Phones[].Number               /phones/{id}/number         identity
//	impp   Card.ImppAddresses[].URI           /onlineServices/{id}/uri    identity (routed by vCardName="impp")
//	social Card.SocialProfiles[].Service      /onlineServices/{id}        onlineservice (anchor
//	       field Service; jointly handles sibling .User, per the row's notes; routed by vCardName="socialprofile")
//	email.label          Card.Emails[].Label                /emails/{id}/label   identity (issue #968)
//	phone.label          Card.Phones[].Label                /phones/{id}/label   identity (issue #968)
//	onlineservice.other  Card.OtherOnlineServices[].Service  /onlineServices/{id} identity (issue #968;
//	                     exercised by TestImport_OnlineServiceNoVCardNameHint below)
func init() {
	registerImportCoverage("email", "phone", "impp", "social", "email.label", "phone.label", "onlineservice.other")
}

func TestImport_Email(t *testing.T) {
	t.Parallel()
	// email.jscontact.json: hand-authored minimal fixture, RFC 9553
	// §2.3.1 EmailAddress object shape.
	raw := rfctest.LoadFixture("email.jscontact.json")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Emails) != 1 || rec.Card.Emails[0].Address != "alice@example.com" {
		t.Errorf("Emails = %+v", rec.Card.Emails)
	}
	if rec.Card.Emails[0].ID != "k1" {
		t.Errorf("Emails[0].ID = %q, want k1 (the fixture's map key)", rec.Card.Emails[0].ID)
	}
}

func TestImport_Phone(t *testing.T) {
	t.Parallel()
	// phone.jscontact.json: hand-authored minimal fixture, RFC 9553
	// §2.3.3 Phone object shape.
	raw := rfctest.LoadFixture("phone.jscontact.json")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Phones) != 1 || rec.Card.Phones[0].Number != "+15551234567" {
		t.Errorf("Phones = %+v", rec.Card.Phones)
	}
	if len(rec.Card.Phones[0].Features) != 1 || rec.Card.Phones[0].Features[0] != "voice" {
		t.Errorf("Phones[0].Features = %v, want [voice]", rec.Card.Phones[0].Features)
	}
}

func TestImport_IMPP(t *testing.T) {
	t.Parallel()
	// vCardName="impp" (RFC 9555 §2.7.2/§2.15.3) is the hint that routes this
	// entry into Card.ImppAddresses rather than Card.OtherOnlineServices.
	raw := []byte(`{
		"@type": "Card", "version": "1.0", "uid": "impp-example",
		"onlineServices": {
			"OS-1": { "@type": "OnlineService", "uri": "xmpp:alice@example.com", "pref": 1, "vCardName": "impp" }
		}
	}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.ImppAddresses) != 1 || rec.Card.ImppAddresses[0].URI != "xmpp:alice@example.com" {
		t.Errorf("ImppAddresses = %+v", rec.Card.ImppAddresses)
	}
	if len(rec.Card.SocialProfiles) != 0 || len(rec.Card.OtherOnlineServices) != 0 {
		t.Errorf("expected no SocialProfiles/OtherOnlineServices, got %+v / %+v", rec.Card.SocialProfiles, rec.Card.OtherOnlineServices)
	}
}

func TestImport_SocialProfile(t *testing.T) {
	t.Parallel()
	// vCardName="socialprofile" (RFC 9555 §2.7.5/§2.15.3) is the hint that
	// routes this entry into Card.SocialProfiles rather than
	// Card.OtherOnlineServices.
	raw := []byte(`{
		"@type": "Card", "version": "1.0", "uid": "social-example",
		"onlineServices": {
			"OS-1": { "@type": "OnlineService", "service": "Mastodon", "user": "alice", "uri": "https://example.social/@alice", "vCardName": "socialprofile" }
		}
	}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.SocialProfiles) != 1 {
		t.Fatalf("len(SocialProfiles) = %d, want 1", len(rec.Card.SocialProfiles))
	}
	o := rec.Card.SocialProfiles[0]
	if o.Service != "Mastodon" || o.User != "alice" {
		t.Errorf("SocialProfiles[0] = %+v", o)
	}
	if len(rec.Card.ImppAddresses) != 0 || len(rec.Card.OtherOnlineServices) != 0 {
		t.Errorf("expected no ImppAddresses/OtherOnlineServices, got %+v / %+v", rec.Card.ImppAddresses, rec.Card.OtherOnlineServices)
	}
}

// TestImport_EmailLabel pins issue #968's email.label concept: the RFC 9553
// §2.3.1 EmailAddress `label` member round-trips into Card.Emails[].Label,
// unlike the vCard formats which have no carrier for it at all.
func TestImport_EmailLabel(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"@type": "Card", "version": "1.0", "uid": "email-label-example",
		"emails": {
			"k1": { "@type": "EmailAddress", "address": "alice@example.com", "label": "Work Email" }
		}
	}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Emails) != 1 || rec.Card.Emails[0].Label != "Work Email" {
		t.Errorf("Emails = %+v, want Label \"Work Email\"", rec.Card.Emails)
	}
}

// TestImport_PhoneLabel pins issue #968's phone.label concept: the RFC 9553
// §2.3.3 Phone `label` member round-trips into Card.Phones[].Label, unlike
// the vCard formats which have no carrier for it at all.
func TestImport_PhoneLabel(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"@type": "Card", "version": "1.0", "uid": "phone-label-example",
		"phones": {
			"k1": { "@type": "Phone", "number": "+15551234567", "label": "Mobile" }
		}
	}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Phones) != 1 || rec.Card.Phones[0].Label != "Mobile" {
		t.Errorf("Phones = %+v, want Label \"Mobile\"", rec.Card.Phones)
	}
}

func TestImport_OnlineServiceNoVCardNameHint(t *testing.T) {
	t.Parallel()
	// No vCardName hint at all: per docs/adrs/0002-correspondence-table-locked-oracle.md, this must
	// route to Card.OtherOnlineServices — never guessed into ImppAddresses
	// or SocialProfiles by a presence-based heuristic (e.g. service/user set).
	raw := []byte(`{
		"@type": "Card", "version": "1.0", "uid": "unclassified-example",
		"onlineServices": {
			"OS-1": { "@type": "OnlineService", "service": "Mastodon", "user": "alice", "uri": "https://example.social/@alice" }
		}
	}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.OtherOnlineServices) != 1 {
		t.Fatalf("len(OtherOnlineServices) = %d, want 1", len(rec.Card.OtherOnlineServices))
	}
	o := rec.Card.OtherOnlineServices[0]
	if o.Service != "Mastodon" || o.User != "alice" {
		t.Errorf("OtherOnlineServices[0] = %+v", o)
	}
	if len(rec.Card.ImppAddresses) != 0 || len(rec.Card.SocialProfiles) != 0 {
		t.Errorf("expected no ImppAddresses/SocialProfiles, got %+v / %+v", rec.Card.ImppAddresses, rec.Card.SocialProfiles)
	}
}
