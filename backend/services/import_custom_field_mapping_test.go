package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/vcard4"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file tests issue #514's import half of the custom-field/vCard bridge:
// discovered X-* passthrough properties surface as CustomFieldCandidates on
// the preview, and the wizard's ImportFieldMapping decisions (plus the
// projection-match auto-map) are materialized into FieldValues at confirm
// time while promoted properties are stripped from the saved contact's
// passthrough. All persistence tests use dbtest.New (the real migrated
// schema, which includes field_definitions/field_values) per CLAUDE.md backend
// trap #1 — AutoMigrate schemas silently lack those tables.

var importFieldMapUserSeq int

func createImportFieldMapTestUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	importFieldMapUserSeq++
	suffix := fmt.Sprintf("%d", importFieldMapUserSeq)
	user := models.User{Username: "cfmuser" + suffix, Password: "password123!A", Email: "cfm" + suffix + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	models.AuditFlush()
	return user
}

// fieldMapVCard builds a minimal parseable vCard 4.0 with the given extra
// properties appended.
func fieldMapVCard(extra ...string) string {
	raw := "BEGIN:VCARD\r\n" +
		"VERSION:4.0\r\n" +
		"FN:Casey Mapper\r\n" +
		"N:Mapper;Casey;;;\r\n" +
		"EMAIL:casey@example.com\r\n" +
		"UID:11111111-1111-4111-8111-111111111111\r\n"
	for _, p := range extra {
		raw += p + "\r\n"
	}
	raw += "END:VCARD\r\n"
	return raw
}

// confirmFieldMapImport runs ParseVCF + CreateVCFSession + ConfirmVCF for the
// given raw vCard and returns the import result, the created/updated contact
// (reloaded), and the manager so the caller can assert on the outcome.
// --- unit tests ------------------------------------------------------------------

func TestDiscoverCustomFieldCandidates(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	def := models.FieldDefinition{
		UserID:      user.ID,
		Label:       "Favorite Color",
		Key:         "favorite_color",
		Target:      models.FieldDefinitionTargetContact,
		Type:        models.FieldTypeText,
		Projection:  "vcard:X-FAVORITE-COLOR",
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&def).Error)

	contact := &models.Contact{
		UserID:    user.ID,
		Firstname: "Casey",
		Lastname:  "Mapper",
		Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{
			{Name: "x-favorite-color", Type: "text", Value: json.RawMessage(`"green"`)},
			{Name: "x-hometown", Type: "text", Value: json.RawMessage(`"Springfield"`)},
			// Non-X properties are not custom-field candidates.
			{Name: "note", Type: "text", Value: json.RawMessage(`"plain note"`)},
		}},
	}

	candidates := DiscoverCustomFieldCandidates(db, user.ID, contact)
	require.Len(t, candidates, 2)

	// The projection match is surfaced so the wizard can pre-select it.
	assert.Equal(t, "x-favorite-color", candidates[0].Name)
	assert.Equal(t, "green", candidates[0].Value)
	assert.Equal(t, def.ID, candidates[0].MatchedDefinitionID)
	assert.Equal(t, def.Label, candidates[0].MatchedDefinitionLabel)

	// Unmatched property has no suggestion.
	assert.Equal(t, "x-hometown", candidates[1].Name)
	assert.Empty(t, candidates[1].MatchedDefinitionID)

	// Repeated values collapse to a single candidate.
	contact.Passthrough.VCard = append(contact.Passthrough.VCard,
		contactmodel.JCardProp{Name: "x-favorite-color", Type: "text", Value: json.RawMessage(`"blue"`)})
	candidates = DiscoverCustomFieldCandidates(db, user.ID, contact)
	require.Len(t, candidates, 2)

	// A contact with no X-* props yields no candidates.
	assert.Nil(t, DiscoverCustomFieldCandidates(db, user.ID, &models.Contact{}))
	assert.Nil(t, DiscoverCustomFieldCandidates(db, user.ID, nil))
}

