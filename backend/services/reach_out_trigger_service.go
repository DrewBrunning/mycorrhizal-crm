package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"sort"
	"strings"
	"time"

	apperrors "mycorrhizal/errors"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Event-driven "reach out" triggers (issue #177) — the change-driven
// counterpart to cadence.overdue. Detects a meaningful change to a contact's
// organization, job title, or address by diffing AuditEvent before-snapshots
// against the live contact, and surfaces it as a dismissible
// ReachOutSuggestion + a companion one-off Reminder (which rides the
// existing email/ntfy/Gotify/push delivery pipeline).
//
// Key simplification: Contact.Organization/JobTitle/Addresses are flat
// fields with normal JSON tags (unlike Card/CRM/Passthrough's json:"-"), so
// they were captured by AuditEvent.BeforeSnapshot even before T82's
// ContactAuditSnapshot existed. Unmarshaling any BeforeSnapshot, old or new,
// into models.ContactAuditSnapshot recovers them uniformly — no HasNested()
// branching needed here.
// ---------------------------------------------------------------------------

// reachOutDetectionMinInterval mirrors cadenceOverdueMinInterval's reasoning:
// slightly less than the daily cron cadence so natural clock-skew doesn't
// skip a run.
const reachOutDetectionMinInterval = 23 * time.Hour

// DetectReachOutSuggestions is the scheduled job that scans every user's new
// contact AuditEvent rows since their ReachOutCursor watermark, detects
// meaningful org/title/address changes, and creates a ReachOutSuggestion +
// companion Reminder + reach_out_suggested webhook for each. Job-lock guarded
// (T19's pattern) so a multi-instance deploy does not double-fire.
func DetectReachOutSuggestions(db *gorm.DB, cfg config.Config) {
	acquired, err := acquireJobLock(db, models.JobNameReachOutDetection, reachOutDetectionMinInterval)
	if err != nil {
		logger.Error().Err(err).Msg("reach-out: failed to check job lock")
		return
	}
	if !acquired {
		logger.Info().Msg("reach-out: skipping detection job - rate limited")
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameReachOutDetection, true); err != nil {
			logger.Error().Err(err).Msg("reach-out: failed to release job lock")
		}
	}()

	var userIDs []uint
	if err := db.Model(&models.AuditEvent{}).
		Where("entity_type = ?", models.AuditEntityContact).
		Distinct().Pluck("user_id", &userIDs).Error; err != nil {
		logger.Error().Err(err).Msg("reach-out: failed to load users with contact audit events")
		return
	}

	created := 0
	for _, userID := range userIDs {
		n, err := detectReachOutSuggestionsForUser(db, cfg, userID)
		if err != nil {
			logger.Error().Err(err).Uint("user_id", userID).Msg("reach-out: detection failed for user")
			continue
		}
		created += n
	}
	if created > 0 {
		logger.Info().Int("created", created).Msg("reach-out: created suggestions")
	}
}

// detectReachOutSuggestionsForUser runs the detection algorithm for one user
// and returns the number of suggestions created.
func detectReachOutSuggestionsForUser(db *gorm.DB, cfg config.Config, userID uint) (int, error) {
	var cursor models.ReachOutCursor
	if err := db.Where("user_id = ?", userID).First(&cursor).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("loading reach-out cursor: %w", err)
		}
		cursor = models.ReachOutCursor{UserID: userID, LastAuditEventID: 0}
	}

	var events []models.AuditEvent
	if err := db.Where("entity_type = ? AND user_id = ? AND id > ?", models.AuditEntityContact, userID, cursor.LastAuditEventID).
		Order("id ASC").Find(&events).Error; err != nil {
		return 0, fmt.Errorf("loading contact audit events: %w", err)
	}
	if len(events) == 0 {
		return 0, nil
	}

	// The earliest update-op event per contact (EntityID = VCardUID) is the
	// baseline "before" state for this batch's diff. Create/delete events
	// don't carry a usable prior snapshot but still advance the cursor.
	baselineByContact := make(map[string]models.AuditEvent)
	maxID := events[0].ID
	for _, e := range events {
		if e.ID > maxID {
			maxID = e.ID
		}
		if e.Operation != models.AuditOpUpdate {
			continue
		}
		if _, ok := baselineByContact[e.EntityID]; !ok {
			baselineByContact[e.EntityID] = e
		}
	}

	created := 0
	anyFailed := false
	// Deterministic order so tests and logs are stable across runs.
	uids := make([]string, 0, len(baselineByContact))
	for uid := range baselineByContact {
		uids = append(uids, uid)
	}
	sort.Strings(uids)

	for _, uid := range uids {
		baseline := baselineByContact[uid]
		n, err := processContactBaseline(db, cfg, userID, baseline)
		if err != nil {
			logger.Error().Err(err).Str("contact_vcard_uid", uid).Msg("reach-out: failed to process contact")
			anyFailed = true
			continue
		}
		created += n
	}

	// Only advance the watermark once every contact in this batch was
	// processed successfully. If any contact errored, advancing anyway would
	// permanently skip its change (it would fall outside the id > cursor
	// window on every future run). Re-scanning the whole batch next run is
	// safe: already-created-and-still-pending suggestions are caught by
	// pendingSuggestionExists, so only the genuinely failed contacts actually
	// get retried.
	if anyFailed {
		logger.Warn().Uint("user_id", userID).Msg("reach-out: not advancing cursor — at least one contact failed this run")
		return created, nil
	}
	if err := upsertReachOutCursor(db, userID, maxID); err != nil {
		return created, fmt.Errorf("advancing reach-out cursor: %w", err)
	}
	return created, nil
}

