package controllers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/jscontact"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// csvFormulaLeaders are the characters spreadsheet applications treat as the
// start of a formula rather than text. Tab and carriage return are included
// because Excel strips leading whitespace before deciding.
const csvFormulaLeaders = "=+-@\t\r"

// csvSafe neutralizes spreadsheet formula injection.
//
// encoding/csv quotes delimiters and newlines, which makes the file parse
// correctly, but quoting does nothing about a value like
// `=HYPERLINK("http://attacker","click")` — Excel and LibreOffice still
// evaluate it on open. Contact data is not all self-authored: it arrives from
// CardDAV sync and VCF/CSV import, so a field can carry a payload chosen by
// someone else and fire when the user exports and opens the result.
//
// Prefixing with a single quote is the conventional fix: spreadsheets consume
// it as a "treat as text" marker, and any consumer that is not a spreadsheet
// sees one extra leading character rather than executable content.
func csvSafe(value string) string {
	if value == "" {
		return value
	}
	if strings.ContainsRune(csvFormulaLeaders, rune(value[0])) {
		return "'" + value
	}
	return value
}

// csvSafeRecord applies csvSafe to every field of a record.
func csvSafeRecord(record []string) []string {
	for i, field := range record {
		record[i] = csvSafe(field)
	}
	return record
}

// serializeFieldValueForCSV renders one FieldValue's raw JSON payload as the
// single string a CSV cell holds (docs/adrs/0001-neutral-hub-and-spoke-contact-model.md):
// a scalar keeps its text form (strings verbatim, numbers/bools stringified)
// and a Multi field joins its elements with "; " — the same separator the
// export uses for Circles and food preferences. String-first parsing is the
// right degradation for every FieldDefinition.Type (an enum or phone value is
// a JSON string, so it lands on the first branch; a number or boolean value
// isn't and falls through to its branch), and a value that fails all shapes
// degrades to its raw bytes rather than failing the whole export.
func serializeFieldValueForCSV(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return strconv.FormatBool(b)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		parts := make([]string, 0, len(arr))
		for _, el := range arr {
			parts = append(parts, serializeFieldValueForCSV(el))
		}
		return strings.Join(parts, "; ")
	}
	return string(raw)
}

// groupingNamesByContact batch-loads a contact-grouping join table into a
// VCardUID -> sorted names map, so the CSV exporter emits Circle/Tag
// memberships in one query per grouping rather than two per contact.
//
// joinTable/entityTable/joinFKColumn/memberColumn are compile-time constants
// supplied by this file's two call sites (circle_members/circles and
// contact_tags/tags) — never user input. They are interpolated because GORM
// cannot parameterize identifiers; the only bound value is userID. If a caller
// ever needs to pass something derived from a request, this signature is wrong
// and the query must be rewritten, not the argument sanitized.
func groupingNamesByContact(db *gorm.DB, userID uint, joinTable, entityTable, joinFKColumn, memberColumn string) (map[string][]string, error) {
	type row struct {
		VCardUID string
		Name     string
	}
	var rows []row
	query := fmt.Sprintf(
		`SELECT j.%s AS v_card_uid, e.name AS name
		 FROM %s j
		 JOIN %s e ON e.id = j.%s
		 WHERE j.user_id = ?
		 ORDER BY e.name`,
		memberColumn, joinTable, entityTable, joinFKColumn,
	)
	if err := db.Raw(query, userID).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[string][]string)
	for _, r := range rows {
		out[r.VCardUID] = append(out[r.VCardUID], r.Name)
	}
	return out, nil
}

// Export operation tokens and failure categories. Milestone v0.6.2 gate
// (issue #532): an export failure must identify the relevant operation and
// failure category in both the operator log and the client response — the
// same contract import_runs gives the import side.
const (
	exportOpCSV       = "export:csv"
	exportOpVCard4    = "export:vcard4"
	exportOpJSContact = "export:jscontact"
	exportOpAudit     = "export:audit"
)

const (
	exportCatDatabase      = "database"
	exportCatSerialization = "serialization"
	exportCatValidation    = "validation"
)

