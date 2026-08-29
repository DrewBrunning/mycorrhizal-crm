package semanticequal

import (
	"encoding/json"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
)

func rec(card contactmodel.Card) *contactmodel.Record {
	return &contactmodel.Record{Card: card}
}

func ptrInt(v int) *int    { return &v }
func ptrBool(v bool) *bool { return &v }
func ts(s string) *contactmodel.Timestamp {
	return &contactmodel.Timestamp{UTC: s}
}

// equal reports the semantic result of Compare(a, b).
func equal(a, b *contactmodel.Record) bool { return Compare(a, b).Equal() }

// diffFor returns the Difference for one concept, or nil if none.
func diffFor(a, b *contactmodel.Record, concept string) *Difference {
	for _, d := range Compare(a, b).Differences {
		if d.Concept == concept {
			return &d
		}
	}
	return nil
}

func TestEqual_EmptyRecords(t *testing.T) {
	if !equal(rec(contactmodel.Card{}), rec(contactmodel.Card{})) {
		t.Error("two empty records must be semantically equal")
	}
	if !equal(nil, nil) {
		t.Error("two nil records must be semantically equal")
	}
	if !equal(nil, rec(contactmodel.Card{})) {
		t.Error("nil and empty record must be semantically equal")
	}
}

func TestEqual_OrderInsensitive_RepeatableProperties(t *testing.T) {
	// Reordering emails, phones, nicknames, keywords, components must not
	// change the semantic result — the comparison is a multiset per the
	// correspondence table's repeatable rows.
	a := rec(contactmodel.Card{
		Emails:    []contactmodel.Email{{Address: "a@example.com"}, {Address: "b@example.com"}},
		Phones:    []contactmodel.Phone{{Number: "+1"}, {Number: "+2"}},
		Nicknames: []contactmodel.Nickname{{Name: "Bobby"}, {Name: "The Accountant"}},
		Keywords:  []string{"math", "computing"},
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
			{Kind: "given", Value: "Bob"}, {Kind: "surname", Value: "Smith"},
		}},
		Organizations: []contactmodel.Organization{
			{Name: "One", Units: []contactmodel.OrgUnit{{Name: "A"}, {Name: "B"}}},
			{Name: "Two", Units: []contactmodel.OrgUnit{{Name: "C"}}},
		},
		Anniversaries: []contactmodel.Anniversary{
			{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(2005), Month: ptrInt(6), Day: ptrInt(18)}}},
			{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(1975), Month: ptrInt(11), Day: ptrInt(5)}}},
		},
		Titles: []contactmodel.Title{{Name: "Accountant", Kind: "title"}, {Name: "Treasurer", Kind: "role"}},
		RelatedTo: []contactmodel.Relation{
			{Target: "urn:uuid:1", Relations: []string{"friend"}},
			{Target: "urn:uuid:2", Relations: []string{"child", "parent"}},
		},
	})
	b := rec(contactmodel.Card{
		Emails:    []contactmodel.Email{{Address: "b@example.com"}, {Address: "a@example.com"}},
		Phones:    []contactmodel.Phone{{Number: "+2"}, {Number: "+1"}},
		Nicknames: []contactmodel.Nickname{{Name: "The Accountant"}, {Name: "Bobby"}},
		Keywords:  []string{"computing", "math"},
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
			{Kind: "surname", Value: "Smith"}, {Kind: "given", Value: "Bob"},
		}},
		Organizations: []contactmodel.Organization{
			{Name: "Two", Units: []contactmodel.OrgUnit{{Name: "C"}}},
			{Name: "One", Units: []contactmodel.OrgUnit{{Name: "B"}, {Name: "A"}}},
		},
		Anniversaries: []contactmodel.Anniversary{
			{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(1975), Month: ptrInt(11), Day: ptrInt(5)}}},
			{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(2005), Month: ptrInt(6), Day: ptrInt(18)}}},
		},
		Titles: []contactmodel.Title{{Name: "Treasurer", Kind: "role"}, {Name: "Accountant", Kind: "title"}},
		RelatedTo: []contactmodel.Relation{
			{Target: "urn:uuid:2", Relations: []string{"parent", "child"}},
			{Target: "urn:uuid:1", Relations: []string{"friend"}},
		},
	})
	if !equal(a, b) {
		t.Errorf("reordered repeatable properties must compare equal:\n%s", Compare(a, b).DiffText())
	}
}

