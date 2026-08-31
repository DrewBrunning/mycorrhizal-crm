package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"mycorrhizal/contactmodel"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// This file is issue #514's import half of the custom-field/vCard bridge: a
// discovered unknown X-* property in a contact's Passthrough.VCard can be
// *promoted* into a first-class FieldValue — editable, searchable, merge-safe
// — via the import wizard, mirroring how the CSV column-mapping wizard routes
// a CSV column into a contact field. The export half already exists:
// projectCustomFields (models/contact_record.go) re-emits a definition whose
// Projection is "vcard:X-<NAME>" as a Passthrough.VCard JCardProp on every
// read/export, so the promotion's inverse (a FieldValue rides back out as the
// same X- property) is what closes the round-trip.
//
// Two entry points:
//
//   - DiscoverCustomFieldCandidates (BuildImportRowPreview, import_service.go)
//     reads the X-* properties a contact carries in passthrough and, for each,
//     whether an existing definition's projection name matches — the preview
//     surface the wizard renders.
//
//   - buildImportFieldPlan + promoteImportedCustomFields (ConfirmVCF,
//     import_session.go) resolve the wizard's ImportFieldMapping decisions
//     into a per-property plan, materialize FieldValues on the affected
//     contacts, and strip the promoted properties from the saved contact's
//     passthrough so the value is not double-represented on export.
//
// Unmapped properties stay in passthrough untouched — fidelity is preserved
// (already pinned by the adapters' passthrough round-trip tests); this is a
// promotion step, not a loss fix.

// customFieldCandidateProps returns the X-* properties in a contact's
// Passthrough.VCard — the unknown vCard extension properties that can be
// promoted to custom fields (issue #514). Only names starting with "x-"
// qualify: that is the vCard extension-property namespace, and it is the
// naming convention the projection validator (middleware/validation.go's
// fieldDefinitionProjectionPattern) requires for projected definitions, so a
// round-trip re-import of an exported custom field always lands here.
func customFieldCandidateProps(contact *models.Contact) []contactmodel.JCardProp {
	if contact == nil {
		return nil
	}
	var out []contactmodel.JCardProp
	for _, p := range contact.Passthrough.VCard {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(p.Name)), "x-") {
			out = append(out, p)
		}
	}
	return out
}

// jcardPropValueString renders a JCardProp's value as a string for display in
// the wizard (the stored value is a JSON scalar; a plain-text vCard property
// comes through as a JSON string, which unmarshals cleanly).
func jcardPropValueString(p contactmodel.JCardProp) string {
	var s string
	if err := json.Unmarshal(p.Value, &s); err == nil {
		return s
	}
	return string(p.Value)
}

// DiscoverCustomFieldCandidates returns the X-* properties in contact's
// passthrough that can be promoted to custom fields, each with its first
// value rendered and — when an existing FieldDefinition's vcard:X-<NAME>
// projection matches the property name (case-insensitively) — the matched
// definition's ID and label so the wizard can pre-select the mapping. A
// nil-safe, best-effort lookup: a query failure degrades to candidates with
// no match rather than failing the whole preview (same discipline as
// projectCustomFields). db nil skips the match lookup entirely.
func DiscoverCustomFieldCandidates(db *gorm.DB, userID uint, contact *models.Contact) []models.DiscoveredProperty {
	props := customFieldCandidateProps(contact)
	if len(props) == 0 {
		return nil
	}

	// Resolve the user's vcard-projected definitions once, keyed by the
	// projection name (lowercased), so the match lookup is a map access
	// instead of an N+1 query.
	projectionByName := map[string]models.FieldDefinition{}
	if db != nil && userID != 0 {
		var defs []models.FieldDefinition
		if err := db.Where("user_id = ? AND projection LIKE ?", userID, "vcard:%").Find(&defs).Error; err == nil {
			for _, d := range defs {
				if name := strings.TrimPrefix(d.Projection, "vcard:"); name != "" {
					projectionByName[strings.ToLower(name)] = d
				}
			}
		}
	}

	seen := map[string]bool{}
	out := make([]models.DiscoveredProperty, 0, len(props))
	for _, p := range props {
		lower := strings.ToLower(strings.TrimSpace(p.Name))
		if lower == "" || seen[lower] {
			continue
		}
		seen[lower] = true
		dp := models.DiscoveredProperty{Name: p.Name, Value: jcardPropValueString(p)}
		if d, ok := projectionByName[lower]; ok {
			dp.MatchedDefinitionID = d.ID
			dp.MatchedDefinitionLabel = d.Label
		}
		out = append(out, dp)
	}
	return out
}

// importFieldPlan maps a lowercased X-* property name to the FieldDefinition
// it should be promoted into. A property absent from the plan stays in
// passthrough (issue #514's fidelity rule).
type importFieldPlan map[string]models.FieldDefinition

// errInvalidImportFieldMapping wraps buildImportFieldPlan failures (an
// unknown/not-owned definition in a "map" action, a definition-create error)
// so ConfirmVCF can distinguish a client-contract error (400) from a database
// failure (500).
var errInvalidImportFieldMapping = errors.New("invalid import field mapping")

