package controllers

// Issue #970: the flat CSV export (`GET /api/v1/export`, ExportData in
// export_controller.go) is documented — docs/privacy.md, openapi.yaml's
// `/export` description, docs/security/data-retention-lifecycle.md §11 — as
// the user's own full personal-data backup that "holds everything". Until
// this file, it silently omitted five entire user-authored entity types
// (LifeEvent, Gift, ConversationAgenda, CadencePolicy, ReminderCompletion)
// and every Preference category except "food". A user relying on the
// promised single-file backup lost all of that data with no indication.
//
// Deliberately a separate file from export_controller.go rather than
// inlined into ExportData: several docs/security/asvs-l2.md rows cite
// exact `export_controller.go:NNN` line ranges for the functions/constants
// that already live past ExportData (ExportContactsAsVCF,
// ExportContactsAsJSContact, MaxAuditExportRows, ...). Growing ExportData in
// place would shift every one of those citations for no reason connected to
// what they're about; putting the new sections' fetch-and-write logic here
// keeps that file's line numbers stable. See CLAUDE.md's
// "citecheck line-shift tax" note.
//
// None of the five new entities have a Sensitivity field to filter (only
// Preference does), and per the #861 precedent this exporter never filters
// sensitivity anyway — see ExportData's own comments on the RELATIONSHIPS
// and food-preference sections it already writes.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// exportContactRef resolves a Contact's ID and display name for the CSV
// export's join sections. Shared between export_controller.go (which builds
// the maps from the already-fetched contacts) and this file (which reads
// them) since every new section here references a contact by either
// EntityID (Contact.VCardUID) or, for ReminderCompletion, a plain
// Contact.ID.
type exportContactRef struct {
	ID   uint
	Name string
}

// formatExportPartialDate renders a contactmodel.PartialDate for the CSV
// export using the same YYYY[-MM[-DD]] degradation as vcard4's
// formatPartialDate (vcard4/adapter.go) — that helper is unexported to
// package vcard4, so this is a deliberate small duplication rather than a
// new exported cross-package dependency for one call site.
func formatExportPartialDate(p *contactmodel.PartialDate) string {
	if p == nil {
		return ""
	}
	switch {
	case p.Year != nil && p.Month != nil && p.Day != nil:
		return fmt.Sprintf("%04d-%02d-%02d", *p.Year, *p.Month, *p.Day)
	case p.Year != nil && p.Month != nil:
		return fmt.Sprintf("%04d-%02d", *p.Year, *p.Month)
	case p.Year != nil:
		return fmt.Sprintf("%04d", *p.Year)
	case p.Month != nil && p.Day != nil:
		return fmt.Sprintf("--%02d-%02d", *p.Month, *p.Day)
	case p.Month != nil:
		return fmt.Sprintf("--%02d", *p.Month)
	case p.Day != nil:
		return fmt.Sprintf("---%02d", *p.Day)
	}
	return ""
}

// exportContactByEntityID resolves an EntityID (Contact.VCardUID) to its
// display ID/name, falling back to the raw EntityID for both when the
// referenced contact isn't found (mirrors ExportData's own RelationshipEdge
// fallback: a dangling reference still round-trips as something rather than
// vanishing from the row).
func exportContactByEntityID(contactByVCardUID map[string]exportContactRef, entityID string) (id string, name string) {
	if ref, ok := contactByVCardUID[entityID]; ok {
		return fmt.Sprintf("%d", ref.ID), ref.Name
	}
	return entityID, entityID
}

// writeExportExtraSections writes the LIFE_EVENTS, GIFTS,
// CONVERSATION_AGENDA, CADENCE_POLICIES, PREFERENCES, and
// REMINDER_COMPLETIONS sections onto the in-progress CSV backup. Returns
// false when a write failed and the caller (ExportData) has already had
// abortExportError called on its behalf — matching every other per-section
// helper's control-flow shape in this package.
func writeExportExtraSections(
	c *gin.Context,
	log *zerolog.Logger,
	buf *bytes.Buffer,
	writer *csv.Writer,
	contactByVCardUID map[string]exportContactRef,
	contactByID map[uint]exportContactRef,
	lifeEvents []models.LifeEvent,
	gifts []models.Gift,
	agendaItems []models.ConversationAgenda,
	cadencePolicies []models.CadencePolicy,
	preferences []models.Preference,
	reminderCompletions []models.ReminderCompletion,
) bool {
	if !writeExportLifeEvents(c, log, buf, writer, contactByVCardUID, lifeEvents) {
		return false
	}
	if !writeExportGifts(c, log, buf, writer, contactByVCardUID, gifts) {
		return false
	}
	if !writeExportConversationAgenda(c, log, buf, writer, contactByVCardUID, agendaItems) {
		return false
	}
	if !writeExportCadencePolicies(c, log, buf, writer, contactByVCardUID, cadencePolicies) {
		return false
	}
	if !writeExportPreferences(c, log, buf, writer, contactByVCardUID, preferences) {
		return false
	}
	if !writeExportReminderCompletions(c, log, buf, writer, contactByID, reminderCompletions) {
		return false
	}
	return true
}