func TestEqual_ReorderedAddressComponents(t *testing.T) {
	// The adr row compares the whole Address element; component order is not
	// semantic, so reordering components must not fail.
	addr := func(comps []contactmodel.AddressComponent) *contactmodel.Record {
		return rec(contactmodel.Card{Addresses: []contactmodel.Address{{Components: comps, CountryCode: "GB"}}})
	}
	ordered := addr([]contactmodel.AddressComponent{
		{Kind: "name", Value: "1 St James Square"},
		{Kind: "locality", Value: "London"},
		{Kind: "postcode", Value: "SW1Y 4JX"},
	})
	reordered := addr([]contactmodel.AddressComponent{
		{Kind: "postcode", Value: "SW1Y 4JX"},
		{Kind: "name", Value: "1 St James Square"},
		{Kind: "locality", Value: "London"},
	})
	if !equal(ordered, reordered) {
		t.Errorf("reordered address components must compare equal:\n%s", Compare(ordered, reordered).DiffText())
	}
}

func TestEqual_TimestampNormalization(t *testing.T) {
	// ts_rfc3339 transform: both sides normalize to the UTC instant, so a
	// different-but-equal instant representation must not fail the comparison.
	a := rec(contactmodel.Card{Created: ts("2026-01-01T00:00:00Z"), Updated: ts("2026-01-01T00:00:00Z")})
	b := rec(contactmodel.Card{Created: ts("2026-01-01T01:00:00+01:00"), Updated: ts("2026-01-01T01:00:00+01:00")})
	if !equal(a, b) {
		t.Errorf("same instant in different representations must compare equal:\n%s", Compare(a, b).DiffText())
	}

	// An unparseable timestamp passes through unchanged (preserved verbatim).
	c := rec(contactmodel.Card{Created: ts("not-a-timestamp")})
	if !equal(c, rec(contactmodel.Card{Created: ts("not-a-timestamp")})) {
		t.Error("unparseable timestamp must compare equal to itself")
	}
	if equal(c, rec(contactmodel.Card{})) {
		t.Error("present timestamp must differ from absent")
	}
}

func TestEqual_PartialDateForms(t *testing.T) {
	// date_partial: --MM-DD and the timestamp-vs-partial split are distinct
	// representational cases, but the same partial date compares equal.
	mk := func(y, m, d *int) *contactmodel.Record {
		return rec(contactmodel.Card{Anniversaries: []contactmodel.Anniversary{{
			Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: y, Month: m, Day: d}},
		}}})
	}
	a := mk(ptrInt(1815), ptrInt(12), ptrInt(10))
	b := mk(ptrInt(1815), ptrInt(12), ptrInt(10))
	if !equal(a, b) {
		t.Errorf("identical partial dates must compare equal:\n%s", Compare(a, b).DiffText())
	}
	// year-less leap day
	c := mk(nil, ptrInt(2), ptrInt(29))
	d := mk(nil, ptrInt(2), ptrInt(29))
	if !equal(c, d) {
		t.Errorf("identical year-less partial dates must compare equal:\n%s", Compare(c, d).DiffText())
	}
	// a full timestamp and a partial date are genuinely different cases
	tsRec := rec(contactmodel.Card{Anniversaries: []contactmodel.Anniversary{{
		Kind: "wedding", Date: contactmodel.AnniversaryDate{Timestamp: ptrStr("1835-07-08T00:00:00Z")},
	}}})
	if equal(tsRec, mk(ptrInt(1835), ptrInt(7), ptrInt(8))) {
		t.Error("timestamp date and partial date must not compare equal (they are different representational cases)")
	}
}

func ptrStr(s string) *string { return &s }

func TestEqual_GramGenderCaseInsensitive(t *testing.T) {
	// enum_lower transform: grammatical gender values compare case-insensitively.
	a := rec(contactmodel.Card{SpeakToAs: &contactmodel.SpeakToAs{
		GrammaticalGenders: []contactmodel.GrammaticalGender{{Value: "Feminine"}},
	}})
	b := rec(contactmodel.Card{SpeakToAs: &contactmodel.SpeakToAs{
		GrammaticalGenders: []contactmodel.GrammaticalGender{{Value: "feminine"}},
	}})
	if !equal(a, b) {
		t.Errorf("grammatical gender must compare case-insensitively (enum_lower):\n%s", Compare(a, b).DiffText())
	}
}