func TestDiscoverCustomFieldCandidates_NonStringValueRendersRaw(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	contact := &models.Contact{
		UserID:    user.ID,
		Firstname: "Casey",
		Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{
			// A vCard value that is not a plain JSON string (e.g. a structured
			// value) still renders as something readable for the wizard.
			{Name: "x-meta", Type: "text", Value: json.RawMessage(`{"a":1}`)},
		}},
	}
	candidates := DiscoverCustomFieldCandidates(db, user.ID, contact)
	require.Len(t, candidates, 1)
	assert.Equal(t, `{"a":1}`, candidates[0].Value)
}

func TestFieldKeyFromPropertyAndLabel(t *testing.T) {
	cases := []struct {
		prop, key, label string
	}{
		{"x-favorite-color", "favorite_color", "Favorite Color"},
		{"x-HOMETOWN", "hometown", "Hometown"},
		{"x-custom-field-name", "custom_field_name", "Custom Field Name"},
		{"x-42number", "42number", "42number"},
	}
	for _, tc := range cases {
		t.Run(tc.prop, func(t *testing.T) {
			assert.Equal(t, tc.key, fieldKeyFromProperty(tc.prop))
			assert.Equal(t, tc.label, fieldLabelFromKey(tc.key))
		})
	}

	// A key with empty words (double underscores) still renders a label
	// without crashing.
	assert.Equal(t, "A  B", fieldLabelFromKey("a__b"))
	// A degenerate property name produces an unusable (empty) key.
	assert.Equal(t, "", fieldKeyFromProperty("x-"))
}