// buildImportFieldPlan resolves the wizard's ImportFieldMapping decisions
// into a per-property plan. Precedence, from strongest to weakest:
//
//  1. An explicit decision — "ignore" excludes the property entirely (even
//     suppressing a projection match), "map" targets the named definition,
//     "create" creates (or reuses) a definition whose key and vcard
//     projection are derived from the property name.
//  2. Auto-map: any property name that matches an existing definition's
//     vcard:X-<NAME> projection promotes into it when the contact carries the
//     property (the export round-trip guarantee — re-importing an exported
//     card lands the value back in the same custom field without wizard
//     interaction).
//
// Creating a definition is idempotent: the key is derived deterministically
// from the property name (e.g. X-FAVORITE-COLOR -> favorite_color), so a
// definition already present with that key is reused rather than colliding
// with the (user_id, key) unique index. The new definition's projection is
// set to "vcard:X-<NAME>" (uppercased) so the field round-trips on export.
func buildImportFieldPlan(tx *gorm.DB, userID uint, mappings []models.ImportFieldMapping) (importFieldPlan, error) {
	plan := importFieldPlan{}
	projectionByName := map[string]models.FieldDefinition{}

	var projected []models.FieldDefinition
	if err := tx.Where("user_id = ? AND projection LIKE ?", userID, "vcard:%").Find(&projected).Error; err != nil {
		// Best-effort auto-map, same discipline as projectCustomFields
		// (models/contact_record.go): a lookup failure degrades to "no
		// auto-match" rather than failing the whole import — including the
		// AutoMigrate test schemas that predate field_definitions. Explicit
		// wizard mappings below are unaffected (their per-definition lookups
		// fail loudly if the schema is genuinely broken).
		logger.Warn().Err(err).Uint("user_id", userID).Msg("buildImportFieldPlan: failed to load projected definitions; auto-match disabled")
	} else {
		for _, d := range projected {
			if name := strings.TrimPrefix(d.Projection, "vcard:"); name != "" {
				projectionByName[strings.ToLower(name)] = d
			}
		}
	}

	// decided records every property the wizard made an explicit decision
	// about, so an "ignore" also suppresses the projection-match auto-map
	// below (delete alone would leave the name eligible again).
	decided := map[string]bool{}
	for _, m := range mappings {
		lower := strings.ToLower(strings.TrimSpace(m.PropertyName))
		if lower == "" {
			continue
		}
		decided[lower] = true
		switch m.Action {
		case "ignore":
			// Explicitly excluded: leave it out of the plan entirely, which
			// also suppresses the projection-match auto-map below.
			delete(plan, lower)
		case "map":
			var def models.FieldDefinition
			if err := tx.Where("id = ? AND user_id = ?", m.FieldDefinitionID, userID).First(&def).Error; err != nil {
				return nil, fmt.Errorf("%w: field_definition_id %q is not one of your definitions: %v", errInvalidImportFieldMapping, m.FieldDefinitionID, err)
			}
			plan[lower] = def
		case "create":
			def, err := createOrReuseImportedFieldDefinition(tx, userID, lower, m.Label, m.Type)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", errInvalidImportFieldMapping, err)
			}
			plan[lower] = def
		}
	}

	// Auto-map: a projection match promotes even without a wizard decision.
	for name, def := range projectionByName {
		if decided[name] {
			continue
		}
		plan[name] = def
	}

	return plan, nil
}

// createOrReuseImportedFieldDefinition creates the FieldDefinition backing a
// "create" mapping decision, or reuses an existing definition with the same
// derived key (idempotent re-imports). The key is derived from the property
// name (X-FAVORITE-COLOR -> favorite_color); the projection is
// "vcard:X-<NAME>" (uppercased) so the promoted field round-trips on export.
func createOrReuseImportedFieldDefinition(tx *gorm.DB, userID uint, propLower, label, fieldType string) (models.FieldDefinition, error) {
	if fieldType == "" {
		fieldType = models.FieldTypeText
	}
	key := fieldKeyFromProperty(propLower)
	if key == "" {
		return models.FieldDefinition{}, fmt.Errorf("property %q has no usable name for a field key", propLower)
	}

	var existing models.FieldDefinition
	err := tx.Where("user_id = ? AND key = ?", userID, key).First(&existing).Error
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.FieldDefinition{}, fmt.Errorf("failed to check existing field definition: %v", err)
	}

	if label == "" {
		label = fieldLabelFromKey(key)
	}
	def := models.FieldDefinition{
		UserID:      userID,
		Label:       label,
		Key:         key,
		Target:      models.FieldDefinitionTargetContact,
		Type:        fieldType,
		Projection:  "vcard:X-" + strings.ToUpper(strings.TrimPrefix(propLower, "x-")),
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	if err := tx.Create(&def).Error; err != nil { // # pragma: no cover — the (user_id, key) unique index is pre-checked above and every NOT NULL column is set; unreachable on the real schema
		return models.FieldDefinition{}, fmt.Errorf("failed to create field definition: %v", err)
	}
	return def, nil
}

