package services

import (
	"fmt"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Source-agnostic pieces of an import assistant's session — everything from
// the review step on. The Monica (#549) and Meerkat (#550) managers differ
// only in how they acquire the ImportSourcePlan (a live API fetch vs a parsed
// database file); the preview build, the review→engine action translation,
// and the result projection are identical and live here.

// buildSourceImportPreview turns a mapped plan into review rows: parsed
// fields, validation, and duplicate detection with a merge diff (the same
// BuildImportRowPreview every file import uses), plus the per-contact tally
// of graph entities the import will create.
func buildSourceImportPreview(db *gorm.DB, userID uint, plan *ImportSourcePlan) []models.SourceImportRowPreview {
	refIndex := make(map[string]int, len(plan.Contacts))
	for i := range plan.Contacts {
		refIndex[plan.Contacts[i].Ref.ExternalID] = i
	}

	related := make([]models.SourceRelatedCounts, len(plan.Contacts))
	bump := func(r SourceRef, f func(*models.SourceRelatedCounts)) {
		if i, ok := refIndex[r.ExternalID]; ok {
			f(&related[i])
		}
	}
	for _, n := range plan.Notes {
		bump(n.Contact, func(c *models.SourceRelatedCounts) { c.Notes++ })
	}
	for _, a := range plan.Activities {
		for _, cr := range a.Contacts {
			bump(cr, func(c *models.SourceRelatedCounts) { c.Activities++ })
		}
	}
	for _, r := range plan.Reminders {
		bump(r.Contact, func(c *models.SourceRelatedCounts) { c.Reminders++ })
	}
	for _, g := range plan.Gifts {
		bump(g.Contact, func(c *models.SourceRelatedCounts) { c.Gifts++ })
	}
	for _, rel := range plan.Relationships {
		bump(rel.Source, func(c *models.SourceRelatedCounts) { c.Relationships++ })
		bump(rel.Target, func(c *models.SourceRelatedCounts) { c.Relationships++ })
	}

	previews := make([]models.SourceImportRowPreview, 0, len(plan.Contacts))
	var batch []*models.Contact
	var stats ImportStats
	for i := range plan.Contacts {
		contact := &models.Contact{}
		models.ApplyRecordToContact(contact, plan.Contacts[i].Record, "")
		row := BuildImportRowPreview(db, userID, contact, i, batch, nil, &stats)
		batch = append(batch, contact)
		previews = append(previews, models.SourceImportRowPreview{
			ImportRowPreview: row,
			Related:          related[i],
			HasPhoto:         plan.Contacts[i].PhotoURL != "",
		})
	}
	return previews
}

// previewTotals sums a preview's rows for the review-step summary numbers.
func previewTotals(previews []models.SourceImportRowPreview, planRelationships int) (validRows, dupCount, errCount int, totals models.SourceRelatedCounts) {
	for _, row := range previews {
		if len(row.ValidationErrors) > 0 {
			errCount++
		} else {
			validRows++
		}
		if row.DuplicateMatch != nil {
			dupCount++
		}
		totals.Activities += row.Related.Activities
		totals.Notes += row.Related.Notes
		totals.Reminders += row.Related.Reminders
		totals.Gifts += row.Related.Gifts
	}
	totals.Relationships = planRelationships
	return
}

// resolveSourceContactActions turns the review step's RowImportActions into
// the engine's per-contact action map, keyed by SourceRef.ExternalID. A row
// with no action defaults to skip (never import what the user did not tick).
// "update" needs the local contact's VCardUID, resolved from the row's
// DuplicateMatch; an unresolvable target falls back to a plain add.
func resolveSourceContactActions(db *gorm.DB, userID uint, plan *ImportSourcePlan, previews []models.SourceImportRowPreview, rows []models.RowImportAction) (map[string]SourceContactAction, *apperrors.AppError) {
	byRow := make(map[int]string, len(rows))
	for _, r := range rows {
		byRow[r.RowIndex] = r.Action
	}

	var targetIDs []uint
	for i := range previews {
		if byRow[previews[i].RowIndex] == "update" && previews[i].DuplicateMatch != nil {
			targetIDs = append(targetIDs, previews[i].DuplicateMatch.ExistingContactID)
		}
	}
	uidByID := map[uint]string{}
	if len(targetIDs) > 0 {
		var existing []models.Contact
		if err := db.Where("user_id = ? AND id IN ?", userID, targetIDs).
			Select("id", "vcard_uid").Find(&existing).Error; err != nil { // # pragma: no cover — defensive: a healthy schema does not fail this select
			return nil, apperrors.ErrDatabase("Failed to load merge targets").WithError(err)
		}
		for _, c := range existing {
			uidByID[c.ID] = c.VCardUID
		}
	}

	actions := make(map[string]SourceContactAction, len(plan.Contacts))
	for i := range plan.Contacts {
		extID := plan.Contacts[i].Ref.ExternalID
		switch byRow[i] {
		case "add":
			actions[extID] = SourceContactAction{Action: SourceActionAdd}
		case "update":
			var uid string
			if i < len(previews) && previews[i].DuplicateMatch != nil {
				uid = uidByID[previews[i].DuplicateMatch.ExistingContactID]
			}
			if uid == "" {
				actions[extID] = SourceContactAction{Action: SourceActionAdd}
			} else {
				actions[extID] = SourceContactAction{Action: SourceActionMerge, MergeTargetUID: uid}
			}
		default:
			actions[extID] = SourceContactAction{Action: SourceActionSkip}
		}
	}
	return actions, nil
}

// sourceImportResultFromReport projects the shared engine report onto the
// wizard's result DTO. Only category "invalid" issues become named errors;
// the rest belong to the loss report shown before confirm.
func sourceImportResultFromReport(report *ImportReport, totalRows int) models.SourceImportResult {
	errs := []string{}
	for _, iss := range report.Issues {
		if iss.Category == ImportIssueCategoryInvalid {
			errs = append(errs, fmt.Sprintf("%s (%s): %s", iss.Record, iss.Field, iss.Message))
		}
	}
	return models.SourceImportResult{
		ImportResult: models.ImportResult{
			TotalProcessed: totalRows,
			Created:        report.ContactsCreated,
			Updated:        report.ContactsUpdated,
			Skipped:        report.ContactsSkipped,
			Errors:         errs,
		},
		RelationshipsCreated: report.RelationshipsCreated,
		NotesCreated:         report.NotesCreated,
		ActivitiesCreated:    report.ActivitiesCreated,
		RemindersCreated:     report.RemindersCreated,
		GiftsCreated:         report.GiftsCreated,
		CustomFieldsCreated:  report.CustomFieldsCreated,
	}
}

// mapSourceImportIssues converts the engine's issue list to the wire shape.
func mapSourceImportIssues(issues []ImportIssue) []models.SourceImportIssue {
	out := make([]models.SourceImportIssue, 0, len(issues))
	for _, iss := range issues {
		out = append(out, models.SourceImportIssue{
			Record:   iss.Record,
			Field:    iss.Field,
			Category: iss.Category,
			Message:  iss.Message,
		})
	}
	return out
}