// upsertReachOutCursor advances the user's watermark to lastID, creating the
// row on first use.
func upsertReachOutCursor(db *gorm.DB, userID uint, lastID uint) error {
	var cursor models.ReachOutCursor
	err := db.Where("user_id = ?", userID).First(&cursor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&models.ReachOutCursor{UserID: userID, LastAuditEventID: lastID}).Error
	}
	if err != nil {
		return err
	}
	return db.Model(&cursor).Where("user_id = ?", userID).Update("last_audit_event_id", lastID).Error
}

// detectedChange is one meaningful field change found by diffContactChange.
type detectedChange struct {
	Kind     string
	OldValue string
	NewValue string
}

// processContactBaseline diffs one contact's baseline before-snapshot
// against its live current state, and creates a ReachOutSuggestion +
// Reminder + webhook for each meaningful change detected. Returns the number
// created.
func processContactBaseline(db *gorm.DB, cfg config.Config, userID uint, baseline models.AuditEvent) (int, error) {
	if baseline.BeforeSnapshot == "" {
		return 0, nil
	}
	var before models.ContactAuditSnapshot
	if err := json.Unmarshal([]byte(baseline.BeforeSnapshot), &before); err != nil {
		return 0, fmt.Errorf("parsing before-snapshot: %w", err)
	}

	var after models.Contact
	if err := db.Where("vcard_uid = ? AND user_id = ?", baseline.EntityID, userID).First(&after).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil // contact deleted since — nothing to reach out to
		}
		return 0, fmt.Errorf("loading live contact: %w", err)
	}

	changes := diffContactChange(before.Contact, after)
	if len(changes) == 0 {
		return 0, nil
	}

	created := 0
	for _, ch := range changes {
		exists, err := pendingSuggestionExists(db, userID, after.VCardUID, ch.Kind, ch.NewValue)
		if err != nil {
			return created, err
		}
		if exists {
			continue
		}
		if err := createReachOutSuggestion(db, cfg, userID, after, baseline.ID, ch); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// pendingSuggestionExists guards against redundant fires (e.g. a retried
// run) by checking whether a pending suggestion for this exact
// (contact, kind, new_value) already exists.
func pendingSuggestionExists(db *gorm.DB, userID uint, contactVCardUID, kind, newValue string) (bool, error) {
	var count int64
	err := db.Model(&models.ReachOutSuggestion{}).
		Where("user_id = ? AND contact_vcard_uid = ? AND kind = ? AND new_value = ? AND status = ?",
			userID, contactVCardUID, kind, newValue, models.ReachOutStatusPending).
		Count(&count).Error
	return count > 0, err
}

// diffContactChange compares before/after Contact state and returns every
// meaningful change detected (org, title, address). Only fires when the new
// value is non-empty/known — a value being cleared is not a reason to reach
// out (matches the "meaningful, not noisy" bar).
func diffContactChange(before, after models.Contact) []detectedChange {
	var changes []detectedChange

	if before.Organization != after.Organization && after.Organization != "" {
		changes = append(changes, detectedChange{
			Kind: models.ReachOutKindOrganization, OldValue: before.Organization, NewValue: after.Organization,
		})
	}
	if before.JobTitle != after.JobTitle && after.JobTitle != "" {
		changes = append(changes, detectedChange{
			Kind: models.ReachOutKindTitle, OldValue: before.JobTitle, NewValue: after.JobTitle,
		})
	}

	if addr, ok := diffAddressChange(before.Addresses, after.Addresses); ok {
		changes = append(changes, addr)
	}

	return changes
}

// diffAddressChange reports whether after's addresses contain a normalized
// key absent from before's — a meaningful move or new address. A pure
// removal with nothing added is not surfaced (mirrors the org/title rule).
// afterKeys is built in the same pass as added (rather than a second full
// traversal of after) since every added address's key is already computed.
func diffAddressChange(before, after []models.ContactAddress) (detectedChange, bool) {
	beforeKeys := make(map[string]bool, len(before))
	for _, a := range before {
		if k := AddressNormalizedKey(a); k != "" {
			beforeKeys[k] = true
		}
	}

	var added []models.ContactAddress
	afterKeys := make(map[string]bool, len(after))
	for _, a := range after {
		k := AddressNormalizedKey(a)
		if k != "" {
			afterKeys[k] = true
		}
		if k == "" || beforeKeys[k] {
			continue
		}
		added = append(added, a)
	}
	if len(added) == 0 {
		return detectedChange{}, false
	}

	var removed []models.ContactAddress
	for _, a := range before {
		if k := AddressNormalizedKey(a); k != "" && !afterKeys[k] {
			removed = append(removed, a)
		}
	}

	oldValue := joinAddressDisplayStrings(removed)
	newValue := joinAddressDisplayStrings(added)
	// AddressNormalizedKey and FormatAddress don't cover identical component
	// sets (e.g. normalization differences that don't affect the rendered
	// string), so a distinct key pair can still render the same or blank.
	// Guard the rendered value too, mirroring the org/title "new value must
	// be known and actually different" rule -- otherwise a suggestion could
	// fire showing "X → X" or a blank new value, which is meaningless.
	if newValue == "" || newValue == oldValue {
		return detectedChange{}, false
	}

	return detectedChange{
		Kind:     models.ReachOutKindAddress,
		OldValue: oldValue,
		NewValue: newValue,
	}, true
}

// joinAddressDisplayStrings renders a list of addresses as a "; "-joined
// string via models.FormatAddress (the codebase's single canonical
// address-to-string renderer, also used by CardDAV export and the import
// merge notes) — reused here rather than a bespoke rendering so this
// suggestion surface never silently drops a component (postal code,
// PO box/apartment/floor) that every other address display includes.
func joinAddressDisplayStrings(addrs []models.ContactAddress) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if s := models.FormatAddress(a); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// reachOutKindLabel is the plain-English label for a suggestion kind, used
// in the server-generated Reminder message (no i18n — matches
// life_event_controller.go's "Life event — %s: %s" convention for
// server-generated reminder text).
func reachOutKindLabel(kind string) string {
	switch kind {
	case models.ReachOutKindOrganization:
		return "organization"
	case models.ReachOutKindTitle:
		return "job title"
	case models.ReachOutKindAddress:
		return "address"
	default:
		return kind
	}
}

// createReachOutSuggestion persists one ReachOutSuggestion + its companion
// one-off Reminder in a single transaction, then fires the
// reach_out_suggested webhook (fire-and-forget, matching
// cadence_service.go's ProcessOverdueCadences).
func createReachOutSuggestion(db *gorm.DB, cfg config.Config, userID uint, contact models.Contact, auditEventID uint, ch detectedChange) error {
	// displayContactName (address_suggestion_service.go) has the fallback
	// chain a bare firstname+lastname concat lacks (full name -> FN ->
	// nickname -> VCardUID), so an org/nickname-only contact still gets an
	// identifiable name in the reminder message rather than a blank one.
	contactName := displayContactName(contact)

	var reminderID *uint
	err := db.Transaction(func(tx *gorm.DB) error {
		reminder := models.Reminder{
			UserID:     userID,
			Message:    fmt.Sprintf("Reach out — %s changed: %s (%s → %s)", reachOutKindLabel(ch.Kind), contactName, ch.OldValue, ch.NewValue),
			RemindAt:   time.Now(),
			Recurrence: "once",
			ContactID:  &contact.ID,
		}
		if err := tx.Create(&reminder).Error; err != nil {
			return err
		}
		reminderID = &reminder.ID

		suggestion := models.ReachOutSuggestion{
			UserID:          userID,
			ContactVCardUID: contact.VCardUID,
			Kind:            ch.Kind,
			OldValue:        ch.OldValue,
			NewValue:        ch.NewValue,
			AuditEventID:    auditEventID,
			ReminderID:      reminderID,
			Status:          models.ReachOutStatusPending,
		}
		if err := tx.Create(&suggestion).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("creating reach-out suggestion: %w", err)
	}

	payload := map[string]interface{}{
		"contact_id":        contact.ID,
		"contact_vcard_uid": contact.VCardUID,
		"contact_name":      contactName,
		"kind":              ch.Kind,
		"old_value":         ch.OldValue,
		"new_value":         ch.NewValue,
		"reminder_id":       reminderID,
	}
	go TriggerWebhooks(db, cfg, userID, "reach_out_suggested", payload)
	return nil
}

// ListReachOutSuggestions returns every pending suggestion for the user,
// newest first, enriched with the contact's numeric ID/display name/photo
// thumbnail in a single follow-up query (same N+1-avoidance shape
// ListOverdueCadences uses).
func ListReachOutSuggestions(db *gorm.DB, userID uint) ([]models.ReachOutSuggestionResponse, error) {
	var suggestions []models.ReachOutSuggestion
	if err := db.Where("user_id = ? AND status = ?", userID, models.ReachOutStatusPending).
		Order("created_at DESC").Find(&suggestions).Error; err != nil {
		return nil, fmt.Errorf("loading reach-out suggestions: %w", err)
	}
	if len(suggestions) == 0 {
		return nil, nil
	}

	uidSet := make(map[string]bool, len(suggestions))
	for _, s := range suggestions {
		uidSet[s.ContactVCardUID] = true
	}
	uids := make([]string, 0, len(uidSet))
	for uid := range uidSet {
		uids = append(uids, uid)
	}
	// archived=false matches every other dashboard block's contact filter
	// (dashboard_controller.go's fetchRandomContacts/fetchFavoriteContacts):
	// an archived contact is out of rotation, so a suggestion pointing at one
	// is silently dropped here rather than surfaced. This also covers a
	// suggestion whose contact no longer exists at all (the map lookup below
	// simply never finds it) — belt-and-suspenders against DeleteContact
	// racing this read, since a hard-vanished contact has no display
	// identity to show anyway.
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ? AND archived = ?", userID, uids, false).Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("loading contacts for reach-out suggestions: %w", err)
	}
	contactByUID := make(map[string]models.Contact, len(contacts))
	for _, c := range contacts {
		contactByUID[c.VCardUID] = c
	}

	result := make([]models.ReachOutSuggestionResponse, 0, len(suggestions))
	for _, s := range suggestions {
		c, ok := contactByUID[s.ContactVCardUID]
		if !ok {
			continue
		}
		result = append(result, models.ReachOutSuggestionResponse{
			ReachOutSuggestion: s,
			ContactID:          c.ID,
			ContactName:        displayContactName(c),
			PhotoThumbnail:     c.PhotoThumbnail,
		})
	}
	return result, nil
}

// DismissReachOutSuggestion marks a suggestion dismissed. Idempotent: an
// already-dismissed suggestion is a no-op success, not an error.
func DismissReachOutSuggestion(db *gorm.DB, userID uint, id string) error {
	var suggestion models.ReachOutSuggestion
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&suggestion).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrNotFound("Reach-out suggestion").WithDetails("id", id)
		}
		return fmt.Errorf("loading reach-out suggestion: %w", err)
	}
	if suggestion.Status == models.ReachOutStatusDismissed {
		return nil
	}
	if err := db.Model(&suggestion).Update("status", models.ReachOutStatusDismissed).Error; err != nil {
		return fmt.Errorf("dismissing reach-out suggestion: %w", err)
	}
	return nil
}

// DismissReachOutSuggestionByReminderID flips a suggestion's status to
// dismissed given its companion Reminder's ID (rather than its own ID) —
// used by CompleteReminder (controllers/reminder_controller.go) to couple a
// suggestion's lifecycle to its reminder's complete/skip, per the delivery-
// mechanism decision (issue #177). A no-op when no suggestion references
// this reminder.
func DismissReachOutSuggestionByReminderID(db *gorm.DB, userID uint, reminderID uint) error {
	err := db.Model(&models.ReachOutSuggestion{}).
		Where("user_id = ? AND reminder_id = ? AND status = ?", userID, reminderID, models.ReachOutStatusPending).
		Update("status", models.ReachOutStatusDismissed).Error
	if err != nil {
		return fmt.Errorf("dismissing reach-out suggestion by reminder: %w", err)
	}
	return nil
}
