package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1355: a bundle imported into another account on the SAME instance
// must not reuse the source's UUID primary keys (globally unique), and every
// intra-bundle reference must be rewritten to the rows created in the
// destination account.

// bundleIDSections lists every AccountBundlePlan section whose rows have a
// UUID primary key the import must remap, keyed by JSON name.
var bundleIDSections = map[string]string{
	"relationships": "id", "life_events": "id", "gifts": "id", "preferences": "id",
	"conversation_agenda": "id", "cadence_policies": "id", "data_decay_policies": "id",
	"households": "id", "circles": "id", "tags": "id", "custom_field_definitions": "id",
	"occasions": "id", "occasion_events": "id",
}

// bundleIDRefs lists the cross-row references: section -> field -> the section
// whose id it holds.
var bundleIDRefs = map[string]map[string]string{
	"gifts":               {"life_event_id": "life_events"},
	"occasions":           {"linked_life_event_id": "life_events"},
	"reminders":           {"life_event_id": "life_events", "occasion_obligation_id": "occasions"},
	"custom_field_values": {"field_definition_id": "custom_field_definitions"},
}

func planSections(t *testing.T, b *models.AccountBundle) map[string][]map[string]any {
	t.Helper()
	raw, err := json.Marshal(b.Plan)
	require.NoError(t, err)
	var out map[string][]map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func sectionIDs(sections map[string][]map[string]any) map[string]bool {
	ids := map[string]bool{}
	for name, field := range bundleIDSections {
		for _, item := range sections[name] {
			ids[item[field].(string)] = true
		}
	}
	return ids
}

// canonicalPlanJSON renders a bundle plan with every remapped UUID primary key
// (and every reference to one) replaced by a content signature, and each
// section sorted, so two plans compare equal iff they hold equivalent rows with
// equivalent references -- regardless of the IDs each account minted.
func canonicalPlanJSON(t *testing.T, b *models.AccountBundle) string {
	t.Helper()
	sections := planSections(t, b)
	// The contact etag embeds the row's local numeric id, so it legitimately
	// differs between accounts on one database.
	for _, contact := range sections["contacts"] {
		delete(contact, "etag")
	}
	sigOf := map[string]map[string]string{} // section -> old id -> signature
	for name, idField := range bundleIDSections {
		sigOf[name] = map[string]string{}
		for _, item := range sections[name] {
			clone := map[string]any{}
			for k, v := range item {
				if k == idField {
					continue
				}
				clone[k] = v
			}
			for ref := range bundleIDRefs[name] {
				delete(clone, ref)
			}
			raw, err := json.Marshal(clone)
			require.NoError(t, err)
			sum := sha256.Sum256(raw)
			sig := name + ":" + hex.EncodeToString(sum[:8])
			for _, existing := range sigOf[name] {
				require.NotEqual(t, sig, existing, "seed rows in %s must be distinguishable", name)
			}
			sigOf[name][item[idField].(string)] = sig
		}
	}
	for name, section := range sections {
		for _, item := range section {
			if idField, ok := bundleIDSections[name]; ok {
				item[idField] = sigOf[name][item[idField].(string)]
			}
			for field, target := range bundleIDRefs[name] {
				if v, ok := item[field].(string); ok && v != "" {
					sig, found := sigOf[target][v]
					require.True(t, found, "%s.%s -> %q must resolve inside the bundle", name, field, v)
					item[field] = sig
				}
			}
		}
		sort.Slice(section, func(i, j int) bool {
			a, _ := json.Marshal(section[i])
			b, _ := json.Marshal(section[j])
			return string(a) < string(b)
		})
	}
	out, err := json.Marshal(sections)
	require.NoError(t, err)
	return string(out)
}

// TestAccountBundle_SameInstanceImportIntoOtherAccounts is the #1355
// regression: A exports, B and C import into the SAME database, each receives
// every entity type with resolved references, A is untouched, and no account
// shares a UUID primary key with another.
func TestAccountBundle_SameInstanceImportIntoOtherAccounts(t *testing.T) {
	db := dbtest.New(t)
	a := bundleTestUser(t, db, "remap-a")
	b := bundleTestUser(t, db, "remap-b")
	c := bundleTestUser(t, db, "remap-c")
	seed := seedFullAccount(t, db, a, "REMAP")
	photoDir := useBundlePhotoDir(t)

	exportedA, _, err := BuildAccountBundle(db, a.ID, seed.PhotoDir)
	require.NoError(t, err)
	rawA, err := json.Marshal(exportedA.Plan)
	require.NoError(t, err)

	// Guard: every section of the plan is seeded (non-empty) and every
	// section carrying a UUID `ID` field is classified for remapping, so a
	// new UUID-PK entity added to the bundle cannot ship un-remapped.
	sectionsA := planSections(t, exportedA)
	planType := reflect.TypeOf(models.AccountBundlePlan{})
	for i := 0; i < planType.NumField(); i++ {
		field := planType.Field(i)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		require.NotEmpty(t, sectionsA[jsonName], "seedFullAccount must seed section %s", jsonName)
		if idField, ok := field.Type.Elem().FieldByName("ID"); ok && idField.Type.Kind() == reflect.String {
			_, classified := bundleIDSections[jsonName]
			require.True(t, classified, "section %s has a UUID ID and must be remapped and listed in bundleIDSections", jsonName)
		}
	}

	var idsSeen []map[string]bool
	idsSeen = append(idsSeen, sectionIDs(sectionsA))
	for _, dest := range []models.User{b, c} {
		report, _, err := ExecuteSourceImportWithActions(t.Context(), db, dest.ID,
			MapAccountBundle(exportedA), map[string]SourceContactAction{}, nil)
		require.NoError(t, err)
		require.Empty(t, report.Issues, "a same-instance import must report no issues")
		require.Equal(t, 3, report.ContactsCreated)

		reexported, _, err := BuildAccountBundle(db, dest.ID, photoDir)
		require.NoError(t, err)
		require.Equal(t, canonicalPlanJSON(t, exportedA), canonicalPlanJSON(t, reexported),
			"the destination must hold every entity with equivalent references")
		idsSeen = append(idsSeen, sectionIDs(planSections(t, reexported)))
	}

	// No UUID primary key is shared between any two accounts.
	for i := range idsSeen {
		for j := i + 1; j < len(idsSeen); j++ {
			for id := range idsSeen[i] {
				assert.False(t, idsSeen[j][id], "accounts %d and %d share primary key %s", i, j, id)
			}
		}
	}

	// A's rows are untouched.
	afterA, _, err := BuildAccountBundle(db, a.ID, seed.PhotoDir)
	require.NoError(t, err)
	afterRaw, err := json.Marshal(afterA.Plan)
	require.NoError(t, err)
	require.JSONEq(t, string(rawA), string(afterRaw), "importing into other accounts must not touch the source account")

	// Re-importing into B stays idempotent (import_source_links).
	report, _, err := ExecuteSourceImportWithActions(t.Context(), db, b.ID,
		MapAccountBundle(exportedA), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	assert.Zero(t, report.ContactsCreated)
	again, _, err := BuildAccountBundle(db, b.ID, photoDir)
	require.NoError(t, err)
	require.Equal(t, canonicalPlanJSON(t, exportedA), canonicalPlanJSON(t, again))
	require.Equal(t, idsSeen[1], sectionIDs(planSections(t, again)), "re-import must keep B's IDs")
}

// TestAccountBundle_DanglingReferencesAreDroppedAndReported: a reference whose
// target is not in the plan is never left pointing at the source's ID.
func TestAccountBundle_DanglingReferencesAreDroppedAndReported(t *testing.T) {
	db := dbtest.New(t)
	a := bundleTestUser(t, db, "dangle-a")
	b := bundleTestUser(t, db, "dangle-b")
	seed := seedFullAccount(t, db, a, "DANGLE")
	useBundlePhotoDir(t)

	exported, _, err := BuildAccountBundle(db, a.ID, seed.PhotoDir)
	require.NoError(t, err)
	plan := MapAccountBundle(exported)
	plan.LifeEvents = nil
	plan.FieldDefinitions = nil
	// Drop a contact that the life event is not owned by but that it relates to.
	var kept []MappedContact
	for _, mc := range plan.Contacts {
		if !strings.Contains(mc.Record.Card.Name.Full, "Bob") {
			kept = append(kept, mc)
		}
	}
	plan.Contacts = kept

	report, _, err := ExecuteSourceImportWithActions(t.Context(), db, b.ID, plan, map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	fields := map[string]bool{}
	for _, issue := range report.Issues {
		fields[issue.Field] = true
	}
	for _, f := range []string{"gift.life_event_id", "occasion.linked_life_event_id", "reminder.life_event_id"} {
		assert.True(t, fields[f], "missing lossy issue for %s", f)
	}
	assert.True(t, fields["custom_field."], "a value whose definition is not in the plan is reported")

	var gift models.Gift
	require.NoError(t, db.Where("user_id = ?", b.ID).First(&gift).Error)
	assert.Empty(t, gift.LifeEventID)
	var rem models.Reminder
	require.NoError(t, db.Where("user_id = ?", b.ID).First(&rem).Error)
	assert.Nil(t, rem.LifeEventID)
	require.NotNil(t, rem.OccasionObligationID, "the resolvable occasion link survives")
}

// TestAccountBundle_RelatedEntityIDsRemapped covers the related-contact
// remap and drop of a life event.
func TestAccountBundle_RelatedEntityIDsRemapped(t *testing.T) {
	report := &ImportReport{}
	refToUID := map[string]string{bundleContactRef("src-1").ExternalID: "local-1"}
	out := remapRelatedEntityIDs(report, "rec", []string{"src-1", "gone"}, refToUID)
	assert.Equal(t, []string{"local-1"}, out)
	require.Len(t, report.Issues, 1)
	assert.Equal(t, ImportIssueCategoryLossy, report.Issues[0].Category)
	assert.Empty(t, remapRelatedEntityIDs(report, "rec", nil, refToUID))
}

// TestAccountBundle_WriteFailuresUseStableMessage hides each destination table
// in turn: the row is reported with a stable user-facing message (never the
// driver's SQL text) and the rest of the import still completes.
func TestAccountBundle_WriteFailuresUseStableMessage(t *testing.T) {
	cases := []struct{ table, field string }{
		{"life_events", "life_event"},
		{"conversation_agenda", "conversation_agenda"},
		{"cadence_policies", "cadence_policy"},
		{"data_decay_policies", "data_decay_policy"},
		{"occasion_obligations", "occasion"},
		{"occasion_events", "occasion_event"},
		{"occasion_event_attendees", "occasion_event.attendee"},
		{"reminder_completions", "reminder_completion"},
		{"field_definitions", "field_definition"},
		{"households", "household"},
		{"household_members", "household.member"},
		{"circles", "circle"},
		{"circle_members", "circle.member"},
		{"tags", "tag"},
		{"contact_tags", "tag.contact"},
		{"gifts", "gift"},
		{"preferences", "preference"},
		{"relationship_edges", "relationship"},
		{"notes", "note"},
		{"reminders", "reminder"},
	}
	srcDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "fail-owner")
	seed := seedFullAccount(t, srcDB, owner, "FAIL")
	useBundlePhotoDir(t)
	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)

	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			db := dbtest.New(t)
			dest := bundleTestUser(t, db, "fail-dest")
			dbtest.HideTable(t, db, tc.table)
			report, _, err := ExecuteSourceImportWithActions(t.Context(), db, dest.ID,
				MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
			require.NoError(t, err)
			var found bool
			for _, issue := range report.Issues {
				assert.NotContains(t, strings.ToLower(issue.Message), "no such table", "raw driver text leaked: %+v", issue)
				if issue.Field == tc.field && issue.Message == importWriteFailureMessage {
					found = true
					assert.Equal(t, ImportIssueCategoryInvalid, issue.Category)
				}
			}
			assert.True(t, found, "no stable write-failure issue for %s; issues=%+v", tc.field, report.Issues)
		})
	}
}
