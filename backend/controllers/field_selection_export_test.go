package controllers

import (
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/jscontact"
	"mycorrhizal/models"
	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// selectiveExportFullRecord returns a Record with every picker-covered
// section populated, so the cross-format test can prove the SAME selection
// omits the same sections through all three exporters.
func selectiveExportFullRecord() *contactmodel.Record {
	return &contactmodel.Record{
		UID:  "rec-uid",
		ETag: `"etag-1"`,
		Card: contactmodel.Card{
			UID:  "card-uid",
			Kind: "group",
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Alice"}, {Kind: "surname", Value: "Anderson"}}},
			Emails: []contactmodel.Email{
				{Address: "alice@example.com", Contexts: []string{"work"}, Pref: intPtr(1)},
			},
			Phones: []contactmodel.Phone{
				{Number: "555-0100", Features: []string{"voice"}},
			},
			Addresses: []contactmodel.Address{
				{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "123 Main St"}}, Full: "123 Main St", Contexts: []string{"home"}},
			},
			Organizations:      []contactmodel.Organization{{Name: "Acme"}},
			Titles:             []contactmodel.Title{{Name: "Engineer", Kind: "title"}},
			Anniversaries:      []contactmodel.Anniversary{{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: intPtr(1990), Month: intPtr(5), Day: intPtr(10)}}}},
			Media:              []contactmodel.Resource{{Kind: "photo", URI: "data:image/png;base64,abc"}},
			ImppAddresses:      []contactmodel.OnlineService{{Service: "telegram", URI: "tg:@alice"}},
			Links:              []contactmodel.Resource{{URI: "https://example.com"}},
			Notes:              []contactmodel.Note{{Note: "met at conference"}},
			Keywords:           []string{"vegan"},
			RelatedTo:          []contactmodel.Relation{{Target: "urn:uuid:bob", Relations: []string{"spouse"}}},
			PersonalInfo:       []contactmodel.PersonalInfo{{Kind: "hobby", Value: "climbing"}},
			SpeakToAs:          &contactmodel.SpeakToAs{Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}}},
			Members:            []string{"urn:uuid:m1"},
			PreferredLanguages: []contactmodel.LanguagePref{{Language: "en"}},
		},
		Passthrough: contactmodel.Passthrough{
			VCard: []contactmodel.JCardProp{{Name: "X-CUSTOM", Type: "text", Value: json.RawMessage(`"custom-value"`)}},
		},
	}
}

// vcard4SectionMarkers maps each section to the vCard 4.0 property name that
// carries it. vcard3SectionMarkers is the subset vCard 3.0 can represent
// (v3 has no HOBBY/PRONOUNS/MEMBER/LANG and maps RELATED to AGENT); the
// cross-format test asserts each format against its own capability set so it
// proves the FILTER is what omits data, not the format's inherent limits.
var vcard4SectionMarkers = map[string]string{
	models.SectionEmails:         "EMAIL",
	models.SectionPhones:         "TEL",
	models.SectionAddresses:      "ADR",
	models.SectionOrganizations:  "ORG",
	models.SectionAnniversaries:  "BDAY",
	models.SectionMedia:          "PHOTO",
	models.SectionOnlineServices: "IMPP",
	models.SectionLinks:          "URL",
	models.SectionNotes:          "NOTE",
	models.SectionKeywords:       "CATEGORIES",
	models.SectionRelatedTo:      "RELATED",
	models.SectionPersonalInfo:   "HOBBY",
	models.SectionSpeakToAs:      "PRONOUNS",
	models.SectionMembers:        "MEMBER",
	models.SectionLanguages:      "LANG",
	models.SectionCustomFields:   "X-CUSTOM",
}

var vcard3SectionMarkers = map[string]string{
	models.SectionEmails:         "EMAIL",
	models.SectionPhones:         "TEL",
	models.SectionAddresses:      "ADR",
	models.SectionOrganizations:  "ORG",
	models.SectionAnniversaries:  "BDAY",
	models.SectionMedia:          "PHOTO",
	models.SectionOnlineServices: "IMPP",
	models.SectionLinks:          "URL",
	models.SectionNotes:          "NOTE",
	models.SectionKeywords:       "CATEGORIES",
	models.SectionRelatedTo:      "AGENT", // v3's RELATED equivalent
	models.SectionCustomFields:   "X-CUSTOM",
}

// jscontactSectionMarkers maps each section to the JSContact card key that
// carries it.
var jscontactSectionMarkers = map[string]string{
	models.SectionEmails:         "emails",
	models.SectionPhones:         "phones",
	models.SectionAddresses:      "addresses",
	models.SectionOrganizations:  "organizations",
	models.SectionAnniversaries:  "anniversaries",
	models.SectionMedia:          "media",
	models.SectionOnlineServices: "onlineServices",
	models.SectionLinks:          "links",
	models.SectionNotes:          "notes",
	models.SectionKeywords:       "keywords",
	models.SectionRelatedTo:      "relatedTo",
	models.SectionPersonalInfo:   "personalInfo",
	models.SectionSpeakToAs:      "speakToAs",
	models.SectionMembers:        "members",
	models.SectionLanguages:      "preferredLanguages",
	models.SectionCustomFields:   "vCardProps",
}

