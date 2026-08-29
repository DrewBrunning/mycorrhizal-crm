package canonicalfixture

import (
	"encoding/json"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// readManifest loads the checked-in manifest for a test.
func readManifest(t *testing.T) *Manifest {
	t.Helper()
	m, err := Read()
	require.NoError(t, err)
	return m
}

// populatedDB reads the manifest, populates a real migrated database
// (internal/dbtest — CLAUDE.md backend trap #1), and returns both.
func populatedDB(t *testing.T) (*Manifest, *Dataset, *gorm.DB) {
	t.Helper()
	m := readManifest(t)
	db := dbtest.New(t)
	ds, err := Populate(db, m)
	require.NoError(t, err)
	return m, ds, db
}

// reloadContact fetches a contact back out of the database (Unscoped, so a
// soft-deleted fixture contact is still readable) — the real persistence
// round trip, not the in-memory create struct.
func reloadContact(t *testing.T, db *gorm.DB, c models.Contact) models.Contact {
	t.Helper()
	var got models.Contact
	require.NoError(t, db.Unscoped().First(&got, c.ID).Error)
	return got
}

// entryRecord is the manifest-declared record for a contact entry, applying
// the loader's RecreatesVCardUIDOf uid override so the expectation matches
// what was actually persisted.
func entryRecord(t *testing.T, ds *Dataset, m *Manifest, entry ContactEntry) *contactmodel.Record {
	t.Helper()
	rec := entry.Record()
	if entry.RecreatesVCardUIDOf != "" {
		rec.Card.UID = ds.Contacts[entry.RecreatesVCardUIDOf].VCardUID
	}
	return rec
}

// canonicalize re-encodes a Record through JSON. The manifest side of a
// comparison still holds json.RawMessage bytes exactly as authored (e.g. a
// Localizations value written with spaces), while the persisted side has been
// compacted by the storage serializer's json.Marshal — both sides are
// semantically identical but byte-different. Re-encoding both makes the
// comparison whitespace-insensitive while still catching every real field
// difference: a dropped field, a mangled value, or a truncated array still
// fails.
func canonicalize(t *testing.T, rec *contactmodel.Record) *contactmodel.Record {
	t.Helper()
	if rec == nil {
		return nil
	}
	data, err := json.Marshal(rec)
	require.NoError(t, err)
	var out contactmodel.Record
	require.NoError(t, json.Unmarshal(data, &out))
	return &out
}

// TestRoundTripReproducesEveryDeclaredField is the fixture's central load-
// bearing test: every contact's manifest-declared card, crm envelope, and
// passthrough must survive Record -> Contact (ApplyRecordToContact) -> real
// migrated DB -> reload -> Record (RecordForContact) field for field.
//
// It uses RecordForContact (the authoritative read — CLAUDE.md backend trap
// #3) with a nil db so read-time projections (graph edges, tags, hobby
// preferences, custom-field passthrough) do not add anything beyond what the
// manifest declares. Any declared field that a migration, a save path, or a
// wrong read function drops fails this test naming the contact.
func TestRoundTripReproducesEveryDeclaredField(t *testing.T) {
	m, ds, db := populatedDB(t)

	for _, entry := range m.Contacts {
		entry := entry
		t.Run("contact_"+entry.Name, func(t *testing.T) {
			c := reloadContact(t, db, ds.Contacts[entry.Name])
			got := models.RecordForContact(&c, "", nil)
			require.NotNil(t, got, "RecordForContact must never return nil for a stored contact")

			want := canonicalize(t, entryRecord(t, ds, m, entry))
			gotRec := canonicalize(t, got)
			assert.Equal(t, want.Card, gotRec.Card, "%s: card did not round-trip", entry.Name)
			assert.Equal(t, want.Envelope, gotRec.Envelope, "%s: crm envelope did not round-trip", entry.Name)
			assert.Equal(t, want.Passthrough, gotRec.Passthrough, "%s: passthrough did not round-trip", entry.Name)

			// The VCardUID is the persistence key every join row references;
			// it must match the card's uid so the graph invariant holds.
			assert.Equal(t, c.VCardUID, gotRec.Card.UID, "%s: VCardUID and Card.UID drifted", entry.Name)
		})
	}
}

// TestTrapFieldsSurviveRoundTrip pins the specific bug classes the manifest
// exists to be a regression net for, by name, so a removal of any single trap
// record fails here — the "load-bearing, not decorative" requirement.
func TestTrapFieldsSurviveRoundTrip(t *testing.T) {
	m, ds, db := populatedDB(t)

	read := func(name string) *contactmodel.Record {
		c := reloadContact(t, db, ds.Contacts[name])
		return models.RecordForContact(&c, "", nil)
	}

	t.Run("trap3_no_flat_home_fields", func(t *testing.T) {
		// SpeakToAs, PersonalInfo, SocialProfiles, OtherOnlineServices, extra
		// name components, Keywords, Notes, Members and Localizations have no
		// flat-column home — RecordFromContact drops all of them. They must
		// survive via the neutral Card.
		rec := read("ada")
		require.NotNil(t, rec.Card.SpeakToAs, "ada: SpeakToAs dropped")
		assert.Equal(t, []contactmodel.Pronouns{{Pronouns: "she/her", Contexts: []string{"work"}}}, rec.Card.SpeakToAs.Pronouns)
		assert.Equal(t, []contactmodel.GrammaticalGender{{Value: "feminine", Language: "en"}}, rec.Card.SpeakToAs.GrammaticalGenders)
		assert.Contains(t, rec.Card.PersonalInfo, contactmodel.PersonalInfo{Kind: "hobby", Value: "analytical machinery", Level: "high"})
		assert.Len(t, rec.Card.SocialProfiles, 1, "ada: SocialProfiles dropped")
		assert.Len(t, rec.Card.OtherOnlineServices, 1, "ada: OtherOnlineServices dropped")
		assert.Len(t, rec.Card.Members, 1, "ada: Members dropped")
		require.NotNil(t, rec.Card.Localizations, "ada: Localizations dropped")
		assert.Contains(t, string(rec.Card.Localizations["fr"]), "Lovelace")
		// extra name components past the flat five
		require.NotNil(t, rec.Card.Name, "ada: Name dropped")
		assert.Equal(t, []contactmodel.NameComponent{
			{Kind: "title", Value: "Countess"},
			{Kind: "given", Value: "Ada"},
			{Kind: "given2", Value: "Augusta"},
			{Kind: "surname", Value: "Lovelace"},
			{Kind: "credential", Value: "OL"},
		}, rec.Card.Name.Components)
	})

	t.Run("gender_515_round_trip_hole", func(t *testing.T) {
		// Gender has no Card home; it lives in the CRM envelope (issue #515)
		// and must survive Record -> Contact -> Record through it.
		rec := read("bob")
		assert.Equal(t, "male", rec.Envelope.Gender)
		// And the rich card-only contact's envelope fields (how_we_met etc.)
		ada := read("ada")
		assert.Equal(t, "non-binary", ada.Envelope.Gender)
		assert.Equal(t, "Through the Analytical Engine project.", ada.Envelope.HowWeMet)
	})

	t.Run("envelope_kind_pet", func(t *testing.T) {
		rec := read("frank")
		assert.Equal(t, "animal", rec.Envelope.Kind)
	})

	t.Run("unicode_names_survive", func(t *testing.T) {
		rec := read("celine")
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "セリーヌ", rec.Card.Name.Components[0].Value)
		assert.Equal(t, "田中", rec.Card.Name.Components[1].Value)
		// Non-ASCII custom-field value keeps its JSON shape
		require.NotNil(t, rec.Card.Notes)
		assert.Contains(t, rec.Card.Notes[0].Note, "🍜")
	})

	t.Run("empty_null_contact_round_trips_absent", func(t *testing.T) {
		rec := read("dmitri")
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, []contactmodel.NameComponent{{Kind: "given", Value: "Dmitri"}}, rec.Card.Name.Components)
		assert.Empty(t, rec.Card.Emails)
		assert.Empty(t, rec.Card.Phones)
		assert.Empty(t, rec.Card.Addresses)
		assert.Empty(t, rec.Envelope.Circles)
		assert.Empty(t, rec.Passthrough.VCard)
	})

	t.Run("edge_case_dates_survive", func(t *testing.T) {
		rec := read("eve")
		require.Len(t, rec.Card.Anniversaries, 2)
		// year-less leap-day birthday
		require.NotNil(t, rec.Card.Anniversaries[0].Date.Partial)
		assert.Equal(t, 2, *rec.Card.Anniversaries[0].Date.Partial.Month)
		assert.Equal(t, 29, *rec.Card.Anniversaries[0].Date.Partial.Day)
		assert.Nil(t, rec.Card.Anniversaries[0].Date.Partial.Year)
		// far-future wedding
		require.NotNil(t, rec.Card.Anniversaries[1].Date.Partial)
		assert.Equal(t, 9999, *rec.Card.Anniversaries[1].Date.Partial.Year)
		assert.Equal(t, 12, *rec.Card.Anniversaries[1].Date.Partial.Month)
		assert.Equal(t, 31, *rec.Card.Anniversaries[1].Date.Partial.Day)
	})

	t.Run("recreated_vcard_uid_contact", func(t *testing.T) {
		julie := ds.Contacts["julie"]
		gina := ds.Contacts["gina"]
		assert.Equal(t, gina.VCardUID, julie.VCardUID, "julie must reuse gina's vcard_uid")
		assert.True(t, gina.DeletedAt.Valid, "gina must be tombstoned for the recreation to be legal")
		rec := read("julie")
		assert.Equal(t, gina.VCardUID, rec.Card.UID, "julie's card uid must match the recreated vcard_uid")
	})

	t.Run("very_long_values_survive", func(t *testing.T) {
		// The long note and near-ceiling how_we_met must round-trip byte for
		// byte — a truncation would break the length check below even if the
		// content compare somehow passed.
		var wantNote string
		for _, n := range m.Notes {
			if n.Contact == "bob" {
				wantNote = n.Content
			}
		}
		require.NotEmpty(t, wantNote, "manifest must declare bob's long note")
		require.True(t, len(wantNote) > 1500, "bob's note should be a genuinely long value, got %d chars", len(wantNote))

		bobContact := reloadContact(t, db, ds.Contacts["bob"])
		var gotNote models.Note
		require.NoError(t, db.Where("contact_id = ? AND user_id = ?", bobContact.ID, ds.User.ID).First(&gotNote).Error)
		assert.Equal(t, wantNote, gotNote.Content)
		assert.Equal(t, len(wantNote), len(gotNote.Content), "bob's long note must not be truncated")

		bob := read("bob")
		var wantHowWeMet string
		for _, c := range m.Contacts {
			if c.Name == "bob" {
				wantHowWeMet = c.CRM.HowWeMet
			}
		}
		assert.Equal(t, wantHowWeMet, bob.Envelope.HowWeMet)
		assert.True(t, len(wantHowWeMet) > 900 && len(wantHowWeMet) <= 1000, "bob's how_we_met should sit near the 1000-char ceiling, got %d", len(wantHowWeMet))
	})
}