func TestEqual_NamePhoneticPair(t *testing.T) {
	// name.phonetic compares PhoneticScript + PhoneticSystem as one key.
	a := rec(contactmodel.Card{Name: &contactmodel.Name{PhoneticScript: "latin", PhoneticSystem: "latn"}})
	b := rec(contactmodel.Card{Name: &contactmodel.Name{PhoneticScript: "latin", PhoneticSystem: "latn"}})
	if !equal(a, b) {
		t.Error("identical phonetic script/system must compare equal")
	}
	// differing script is a real change
	if d := diffFor(a, rec(contactmodel.Card{Name: &contactmodel.Name{PhoneticScript: "hiragana", PhoneticSystem: "latn"}}), "name.phonetic"); d == nil {
		t.Error("differing phonetic script must be reported")
	}
	// system alone (no script) still compares
	c := rec(contactmodel.Card{Name: &contactmodel.Name{PhoneticSystem: "latn"}})
	if !equal(c, rec(contactmodel.Card{Name: &contactmodel.Name{PhoneticSystem: "latn"}})) {
		t.Error("phonetic system alone must compare equal")
	}
}

func TestEqual_OrgElementAndUnits(t *testing.T) {
	// org compares name + sorted units; org.unit compares the unit-name multiset.
	a := rec(contactmodel.Card{Organizations: []contactmodel.Organization{{
		Name: "Royal Society", Units: []contactmodel.OrgUnit{{Name: "Mathematics"}, {Name: "Computing"}},
	}}})
	b := rec(contactmodel.Card{Organizations: []contactmodel.Organization{{
		Name: "Royal Society", Units: []contactmodel.OrgUnit{{Name: "Computing"}, {Name: "Mathematics"}},
	}}})
	if !equal(a, b) {
		t.Errorf("org with reordered units must compare equal:\n%s", Compare(a, b).DiffText())
	}
	if d := diffFor(a, rec(contactmodel.Card{Organizations: []contactmodel.Organization{{Name: "Royal Society", Units: []contactmodel.OrgUnit{{Name: "Physics"}}}}}), "org.unit"); d == nil {
		t.Error("a changed unit must be reported as an org.unit difference")
	}
	if d := diffFor(a, rec(contactmodel.Card{Organizations: []contactmodel.Organization{{Name: "Other Society", Units: []contactmodel.OrgUnit{{Name: "Mathematics"}}}}}), "org"); d == nil {
		t.Error("a changed org name must be reported")
	}
}

func TestEqual_OnlineServiceElement(t *testing.T) {
	// impp/social compare service + user + uri as one element key.
	a := rec(contactmodel.Card{SocialProfiles: []contactmodel.OnlineService{{
		Service: "Mastodon", User: "@ada@fosstodon.org", URI: "https://fosstodon.org/@ada",
	}}})
	b := rec(contactmodel.Card{SocialProfiles: []contactmodel.OnlineService{{
		Service: "Mastodon", User: "@ada@fosstodon.org", URI: "https://fosstodon.org/@ada",
	}}})
	if !equal(a, b) {
		t.Error("identical social profiles must compare equal")
	}
	// losing the user handle is a real change
	if d := diffFor(a, rec(contactmodel.Card{SocialProfiles: []contactmodel.OnlineService{{
		Service: "Mastodon", URI: "https://fosstodon.org/@ada",
	}}}), "social"); d == nil {
		t.Error("a lost social-profile user handle must be reported")
	}
	// impp and social are separate arrays: moving an entry between them is a
	// real change on both concepts.
	impp := rec(contactmodel.Card{ImppAddresses: []contactmodel.OnlineService{{Service: "matrix", URI: "https://matrix.to/#/@ada"}}})
	social := rec(contactmodel.Card{SocialProfiles: []contactmodel.OnlineService{{Service: "matrix", URI: "https://matrix.to/#/@ada"}}})
	if equal(impp, social) {
		t.Error("impp and social profiles are different concepts; moving between them must differ")
	}
}

