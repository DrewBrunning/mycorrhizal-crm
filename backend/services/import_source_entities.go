package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Account-bundle entity import (issue #2300, ADR 0043 decision 7).
//
// Entities arrive ONLY from an account bundle (the user's own full-fidelity
// backup). No other import path -- vCard/JSContact, CardDAV, Monica, Meerkat,
// CSV column mapping -- creates, matches or rewrites an entity (decisions 3
// and 7); TestNoImportOrSyncPathTouchesEntities pins that.
//
// IDs follow the rest of the bundle engine (issue #1355): a row is never
// inserted under the source's primary key, the import ledger makes a re-run
// idempotent, and intra-bundle references are rewritten through importIDMap.
// On top of that, an entity or link whose bundle id is already a row the
// account owns (live or trashed) is a per-row conflict, reported and skipped:
// the existing row is never overwritten and no duplicate is created.

// MappedEntity is one entity keyed by its stable UUID.
type MappedEntity struct {
	Ref         SourceRef
	ID          string
	Kind        string
	Name        string
	Sensitivity string
	Core        json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MappedEntityLink is one contact-to-entity link keyed by its stable UUID.
type MappedEntityLink struct {
	Ref         SourceRef
	ID          string
	Contact     SourceRef
	EntityID    string
	Role        string
	RoleLabel   string
	IsPrimary   bool
	Start       *contactmodel.PartialDate
	End         *contactmodel.PartialDate
	Sensitivity string
}

// MappedEntityFieldValue is one custom-field value of an entity.
type MappedEntityFieldValue struct {
	Ref               SourceRef
	EntityID          string
	FieldDefinitionID string
	Value             json.RawMessage
}

// mapBundleEntities carries the bundle's entity collections into the plan.
func mapBundleEntities(bundle *models.AccountBundle, plan *ImportSourcePlan) {
	for _, e := range bundle.Plan.Entities {
		plan.Entities = append(plan.Entities, MappedEntity{
			Ref: bundleRef(sourceKindEntity, e.ID), ID: e.ID, Kind: e.Kind, Name: e.Name,
			Sensitivity: e.Sensitivity, Core: e.Core, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		})
	}
	for _, l := range bundle.Plan.EntityLinks {
		plan.EntityLinks = append(plan.EntityLinks, MappedEntityLink{
			Ref: bundleRef(sourceKindEntityLink, l.ID), ID: l.ID, Contact: bundleContactRef(l.ContactVCardUID),
			EntityID: l.EntityID, Role: l.Role, RoleLabel: l.RoleLabel, IsPrimary: l.IsPrimary,
			Start: l.Start, End: l.End, Sensitivity: l.Sensitivity,
		})
	}
	for _, v := range bundle.Plan.EntityFieldValues {
		plan.EntityFieldValues = append(plan.EntityFieldValues, MappedEntityFieldValue{
			Ref:               bundleRef("entity_field_value", v.FieldDefinitionID+"/"+v.EntityID),
			EntityID:          v.EntityID,
			FieldDefinitionID: v.FieldDefinitionID,
			Value:             v.Value,
		})
	}
}

func validEntitySensitivity(s string) bool {
	return s == models.RelationshipSensitivityNormal || s == models.RelationshipSensitivityPrivate || s == models.RelationshipSensitivitySecret
}

// idTakenByAccount reports whether the account already owns a row of table
// with this primary key, trashed rows included (a soft-deleted row still owns
// its id, so inserting under it would collide).
func idTakenByAccount(tx *gorm.DB, userID uint, model any, id string) (bool, error) {
	var n int64
	if err := tx.Unscoped().Model(model).Where("user_id = ? AND id = ?", userID, id).Count(&n).Error; err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
		return false, err // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
	}
	return n > 0, nil
}

func importEntities(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), ids importIDMap, skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	kindOf := map[string]string{} // source entity id -> kind, for the links and values that follow

	for _, e := range plan.Entities {
		record := e.Ref.String()
		if skipImported(record, e.Ref) {
			// Imported by an earlier run: remember the local row so a link or
			// value that is new in this run still resolves to it.
			var led models.ImportSourceLink
			if tx.Where("user_id = ? AND system = ? AND external_id = ?", userID, plan.System, e.Ref.ExternalID).First(&led).Error == nil {
				ids.remember(e.Ref, led.EntityUID)
			}
			continue
		}
		core := string(e.Core)
		switch {
		case !validEntityKind(e.Kind):
			report.appendIssue(ImportIssue{Record: record, Field: "entity", Category: ImportIssueCategoryInvalid, Message: "unknown entity kind; the entity was not imported"})
			continue
		case e.Name == "":
			report.appendIssue(ImportIssue{Record: record, Field: "entity", Category: ImportIssueCategoryInvalid, Message: "an entity needs a name; it was not imported"})
			continue
		case !validEntitySensitivity(e.Sensitivity):
			report.appendIssue(ImportIssue{Record: record, Field: "entity", Category: ImportIssueCategoryInvalid, Message: "unknown sensitivity; the entity was not imported"})
			continue
		}
		if err := models.ValidateEntityCore(e.Kind, core); err != nil {
			report.appendIssue(ImportIssue{Record: record, Field: "entity", Category: ImportIssueCategoryInvalid, Message: "the entity's details are not valid for its kind; it was not imported"})
			continue
		}
		taken, err := idTakenByAccount(tx, userID, &models.Entity{}, e.ID)
		if err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err
		}
		if taken {
			report.appendIssue(ImportIssue{Record: record, Field: "entity", Category: ImportIssueCategorySkipped, Message: "an entity with this id already exists in this account; it was not overwritten"})
			continue
		}
		entity := models.Entity{
			UserID: userID, Kind: e.Kind, Name: e.Name, Sensitivity: e.Sensitivity, Core: core,
			CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		}
		if err := tx.Create(&entity).Error; err != nil { // # pragma: no cover — defensive: the row is validated up front, so the insert only fails on a database fault
			report.appendIssue(writeFailureIssue(record, "entity", err))
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, e.Ref.ExternalID,
			models.ImportSourceLinkKindEntity, entity.ID); err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		ids.remember(e.Ref, entity.ID)
		kindOf[e.ID] = entity.Kind
		imported[e.Ref.ExternalID] = true
	}

	for _, l := range plan.EntityLinks {
		record := l.Ref.String()
		if skipImported(record, l.Ref) {
			continue
		}
		contactUID, ok := uidOf(record, l.Contact)
		if !ok {
			continue
		}
		entityID, ok := ids.resolve(sourceKindEntity, l.EntityID)
		if !ok {
			report.appendIssue(ImportIssue{Record: record, Field: "entity_link.entity_id", Category: ImportIssueCategoryUnsupported, Message: "references an entity that was not imported"})
			continue
		}
		kind := kindOf[l.EntityID]
		if kind == "" { // entity imported by an earlier run: read its kind
			var e models.Entity
			if err := tx.Select("kind").Where("user_id = ? AND id = ?", userID, entityID).First(&e).Error; err != nil {
				report.appendIssue(ImportIssue{Record: record, Field: "entity_link.entity_id", Category: ImportIssueCategoryUnsupported, Message: "references an entity that was not imported"})
				continue
			}
			kind = e.Kind
		}
		if !models.ValidEntityLinkRole(kind, l.Role) {
			report.appendIssue(ImportIssue{Record: record, Field: "entity_link.role", Category: ImportIssueCategoryInvalid, Message: "the role is not valid for this kind of entity; the link was not imported"})
			continue
		}
		if !validEntitySensitivity(l.Sensitivity) {
			report.appendIssue(ImportIssue{Record: record, Field: "entity_link.sensitivity", Category: ImportIssueCategoryInvalid, Message: "unknown sensitivity; the link was not imported"})
			continue
		}
		taken, err := idTakenByAccount(tx, userID, &models.ContactEntityLink{}, l.ID)
		if err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err
		}
		if taken {
			report.appendIssue(ImportIssue{Record: record, Field: "entity_link", Category: ImportIssueCategorySkipped, Message: "an entity link with this id already exists in this account; it was not overwritten"})
			continue
		}
		link := models.ContactEntityLink{
			UserID: userID, ContactVCardUID: contactUID, EntityID: entityID, Kind: kind,
			Role: l.Role, RoleLabel: l.RoleLabel, IsPrimary: l.IsPrimary, Sensitivity: l.Sensitivity,
		}
		if l.Start != nil || l.End != nil {
			link.Period = &contactmodel.TemporalRange{Start: l.Start, End: l.End}
		}
		if err := CreateContactEntityLink(tx, &link); err != nil { // # pragma: no cover — defensive: the row is validated up front, so the insert only fails on a database fault
			report.appendIssue(writeFailureIssue(record, "entity_link", err))
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, l.Ref.ExternalID,
			models.ImportSourceLinkKindEntityLink, link.ID); err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		ids.remember(l.Ref, link.ID)
		imported[l.Ref.ExternalID] = true
	}

	for _, v := range plan.EntityFieldValues {
		record := v.Ref.String()
		if skipImported(record, v.Ref) {
			continue
		}
		entityID, ok := ids.resolve(sourceKindEntity, v.EntityID)
		if !ok {
			report.appendIssue(ImportIssue{Record: record, Field: "entity_field_value.entity_id", Category: ImportIssueCategoryUnsupported, Message: "references an entity that was not imported"})
			continue
		}
		defID, ok := ids.resolve(sourceKindFieldDefinition, v.FieldDefinitionID)
		var def models.FieldDefinition
		if !ok || errors.Is(tx.Where("user_id = ? AND id = ?", userID, defID).First(&def).Error, gorm.ErrRecordNotFound) {
			report.appendIssue(ImportIssue{Record: record, Field: "entity_field_value.field_definition_id", Category: ImportIssueCategoryUnsupported, Message: "references a field definition that was not imported"})
			continue
		}
		var entity models.Entity
		if err := tx.Select("kind").Where("user_id = ? AND id = ?", userID, entityID).First(&entity).Error; err != nil || entity.Kind != def.Target {
			report.appendIssue(ImportIssue{Record: record, Field: "entity_field_value.field_definition_id", Category: ImportIssueCategoryInvalid, Message: "the field's definition does not apply to this kind of entity; the value was not imported"})
			continue
		}
		fv := models.FieldValue{FieldDefinitionID: def.ID, UserID: userID, EntityID: entityID, Value: v.Value}
		if err := tx.Create(&fv).Error; err != nil { // # pragma: no cover — defensive: the plan keys a value by (definition, entity), so a duplicate never reaches the insert
			report.appendIssue(writeFailureIssue(record, "entity_field_value", err))
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, v.Ref.ExternalID,
			models.ImportSourceLinkKindCustomField, fmt.Sprintf("id:%d", fv.ID)); err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[v.Ref.ExternalID] = true
	}
	return nil
}

func validEntityKind(kind string) bool {
	for _, k := range models.EntityKinds() {
		if k == kind {
			return true
		}
	}
	return false
}