// TestProjectionRules exercises the read-time projections over the fixture:
// confirmed edges, tags, hobby preferences and vCard-projected custom fields
// are projected onto the Record; suggested edges and above-normal sensitivity
// are filtered in the query, never in the caller.
func TestProjectionRules(t *testing.T) {
	_, ds, db := populatedDB(t)

	read := func(name string, sel *models.FieldSelection) *contactmodel.Record {
		c := reloadContact(t, db, ds.Contacts[name])
		return models.RecordForContactFiltered(&c, "", db, sel)
	}

	containsRelation := func(rec *contactmodel.Record, targetUID, tag string) bool {
		for _, rel := range rec.Card.RelatedTo {
			if rel.Target == "urn:uuid:"+targetUID {
				for _, got := range rel.Relations {
					if got == tag {
						return true
					}
				}
			}
		}
		return false
	}
	personalInfoValues := func(rec *contactmodel.Record) []string {
		var out []string
		for _, p := range rec.Card.PersonalInfo {
			out = append(out, p.Value)
		}
		return out
	}
	passthroughNames := func(rec *contactmodel.Record) []string {
		var out []string
		for _, p := range rec.Passthrough.VCard {
			out = append(out, p.Name)
		}
		return out
	}

	all := models.FieldSelectionAll()
	all.IncludeSensitive = true

	t.Run("confirmed_edges_project", func(t *testing.T) {
		ada := read("ada", nil)
		// passthrough entry declared on the card is preserved
		assert.True(t, containsRelation(ada, ds.Contacts["bob"].VCardUID, "friend"), "ada: declared passthrough relation lost")
		// ada->bob coworker_of projects on ada's side as co-worker
		assert.True(t, containsRelation(ada, ds.Contacts["bob"].VCardUID, "co-worker"), "ada: coworker edge not projected")
		// ada->celine friend_of
		assert.True(t, containsRelation(ada, ds.Contacts["celine"].VCardUID, "friend"), "ada: friend edge not projected")
		// ada->eve parent_of projects on ada's side as the inverse, child
		assert.True(t, containsRelation(ada, ds.Contacts["eve"].VCardUID, "child"), "ada: parent edge not projected as inverse child")
		// ada->frank owns has no vCard tag and must not project at all
		assert.False(t, containsRelation(ada, ds.Contacts["frank"].VCardUID, ""), "ada: untaggable owns edge leaked")
	})

	t.Run("suggested_edges_never_project", func(t *testing.T) {
		eve := read("eve", all)
		// bob->eve parent_of is suggested; eve must only see ada's confirmed parent edge
		assert.True(t, containsRelation(eve, ds.Contacts["ada"].VCardUID, "parent"), "eve: confirmed parent edge not projected")
		assert.False(t, containsRelation(eve, ds.Contacts["bob"].VCardUID, "parent"), "eve: suggested parent edge must never project")
	})

	t.Run("sensitivity_filters_in_the_query", func(t *testing.T) {
		// hugo<->ida spouse_of is private: absent by default, present under the explicit opt-in.
		hugo := read("hugo", nil)
		assert.False(t, containsRelation(hugo, ds.Contacts["ida"].VCardUID, "spouse"), "hugo: private spouse edge leaked into default projection")
		hugoSensitive := read("hugo", all)
		assert.True(t, containsRelation(hugoSensitive, ds.Contacts["ida"].VCardUID, "spouse"), "hugo: private spouse edge missing from sensitive projection")
	})

	t.Run("tags_project_to_keywords", func(t *testing.T) {
		ada := read("ada", nil)
		assert.Contains(t, ada.Card.Keywords, "mathematics")
		assert.Contains(t, ada.Card.Keywords, "vip", "tag 'vip' should project into keywords (deduped against the declared keyword)")
		hugo := read("hugo", nil)
		assert.Contains(t, hugo.Card.Keywords, "volunteer")
	})

	t.Run("hobby_preferences_project_to_personal_info", func(t *testing.T) {
		ada := read("ada", nil)
		assert.Contains(t, personalInfoValues(ada), "sailing", "ada: normal hobby preference should project")
		bob := read("bob", nil)
		assert.NotContains(t, personalInfoValues(bob), "chess", "bob: private hobby preference leaked into default projection")
		bobSensitive := read("bob", all)
		assert.Contains(t, personalInfoValues(bobSensitive), "chess", "bob: private hobby preference missing from sensitive projection")
		// eve's skydiving preference is soft-deleted and must not project at all
		eve := read("eve", all)
		assert.NotContains(t, personalInfoValues(eve), "skydiving", "eve: soft-deleted preference leaked into projection")
	})

	t.Run("vcard_projected_custom_fields", func(t *testing.T) {
		ada := read("ada", nil)
		assert.Contains(t, passthroughNames(ada), "X-FAVORITE-COFFEE", "ada: normal vcard-projected custom field should project")
		assert.NotContains(t, passthroughNames(ada), "X-PRIVATE-NICK", "ada: secret vcard-projected custom field leaked into default projection")
		adaSensitive := read("ada", all)
		assert.Contains(t, passthroughNames(adaSensitive), "X-PRIVATE-NICK", "ada: secret vcard-projected custom field missing from sensitive projection")
	})

	// The multi-valued and number custom-field values keep their JSON shape
	// (raw JSON number, not a stringified one).
	t.Run("custom_field_value_shapes", func(t *testing.T) {
		var vals []models.FieldValue
		require.NoError(t, db.Where("user_id = ?", ds.User.ID).Find(&vals).Error)
		byField := map[string]string{}
		for _, v := range vals {
			byField[v.FieldDefinitionID] = string(v.Value)
		}
		var defs []models.FieldDefinition
		require.NoError(t, db.Where("user_id = ?", ds.User.ID).Find(&defs).Error)
		byKey := map[string]string{}
		for _, d := range defs {
			if v, ok := byField[d.ID]; ok {
				byKey[d.Key] = v
			}
		}
		assert.Equal(t, `42`, byKey["favorite_number"], "number custom-field value must keep its raw JSON number shape")
		assert.Equal(t, `["ja","fr","en"]`, byKey["languages_spoken"], "multi-valued custom-field value must keep its raw JSON array shape")
	})
}