func TestEqual_AddressElementGranularity(t *testing.T) {
	a := rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components:  []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		CountryCode: "GB",
		TimeZone:    "Europe/London",
	}}})
	b := rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components:  []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		CountryCode: "GB",
		TimeZone:    "Europe/London",
	}}})
	if !equal(a, b) {
		t.Error("identical addresses must compare equal")
	}
	if d := diffFor(a, rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components:  []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		CountryCode: "GB",
	}}}), "adr.tz"); d == nil {
		t.Error("a lost timezone must be reported as adr.tz (and the whole-element adr)")
	}
	// adr.geo / adr.tz are separate rows from adr.
	if d := diffFor(a, rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components:  []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		CountryCode: "GB",
		TimeZone:    "Europe/London",
		Coordinates: "geo:1,2",
	}}}), "adr.geo"); d == nil {
		t.Error("an added coordinate must be reported as adr.geo")
	}
}

func TestEqual_AnniversaryKindAndPlace(t *testing.T) {
	a := rec(contactmodel.Card{Anniversaries: []contactmodel.Anniversary{{
		Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(1815)}},
		Place: &contactmodel.Address{Full: "London, UK"},
	}}})
	if d := diffFor(a, rec(contactmodel.Card{Anniversaries: []contactmodel.Anniversary{{
		Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(1815)}},
	}}}), "anniversary.place.birth"); d == nil {
		t.Error("a lost anniversary place must be reported")
	}
	// kind discriminates: a wedding anniversary and a birth anniversary are
	// different concepts.
	wedding := rec(contactmodel.Card{Anniversaries: []contactmodel.Anniversary{{
		Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(1815)}},
	}}})
	if equal(a, wedding) {
		t.Error("birth and wedding anniversaries are different concepts")
	}
}

func TestEqual_PersonalInfoElement(t *testing.T) {
	a := rec(contactmodel.Card{PersonalInfo: []contactmodel.PersonalInfo{{Kind: "expertise", Value: "Go", Level: "high", ListAs: ptrInt(2)}}})
	b := rec(contactmodel.Card{PersonalInfo: []contactmodel.PersonalInfo{{Kind: "expertise", Value: "Go", Level: "high", ListAs: ptrInt(2)}}})
	if !equal(a, b) {
		t.Error("identical personal info must compare equal")
	}
	if d := diffFor(a, rec(contactmodel.Card{PersonalInfo: []contactmodel.PersonalInfo{{Kind: "expertise", Value: "Go", Level: "medium", ListAs: ptrInt(2)}}}), "expertise"); d == nil {
		t.Error("a changed level must be reported")
	}
	if d := diffFor(a, rec(contactmodel.Card{PersonalInfo: []contactmodel.PersonalInfo{{Kind: "hobby", Value: "Go", Level: "high", ListAs: ptrInt(2)}}}), "expertise"); d == nil {
		t.Error("a kind change must be reported (hobby row, not expertise)")
	}
}

func TestEqual_NoteParams(t *testing.T) {
	a := rec(contactmodel.Card{Notes: []contactmodel.Note{{
		Note: "hi", Author: &contactmodel.Author{Name: "Bob", URI: "mailto:bob@example.com"}, Created: ts("2026-01-01T00:00:00Z"),
	}}})
	b := rec(contactmodel.Card{Notes: []contactmodel.Note{{
		Note: "hi", Author: &contactmodel.Author{Name: "Bob", URI: "mailto:bob@example.com"}, Created: ts("2026-01-01T00:00:00Z"),
	}}})
	if !equal(a, b) {
		t.Error("identical notes with params must compare equal")
	}
	if d := diffFor(a, rec(contactmodel.Card{Notes: []contactmodel.Note{{Note: "hi"}}}), "note"); d == nil {
		t.Error("a lost note author/created must be reported (note params are part of the note row)")
	}
}

func TestEqual_RelatedAndMembers(t *testing.T) {
	a := rec(contactmodel.Card{
		RelatedTo: []contactmodel.Relation{{Target: "urn:uuid:1", Relations: []string{"friend", "acquaintance"}}},
		Members:   []string{"urn:uuid:2"},
	})
	if d := diffFor(a, rec(contactmodel.Card{RelatedTo: []contactmodel.Relation{{Target: "urn:uuid:1", Relations: []string{"friend"}}}}), "related"); d == nil {
		t.Error("a lost relation tag must be reported")
	}
	if d := diffFor(a, rec(contactmodel.Card{RelatedTo: []contactmodel.Relation{{Target: "urn:uuid:1", Relations: []string{"friend", "acquaintance"}}}}), "member"); d == nil {
		t.Error("a lost member must be reported")
	}
}