// abortExportError logs one structured export failure — operation, category,
// and the underlying error — and aborts with an INTERNAL_ERROR whose response
// details carry the same operation and category. The HTTP status is unchanged
// from the pre-existing behavior (500) so the frontend's download flow is
// unaffected; only the diagnostic signal is added.
func abortExportError(c *gin.Context, log *zerolog.Logger, operation, category, message string, err error) {
	log.Error().
		Err(err).
		Str("operation", operation).
		Str("category", category).
		Msg(message)
	apperrors.AbortWithError(c, apperrors.ErrInternal(message).
		WithDetails("operation", operation).
		WithDetails("category", category))
}

// parseExportFieldSelection reads the ?sections= and ?include_sensitive=
// query params accepted by the vCard/JSContact export handlers (T9,
// T9). sections is a
// comma-separated list of field-selection section tokens
// (models.FieldSections); identity data (name/uid/...) is always included.
//
// An absent sections param means "all sections" — the pre-T9 behavior,
// preserved so existing export links/URLs keep working. An unknown token is
// an explicit 400 rather than a silent narrowing, so a typo can't silently
// drop fields from an export. include_sensitive accepts true/1 as the
// explicit opt-in override (never implied by any section selection).
//
// The second return is false when the caller should abort (an error has
// already been written to the response).
func parseExportFieldSelection(c *gin.Context, operation string) (*models.FieldSelection, bool) {
	raw := c.Query("sections")
	var sel *models.FieldSelection
	if raw == "" {
		sel = models.FieldSelectionAll()
	} else {
		sel = models.NewFieldSelection()
		for _, token := range strings.Split(raw, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if err := sel.Enable(token); err != nil {
				apperrors.AbortWithError(c, apperrors.ErrValidation(err.Error()).
					WithDetails("operation", operation).
					WithDetails("category", exportCatValidation))
				return nil, false
			}
		}
		if len(sel.Sections) == 0 {
			sel = models.FieldSelectionAll()
		}
	}
	switch c.Query("include_sensitive") {
	case "true", "1":
		sel.IncludeSensitive = true
	}
	return sel, true
}