func writeExportLifeEvents(c *gin.Context, log *zerolog.Logger, buf *bytes.Buffer, writer *csv.Writer, contactByVCardUID map[string]exportContactRef, lifeEvents []models.LifeEvent) bool {
	buf.WriteString("\n=== LIFE_EVENTS ===\n")

	headers := []string{
		"ID", "Contact ID", "Contact Name", "Type", "Category", "Date",
		"Description", "Source", "Related Contact Names", "Remind", "Created At", "Updated At",
	}
	if err := writer.Write(headers); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write life event headers", err)
		return false
	}

	for _, event := range lifeEvents {
		contactID, contactName := exportContactByEntityID(contactByVCardUID, event.EntityID)

		relatedNames := make([]string, 0, len(event.RelatedEntityIDs))
		for _, relatedID := range event.RelatedEntityIDs {
			_, name := exportContactByEntityID(contactByVCardUID, relatedID)
			relatedNames = append(relatedNames, name)
		}

		record := []string{
			event.ID,
			contactID,
			contactName,
			event.Type,
			event.Category,
			formatExportPartialDate(event.Date),
			event.Description,
			event.Source,
			strings.Join(relatedNames, "; "),
			fmt.Sprintf("%t", event.Remind),
			event.CreatedAt.Format(time.RFC3339),
			event.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write life event record", err)
			return false
		}
	}
	writer.Flush()
	return true
}

func writeExportGifts(c *gin.Context, log *zerolog.Logger, buf *bytes.Buffer, writer *csv.Writer, contactByVCardUID map[string]exportContactRef, gifts []models.Gift) bool {
	buf.WriteString("\n=== GIFTS ===\n")

	headers := []string{
		"ID", "Contact ID", "Contact Name", "Status", "Occasion", "Description",
		"URL", "Notes", "Date", "Value Cents", "Currency", "Life Event ID",
		"Activity ID", "Created At", "Updated At",
	}
	if err := writer.Write(headers); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write gift headers", err)
		return false
	}

	for _, gift := range gifts {
		contactID, contactName := exportContactByEntityID(contactByVCardUID, gift.EntityID)

		date := ""
		if gift.Date != nil {
			date = gift.Date.Format(time.RFC3339)
		}
		activityID := ""
		if gift.ActivityID != nil {
			activityID = fmt.Sprintf("%d", *gift.ActivityID)
		}

		record := []string{
			gift.ID,
			contactID,
			contactName,
			gift.Status,
			gift.Occasion,
			gift.Description,
			gift.URL,
			gift.Notes,
			date,
			fmt.Sprintf("%d", gift.ValueCents),
			gift.Currency,
			gift.LifeEventID,
			activityID,
			gift.CreatedAt.Format(time.RFC3339),
			gift.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write gift record", err)
			return false
		}
	}
	writer.Flush()
	return true
}

func writeExportConversationAgenda(c *gin.Context, log *zerolog.Logger, buf *bytes.Buffer, writer *csv.Writer, contactByVCardUID map[string]exportContactRef, agendaItems []models.ConversationAgenda) bool {
	buf.WriteString("\n=== CONVERSATION_AGENDA ===\n")

	headers := []string{
		"ID", "Contact ID", "Contact Name", "Content", "Reference URL",
		"Discussed At", "Activity ID", "Created At", "Updated At",
	}
	if err := writer.Write(headers); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write conversation agenda headers", err)
		return false
	}

	for _, item := range agendaItems {
		contactID, contactName := exportContactByEntityID(contactByVCardUID, item.EntityID)

		discussedAt := ""
		if item.DiscussedAt != nil {
			discussedAt = item.DiscussedAt.Format(time.RFC3339)
		}
		activityID := ""
		if item.ActivityID != nil {
			activityID = fmt.Sprintf("%d", *item.ActivityID)
		}

		record := []string{
			item.ID,
			contactID,
			contactName,
			item.Content,
			item.ReferenceURL,
			discussedAt,
			activityID,
			item.CreatedAt.Format(time.RFC3339),
			item.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write conversation agenda record", err)
			return false
		}
	}
	writer.Flush()
	return true
}