func TestEqual_MediaAndResourcesByKind(t *testing.T) {
	a := rec(contactmodel.Card{
		Media:        []contactmodel.Resource{{Kind: "photo", URI: "data:image/png;base64,AAA"}, {Kind: "logo", URI: "https://example.com/logo"}},
		Calendars:    []contactmodel.Resource{{URI: "https://cal.example.com/1"}},
		FreeBusyURLs: []contactmodel.Resource{{URI: "https://cal.example.com/free"}},
		Directories:  []contactmodel.Resource{{Kind: "entry", URI: "https://example.com/vcf"}, {Kind: "directory", URI: "https://dir.example.com/1"}},
		Links:        []contactmodel.Resource{{URI: "https://example.com"}},
		ContactURIs:  []contactmodel.Resource{{URI: "https://contact.example.com/me"}},
	})
	if d := diffFor(a, rec(contactmodel.Card{
		Media:        []contactmodel.Resource{{Kind: "photo", URI: "data:image/png;base64,AAA"}},
		Calendars:    []contactmodel.Resource{{URI: "https://cal.example.com/1"}},
		FreeBusyURLs: []contactmodel.Resource{{URI: "https://cal.example.com/free"}},
		Directories:  []contactmodel.Resource{{Kind: "entry", URI: "https://example.com/vcf"}},
		Links:        []contactmodel.Resource{{URI: "https://example.com"}},
		ContactURIs:  []contactmodel.Resource{{URI: "https://contact.example.com/me"}},
	}), "logo"); d == nil {
		t.Error("a lost logo must be reported")
	}
	if d := diffFor(a, rec(contactmodel.Card{
		Media:        []contactmodel.Resource{{Kind: "photo", URI: "data:image/png;base64,AAA"}, {Kind: "logo", URI: "https://example.com/logo"}},
		Calendars:    []contactmodel.Resource{{URI: "https://cal.example.com/1"}},
		FreeBusyURLs: []contactmodel.Resource{{URI: "https://cal.example.com/free"}},
		Directories:  []contactmodel.Resource{{Kind: "directory", URI: "https://dir.example.com/1"}},
		Links:        []contactmodel.Resource{{URI: "https://example.com"}},
		ContactURIs:  []contactmodel.Resource{{URI: "https://contact.example.com/me"}},
	}), "source"); d == nil {
		t.Error("a lost SOURCE (entry-kind directory) must be reported")
	}
}

func TestEqual_PassthroughVCard(t *testing.T) {
	// pt.vcard compares the vCardProps array; param-value spelling differences
	// (single string vs single-element slice) do not fail the comparison.
	a := &contactmodel.Record{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{{
		Name: "X-CUSTOM-ADA", Params: map[string]any{"type": []string{"custom"}}, Type: "text", Value: jsonRaw(`"keep-me"`),
	}}}}
	b := &contactmodel.Record{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{{
		Name: "x-custom-ada", Params: map[string]any{"type": "custom"}, Type: "text", Value: jsonRaw(`"keep-me"`),
	}}}}
	if !equal(a, b) {
		t.Errorf("passthrough vCardProps must compare case/param-shape-insensitively:\n%s", Compare(a, b).DiffText())
	}
	// a changed value is a real change
	if d := diffFor(a, &contactmodel.Record{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{{
		Name: "X-CUSTOM-ADA", Params: map[string]any{"type": "custom"}, Type: "text", Value: jsonRaw(`"changed"`),
	}}}}, "pt.vcard"); d == nil {
		t.Error("a changed passthrough value must be reported")
	}
	// a dropped entry is a real change
	if d := diffFor(a, &contactmodel.Record{}, "pt.vcard"); d == nil {
		t.Error("a dropped passthrough entry must be reported")
	}
}

func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }

func TestEqual_PassthroughJSContact(t *testing.T) {
	a := &contactmodel.Record{Passthrough: contactmodel.Passthrough{JSContact: map[string]json.RawMessage{
		"/x-vendor": jsonRaw(`{"a": 1}`),
	}}}
	b := &contactmodel.Record{Passthrough: contactmodel.Passthrough{JSContact: map[string]json.RawMessage{
		"/x-vendor": jsonRaw(`{"a": 1}`),
	}}}
	if !equal(a, b) {
		t.Error("identical JSContact passthrough must compare equal")
	}
	if d := diffFor(a, &contactmodel.Record{}, "pt.jscontact"); d == nil {
		t.Error("a dropped JSContact passthrough entry must be reported")
	}
}