// ExportData exports all user data as CSV files in a combined format
func ExportData(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// T7: the CSV
	// export's custom-field columns now source from the v2 system
	// (FieldDefinition + FieldValue) instead of the retired untyped v1. Every
	// definition the user has becomes a header column -- like the v1 names
	// they replace -- and each contact's row carries the matching FieldValue,
	// if any.
	var definitions []models.FieldDefinition
	if err := db.Where("user_id = ?", userID).Order("label ASC").Find(&definitions).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch field definitions for export", err)
		return
	}

	// Fetch all user data
	var contacts []models.Contact
	if err := db.Where("user_id = ?", userID).
		Order("firstname ASC, lastname ASC").
		Find(&contacts).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch contacts for export", err)
		return
	}

	// FieldValues are keyed by Contact.VCardUID (the graph invariant), so
	// one user-wide query + a two-level map beats N+1 lookups per contact.
	// This is the user's own full personal-data backup, so every sensitivity
	// is included -- the same choice the relationships section documents
	// below (it is not a share to another party, unlike vCard/JSContact
	// export, which filters non-normal in projectCustomFields' query).
	var fieldValues []models.FieldValue
	if err := db.Where("user_id = ?", userID).Find(&fieldValues).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch field values for export", err)
		return
	}
	valueByContactAndDef := make(map[string]map[string]json.RawMessage, len(fieldValues))
	for _, fv := range fieldValues {
		if valueByContactAndDef[fv.EntityID] == nil {
			valueByContactAndDef[fv.EntityID] = make(map[string]json.RawMessage)
		}
		valueByContactAndDef[fv.EntityID][fv.FieldDefinitionID] = fv.Value
	}

	// relationships now come from RelationshipEdge, not the legacy
	// models.Relationship table. Names are resolved via a VCardUID map built
	// from the contacts already fetched above, since an edge only carries
	// its endpoints' VCardUID, not a nested contact.
	var relationshipEdges []models.RelationshipEdge
	if err := db.Where("user_id = ?", userID).
		Order("created_at ASC").
		Find(&relationshipEdges).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch relationship edges for export", err)
		return
	}
	type contactRef struct {
		ID   uint
		Name string
	}
	contactByVCardUID := make(map[string]contactRef, len(contacts))
	for _, contact := range contacts {
		contactByVCardUID[contact.VCardUID] = contactRef{
			ID:   contact.ID,
			Name: fmt.Sprintf("%s %s", contact.Firstname, contact.Lastname),
		}
	}

	var activities []models.Activity
	if err := db.Where("user_id = ?", userID).
		Preload("Contacts").
		Order("date DESC").
		Find(&activities).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch activities for export", err)
		return
	}

	var notes []models.Note
	if err := db.Where("user_id = ?", userID).
		Preload("Contact").
		Order("date DESC").
		Find(&notes).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch notes for export", err)
		return
	}

	var reminders []models.Reminder
	if err := db.Where("user_id = ?", userID).
		Preload("Contact").
		Order("remind_at ASC").
		Find(&reminders).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch reminders for export", err)
		return
	}

	// T20a: the "Food Preference" column now sources from the structured
	// preferences table (category=food) rather than the retired free-text
	// Contact.FoodPreference. Deliberately includes every sensitivity — this
	// is the user's own full personal-data backup, the same choice the
	// relationships section below documents for RelationshipEdge.
	var foodPreferences []models.Preference
	if err := db.Where("user_id = ? AND category = ?", userID, models.PreferenceCategoryFood).
		Find(&foodPreferences).Error; err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch food preferences for export", err)
		return
	}
	foodByVCardUID := make(map[string][]string, len(foodPreferences))
	for _, pref := range foodPreferences {
		foodByVCardUID[pref.EntityID] = append(foodByVCardUID[pref.EntityID], pref.Value)
	}

	// Circle/Tag memberships (T3). These come from the real Circle/Tag
	// entities, NOT the legacy flat Contact.Circles column this exporter used
	// to join on: every read surface in the app moved to the entities, so the
	// flat column exported stale legacy strings while omitting every
	// membership the user had actually created. Batched into maps for the same
	// reason foodByVCardUID is — one query each rather than two per contact.
	circlesByVCardUID, err := groupingNamesByContact(db, userID,
		"circle_members", "circles", "circle_id", "member_vcard_uid")
	if err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch circle memberships for export", err)
		return
	}
	tagsByVCardUID, err := groupingNamesByContact(db, userID,
		"contact_tags", "tags", "tag_id", "contact_vcard_uid")
	if err != nil {
		abortExportError(c, log, "export:csv", "database", "Failed to fetch tag assignments for export", err)
		return
	}

	// Generate combined CSV content
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write contacts section
	buf.WriteString("=== CONTACTS ===\n")
	writer.Flush()

	contactHeaders := []string{
		"ID", "Firstname", "Lastname", "Nickname", "Gender", "Email", "Phone",
		"Birthday", "Address", "How We Met", "Food Preference", "Work Information",
		"Contact Information", "Circles", "Tags", "Created At", "Updated At",
	}
	// Custom-field headers come from the v2 definitions' Labels (user-
	// authored, so the header row gets the same csvSafe treatment as the data
	// rows -- the formula-injection finding stays closed).
	for _, def := range definitions {
		contactHeaders = append(contactHeaders, def.Label)
	}
	if err := writer.Write(csvSafeRecord(contactHeaders)); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write contact headers", err)
		return
	}

	for _, contact := range contacts {
		record := []string{
			fmt.Sprintf("%d", contact.ID),
			contact.Firstname,
			contact.Lastname,
			contact.Nickname,
			contact.Gender,
			contact.Email,
			contact.Phone,
			contact.Birthday,
			contact.Address,
			contact.HowWeMet,
			strings.Join(foodByVCardUID[contact.VCardUID], "; "),
			contact.WorkInformation,
			contact.ContactInformation,
			strings.Join(circlesByVCardUID[contact.VCardUID], "; "),
			strings.Join(tagsByVCardUID[contact.VCardUID], "; "),
			contact.CreatedAt.Format(time.RFC3339),
			contact.UpdatedAt.Format(time.RFC3339),
		}
		// Custom-field values: one column per definition, empty when the
		// contact has no value for it.
		values := valueByContactAndDef[contact.VCardUID]
		for _, def := range definitions {
			record = append(record, serializeFieldValueForCSV(values[def.ID]))
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write contact record", err)
			return
		}
	}
	writer.Flush()

	// Write relationships section. reads RelationshipEdge, not the
	// legacy models.Relationship table. Deliberately includes every
	// status/sensitivity — this is the user's own full personal-data backup,
	// not a share to another party, so unlike RecordForContact's vCard/
	// JSContact export path (which filters non-normal sensitivity because
	// that projection can leave the instance), nothing here is held back.
	buf.WriteString("\n=== RELATIONSHIPS ===\n")

	relationshipHeaders := []string{
		"ID", "Source Contact ID", "Source Contact Name", "Type", "Target Contact ID",
		"Target Contact Name", "Status", "Sensitivity", "Source", "Created At", "Updated At",
	}
	if err := writer.Write(relationshipHeaders); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write relationship headers", err)
		return
	}

	for _, edge := range relationshipEdges {
		sourceContactID := edge.SourceID
		sourceContactName := edge.SourceID
		if ref, ok := contactByVCardUID[edge.SourceID]; ok {
			sourceContactID = fmt.Sprintf("%d", ref.ID)
			sourceContactName = ref.Name
		}
		targetContactID := edge.TargetID
		targetContactName := edge.TargetID
		if ref, ok := contactByVCardUID[edge.TargetID]; ok {
			targetContactID = fmt.Sprintf("%d", ref.ID)
			targetContactName = ref.Name
		}

		record := []string{
			edge.ID,
			sourceContactID,
			sourceContactName,
			edge.Type,
			targetContactID,
			targetContactName,
			edge.Status,
			edge.Sensitivity,
			edge.Source,
			edge.CreatedAt.Format(time.RFC3339),
			edge.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write relationship record", err)
			return
		}
	}
	writer.Flush()

	// Write activities section
	buf.WriteString("\n=== ACTIVITIES ===\n")

	activityHeaders := []string{
		"ID", "Title", "Description", "Location", "Date", "Contact Names", "Created At", "Updated At",
	}
	if err := writer.Write(activityHeaders); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write activity headers", err)
		return
	}

	for _, activity := range activities {
		contactNames := make([]string, len(activity.Contacts))
		for i, contact := range activity.Contacts {
			contactNames[i] = fmt.Sprintf("%s %s", contact.Firstname, contact.Lastname)
		}
		record := []string{
			fmt.Sprintf("%d", activity.ID),
			activity.Title,
			activity.Description,
			activity.Location,
			activity.Date.Format(time.RFC3339),
			strings.Join(contactNames, "; "),
			activity.CreatedAt.Format(time.RFC3339),
			activity.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write activity record", err)
			return
		}
	}
	writer.Flush()

	// Write notes section
	buf.WriteString("\n=== NOTES ===\n")

	noteHeaders := []string{
		"ID", "Contact ID", "Contact Name", "Content", "Date", "Created At", "Updated At",
	}
	if err := writer.Write(noteHeaders); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write note headers", err)
		return
	}

	for _, note := range notes {
		contactID := ""
		contactName := ""
		if note.ContactID != nil {
			contactID = fmt.Sprintf("%d", *note.ContactID)
			contactName = fmt.Sprintf("%s %s", note.Contact.Firstname, note.Contact.Lastname)
		}
		record := []string{
			fmt.Sprintf("%d", note.ID),
			contactID,
			contactName,
			note.Content,
			note.Date.Format(time.RFC3339),
			note.CreatedAt.Format(time.RFC3339),
			note.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write note record", err)
			return
		}
	}
	writer.Flush()

	// Write reminders section
	buf.WriteString("\n=== REMINDERS ===\n")

	reminderHeaders := []string{
		"ID", "Contact ID", "Contact Name", "Message", "Remind At", "Recurrence",
		"By Mail", "Reoccur From Completion", "Completed", "Last Sent", "Created At", "Updated At",
	}
	if err := writer.Write(reminderHeaders); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "Failed to write reminder headers", err)
		return
	}

	for _, reminder := range reminders {
		contactID := ""
		contactName := ""
		if reminder.ContactID != nil {
			contactID = fmt.Sprintf("%d", *reminder.ContactID)
			contactName = fmt.Sprintf("%s %s", reminder.Contact.Firstname, reminder.Contact.Lastname)
		}

		byMail := "false"
		if reminder.ByMail != nil && *reminder.ByMail {
			byMail = "true"
		}

		reoccurFromCompletion := "true"
		if reminder.ReoccurFromCompletion != nil && !*reminder.ReoccurFromCompletion {
			reoccurFromCompletion = "false"
		}

		lastSent := ""
		if reminder.LastSent != nil {
			lastSent = reminder.LastSent.Format(time.RFC3339)
		}

		record := []string{
			fmt.Sprintf("%d", reminder.ID),
			contactID,
			contactName,
			reminder.Message,
			reminder.RemindAt.Format(time.RFC3339),
			reminder.Recurrence,
			byMail,
			reoccurFromCompletion,
			fmt.Sprintf("%t", reminder.Completed),
			lastSent,
			reminder.CreatedAt.Format(time.RFC3339),
			reminder.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:csv", "serialization", "Failed to write reminder record", err)
			return
		}
	}
	writer.Flush()

	// Check for any CSV writer errors
	if err := writer.Error(); err != nil {
		abortExportError(c, log, "export:csv", "serialization", "CSV writer error", err)
		return
	}

	// Generate filename with timestamp
	filename := fmt.Sprintf("mycorrhizal-export-%s.csv", time.Now().Format("2006-01-02"))

	// Set headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Length", fmt.Sprintf("%d", buf.Len()))

	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())

	log.Info().
		Int("contacts", len(contacts)).
		Int("activities", len(activities)).
		Int("notes", len(notes)).
		Int("reminders", len(reminders)).
		Msg("Data export completed successfully")
}