// fieldKeyFromProperty derives a stable machine key from a vCard property
// name: the "x-" extension prefix is dropped and any non-alphanumeric run
// collapses to a single underscore (X-FAVORITE-COLOR -> favorite_color).
func fieldKeyFromProperty(prop string) string {
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(prop)), "x-")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// fieldLabelFromKey renders a derived key as a display label
// (favorite_color -> "Favorite Color").
func fieldLabelFromKey(key string) string {
	words := strings.Split(key, "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// coerceImportedFieldValue best-effort converts a text vCard property value
// (which always arrives as a JSON string) into the JSON shape a non-string
// FieldDefinition expects, so a string "42" can promote into a number field
// and "true"/"1"/"yes" into a boolean field. Values that don't coerce are
// returned unchanged, where validation then rejects them. Multi fields are
// left alone (a single vCard value cannot populate a list without a real
// serialization decision).
func coerceImportedFieldValue(def models.FieldDefinition, raw json.RawMessage) json.RawMessage {
	if def.Constraints.Multi {
		return raw
	}
	switch def.Type {
	case models.FieldTypeNumber:
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if n, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				if b, err := json.Marshal(n); err == nil {
					return b
				}
			}
		}
	case models.FieldTypeBoolean:
		var s string
		if json.Unmarshal(raw, &s) == nil {
			switch strings.ToLower(strings.TrimSpace(s)) {
			case "true", "1", "yes":
				return json.RawMessage("true")
			case "false", "0", "no":
				return json.RawMessage("false")
			}
		}
	}
	return raw
}

// promoteImportedCustomFields materializes a FieldValue on target for every
// X-* property in props that the plan promotes into a definition, returning
// the names of the properties actually promoted (so the caller can strip them
// from the contact being saved) and any warnings (an invalid value or a
// write failure skips that value without failing the import — the definition
// was the user's explicit choice, so a mismatch is a warn, not a 500). Values
// are validated against the definition after coercion; unmapped properties
// are ignored and stay in passthrough.
func promoteImportedCustomFields(tx *gorm.DB, userID uint, target *models.Contact, props []contactmodel.JCardProp, plan importFieldPlan) (promoted []string, notes []string) {
	if target == nil || target.VCardUID == "" || len(plan) == 0 {
		return nil, nil
	}
	for _, p := range props {
		lower := strings.ToLower(strings.TrimSpace(p.Name))
		if !strings.HasPrefix(lower, "x-") {
			continue
		}
		def, ok := plan[lower]
		if !ok {
			continue
		}
		value := coerceImportedFieldValue(def, p.Value)
		if err := ValidateFieldValue(def, value); err != nil {
			notes = append(notes, fmt.Sprintf("X-%s: value not stored for field %q: %v", strings.TrimPrefix(lower, "x-"), def.Label, err))
			continue
		}
		if err := upsertFieldValue(tx, userID, target.VCardUID, def.ID, value); err != nil {
			notes = append(notes, fmt.Sprintf("X-%s: failed to store value for field %q: %v", strings.TrimPrefix(lower, "x-"), def.Label, err))
			continue
		}
		promoted = append(promoted, p.Name)
	}
	return promoted, notes
}

// upsertFieldValue creates or updates the single FieldValue of defID for
// entityID (the contact's VCardUID), mirroring ReplaceContactFieldValues'
// upsert semantics so an import doesn't collide with the (definition, entity)
// unique index.
func upsertFieldValue(tx *gorm.DB, userID uint, entityID, defID string, value json.RawMessage) error {
	var existing models.FieldValue
	err := tx.Where("field_definition_id = ? AND entity_id = ?", defID, entityID).First(&existing).Error
	switch {
	case err == nil:
		existing.Value = value
		return tx.Save(&existing).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return tx.Create(&models.FieldValue{
			FieldDefinitionID: defID,
			UserID:            userID,
			EntityID:          entityID,
			Value:             value,
		}).Error
	default:
		return err
	}
}

// stripPromotedProps removes the named properties from a contact's
// Passthrough.VCard, so a promoted value is not double-represented on export
// (the FieldValue itself projects via projectCustomFields; keeping the raw
// passthrough entry too would emit the X- property twice). Returns the
// stripped slice, never mutating the caller's backing array.
func stripPromotedProps(contact *models.Contact, promoted []string) {
	if contact == nil || len(promoted) == 0 {
		return
	}
	remove := make(map[string]bool, len(promoted))
	for _, n := range promoted {
		remove[strings.ToLower(strings.TrimSpace(n))] = true
	}
	kept := make([]contactmodel.JCardProp, 0, len(contact.Passthrough.VCard))
	for _, p := range contact.Passthrough.VCard {
		if remove[strings.ToLower(strings.TrimSpace(p.Name))] {
			continue
		}
		kept = append(kept, p)
	}
	contact.Passthrough.VCard = kept
}