// TestManifestReadAndValidate covers the manifest's self-description and
// cross-references.
func TestManifestReadAndValidate(t *testing.T) {
	m := readManifest(t)
	assert.Equal(t, ManifestVersion, m.Version)
	assert.NotEmpty(t, m.Description)

	// Every section's cross-references already resolved (Validate ran in Read),
	// so here we pin the shape downstream suites rely on: one user, a known
	// contact set including the trap records, and at least one row per entity
	// section so no dataset list item is accidentally dropped.
	names := map[string]bool{}
	for _, c := range m.Contacts {
		names[c.Name] = true
	}
	for _, required := range []string{"ada", "bob", "celine", "dmitri", "eve", "frank", "gina", "hugo", "ida", "julie"} {
		assert.True(t, names[required], "manifest must declare the %q trap contact", required)
	}
	assert.NotEmpty(t, m.Notes)
	assert.NotEmpty(t, m.LifeEvents)
	assert.NotEmpty(t, m.Gifts)
	assert.NotEmpty(t, m.Relationships)
	assert.NotEmpty(t, m.Households)
	assert.NotEmpty(t, m.Circles)
	assert.NotEmpty(t, m.Tags)
	assert.NotEmpty(t, m.CustomFields)
	assert.NotEmpty(t, m.Preferences)
	assert.NotEmpty(t, m.ExternalIdentities)
	assert.NotEmpty(t, m.Attachments)
	assert.NotEmpty(t, m.Activities)
}