func TestDiffClassified_BySide(t *testing.T) {
	a := rec(contactmodel.Card{Emails: []contactmodel.Email{{Address: "a@example.com"}}})
	empty := rec(contactmodel.Card{})

	d := diffFor(a, empty, "email")
	if d == nil || !d.PresentA || d.PresentB {
		t.Errorf("loss must be PresentA-only, got %+v", d)
	}

	d = diffFor(empty, a, "email")
	if d == nil || d.PresentA || !d.PresentB {
		t.Errorf("gain must be PresentB-only, got %+v", d)
	}

	other := rec(contactmodel.Card{Emails: []contactmodel.Email{{Address: "b@example.com"}}})
	d = diffFor(a, other, "email")
	if d == nil || !d.PresentA || !d.PresentB {
		t.Errorf("change must be PresentA-and-PresentB, got %+v", d)
	}
}

func TestDiffText_EmptyWhenEqual(t *testing.T) {
	if got := Compare(rec(contactmodel.Card{}), rec(contactmodel.Card{})).DiffText(); got != "" {
		t.Errorf("DiffText of equal records must be empty, got %q", got)
	}
	text := Compare(rec(contactmodel.Card{Emails: []contactmodel.Email{{Address: "a@example.com"}}}), rec(contactmodel.Card{})).DiffText()
	if !strings.Contains(text, "email") {
		t.Errorf("DiffText must name the concept, got %q", text)
	}
}

func TestDiffString_RendersEachKind(t *testing.T) {
	rich := rec(contactmodel.Card{Emails: []contactmodel.Email{{Address: "a@example.com"}}})

	// loss: present only in the original record
	loss := Compare(rich, rec(contactmodel.Card{})).Differences
	if len(loss) == 0 {
		t.Fatal("expected a loss difference")
	}
	if !strings.Contains(loss[0].String(), "present only in the original record") {
		t.Errorf("loss rendering = %q, want 'present only in the original record'", loss[0].String())
	}

	// gain: present only in the round-tripped record
	gain := Compare(rec(contactmodel.Card{}), rich).Differences
	if len(gain) == 0 {
		t.Fatal("expected a gain difference")
	}
	if !strings.Contains(gain[0].String(), "present only in the round-tripped record") {
		t.Errorf("gain rendering = %q, want 'present only in the round-tripped record'", gain[0].String())
	}

	// changed: present on both sides, values differ
	other := rec(contactmodel.Card{Emails: []contactmodel.Email{{Address: "b@example.com"}}})
	changed := Compare(rich, other).Differences
	if len(changed) == 0 {
		t.Fatal("expected a changed difference")
	}
	if !strings.Contains(changed[0].String(), "values differ") {
		t.Errorf("changed rendering = %q, want 'values differ'", changed[0].String())
	}
}

func TestEqual_PreferredLanguages(t *testing.T) {
	// the lang row compares PreferredLanguages[].Language.
	a := rec(contactmodel.Card{PreferredLanguages: []contactmodel.LanguagePref{{Language: "en"}, {Language: "de"}}})
	b := rec(contactmodel.Card{PreferredLanguages: []contactmodel.LanguagePref{{Language: "de"}, {Language: "en"}}})
	if !equal(a, b) {
		t.Error("reordered preferred languages must compare equal")
	}
	if d := diffFor(a, rec(contactmodel.Card{PreferredLanguages: []contactmodel.LanguagePref{{Language: "en"}}}), "lang"); d == nil {
		t.Error("a dropped preferred language must be reported")
	}
}

