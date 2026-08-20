package models

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"mycorrhizal/contactmodel"
)

// contact_card_merge_property_test.go: property-based (randomized, seeded)
// tests pinning contact_card_merge.go's T75 contract (issue #255) — the
// years-long silent data-drop bug (CLAUDE.md backend trap #3) was exactly a
// violation of "Card members with no flat representation at all are
// preserved unconditionally". A table-driven example test can only prove
// the fix for whichever fields it happens to enumerate; these generate many
// random Contacts/Records per run so the invariant is checked across the
// whole lossless-field set, not a hand-picked sample.
//
// Each property seeds its own *rand.Rand from a fixed constant so a red run
// is reproducible without needing -run/-seed plumbing: the seed is part of
// the source, and t.Logf below prints the iteration on failure.

const mergePropertyIterations = 200

// randPropertyContact builds a random, self-consistent flat Contact: no
// validation is applied (this exercises mergeRecordFromFlat directly, not
// the HTTP/DB layer), but every generated value is realistic enough that
// RecordFromContact's build* functions populate the Card fields they map to.
func randPropertyContact(r *rand.Rand, tag string) *Contact {
	c := &Contact{
		Firstname:  randPropertyString(r, tag+"-first"),
		Lastname:   randPropertyString(r, tag+"-last"),
		Nickname:   randPropertyString(r, tag+"-nick"),
		MiddleName: randPropertyString(r, tag+"-mid"),
		Prefix:     randPropertyString(r, tag+"-prefix"),
		Suffix:     randPropertyString(r, tag+"-suffix"),

		Organization: randPropertyString(r, tag+"-org"),
		Department:   randPropertyString(r, tag+"-dept"),
		JobTitle:     randPropertyString(r, tag+"-title"),
		Role:         randPropertyString(r, tag+"-role"),

		HowWeMet:           randPropertyString(r, tag+"-howwemet"),
		WorkInformation:    randPropertyString(r, tag+"-workinfo"),
		ContactInformation: randPropertyString(r, tag+"-contactinfo"),
		Circles:            []string{randPropertyString(r, tag+"-circle")},
	}
	for i := 0; i < 1+r.Intn(3); i++ {
		c.Emails = append(c.Emails, ContactEmail{
			Type:  randPropertyLabel(r),
			Value: fmt.Sprintf("%s-%d@example.test", tag, r.Intn(1_000_000)),
		})
	}
	for i := 0; i < r.Intn(3); i++ {
		c.Phones = append(c.Phones, ContactPhone{
			Type:  randPropertyLabel(r),
			Value: fmt.Sprintf("+1-555-%04d", r.Intn(10_000)),
		})
	}
	for i := 0; i < r.Intn(3); i++ {
		c.URLs = append(c.URLs, ContactURL{
			Type:  randPropertyLabel(r),
			Value: fmt.Sprintf("https://%s-%d.example.test", tag, r.Intn(1_000_000)),
		})
	}
	for i := 0; i < r.Intn(3); i++ {
		c.IMPPs = append(c.IMPPs, ContactIMPP{
			Type:  randPropertyLabel(r),
			Value: fmt.Sprintf("xmpp:%s-%d@example.test", tag, r.Intn(1_000_000)),
		})
	}
	for i := 0; i < r.Intn(3); i++ {
		c.Addresses = append(c.Addresses, ContactAddress{
			Type:   randPropertyLabel(r),
			Street: randPropertyString(r, tag+"-street"),
			City:   randPropertyString(r, tag+"-city"),
			Region: randPropertyString(r, tag+"-region"),
			Postal: randPropertyString(r, tag+"-postal"),
		})
	}
	return c
}

func randPropertyString(r *rand.Rand, tag string) string {
	return fmt.Sprintf("%s-%d", tag, r.Intn(1_000_000))
}

func randPropertyLabel(r *rand.Rand) string {
	labels := []string{"home", "work", "other"}
	return labels[r.Intn(len(labels))]
}