// ExportContactsAsVCF exports all user contacts as a VCF (vCard) file.
//
// Per docs/adrs/0001-neutral-hub-and-spoke-contact-model.md, this now
// routes through the vcard4/vcard3 adapters instead of the legacy
// carddav.ContactToVCard mapper. ?version=3 (or "3.0") selects vCard 3.0;
// anything else (including absent) defaults to 4.0, per the "advertise 4.0
// by default" precedent this WP sets.
//
// contactmodel.Record is built via models.RecordForContact, not
// RecordFromContact directly — the persisted contact.Card already carries
// any data with no flat-field home (SpeakToAs, PersonalInfo, ...); calling
// RecordFromContact fresh here would silently drop it from the export. See
// RecordForContact's doc comment; this was a real bug found and fixed
// across three call sites while auditing work.
//
// photoDir (config.Config.ProfilePhotoDir, from routes.go's call site) is
// forwarded through RecordForContact: per
// docs/adrs/0001-neutral-hub-and-spoke-contact-model.md photo-bridging
// prerequisite, Contact.Photo/PhotoThumbnail bridges into a
// Card.Media{Kind:"photo"} entry, which the vcard4/vcard3 adapters encode
// as an embedded PHOTO property — closing previously-documented
// "VCF export doesn't embed photos" gap.
func ExportContactsAsVCF(c *gin.Context, photoDir string) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// T9: the ?sections= field picker and ?include_sensitive= opt-in
	// override apply here (the shared Card filter runs before the adapter) —
	// NOT to ExportData, which is the user's own full CSV backup and
	// deliberately includes everything .
	sel, ok := parseExportFieldSelection(c, exportOpVCard4)
	if !ok {
		return
	}

	// Fetch all user contacts, optionally scoped to a single VCardUID.
	query := db.Where("user_id = ?", userID)
	if vcardUID := strings.TrimSpace(c.Query("vcard_uid")); vcardUID != "" {
		query = query.Where("vcard_uid = ?", vcardUID)
	}
	var contacts []models.Contact
	if err := query.
		Order("firstname ASC, lastname ASC").
		Find(&contacts).Error; err != nil {
		abortExportError(c, log, "export:vcard4", "database", "Failed to fetch contacts for VCF export", err)
		return
	}

	version := "4"
	var exporter contactmodel.Exporter = vcard4.Adapter{}
	if v := strings.TrimPrefix(c.Query("version"), "v"); v == "3" || v == "3.0" {
		version = "3"
		exporter = vcard3.Adapter{}
	}

	// Generate VCF content: one full vCard block per contact, concatenated
	// (the standard shape for a multi-contact .vcf file).
	var buf bytes.Buffer
	for _, contact := range contacts {
		record := models.RecordForContactFiltered(&contact, photoDir, db, sel)
		// Issue #515: prepend the CRM-only envelope fields (Gender, Circles,
		// HowWeMet, ...) as named warn diagnostics — they round-trip through
		// the neutral Record but have no vCard home, and the drop must be
		// reported, never silent (ADR-0002 degradation policy).
		diags := models.EnvelopeExportLossDiagnostics(record)
		data, exportDiags, err := exporter.Export(record)
		diags = append(diags, exportDiags...)
		if err != nil {
			log.Error().Err(err).Uint("contact_id", contact.ID).Msg("Failed to encode contact as vCard")
			// Continue with other contacts instead of failing completely
			continue
		}
		for _, d := range diags {
			log.Debug().Str("severity", d.Severity).Str("concept", d.Concept).Uint("contact_id", contact.ID).Msg(logger.SanitizeLogField(d.Message))
		}
		buf.Write(data)
	}

	// Generate filename with timestamp
	filename := fmt.Sprintf("mycorrhizal-contacts-%s.vcf", time.Now().Format("2006-01-02"))

	// Set headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/vcard; charset=utf-8")
	c.Header("Content-Length", fmt.Sprintf("%d", buf.Len()))

	c.Data(http.StatusOK, "text/vcard; charset=utf-8", buf.Bytes())

	log.Info().
		Int("contacts", len(contacts)).
		Str("version", version).
		Msg("VCF export completed successfully")
}