func TestEqual_AddressContextsAndFull(t *testing.T) {
	// addressKey includes the contexts, pref, isOrdered and full (LABEL)
	// fields of the whole Address element.
	a := rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		Contexts:   []string{"work", "private"},
		Pref:       ptrInt(1),
		IsOrdered:  ptrBool(true),
		Full:       "1 St James Square, London",
	}}})
	b := rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		Contexts:   []string{"private", "work"},
		Pref:       ptrInt(1),
		IsOrdered:  ptrBool(true),
		Full:       "1 St James Square, London",
	}}})
	if !equal(a, b) {
		t.Errorf("reordered address contexts must compare equal:\n%s", Compare(a, b).DiffText())
	}
	if d := diffFor(a, rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		Contexts:   []string{"work"},
		Pref:       ptrInt(1),
		IsOrdered:  ptrBool(true),
		Full:       "1 St James Square, London",
	}}}), "adr"); d == nil {
		t.Error("a dropped address context must be reported")
	}
	if d := diffFor(a, rec(contactmodel.Card{Addresses: []contactmodel.Address{{
		Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
		Contexts:   []string{"work", "private"},
		Pref:       ptrInt(1),
		IsOrdered:  ptrBool(true),
		Full:       "",
	}}}), "adr"); d == nil {
		t.Error("a dropped address full/label must be reported")
	}
}

func TestEqual_EmptyAnniversaryDate(t *testing.T) {
	// An anniversary carrying no date at all (e.g. a place-only entry)
	// contributes nothing to the comparison on either side.
	a := rec(contactmodel.Card{Anniversaries: []contactmodel.Anniversary{{Kind: "birth", Date: contactmodel.AnniversaryDate{}}}})
	if !equal(a, rec(contactmodel.Card{Anniversaries: []contactmodel.Anniversary{{Kind: "birth", Date: contactmodel.AnniversaryDate{}}}})) {
		t.Error("identical empty anniversary dates must compare equal")
	}
}

func TestEqual_PassthroughParamVariants(t *testing.T) {
	// paramValueKey: []any values, []any with non-string items, and a
	// non-string scalar (int) all canonicalize.
	a := &contactmodel.Record{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{{
		Name: "X-A", Params: map[string]any{"list": []any{"b", "a"}}, Type: "text", Value: jsonRaw(`"v"`),
	}}}}
	b := &contactmodel.Record{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{{
		Name: "X-A", Params: map[string]any{"list": []any{"a", "b"}}, Type: "text", Value: jsonRaw(`"v"`),
	}}}}
	if !equal(a, b) {
		t.Errorf("[]any param values must compare order-insensitively:\n%s", Compare(a, b).DiffText())
	}
	if d := diffFor(a, &contactmodel.Record{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{{
		Name: "X-A", Params: map[string]any{"list": []any{1, 2}}, Type: "text", Value: jsonRaw(`"v"`),
	}}}}, "pt.vcard"); d == nil {
		t.Error("a changed []any param value must be reported (non-string items canonicalize via JSON)")
	}
	if d := diffFor(a, &contactmodel.Record{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{{
		Name: "X-A", Params: map[string]any{"list": 1}, Type: "text", Value: jsonRaw(`"v"`),
	}}}}, "pt.vcard"); d == nil {
		t.Error("a changed scalar (int) param value must be reported (default branch)")
	}
}

// TestCompareCoversEveryCorrespondenceRow pins the ADR-0002 lock: the
// comparer has an extractor for every row the correspondence table declares,
// and every extractor is backed by a row. init() enforces this by panicking
// on drift, so this test passing is the "no silent concept" guarantee — it
// also doubles as the compile-time check that the table is still loadable.
func TestCompareCoversEveryCorrespondenceRow(t *testing.T) {
	rows := correspondence.Load()
	if len(rows) == 0 {
		t.Fatal("correspondence table must load")
	}
	if len(byConcept) != len(rows) {
		t.Errorf("comparer has %d extractors for %d table rows (init would have panicked on drift — this is a canary)", len(byConcept), len(rows))
	}
	for _, row := range rows {
		if _, ok := byConcept[row.ConceptID]; !ok {
			t.Errorf("concept %q has no extractor", row.ConceptID)
		}
	}
}

// TestCompare_NilSafety guards the comparison against nil records in either
// slot (extractors must read through nil pointers defensively).
func TestCompare_NilSafety(t *testing.T) {
	rich := rec(contactmodel.Card{
		Emails:        []contactmodel.Email{{Address: "a@example.com"}},
		Name:          &contactmodel.Name{Full: "Ada", Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		SpeakToAs:     &contactmodel.SpeakToAs{Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}}},
		Anniversaries: []contactmodel.Anniversary{{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: ptrInt(1815)}}}},
	})
	if equal(rich, nil) {
		t.Error("a rich record and nil must not compare equal (losses must be reported)")
	}
	if !Compare(nil, nil).Equal() {
		t.Error("nil vs nil must be equal")
	}
}