// injectLosslessOnlyFields sets every Card/Envelope/Passthrough member that
// contact_card_merge.go's doc comment lists as having "no flat
// representation at all" — SpeakToAs, PersonalInfo, Notes, Keywords,
// SocialProfiles, OtherOnlineServices, Members, RelatedTo, Card.Kind,
// Envelope.Kind — to random, distinguishable values. Passthrough is left to
// the caller: RecordFromContact only ever derives an empty Passthrough for a
// Contact with no VCardExtra, so mergeRecordFromFlat's "preserved when fresh
// has none" branch is exercised automatically as long as the generated
// Contact's VCardExtra stays "" (randPropertyContact never sets it).
func injectLosslessOnlyFields(r *rand.Rand, rec *contactmodel.Record, tag string) {
	rec.Card.Kind = "individual"
	rec.Card.SpeakToAs = &contactmodel.SpeakToAs{
		Pronouns: []contactmodel.Pronouns{{Pronouns: randPropertyString(r, tag+"-pronouns")}},
	}
	rec.Card.PersonalInfo = []contactmodel.PersonalInfo{
		{Kind: "hobby", Value: randPropertyString(r, tag+"-hobby")},
	}
	rec.Card.Notes = []contactmodel.Note{
		{Note: randPropertyString(r, tag+"-note")},
	}
	rec.Card.Keywords = []string{randPropertyString(r, tag+"-keyword")}
	rec.Card.SocialProfiles = []contactmodel.OnlineService{
		{Service: "mastodon", URI: randPropertyString(r, tag+"-social")},
	}
	rec.Card.OtherOnlineServices = []contactmodel.OnlineService{
		{Service: "other", URI: randPropertyString(r, tag+"-other-service")},
	}
	rec.Card.Members = []string{randPropertyString(r, tag+"-member")}
	rec.Card.RelatedTo = []contactmodel.Relation{
		{Target: randPropertyString(r, tag+"-related"), Relations: []string{"friend"}},
	}
	rec.Envelope.Kind = "human"
}

// losslessOnlyFieldsEqual reports whether every field
// injectLosslessOnlyFields sets is deep-equal between got and want. Used
// instead of a whole-Record reflect.DeepEqual in the "after a flat edit"
// property, where the flat-derived fields are expected to differ.
func losslessOnlyFieldsEqual(t *testing.T, got, want *contactmodel.Record) bool {
	t.Helper()
	ok := true
	check := func(name string, g, w any) {
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s = %+v, want %+v (lossless-only field must survive unconditionally)", name, g, w)
			ok = false
		}
	}
	check("Card.Kind", got.Card.Kind, want.Card.Kind)
	check("Card.SpeakToAs", got.Card.SpeakToAs, want.Card.SpeakToAs)
	check("Card.PersonalInfo", got.Card.PersonalInfo, want.Card.PersonalInfo)
	check("Card.Notes", got.Card.Notes, want.Card.Notes)
	check("Card.Keywords", got.Card.Keywords, want.Card.Keywords)
	check("Card.SocialProfiles", got.Card.SocialProfiles, want.Card.SocialProfiles)
	check("Card.OtherOnlineServices", got.Card.OtherOnlineServices, want.Card.OtherOnlineServices)
	check("Card.Members", got.Card.Members, want.Card.Members)
	check("Card.RelatedTo", got.Card.RelatedTo, want.Card.RelatedTo)
	check("Envelope.Kind", got.Envelope.Kind, want.Envelope.Kind)
	check("Passthrough", got.Passthrough, want.Passthrough)
	return ok
}

// canonicalRecordJSON marshals rec the same way it's actually persisted
// (Contact.Card/CRM/Passthrough are `serializer:json` GORM columns) and
// served (REST/CardDAV/export), so two records that marshal identically hold
// the same data even if they differ in incidental Go representation — e.g.
// mergeMedia/mergeProjectedArray always allocate a non-nil `make([]T, ...)`
// result even when empty, while a flat field that was never populated stays
// a nil slice; both encode identically under `omitempty`, so raw
// reflect.DeepEqual would flag a false positive that carries no actual data
// loss.
func canonicalRecordJSON(t *testing.T, rec *contactmodel.Record) string {
	t.Helper()
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// TestMergeRecordFromFlat_PreservesLosslessFields_NoOpSave is Property A: a
// plain save of a Contact that was never edited (fresh re-derived from the
// exact same flat fields) must return the loaded Record completely
// unchanged. This is the literal T75 regression: before the fix, BeforeSave
// did `c.Card = RecordFromContact(c, photoDir).Card` unconditionally, which
// would have zeroed every field injectLosslessOnlyFields sets on every
// single save.
func TestMergeRecordFromFlat_PreservesLosslessFields_NoOpSave(t *testing.T) {
	for i := 0; i < mergePropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(1000 + i)))
		tag := fmt.Sprintf("noop-%d", i)

		contact := randPropertyContact(r, tag)
		fresh := RecordFromContact(contact, "")
		loaded := *fresh
		injectLosslessOnlyFields(r, &loaded, tag)

		freshAgain := RecordFromContact(contact, "") // same contact, unedited
		merged := mergeRecordFromFlat(loaded, *freshAgain)

		if got, want := canonicalRecordJSON(t, &merged), canonicalRecordJSON(t, &loaded); got != want {
			t.Fatalf("iteration %d (seed %d): mergeRecordFromFlat(loaded, fresh-from-unedited-contact) != loaded\ngot:  %s\nwant: %s",
				i, 1000+i, got, want)
		}
	}
}