// ExportContactsAsJSContact exports all user contacts as a single JSON
// document: a JSON array of RFC 9553 JSContact Card objects (one per
// contact) — the "Card set" option  task list, chosen over a
// single merged document since each contact is an independent Card with its
// own @type/uid, not sub-objects of one another.
//
// Reads photoDir from currentConfig(c).ProfilePhotoDir directly (rather than
// as an explicit parameter, unlike ExportContactsAsVCF) since this handler is
// registered directly in routes.go with the plain gin.HandlerFunc signature,
// not via a photoDir-carrying closure; this avoids a routes.go signature
// change for a one-line lookup this handler can already make itself.
func ExportContactsAsJSContact(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)
	photoDir := currentConfig(c).ProfilePhotoDir

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// Same T9 field-picker query params as ExportContactsAsVCF; never
	// applied to ExportData (the user's own full CSV backup).
	sel, ok := parseExportFieldSelection(c, exportOpJSContact)
	if !ok {
		return
	}

	var contacts []models.Contact
	if err := db.Where("user_id = ?", userID).
		Order("firstname ASC, lastname ASC").
		Find(&contacts).Error; err != nil {
		abortExportError(c, log, "export:jscontact", "database", "Failed to fetch contacts for JSContact export", err)
		return
	}

	adapter := jscontact.Adapter{}
	cards := make([]json.RawMessage, 0, len(contacts))
	for _, contact := range contacts {
		record := models.RecordForContactFiltered(&contact, photoDir, db, sel)
		// Issue #515: prepend the CRM-only envelope fields as named warn
		// diagnostics, exactly as ExportContactsAsVCF does — a JSContact
		// file has no home for Gender/Circles/HowWeMet/... either.
		diags := models.EnvelopeExportLossDiagnostics(record)
		data, exportDiags, err := adapter.Export(record)
		diags = append(diags, exportDiags...)
		if err != nil {
			log.Error().Err(err).Uint("contact_id", contact.ID).Msg("Failed to encode contact as JSContact")
			continue
		}
		for _, d := range diags {
			log.Debug().Str("severity", d.Severity).Str("concept", d.Concept).Uint("contact_id", contact.ID).Msg(logger.SanitizeLogField(d.Message))
		}
		cards = append(cards, json.RawMessage(data))
	}

	payload, err := json.Marshal(cards)
	if err != nil {
		abortExportError(c, log, "export:jscontact", "serialization", "Failed to marshal JSContact export", err)
		return
	}

	filename := fmt.Sprintf("mycorrhizal-contacts-%s.jscontact.json", time.Now().Format("2006-01-02"))

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/jscontact+json; charset=utf-8")
	c.Header("Content-Length", fmt.Sprintf("%d", len(payload)))

	c.Data(http.StatusOK, "application/jscontact+json; charset=utf-8", payload)

	log.Info().
		Int("contacts", len(contacts)).
		Msg("JSContact export completed successfully")
}