func TestSelectiveExport_AllSectionsAppearsInAllFormats(t *testing.T) {
	record := models.ApplyFieldSelection(selectiveExportFullRecord(), models.FieldSelectionAll())

	v3, diags, err := (vcard3.Adapter{}).Export(record)
	require.NoError(t, err, "vcard3 export failed; diags: %v", diags)
	v4, diags, err := (vcard4.Adapter{}).Export(record)
	require.NoError(t, err, "vcard4 export failed; diags: %v", diags)
	js, diags, err := (jscontact.Adapter{}).Export(record)
	require.NoError(t, err, "jscontact export failed; diags: %v", diags)

	jsCard := parseJSContactCard(t, js)

	for section, marker := range vcard3SectionMarkers {
		t.Run("vcard3_"+section, func(t *testing.T) {
			assert.Contains(t, string(v3), marker, "vcard3 with all sections must carry %q", marker)
		})
	}
	for section, marker := range vcard4SectionMarkers {
		t.Run("vcard4_"+section, func(t *testing.T) {
			assert.Contains(t, string(v4), marker, "vcard4 with all sections must carry %q", marker)
		})
	}
	for section, marker := range jscontactSectionMarkers {
		t.Run("jscontact_"+section, func(t *testing.T) {
			_, present := jsCard[marker]
			assert.True(t, present, "jscontact with all sections must carry %q", marker)
		})
	}
}

// The ticket's core cross-format assertion: the SAME selection produces
// consistent omissions through vcard3, vcard4, and jscontact, because the
// filter runs once on the shared neutral Card before any exporter.
func TestSelectiveExport_ConsistentOmissionsAcrossFormats(t *testing.T) {
	sel := models.NewFieldSelection()
	require.NoError(t, sel.Enable(models.SectionEmails))
	require.NoError(t, sel.Enable(models.SectionPhones))

	record := models.ApplyFieldSelection(selectiveExportFullRecord(), sel)

	v3, _, err := (vcard3.Adapter{}).Export(record)
	require.NoError(t, err)
	v4, _, err := (vcard4.Adapter{}).Export(record)
	require.NoError(t, err)
	js, _, err := (jscontact.Adapter{}).Export(record)
	require.NoError(t, err)

	jsCard := parseJSContactCard(t, js)

	// Positive: the selected sections are present in every format.
	assert.Contains(t, string(v3), "EMAIL")
	assert.Contains(t, string(v4), "EMAIL")
	assert.Contains(t, string(v3), "TEL")
	assert.Contains(t, string(v4), "TEL")
	require.Contains(t, jsCard, "emails")
	require.Contains(t, jsCard, "phones")

	// Negative: every deselected section's marker is absent in ALL formats
	// (each format asserted against its own capability set).
	for section, marker := range vcard3SectionMarkers {
		if section == models.SectionEmails || section == models.SectionPhones {
			continue
		}
		assert.NotContains(t, string(v3), marker, "vcard3 must omit deselected section %q (%s)", section, marker)
	}
	for section, marker := range vcard4SectionMarkers {
		if section == models.SectionEmails || section == models.SectionPhones {
			continue
		}
		assert.NotContains(t, string(v4), marker, "vcard4 must omit deselected section %q (%s)", section, marker)
	}
	for section, marker := range jscontactSectionMarkers {
		if section == models.SectionEmails || section == models.SectionPhones {
			continue
		}
		_, present := jsCard[marker]
		assert.False(t, present, "jscontact must omit deselected section %q (%s)", section, marker)
	}

	// Identity always survives: name and uid must still be present.
	assert.Contains(t, string(v4), "FN")
	assert.Contains(t, string(v4), "Alice")
	_, namePresent := jsCard["name"]
	assert.True(t, namePresent, "jscontact must always keep identity name")
	_, uidPresent := jsCard["uid"]
	assert.True(t, uidPresent, "jscontact must always keep identity uid")
}

func parseJSContactCard(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var card map[string]any
	require.NoError(t, json.Unmarshal(raw, &card), "exported JSContact did not parse: %s", string(raw))
	return card
}

func TestSelectiveExport_SelectingCustomFieldsOnlyKeepsPassthrough(t *testing.T) {
	sel := models.NewFieldSelection()
	require.NoError(t, sel.Enable(models.SectionCustomFields))

	record := models.ApplyFieldSelection(selectiveExportFullRecord(), sel)

	v4, _, err := (vcard4.Adapter{}).Export(record)
	require.NoError(t, err)
	assert.Contains(t, string(v4), "X-CUSTOM")
	assert.NotContains(t, string(v4), "EMAIL")
	assert.NotContains(t, string(v4), "TEL")

	js, _, err := (jscontact.Adapter{}).Export(record)
	require.NoError(t, err)
	jsCard := parseJSContactCard(t, js)
	assert.Contains(t, jsCard, "vCardProps")
	_, emailsPresent := jsCard["emails"]
	assert.False(t, emailsPresent)
}