func writeExportCadencePolicies(c *gin.Context, log *zerolog.Logger, buf *bytes.Buffer, writer *csv.Writer, contactByVCardUID map[string]exportContactRef, cadencePolicies []models.CadencePolicy) bool {
	buf.WriteString("\n=== CADENCE_POLICIES ===\n")

	headers := []string{
		"ID", "Contact ID", "Contact Name", "Target Interval Days",
		"Qualifying Types", "Created At", "Updated At",
	}
	if err := writer.Write(headers); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write cadence policy headers", err)
		return false
	}

	for _, policy := range cadencePolicies {
		contactID, contactName := exportContactByEntityID(contactByVCardUID, policy.EntityID)

		record := []string{
			policy.ID,
			contactID,
			contactName,
			fmt.Sprintf("%d", policy.TargetIntervalDays),
			strings.Join(policy.QualifyingTypes, "; "),
			policy.CreatedAt.Format(time.RFC3339),
			policy.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write cadence policy record", err)
			return false
		}
	}
	writer.Flush()
	return true
}

// writeExportPreferences writes every Preference row across every category
// (not just "food", which the CONTACTS section's rollup column already
// covers) — the bulk of issue #970's finding, since drink/hobby/gift/
// dislike/clothing_size/flowers/fragrance/color/cause/jewelry_*/media_* were
// all previously absent from the export entirely. Deliberately includes
// every Sensitivity value unfiltered, same as every other section here.
func writeExportPreferences(c *gin.Context, log *zerolog.Logger, buf *bytes.Buffer, writer *csv.Writer, contactByVCardUID map[string]exportContactRef, preferences []models.Preference) bool {
	buf.WriteString("\n=== PREFERENCES ===\n")

	headers := []string{
		"ID", "Contact ID", "Contact Name", "Category", "Key", "Value", "Notes",
		"Source", "Confidence", "Last Confirmed", "Sensitivity", "Created At", "Updated At",
	}
	if err := writer.Write(headers); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write preference headers", err)
		return false
	}

	for _, pref := range preferences {
		contactID, contactName := exportContactByEntityID(contactByVCardUID, pref.EntityID)

		confidence := ""
		if pref.Confidence != nil {
			confidence = strconv.FormatFloat(*pref.Confidence, 'f', -1, 64)
		}
		lastConfirmed := ""
		if pref.LastConfirmed != nil {
			lastConfirmed = pref.LastConfirmed.Format(time.RFC3339)
		}

		record := []string{
			pref.ID,
			contactID,
			contactName,
			pref.Category,
			pref.Key,
			pref.Value,
			pref.Notes,
			pref.Source,
			confidence,
			lastConfirmed,
			pref.Sensitivity,
			pref.CreatedAt.Format(time.RFC3339),
			pref.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write preference record", err)
			return false
		}
	}
	writer.Flush()
	return true
}

// writeExportReminderCompletions is the one new section keyed by a plain
// Contact.ID (contactByID) rather than EntityID/VCardUID: ReminderCompletion
// predates that convention (models/reminder.go).
func writeExportReminderCompletions(c *gin.Context, log *zerolog.Logger, buf *bytes.Buffer, writer *csv.Writer, contactByID map[uint]exportContactRef, reminderCompletions []models.ReminderCompletion) bool {
	buf.WriteString("\n=== REMINDER_COMPLETIONS ===\n")

	headers := []string{
		"ID", "Reminder ID", "Contact ID", "Contact Name", "Message", "Completed At", "Created At", "Updated At",
	}
	if err := writer.Write(headers); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write reminder completion headers", err)
		return false
	}

	for _, completion := range reminderCompletions {
		reminderID := ""
		if completion.ReminderID != nil {
			reminderID = fmt.Sprintf("%d", *completion.ReminderID)
		}
		contactID := fmt.Sprintf("%d", completion.ContactID)
		contactName := ""
		if ref, ok := contactByID[completion.ContactID]; ok {
			contactName = ref.Name
		}

		record := []string{
			fmt.Sprintf("%d", completion.ID),
			reminderID,
			contactID,
			contactName,
			completion.Message,
			completion.CompletedAt.Format(time.RFC3339),
			completion.CreatedAt.Format(time.RFC3339),
			completion.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write reminder completion record", err)
			return false
		}
	}
	writer.Flush()
	return true
}