// MaxAuditExportRows is the row bound for the audit-log export (issue #415).
// The audit table grows with every write across every surface (REST, CardDAV,
// CalDAV, background jobs) and is bounded only by AUDIT_RETENTION_DAYS, so
// the export is the one surface where a single request's memory cost is not
// proportional to the user's contact data. The interactive list endpoint caps
// at 500; this backup-style export gets a far larger but still finite cap —
// 100k rows is comfortably beyond what any real instance's 90-day window
// accumulates — and a log beyond it is rejected with a 400 rather than loaded
// into memory and OOM'd. The check runs on a capped query (Limit(cap+1)), so
// an over-limit log never even gets read past the bound.
const MaxAuditExportRows = 100000

// auditExportLimit is the mutable seam over MaxAuditExportRows. Production
// always uses the constant; the boundary test lowers this to a tiny value so
// it can prove over-limit rejection without inserting 100k rows under CI's
// -race -covermode=atomic instrumentation (which blew the controllers
// package's 20-minute go test timeout on this PR). Keep in sync with
// MaxAuditExportRows — a drift between the two would only loosen or tighten
// the seam, never the exported constant's contract.
var auditExportLimit = MaxAuditExportRows

// ExportAuditLog exports the caller's own audit trail as CSV (issue #416).
// Unlike ListAuditEvents (audit_controller.go), which caps at 500 rows for
// an interactive list view, this is a backup-style export with a far larger
// but still finite bound — MaxAuditExportRows (100k, issue #415). A log
// beyond the bound is rejected with a 400 rather than loaded into memory;
// unlike ExportData/ExportContactsAsVCF/ExportContactsAsJSContact (whose
// cost is bounded by the user's contact data), the audit table grows with
// every write and is only bounded by AUDIT_RETENTION_DAYS, so it is the one
// export surface that needed its own explicit bound.
//
// BeforeSnapshot is deliberately NOT included by default. It is already
// redacted for credentials (models.AuditEvent's auditDenyList), but --
// unlike the vCard/JSContact exports -- it is not filtered by contact-field
// sensitivity: a snapshot can carry the full historical value of a field
// the rest of the export system treats as private/secret and gates behind
// include_sensitive. Rather than building a new per-field sensitivity
// filter over historical JSON snapshots (a separate, larger effort), this
// mirrors the existing include_sensitive opt-in convention with a
// distinctly-named ?include_snapshots=true: the whole column is either
// present or absent, never partially filtered. A user opting in should
// understand the column may contain historical private/secret field values
// even though it can never contain a raw credential.
func ExportAuditLog(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	includeSnapshots := false
	switch c.Query("include_snapshots") {
	case "true", "1":
		includeSnapshots = true
	}

	var events []models.AuditEvent
	// Issue #415: bound the export at the audit-export cap with a capped query —
	// Limit(cap+1) both prevents reading past the bound and distinguishes
	// "exactly at the cap" (len == cap) from "over the cap" (len == cap+1).
	if err := db.Where("user_id = ?", userID).
		Order("created_at ASC").
		Limit(auditExportLimit + 1).
		Find(&events).Error; err != nil {
		abortExportError(c, log, "export:audit", "database", "Failed to fetch audit events for export", err)
		return
	}
	if len(events) > auditExportLimit {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("export", fmt.Sprintf("Audit log exceeds the export limit of %d rows; use the paginated /audit endpoint instead", auditExportLimit)).
			WithDetails("operation", exportOpAudit).
			WithDetails("category", exportCatValidation))
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	headers := []string{"ID", "Entity Type", "Entity ID", "Operation", "Hash", "Previous Hash", "Created At"}
	if includeSnapshots {
		headers = append(headers, "Before Snapshot")
	}
	if err := writer.Write(headers); err != nil {
		abortExportError(c, log, "export:audit", "serialization", "Failed to write audit export headers", err)
		return
	}

	for _, event := range events {
		record := []string{
			fmt.Sprintf("%d", event.ID),
			event.EntityType,
			event.EntityID,
			event.Operation,
			event.Hash,
			event.PrevHash,
			event.CreatedAt.Format(time.RFC3339),
		}
		if includeSnapshots {
			record = append(record, event.BeforeSnapshot)
		}
		if err := writer.Write(csvSafeRecord(record)); err != nil {
			abortExportError(c, log, "export:audit", "serialization", "Failed to write audit export record", err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		abortExportError(c, log, "export:audit", "serialization", "CSV writer error", err)
		return
	}

	filename := fmt.Sprintf("mycorrhizal-audit-log-%s.csv", time.Now().Format("2006-01-02"))

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Length", fmt.Sprintf("%d", buf.Len()))

	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())

	log.Info().
		Int("events", len(events)).
		Bool("include_snapshots", includeSnapshots).
		Msg("Audit log export completed successfully")
}
