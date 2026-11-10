package controllers

// Issue #2300 (ADR 0043 decision 7): the flat CSV backup (`GET /api/v1/export`)
// appends the reusable-entity sections after every existing one. Like the
// RELATIONSHIPS section it is the owner's full-fidelity backup: nothing is
// filtered by sensitivity, and every cell goes through csvSafe so an entity
// name or note cannot carry a spreadsheet formula. The CONTACTS section is
// untouched (no projected organisation column), so a pre-1.7 CSV round-trips
// exactly as before.
//
// A separate file from export_csv_extra_sections.go for the same reason that
// file is separate from export_controller.go: docs/security citations pin line
// ranges there (see CLAUDE.md's "citecheck line-shift tax").

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"time"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// CSV section names for the entity sections. services.ParseEntityCSVSections
// reads them back by the same markers.
const (
	csvSectionEntities     = "=== ENTITIES ==="
	csvSectionEntityLinks  = "=== ENTITY LINKS ==="
	csvSectionEntityFields = "=== ENTITY FIELDS ==="
)

// Header rows. "Contact UID" (links) is appended after the ADR 0043 column
// list: the numeric Contact ID is meaningless on another instance, so restore
// resolves a link's contact by its stable vCard UID.
var (
	csvEntityHeaders      = []string{"ID", "Kind", "Name", "Sensitivity", "Core (JSON)", "Notes", "Created At", "Updated At"}
	csvEntityLinkHeaders  = []string{"ID", "Contact ID", "Contact Name", "Entity ID", "Entity Name", "Kind", "Role", "Role Label", "Primary", "Start Date", "End Date", "Sensitivity", "Contact UID"}
	csvEntityFieldHeaders = []string{"Entity ID", "Definition Key", "Value"}
)

// entityNotes extracts the kind's free-text notes from its core, for the
// human-readable Notes column (the Core column is what restore reads).
func entityNotes(e models.Entity) string {
	core, err := e.DecodeCore()
	if err != nil {
		return ""
	}
	switch c := core.(type) {
	case *models.OrganisationCore:
		return c.Notes
	case *models.PlaceCore:
		return c.Notes
	case *models.PetCore:
		return c.Notes
	}
	return ""
}

func writeExportEntities(c *gin.Context, log *zerolog.Logger, buf *bytes.Buffer, writer *csv.Writer,
	db *gorm.DB, userID uint, contactByVCardUID map[string]exportContactRef) bool {
	fail := func(stage, msg string, err error) bool {
		abortExportError(c, log, "export:csv", stage, msg, err)
		return false
	}

	var entities []models.Entity
	if err := db.Where("user_id = ?", userID).Order("created_at ASC, id ASC").Find(&entities).Error; err != nil {
		return fail("database", "Failed to fetch entities for export", err)
	}
	var links []models.ContactEntityLink
	if err := db.Where("user_id = ?", userID).Order("created_at ASC, id ASC").Find(&links).Error; err != nil {
		return fail("database", "Failed to fetch entity links for export", err)
	}
	var defs []models.FieldDefinition
	if err := db.Where("user_id = ? AND target <> ?", userID, models.FieldDefinitionTargetContact).Find(&defs).Error; err != nil {
		return fail("database", "Failed to fetch entity field definitions for export", err)
	}
	defKey := make(map[string]string, len(defs))
	for _, d := range defs {
		defKey[d.ID] = d.Key
	}
	var values []models.FieldValue
	if len(defs) > 0 {
		if err := db.Where("user_id = ? AND field_definition_id IN ?", userID, mapKeys(defKey)).Order("id ASC").Find(&values).Error; err != nil {
			return fail("database", "Failed to fetch entity field values for export", err)
		}
	}
	entityName := make(map[string]string, len(entities))
	liveEntity := make(map[string]bool, len(entities))
	for _, e := range entities {
		entityName[e.ID] = e.Name
		liveEntity[e.ID] = true
	}

	buf.WriteString("\n" + csvSectionEntities + "\n")
	if err := writer.Write(csvEntityHeaders); err != nil {
		return fail("serialization", "Failed to write entity headers", err)
	}
	for _, e := range entities {
		core := e.Core
		if core == "" {
			core = "{}"
		}
		record := []string{e.ID, e.Kind, e.Name, e.Sensitivity, core, entityNotes(e),
			e.CreatedAt.Format(time.RFC3339), e.UpdatedAt.Format(time.RFC3339)}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			return fail("serialization", "Failed to write entity record", err)
		}
	}
	writer.Flush()

	buf.WriteString("\n" + csvSectionEntityLinks + "\n")
	if err := writer.Write(csvEntityLinkHeaders); err != nil {
		return fail("serialization", "Failed to write entity link headers", err)
	}
	for _, l := range links {
		if !liveEntity[l.EntityID] {
			continue
		}
		contactID, contactName := exportContactByEntityID(contactByVCardUID, l.ContactVCardUID)
		var start, end string
		if l.Period != nil {
			start, end = formatExportPartialDate(l.Period.Start), formatExportPartialDate(l.Period.End)
		}
		record := []string{l.ID, contactID, contactName, l.EntityID, entityName[l.EntityID], l.Kind, l.Role, l.RoleLabel,
			fmt.Sprintf("%t", l.IsPrimary), start, end, l.Sensitivity, l.ContactVCardUID}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			return fail("serialization", "Failed to write entity link record", err)
		}
	}
	writer.Flush()

	buf.WriteString("\n" + csvSectionEntityFields + "\n")
	if err := writer.Write(csvEntityFieldHeaders); err != nil {
		return fail("serialization", "Failed to write entity field headers", err)
	}
	for _, v := range values {
		if !liveEntity[v.EntityID] {
			continue
		}
		// The value is written as its raw JSON so every type (number, boolean,
		// list) restores exactly; csvSafe still neutralizes a leading '-'.
		record := []string{v.EntityID, defKey[v.FieldDefinitionID], string(v.Value)}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			return fail("serialization", "Failed to write entity field record", err)
		}
	}
	writer.Flush()
	return true
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