// TestMergeRecordFromFlat_PreservesLosslessFields_AfterFlatEdit is Property
// B: even when the flat fields genuinely changed (a real edit, not a no-op
// save), every lossless-only field must still survive unconditionally, and
// the flat-owned Envelope scalars (which contact_card_merge.go always takes
// from fresh, no dirty check) must track the edited contact exactly.
func TestMergeRecordFromFlat_PreservesLosslessFields_AfterFlatEdit(t *testing.T) {
	for i := 0; i < mergePropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(2000 + i)))
		tag := fmt.Sprintf("edit-%d", i)

		contact1 := randPropertyContact(r, tag+"-before")
		fresh1 := RecordFromContact(contact1, "")
		loaded := *fresh1
		injectLosslessOnlyFields(r, &loaded, tag)

		contact2 := randPropertyContact(r, tag+"-after") // fully independent random edit
		contact2.VCardUID = contact1.VCardUID
		fresh2 := RecordFromContact(contact2, "")

		merged := mergeRecordFromFlat(loaded, *fresh2)

		if !losslessOnlyFieldsEqual(t, &merged, &loaded) {
			t.Fatalf("iteration %d (seed %d): lossless-only fields did not survive a flat edit", i, 2000+i)
		}
		if !reflect.DeepEqual(merged.Envelope.Circles, contact2.Circles) ||
			merged.Envelope.HowWeMet != contact2.HowWeMet ||
			merged.Envelope.WorkInformation != contact2.WorkInformation ||
			merged.Envelope.ContactInformation != contact2.ContactInformation {
			t.Fatalf("iteration %d (seed %d): flat-owned Envelope scalars = %+v, want to match the edited contact %+v",
				i, 2000+i, merged.Envelope, contact2)
		}
	}
}

// TestMergeRecordFromFlat_ProjectedArrayEntry_SurvivesUntouched_Emails is
// Property C, targeting mergeProjectedArray's documented per-entry rule
// directly: an email entry's rich metadata (Pref — no flat-field home)
// survives a save that doesn't touch that entry's flat projection
// (Type/Value), and is correctly dropped (fresh wins) when the caller does
// edit that entry's flat form.
func TestMergeRecordFromFlat_ProjectedArrayEntry_SurvivesUntouched_Emails(t *testing.T) {
	for i := 0; i < mergePropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(3000 + i)))
		tag := fmt.Sprintf("email-%d", i)

		contact1 := randPropertyContact(r, tag)
		fresh1 := RecordFromContact(contact1, "")
		if len(fresh1.Card.Emails) == 0 {
			t.Fatalf("iteration %d: randPropertyContact must always generate at least one email", i)
		}
		loaded := *fresh1
		loadedEmails := append([]contactmodel.Email(nil), fresh1.Card.Emails...)
		pref := r.Intn(10)
		loadedEmails[0].Pref = &pref
		loaded.Card.Emails = loadedEmails

		t.Run("untouched entry survives whole", func(t *testing.T) {
			freshSame := RecordFromContact(contact1, "") // identical contact: entry 0 untouched
			merged := mergeRecordFromFlat(loaded, *freshSame)
			if len(merged.Card.Emails) == 0 || merged.Card.Emails[0].Pref == nil || *merged.Card.Emails[0].Pref != pref {
				t.Fatalf("iteration %d (seed %d): untouched email entry lost its Pref metadata: got %+v, want Pref=%d",
					i, 3000+i, merged.Card.Emails, pref)
			}
		})

		t.Run("edited entry loses rich metadata, gains the new value", func(t *testing.T) {
			contact2 := deepCopyContactForEmailEdit(contact1)
			contact2.Emails[0].Value = fmt.Sprintf("changed-%s-%d@example.test", tag, r.Intn(1_000_000))
			freshChanged := RecordFromContact(contact2, "")
			merged := mergeRecordFromFlat(loaded, *freshChanged)
			if len(merged.Card.Emails) == 0 {
				t.Fatalf("iteration %d (seed %d): merged Emails is empty", i, 3000+i)
			}
			if merged.Card.Emails[0].Pref != nil {
				t.Fatalf("iteration %d (seed %d): edited email entry kept stale Pref=%v, want nil (fresh entry should win)",
					i, 3000+i, *merged.Card.Emails[0].Pref)
			}
			if merged.Card.Emails[0].Address != contact2.Emails[0].Value {
				t.Fatalf("iteration %d (seed %d): edited email entry Address = %q, want %q",
					i, 3000+i, merged.Card.Emails[0].Address, contact2.Emails[0].Value)
			}
		})
	}
}

// deepCopyContactForEmailEdit copies just enough of Contact for the
// email-edit sub-test to mutate contact2.Emails without aliasing contact1's
// backing array.
func deepCopyContactForEmailEdit(c *Contact) *Contact {
	cp := *c
	cp.Emails = append([]ContactEmail(nil), c.Emails...)
	return &cp
}