func TestCoerceImportedFieldValue(t *testing.T) {
	cases := []struct {
		name  string
		ftype string
		raw   string
		want  string
	}{
		{"string stays string", models.FieldTypeString, `"green"`, `"green"`},
		{"number coerces from string", models.FieldTypeNumber, `"42"`, `42`},
		{"number already number", models.FieldTypeNumber, `42`, `42`},
		{"number rejects non-numeric string", models.FieldTypeNumber, `"abc"`, `"abc"`},
		{"boolean true", models.FieldTypeBoolean, `"true"`, `true`},
		{"boolean 1", models.FieldTypeBoolean, `"1"`, `true`},
		{"boolean false", models.FieldTypeBoolean, `"no"`, `false`},
		{"boolean passes through JSON bool", models.FieldTypeBoolean, `false`, `false`},
		{"multi left alone", models.FieldTypeText, `"green"`, `"green"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := models.FieldDefinition{Type: tc.ftype}
			if tc.name == "multi left alone" {
				def.Constraints.Multi = true
			}
			got := coerceImportedFieldValue(def, json.RawMessage(tc.raw))
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestBuildImportFieldPlan_AutoMapByProjection(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Favorite Color", Key: "favorite_color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "vcard:X-FAVORITE-COLOR", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	// No wizard decisions: the projection match auto-maps.
	plan, err := buildImportFieldPlan(db, user.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, def.ID, plan["x-favorite-color"].ID)
}

func TestBuildImportFieldPlan_ExplicitIgnoreSuppressesAutoMap(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Favorite Color", Key: "favorite_color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "vcard:X-FAVORITE-COLOR", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	plan, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-favorite-color", Action: "ignore"},
	})
	require.NoError(t, err)
	_, mapped := plan["x-favorite-color"]
	assert.False(t, mapped, "an explicit ignore must suppress the projection auto-map")
}

func TestBuildImportFieldPlan_CreateDerivesDefinition(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	plan, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-favorite-color", Action: "create"},
	})
	require.NoError(t, err)

	def, ok := plan["x-favorite-color"]
	require.True(t, ok)
	assert.Equal(t, "favorite_color", def.Key)
	assert.Equal(t, "Favorite Color", def.Label, "label should be title-cased from the key")
	assert.Equal(t, models.FieldTypeText, def.Type)
	assert.Equal(t, "vcard:X-FAVORITE-COLOR", def.Projection, "a promoted field must round-trip")
	assert.Equal(t, user.ID, def.UserID)

	// Persisted — the confirm's tx created the row.
	var persisted models.FieldDefinition
	require.NoError(t, db.First(&persisted, "id = ?", def.ID).Error)

	// Idempotent: a second "create" reuses the same definition rather than
	// colliding with the (user, key) unique index.
	plan2, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-favorite-color", Action: "create", Label: "Renamed", Type: models.FieldTypeNumber},
	})
	require.NoError(t, err)
	assert.Equal(t, def.ID, plan2["x-favorite-color"].ID, "a create for an existing derived key must reuse the definition")
}

func TestBuildImportFieldPlan_ExplicitLabelAndType(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	plan, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-birth-country", Action: "create", Label: "Birth Country", Type: models.FieldTypeString},
	})
	require.NoError(t, err)
	def := plan["x-birth-country"]
	assert.Equal(t, "Birth Country", def.Label)
	assert.Equal(t, models.FieldTypeString, def.Type)
	assert.Equal(t, "vcard:X-BIRTH-COUNTRY", def.Projection)
}

func TestBuildImportFieldPlan_EmptyPropertyNameIgnored(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	plan, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "   ", Action: "create"},
		{PropertyName: "x-ok", Action: "create"},
	})
	require.NoError(t, err)
	_, blank := plan[""]
	assert.False(t, blank, "a blank property name must be ignored")
	assert.Contains(t, plan, "x-ok")
}

func TestBuildImportFieldPlan_CreateWithUnusableKeyFails(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	_, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-", Action: "create"},
	})
	require.ErrorIs(t, err, errInvalidImportFieldMapping)
	assert.Contains(t, err.Error(), "no usable name")
}

// TestBuildImportFieldPlan_MissingDefinitionsTableDegrades drives the
// AutoMigrate test schemas that predate field_definitions (CLAUDE.md backend
// trap #1): auto-match degrades to off instead of failing the import, while
// an explicit "create" still fails loudly through the per-definition query.
func TestBuildImportFieldPlan_MissingDefinitionsTableDegrades(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Contact{})) // no field_definitions

	// Auto-match off, no error.
	plan, err := buildImportFieldPlan(db, 1, nil)
	require.NoError(t, err)
	assert.Empty(t, plan)

	// Explicit "create" surfaces the missing-table lookup as a contract error.
	_, err = buildImportFieldPlan(db, 1, []models.ImportFieldMapping{
		{PropertyName: "x-hometown", Action: "create"},
	})
	require.ErrorIs(t, err, errInvalidImportFieldMapping)
}

func TestBuildImportFieldPlan_MapRequiresOwnedDefinition(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)
	other := createImportFieldMapTestUser(t, db)

	otherDef := models.FieldDefinition{
		UserID: other.ID, Label: "Other", Key: "other_key",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&otherDef).Error)

	// Mapping into another user's definition is a 400-class contract error.
	_, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-favorite-color", Action: "map", FieldDefinitionID: otherDef.ID},
	})
	require.ErrorIs(t, err, errInvalidImportFieldMapping)

	// Mapping into an unknown ID likewise fails.
	_, err = buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-favorite-color", Action: "map", FieldDefinitionID: "00000000-0000-4000-8000-000000000000"},
	})
	require.ErrorIs(t, err, errInvalidImportFieldMapping)

	// A valid owned definition maps.
	ownDef := models.FieldDefinition{
		UserID: user.ID, Label: "Color", Key: "color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&ownDef).Error)
	plan, err := buildImportFieldPlan(db, user.ID, []models.ImportFieldMapping{
		{PropertyName: "x-favorite-color", Action: "map", FieldDefinitionID: ownDef.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, ownDef.ID, plan["x-favorite-color"].ID)
}

func TestPromoteImportedCustomFields_ValueAndStrip(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Favorite Color", Key: "favorite_color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "vcard:X-FAVORITE-COLOR", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	contact := &models.Contact{UserID: user.ID, Firstname: "Casey", VCardUID: "22222222-2222-4222-8222-222222222222"}
	props := []contactmodel.JCardProp{
		{Name: "x-favorite-color", Type: "text", Value: json.RawMessage(`"green"`)},
		{Name: "x-hometown", Type: "text", Value: json.RawMessage(`"Springfield"`)}, // unmapped -> stays
	}
	plan := importFieldPlan{"x-favorite-color": def}

	promoted, notes := promoteImportedCustomFields(db, user.ID, contact, props, plan)
	assert.Equal(t, []string{"x-favorite-color"}, promoted)
	assert.Empty(t, notes)

	var fv models.FieldValue
	require.NoError(t, db.Where("entity_id = ? AND field_definition_id = ?", contact.VCardUID, def.ID).First(&fv).Error)
	assert.JSONEq(t, `"green"`, string(fv.Value))
	assert.Equal(t, user.ID, fv.UserID)

	// Unmapped property must not have been touched.
	var count int64
	require.NoError(t, db.Model(&models.FieldValue{}).Where("entity_id = ?", contact.VCardUID).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// Idempotent upsert: promoting again overwrites rather than violating the
	// (definition, entity) unique index.
	_, _ = promoteImportedCustomFields(db, user.ID, contact, props, plan)
	require.NoError(t, db.Where("entity_id = ? AND field_definition_id = ?", contact.VCardUID, def.ID).First(&fv).Error)
	assert.JSONEq(t, `"green"`, string(fv.Value))
}

func TestPromoteImportedCustomFields_NonXPropIgnored(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Favorite Color", Key: "favorite_color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	contact := &models.Contact{UserID: user.ID, VCardUID: "33333333-3333-4333-8333-333333333331"}
	props := []contactmodel.JCardProp{
		{Name: "x-favorite-color", Type: "text", Value: json.RawMessage(`"green"`)},
		// Not an X- property — the plan's name never applies to it.
		{Name: "note", Type: "text", Value: json.RawMessage(`"plain"`)},
	}
	plan := importFieldPlan{"x-favorite-color": def}

	promoted, notes := promoteImportedCustomFields(db, user.ID, contact, props, plan)
	assert.Equal(t, []string{"x-favorite-color"}, promoted)
	assert.Empty(t, notes)

	// An empty plan / empty target short-circuits cleanly.
	assert.Nil(t, mustPromote(t, db, user.ID, contact, props, importFieldPlan{}))
	_, _ = promoteImportedCustomFields(db, user.ID, &models.Contact{}, props, plan)
}

// mustPromote is a tiny wrapper keeping the no-plan assertion one line.
func mustPromote(t *testing.T, db *gorm.DB, userID uint, target *models.Contact, props []contactmodel.JCardProp, plan importFieldPlan) []string {
	t.Helper()
	promoted, _ := promoteImportedCustomFields(db, userID, target, props, plan)
	return promoted
}

// TestPromoteImportedCustomFields_UpsertFailureWarns covers the degradation
// when the field_values write itself fails (a schema without the table): the
// value is skipped with a note rather than failing the import.
func TestPromoteImportedCustomFields_UpsertFailureWarns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Contact{}, &models.FieldDefinition{})) // no field_values

	def := models.FieldDefinition{
		UserID: 1, Label: "Color", Key: "color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	contact := &models.Contact{UserID: 1, VCardUID: "33333333-3333-4333-8333-333333333332"}
	props := []contactmodel.JCardProp{
		{Name: "x-color", Type: "text", Value: json.RawMessage(`"green"`)},
	}
	plan := importFieldPlan{"x-color": def}

	promoted, notes := promoteImportedCustomFields(db, 1, contact, props, plan)
	assert.Empty(t, promoted)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "failed to store value")
}

func TestStripPromotedProps(t *testing.T) {
	contact := &models.Contact{Passthrough: contactmodel.Passthrough{VCard: []contactmodel.JCardProp{
		{Name: "x-favorite-color", Type: "text", Value: json.RawMessage(`"green"`)},
		{Name: "x-hometown", Type: "text", Value: json.RawMessage(`"Springfield"`)},
		{Name: "note", Type: "text", Value: json.RawMessage(`"plain"`)},
	}}}

	// Only the named property is removed; everything else survives.
	stripPromotedProps(contact, []string{"x-favorite-color"})
	require.Len(t, contact.Passthrough.VCard, 2)
	assert.Equal(t, "x-hometown", contact.Passthrough.VCard[0].Name)
	assert.Equal(t, "note", contact.Passthrough.VCard[1].Name)

	// Nil/empty inputs are no-ops.
	stripPromotedProps(nil, []string{"x"})
	stripPromotedProps(contact, nil)
	assert.Len(t, contact.Passthrough.VCard, 2)
}

func TestPromoteImportedCustomFields_InvalidValueWarnsNotFails(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Age", Key: "age",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeNumber,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	contact := &models.Contact{UserID: user.ID, VCardUID: "33333333-3333-4333-8333-333333333333"}
	props := []contactmodel.JCardProp{
		{Name: "x-age", Type: "text", Value: json.RawMessage(`"42"`)},  // coerces to number
		{Name: "x-age", Type: "text", Value: json.RawMessage(`"abc"`)}, // rejected
	}
	plan := importFieldPlan{"x-age": def}

	promoted, notes := promoteImportedCustomFields(db, user.ID, contact, props, plan)
	assert.Equal(t, []string{"x-age"}, promoted, "the first (valid) value promotes")
	require.Len(t, notes, 1, "the invalid value warns instead of failing")
	assert.Contains(t, notes[0], "not stored for field")
	assert.Contains(t, notes[0], "Age")

	var fv models.FieldValue
	require.NoError(t, db.Where("entity_id = ? AND field_definition_id = ?", contact.VCardUID, def.ID).First(&fv).Error)
	assert.JSONEq(t, `42`, string(fv.Value))
}

// --- integration: export -> re-import round trip (verification item 1) ----------

func TestConfirmVCF_CustomFieldExportReimportRoundTrip(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	// A definition projected to vCard, with one contact value. The contact row
	// itself need not exist for the projection query (field_values keys on the
	// VCardUID), so build an in-memory card and export it the same way
	// export_controller.go does.
	def := models.FieldDefinition{
		UserID: user.ID, Label: "Favorite Color", Key: "favorite_color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "vcard:X-FAVORITE-COLOR", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	source := &models.Contact{UserID: user.ID, Firstname: "Casey", Lastname: "Mapper", VCardUID: "44444444-4444-4444-8444-444444444444"}
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: def.ID, UserID: user.ID, EntityID: source.VCardUID,
		Value: json.RawMessage(`"green"`),
	}).Error)

	rec := models.RecordForContactFiltered(source, "", db, nil)
	require.NotNil(t, rec)
	exported, diags, err := vcard4.Adapter{}.Export(rec)
	require.NoError(t, err)
	require.Len(t, diags, 0)
	assert.Contains(t, string(exported), "X-FAVORITE-COLOR:green", "the projected field must ride out as a named extension property")

	// The preview surfaces it as a candidate with the match pre-resolved.
	contacts, previews, _, err := ParseVCF(strings.NewReader(string(exported)), db, user.ID)
	require.NoError(t, err)
	require.Len(t, previews, 1)
	require.Len(t, previews[0].CustomFieldCandidates, 1)
	cand := previews[0].CustomFieldCandidates[0]
	assert.Equal(t, "x-favorite-color", cand.Name)
	assert.Equal(t, "green", cand.Value)
	assert.Equal(t, def.ID, cand.MatchedDefinitionID, "re-import must recognize the exported property")

	// Confirm with no wizard input: the projection auto-map promotes it.
	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	result, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, result.Created)

	var reimported models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "Casey").First(&reimported).Error)

	// The same custom field — the definition is reused, not duplicated.
	var fv models.FieldValue
	require.NoError(t, db.Where("entity_id = ? AND field_definition_id = ?", reimported.VCardUID, def.ID).First(&fv).Error)
	assert.JSONEq(t, `"green"`, string(fv.Value))

	// The promoted property was stripped from the saved passthrough so export
	// won't emit it twice (the FieldValue projects it).
	require.NoError(t, db.Model(&reimported).Select("passthrough").First(&reimported).Error)
	for _, p := range reimported.Passthrough.VCard {
		assert.NotEqual(t, "x-favorite-color", p.Name, "promoted property must be stripped from passthrough")
	}

	// No duplicate definition created for the promoted field.
	var defCount int64
	require.NoError(t, db.Model(&models.FieldDefinition{}).Where("user_id = ? AND key = ?", user.ID, "favorite_color").Count(&defCount).Error)
	assert.Equal(t, int64(1), defCount)
}

// --- integration: unmapped property survives passthrough (verification item 3) ---

func TestConfirmVCF_UnmappedXPropertyStaysInPassthrough(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	raw := fieldMapVCard("X-HOMETOWN:Springfield")
	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, previews, 1)
	require.Len(t, previews[0].CustomFieldCandidates, 1)
	assert.Empty(t, previews[0].CustomFieldCandidates[0].MatchedDefinitionID)

	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	_, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "Casey").First(&contact).Error)
	require.NoError(t, db.Model(&contact).Select("passthrough").First(&contact).Error)

	// Fidelity preserved: the unmapped property rides in passthrough verbatim.
	require.Len(t, contact.Passthrough.VCard, 1)
	assert.Equal(t, "x-hometown", contact.Passthrough.VCard[0].Name)
	assert.JSONEq(t, `"Springfield"`, string(contact.Passthrough.VCard[0].Value))

	// And no definition or value was created.
	var defCount, valCount int64
	require.NoError(t, db.Model(&models.FieldDefinition{}).Where("user_id = ?", user.ID).Count(&defCount).Error)
	require.NoError(t, db.Model(&models.FieldValue{}).Where("entity_id = ?", contact.VCardUID).Count(&valCount).Error)
	assert.Zero(t, defCount)
	assert.Zero(t, valCount)
}

// --- integration: wizard "create" promotes a third-party property ----------------

func TestConfirmVCF_WizardCreatePromotesToNewDefinition(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	raw := fieldMapVCard("X-HOMETOWN:Springfield")
	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)

	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	result, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		FieldMappings: []models.ImportFieldMapping{
			{PropertyName: "x-hometown", Action: "create"},
		},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, result.Created)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "Casey").First(&contact).Error)
	require.NoError(t, db.Model(&contact).Select("passthrough").First(&contact).Error)
	assert.Empty(t, contact.Passthrough.VCard, "the promoted property must be stripped from passthrough")

	var def models.FieldDefinition
	require.NoError(t, db.Where("user_id = ? AND key = ?", user.ID, "hometown").First(&def).Error)
	assert.Equal(t, "Hometown", def.Label)
	assert.Equal(t, models.FieldTypeText, def.Type)
	assert.Equal(t, "vcard:X-HOMETOWN", def.Projection, "a promoted definition must round-trip on export")

	var fv models.FieldValue
	require.NoError(t, db.Where("entity_id = ? AND field_definition_id = ?", contact.VCardUID, def.ID).First(&fv).Error)
	assert.JSONEq(t, `"Springfield"`, string(fv.Value))
}

// --- integration: wizard "map" routes into an existing definition ---------------

func TestConfirmVCF_WizardMapPromotesIntoExistingDefinition(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	existing := models.FieldDefinition{
		UserID: user.ID, Label: "Hometown", Key: "hometown",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&existing).Error)

	raw := fieldMapVCard("X-HOMETOWN:Springfield")
	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)

	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	_, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		FieldMappings: []models.ImportFieldMapping{
			{PropertyName: "x-hometown", Action: "map", FieldDefinitionID: existing.ID},
		},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "Casey").First(&contact).Error)

	var fv models.FieldValue
	require.NoError(t, db.Where("entity_id = ? AND field_definition_id = ?", contact.VCardUID, existing.ID).First(&fv).Error)
	assert.JSONEq(t, `"Springfield"`, string(fv.Value))

	var count int64
	require.NoError(t, db.Model(&models.FieldDefinition{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "mapping must not create a duplicate definition")
}

// --- integration: wizard "ignore" beats the projection auto-map ------------------

func TestConfirmVCF_WizardIgnoreKeepsProjectionMatchInPassthrough(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Favorite Color", Key: "favorite_color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "vcard:X-FAVORITE-COLOR", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	raw := fieldMapVCard("X-FAVORITE-COLOR:green")
	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, previews[0].CustomFieldCandidates, 1)
	assert.Equal(t, def.ID, previews[0].CustomFieldCandidates[0].MatchedDefinitionID)

	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	_, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		FieldMappings: []models.ImportFieldMapping{
			{PropertyName: "x-favorite-color", Action: "ignore"},
		},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "Casey").First(&contact).Error)
	require.NoError(t, db.Model(&contact).Select("passthrough").First(&contact).Error)
	require.Len(t, contact.Passthrough.VCard, 1, "an explicit ignore must keep the property in passthrough")
	assert.Equal(t, "x-favorite-color", contact.Passthrough.VCard[0].Name)

	var valCount int64
	require.NoError(t, db.Model(&models.FieldValue{}).Where("entity_id = ?", contact.VCardUID).Count(&valCount).Error)
	assert.Zero(t, valCount)
}

// --- integration: "update" path promotes onto the matched existing contact ------

func TestConfirmVCF_UpdatePromotesOntoExistingContact(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	// A projected definition so the projection auto-map promotes the incoming
	// X- property during the update.
	def := models.FieldDefinition{
		UserID: user.ID, Label: "Favorite Color", Key: "favorite_color",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "vcard:X-FAVORITE-COLOR", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&def).Error)

	existingContact := models.Contact{UserID: user.ID, Firstname: "Casey", Lastname: "Mapper", Email: "casey@example.com"}
	require.NoError(t, db.Create(&existingContact).Error)

	raw := fieldMapVCard("X-FAVORITE-COLOR:green")
	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.NotNil(t, previews[0].DuplicateMatch, "the import must detect the duplicate so the update path runs")
	assert.Equal(t, "update", previews[0].SuggestedAction)

	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	result, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, result.Updated)

	var fv models.FieldValue
	require.NoError(t, db.Where("entity_id = ?", existingContact.VCardUID).First(&fv).Error)
	assert.JSONEq(t, `"green"`, string(fv.Value))
	assert.Equal(t, existingContact.VCardUID, fv.EntityID, "the value must land on the matched existing contact")
}

// --- integration: invalid value warns, contact still imports ---------------------

func TestConfirmVCF_InvalidMappedValueWarnsContactStillCreated(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	age := models.FieldDefinition{
		UserID: user.ID, Label: "Age", Key: "age",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeNumber,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&age).Error)

	// A non-numeric value for a number field: the wizard chose the
	// definition, so the value is skipped with a warn rather than failing the
	// import.
	raw := fieldMapVCard("X-AGE:not-a-number")
	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)

	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	result, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		FieldMappings: []models.ImportFieldMapping{
			{PropertyName: "x-age", Action: "map", FieldDefinitionID: age.ID},
		},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, result.Created, "the contact imports even though the value was rejected")

	var valueCount int64
	require.NoError(t, db.Model(&models.FieldValue{}).Where("field_definition_id = ?", age.ID).Count(&valueCount).Error)
	assert.Zero(t, valueCount, "the invalid value must not be stored")
}

// --- integration: invalid mapping fails the whole confirm closed ----------------

func TestConfirmVCF_InvalidMappingFailsClosed(t *testing.T) {
	db := dbtest.New(t)
	user := createImportFieldMapTestUser(t, db)

	raw := fieldMapVCard("X-HOMETOWN:Springfield")
	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)

	m := NewImportSessionManager()
	sessionID := m.CreateVCFSession(user.ID, contacts, previews)
	_, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		FieldMappings: []models.ImportFieldMapping{
			{PropertyName: "x-hometown", Action: "map", FieldDefinitionID: "00000000-0000-4000-8000-000000000000"},
		},
	}, testImportConfig(t), testImportLogger())
	require.NotNil(t, appErr)
	assert.Equal(t, apperrors.ErrCodeInvalidInput, appErr.Code)

	// Failed closed: nothing was created, and the session stays consumable.
	var contactCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contactCount).Error)
	assert.Zero(t, contactCount)

	_, sessErr := m.get(sessionID, user.ID)
	require.Nil(t, sessErr, "an invalid mapping must not consume the session")
}