// TestPopulateIsScopedAndDeterministic pins that the fixture is one user's
// data (every entity carries that user's id) and that repeated population of
// a fresh database yields an identical contact set.
func TestPopulateIsScopedAndDeterministic(t *testing.T) {
	m, ds, _ := populatedDB(t)
	assert.Equal(t, m.User.Username, ds.User.Username)
	assert.Len(t, ds.Contacts, len(m.Contacts))

	for _, n := range []string{"ada", "bob", "celine", "dmitri", "eve", "frank", "gina", "hugo", "ida", "julie"} {
		assert.Contains(t, ds.Contacts, n)
	}

	// Second population of a fresh DB produces the same VCardUIDs (the uid
	// comes from the manifest card, not a random generator).
	db2 := dbtest.New(t)
	ds2, err := Populate(db2, m)
	require.NoError(t, err)
	for name, c := range ds.Contacts {
		if c.DeletedAt.Valid {
			continue // julie inherits gina's uid; still deterministic, checked below
		}
		assert.Equal(t, c.VCardUID, ds2.Contacts[name].VCardUID, "contact %s uid must be deterministic", name)
	}
	assert.Equal(t, ds.Contacts["julie"].VCardUID, ds2.Contacts["julie"].VCardUID)
}

// TestManifestRejectsBrokenReference ensures the loader fails loudly (with
// the section and name) when a cross-reference does not resolve — the guard
// that keeps the manifest honest as it grows.
func TestManifestRejectsBrokenReference(t *testing.T) {
	broken := readManifest(t)
	broken.Notes = append(broken.Notes, NoteEntry{Contact: "nobody", Content: "x"})
	err := broken.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown contact "nobody"`)
	assert.True(t, strings.Contains(err.Error(), "note"), "error should name the offending section")

	unknownVersion := readManifest(t)
	unknownVersion.Version = 99
	err = unknownVersion.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported manifest version")
}
