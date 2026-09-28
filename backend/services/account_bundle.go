package services

import (
	"sort"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Account-bundle export (issue #1259, ADR 0028 Decision 3).
//
// BuildAccountBundle reads every user-authored entity for one user into a
// versioned, re-importable document shaped after ImportSourcePlan. It is
// full-fidelity: every sensitivity level and `status: suggested` edge is
// included with no include_sensitive opt-in, exactly like the flat CSV export
// (issue #861) and for the same reason — this is the user's own data going to
// the user's own destination, and withholding there would be silent data loss.
//
// Contacts go through RecordForContact (CLAUDE.md backend trap 3: never
// RecordFromContact, which drops Card-only data). Cross-entity references are
// rewritten to stable portable IDs (contact vcard_uid, entity UUIDs) so the
// document is independent of this instance's uint row ids.

const (
	// MaxAccountBundlePhotoBytes caps one embedded profile photo (its base64
	// data URI length). A photo beyond it is omitted from the bundle and
	// counted in AccountBundleOmissions.PhotosOmitted rather than carried
	// silently. 1 MiB of base64 is ~768 KiB of image, comfortably above a
	// profile picture.
	MaxAccountBundlePhotoBytes = 1 << 20
)

// AccountBundleStats is the out-of-band summary the controller puts in
// response headers: how many photos were embedded, how many were dropped for
// size, and the contact count (for operator logging).
type AccountBundleStats struct {
	PhotosEmbedded int
	PhotosOmitted  int
	Contacts       int
}

// BuildAccountBundle assembles the full bundle for userID. photoDir is the
// configured profile-photo directory used to embed photos as data URIs.
func BuildAccountBundle(db *gorm.DB, userID uint, photoDir string) (*models.AccountBundle, AccountBundleStats, error) {
	stats := AccountBundleStats{}

	bundle := &models.AccountBundle{
		Format:     models.AccountBundleFormat,
		Version:    models.AccountBundleVersion,
		ExportedAt: time.Now().UTC(),
		Omitted:    models.AccountBundleOmissions{Attachments: true},
	}

	contacts, uidByContactID, err := loadBundleContacts(db, userID, photoDir, &stats)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Contacts = contacts
	stats.Contacts = len(contacts)

	var uuidByActivityID, uuidByReminderID map[uint]string
	bundle.Plan.Relationships, err = loadBundleRelationships(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Notes, err = loadBundleNotes(db, userID, uidByContactID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Reminders, uuidByReminderID, err = loadBundleReminders(db, userID, uidByContactID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Activities, uuidByActivityID, err = loadBundleActivities(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.ReminderCompletions, err = loadBundleReminderCompletions(db, userID, uidByContactID, uuidByReminderID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.LifeEvents, err = loadBundleLifeEvents(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Gifts, err = loadBundleGifts(db, userID, uuidByActivityID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Preferences, err = loadBundlePreferences(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.ConversationAgenda, err = loadBundleAgenda(db, userID, uuidByActivityID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.CadencePolicies, err = loadBundleCadence(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.DataDecayPolicies, err = loadBundleDecay(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Households, err = loadBundleHouseholds(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Circles, err = loadBundleCircles(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Tags, err = loadBundleTags(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.CustomFieldDefinitions, bundle.Plan.CustomFieldValues, err = loadBundleCustomFields(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.Occasions, err = loadBundleOccasions(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Plan.OccasionEvents, err = loadBundleOccasionEvents(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	bundle.Attachments, err = loadBundleAttachments(db, userID)
	if err != nil {
		return nil, stats, err // # pragma: no cover — defensive: paired with the query-error guard above
	}

	bundle.Omitted.PhotosOmitted = stats.PhotosOmitted
	return bundle, stats, nil
}

func loadBundleContacts(db *gorm.DB, userID uint, photoDir string, stats *AccountBundleStats) ([]models.AccountBundleContact, map[uint]string, error) {
	var contacts []models.Contact
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&contacts).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleContact, 0, len(contacts))
	uidByID := make(map[uint]string, len(contacts))
	for i := range contacts {
		c := &contacts[i]
		uidByID[c.ID] = c.VCardUID
		record := models.RecordForContact(c, photoDir, db)
		bc := models.AccountBundleContact{
			UID:         c.VCardUID,
			ETag:        c.ETag,
			Gender:      c.Gender,
			Card:        record.Card,
			CRM:         record.Envelope,
			Passthrough: record.Passthrough,
			Archived:    c.Archived,
			IsFavorite:  c.IsFavorite,
		}
		// Photo cap: keep the card's self-contained data URI when it is within
		// the cap, otherwise strip it and name the omission.
		media := make([]contactmodel.Resource, 0, len(bc.Card.Media))
		for _, m := range bc.Card.Media {
			if m.Kind == "photo" {
				if len(m.URI) > MaxAccountBundlePhotoBytes {
					stats.PhotosOmitted++
					bc.PhotoOmitted = true
					continue
				}
				stats.PhotosEmbedded++
			}
			media = append(media, m)
		}
		bc.Card.Media = media
		out = append(out, bc)
	}
	return out, uidByID, nil
}

func loadBundleRelationships(db *gorm.DB, userID uint) ([]models.AccountBundleRelationship, error) {
	var edges []models.RelationshipEdge
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&edges).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleRelationship, 0, len(edges))
	for _, e := range edges {
		out = append(out, models.AccountBundleRelationship{
			ID:          e.ID,
			SourceID:    e.SourceID,
			TargetID:    e.TargetID,
			Type:        e.Type,
			Directional: e.Directional,
			Metadata:    e.Metadata,
			Source:      e.Source,
			Confidence:  e.Confidence,
			Status:      e.Status,
			Sensitivity: e.Sensitivity,
		})
	}
	return out, nil
}

func loadBundleNotes(db *gorm.DB, userID uint, uidByContactID map[uint]string) ([]models.AccountBundleNote, error) {
	var notes []models.Note
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&notes).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleNote, 0, len(notes))
	for _, n := range notes {
		out = append(out, models.AccountBundleNote{
			UUID:       n.UUID,
			ContactUID: resolveContactUID(n.ContactID, uidByContactID),
			Content:    n.Content,
			Date:       n.Date,
		})
	}
	return out, nil
}

func loadBundleReminders(db *gorm.DB, userID uint, uidByContactID map[uint]string) ([]models.AccountBundleReminder, map[uint]string, error) {
	var reminders []models.Reminder
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&reminders).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleReminder, 0, len(reminders))
	uuidByID := make(map[uint]string, len(reminders))
	for _, r := range reminders {
		uuidByID[r.ID] = r.UUID
		out = append(out, models.AccountBundleReminder{
			UUID:                  r.UUID,
			ContactUID:            resolveContactUID(r.ContactID, uidByContactID),
			Message:               r.Message,
			ByMail:                r.ByMail,
			RemindAt:              r.RemindAt,
			Recurrence:            r.Recurrence,
			ReoccurFromCompletion: r.ReoccurFromCompletion,
			Completed:             r.Completed,
			LastSent:              r.LastSent,
			LifeEventID:           r.LifeEventID,
			OccasionObligationID:  r.OccasionObligationID,
		})
	}
	return out, uuidByID, nil
}

func loadBundleActivities(db *gorm.DB, userID uint) ([]models.AccountBundleActivity, map[uint]string, error) {
	var activities []models.Activity
	if err := db.Where("user_id = ?", userID).Preload("Contacts").Order("created_at ASC").Find(&activities).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleActivity, 0, len(activities))
	uuidByID := make(map[uint]string, len(activities))
	for _, a := range activities {
		uuidByID[a.ID] = a.UUID
		uids := make([]string, 0, len(a.Contacts))
		for _, c := range a.Contacts {
			uids = append(uids, c.VCardUID)
		}
		sort.Strings(uids)
		out = append(out, models.AccountBundleActivity{
			UUID:         a.UUID,
			Title:        a.Title,
			Description:  a.Description,
			Location:     a.Location,
			Date:         a.Date,
			Type:         a.Type,
			ExternalRef:  a.ExternalRef,
			AttendeeUIDs: uids,
		})
	}
	return out, uuidByID, nil
}

func loadBundleReminderCompletions(db *gorm.DB, userID uint, uidByContactID map[uint]string, uuidByReminderID map[uint]string) ([]models.AccountBundleReminderCompletion, error) {
	var completions []models.ReminderCompletion
	if err := db.Where("user_id = ?", userID).Order("completed_at ASC").Find(&completions).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleReminderCompletion, 0, len(completions))
	for _, rc := range completions {
		reminderUUID := ""
		if rc.ReminderID != nil {
			reminderUUID = uuidByReminderID[*rc.ReminderID]
		}
		out = append(out, models.AccountBundleReminderCompletion{
			UUID:         rc.UUID,
			ReminderUUID: reminderUUID,
			ContactUID:   uidByContactID[rc.ContactID],
			Message:      rc.Message,
			CompletedAt:  rc.CompletedAt,
		})
	}
	return out, nil
}

func loadBundleLifeEvents(db *gorm.DB, userID uint) ([]models.AccountBundleLifeEvent, error) {
	var events []models.LifeEvent
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&events).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleLifeEvent, 0, len(events))
	for _, e := range events {
		out = append(out, models.AccountBundleLifeEvent{
			ID:               e.ID,
			EntityID:         e.EntityID,
			Type:             e.Type,
			Category:         e.Category,
			Date:             e.Date,
			EndDate:          e.EndDate,
			Description:      e.Description,
			Source:           e.Source,
			RelatedEntityIDs: e.RelatedEntityIDs,
			Remind:           e.Remind,
		})
	}
	return out, nil
}

func loadBundleGifts(db *gorm.DB, userID uint, uuidByActivityID map[uint]string) ([]models.AccountBundleGift, error) {
	var gifts []models.Gift
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&gifts).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleGift, 0, len(gifts))
	for _, g := range gifts {
		activityUUID := ""
		if g.ActivityID != nil {
			activityUUID = uuidByActivityID[*g.ActivityID]
		}
		out = append(out, models.AccountBundleGift{
			ID:           g.ID,
			EntityID:     g.EntityID,
			Status:       g.Status,
			Occasion:     g.Occasion,
			Description:  g.Description,
			URL:          g.URL,
			Notes:        g.Notes,
			Date:         g.Date,
			ValueCents:   g.ValueCents,
			Currency:     g.Currency,
			LifeEventID:  g.LifeEventID,
			ActivityUUID: activityUUID,
		})
	}
	return out, nil
}

func loadBundlePreferences(db *gorm.DB, userID uint) ([]models.AccountBundlePreference, error) {
	var prefs []models.Preference
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&prefs).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundlePreference, 0, len(prefs))
	for _, p := range prefs {
		out = append(out, models.AccountBundlePreference{
			ID:            p.ID,
			EntityID:      p.EntityID,
			Category:      p.Category,
			Key:           p.Key,
			Value:         p.Value,
			Level:         p.Level,
			Notes:         p.Notes,
			Source:        p.Source,
			Confidence:    p.Confidence,
			LastConfirmed: p.LastConfirmed,
			Sensitivity:   p.Sensitivity,
		})
	}
	return out, nil
}

func loadBundleAgenda(db *gorm.DB, userID uint, uuidByActivityID map[uint]string) ([]models.AccountBundleAgendaItem, error) {
	var items []models.ConversationAgenda
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&items).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleAgendaItem, 0, len(items))
	for _, it := range items {
		activityUUID := ""
		if it.ActivityID != nil {
			activityUUID = uuidByActivityID[*it.ActivityID]
		}
		out = append(out, models.AccountBundleAgendaItem{
			ID:           it.ID,
			EntityID:     it.EntityID,
			Content:      it.Content,
			ReferenceURL: it.ReferenceURL,
			DiscussedAt:  it.DiscussedAt,
			ActivityUUID: activityUUID,
		})
	}
	return out, nil
}

func loadBundleCadence(db *gorm.DB, userID uint) ([]models.AccountBundleCadencePolicy, error) {
	var policies []models.CadencePolicy
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&policies).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleCadencePolicy, 0, len(policies))
	for _, p := range policies {
		out = append(out, models.AccountBundleCadencePolicy{
			ID:                 p.ID,
			EntityID:           p.EntityID,
			TargetIntervalDays: p.TargetIntervalDays,
			QualifyingTypes:    p.QualifyingTypes,
		})
	}
	return out, nil
}

func loadBundleDecay(db *gorm.DB, userID uint) ([]models.AccountBundleDataDecayPolicy, error) {
	var policies []models.DataDecayPolicy
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&policies).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleDataDecayPolicy, 0, len(policies))
	for _, p := range policies {
		out = append(out, models.AccountBundleDataDecayPolicy{
			ID:             p.ID,
			EntityID:       p.EntityID,
			IntervalDays:   p.IntervalDays,
			LastVerifiedAt: p.LastVerifiedAt,
			Active:         p.Active,
		})
	}
	return out, nil
}

func loadBundleHouseholds(db *gorm.DB, userID uint) ([]models.AccountBundleHousehold, error) {
	var households []models.Household
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&households).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	var members []models.HouseholdMember
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&members).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	membersByHousehold := map[string][]models.AccountBundleHouseholdMember{}
	for _, m := range members {
		membersByHousehold[m.HouseholdID] = append(membersByHousehold[m.HouseholdID], models.AccountBundleHouseholdMember{
			MemberVCardUID: m.MemberVCardUID,
			Role:           m.Role,
			Since:          m.Since,
			Until:          m.Until,
		})
	}
	for id := range membersByHousehold {
		sort.Slice(membersByHousehold[id], func(i, j int) bool {
			return membersByHousehold[id][i].MemberVCardUID < membersByHousehold[id][j].MemberVCardUID
		})
	}
	out := make([]models.AccountBundleHousehold, 0, len(households))
	for _, h := range households {
		out = append(out, models.AccountBundleHousehold{
			ID:      h.ID,
			Name:    h.Name,
			Type:    h.Type,
			Address: h.Address,
			Members: nonNilHouseholdMembers(membersByHousehold[h.ID]),
		})
	}
	return out, nil
}

func loadBundleCircles(db *gorm.DB, userID uint) ([]models.AccountBundleCircle, error) {
	var circles []models.Circle
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&circles).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	var members []models.CircleMember
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&members).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	membersByCircle := map[string][]string{}
	for _, m := range members {
		membersByCircle[m.CircleID] = append(membersByCircle[m.CircleID], m.MemberVCardUID)
	}
	for id := range membersByCircle {
		sort.Strings(membersByCircle[id])
	}
	out := make([]models.AccountBundleCircle, 0, len(circles))
	for _, c := range circles {
		out = append(out, models.AccountBundleCircle{
			ID:         c.ID,
			Name:       c.Name,
			MemberUIDs: nonNilStrings(membersByCircle[c.ID]),
		})
	}
	return out, nil
}

func loadBundleTags(db *gorm.DB, userID uint) ([]models.AccountBundleTag, error) {
	var tags []models.Tag
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&tags).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	var taggings []models.ContactTag
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&taggings).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	uidsByTag := map[string][]string{}
	for _, t := range taggings {
		uidsByTag[t.TagID] = append(uidsByTag[t.TagID], t.ContactVCardUID)
	}
	for id := range uidsByTag {
		sort.Strings(uidsByTag[id])
	}
	out := make([]models.AccountBundleTag, 0, len(tags))
	for _, t := range tags {
		out = append(out, models.AccountBundleTag{
			ID:          t.ID,
			Name:        t.Name,
			ContactUIDs: nonNilStrings(uidsByTag[t.ID]),
		})
	}
	return out, nil
}

func loadBundleCustomFields(db *gorm.DB, userID uint) ([]models.AccountBundleFieldDefinition, []models.AccountBundleFieldValue, error) {
	var defs []models.FieldDefinition
	if err := db.Where("user_id = ?", userID).Order("position ASC, created_at ASC").Find(&defs).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	outDefs := make([]models.AccountBundleFieldDefinition, 0, len(defs))
	for _, d := range defs {
		outDefs = append(outDefs, models.AccountBundleFieldDefinition{
			ID:          d.ID,
			Label:       d.Label,
			Key:         d.Key,
			Target:      d.Target,
			Type:        d.Type,
			Constraints: d.Constraints,
			Projection:  d.Projection,
			Sensitivity: d.Sensitivity,
			Position:    d.Position,
		})
	}
	var vals []models.FieldValue
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&vals).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	outVals := make([]models.AccountBundleFieldValue, 0, len(vals))
	for _, v := range vals {
		outVals = append(outVals, models.AccountBundleFieldValue{
			FieldDefinitionID: v.FieldDefinitionID,
			EntityID:          v.EntityID,
			Value:             v.Value,
		})
	}
	return outDefs, outVals, nil
}

func loadBundleOccasions(db *gorm.DB, userID uint) ([]models.AccountBundleOccasion, error) {
	var occasions []models.OccasionObligation
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&occasions).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleOccasion, 0, len(occasions))
	for _, o := range occasions {
		out = append(out, models.AccountBundleOccasion{
			ID:                o.ID,
			EntityID:          o.EntityID,
			Kind:              o.Kind,
			Label:             o.Label,
			AnchorMonth:       o.AnchorMonth,
			AnchorDay:         o.AnchorDay,
			LinkedLifeEventID: o.LinkedLifeEventID,
			LeadTimeDays:      o.LeadTimeDays,
			Active:            o.Active,
			Sensitivity:       o.Sensitivity,
			Notes:             o.Notes,
		})
	}
	return out, nil
}

func loadBundleOccasionEvents(db *gorm.DB, userID uint) ([]models.AccountBundleOccasionEvent, error) {
	var events []models.OccasionEvent
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&events).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	var attendees []models.OccasionEventAttendee
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&attendees).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	byEvent := map[string][]models.AccountBundleAttendee{}
	for _, a := range attendees {
		byEvent[a.EventID] = append(byEvent[a.EventID], models.AccountBundleAttendee{EntityID: a.EntityID, RSVP: a.RSVP})
	}
	for id := range byEvent {
		sort.Slice(byEvent[id], func(i, j int) bool { return byEvent[id][i].EntityID < byEvent[id][j].EntityID })
	}
	out := make([]models.AccountBundleOccasionEvent, 0, len(events))
	for _, e := range events {
		out = append(out, models.AccountBundleOccasionEvent{
			ID:          e.ID,
			Title:       e.Title,
			StartsAt:    e.StartsAt,
			EndsAt:      e.EndsAt,
			Location:    e.Location,
			Sensitivity: e.Sensitivity,
			Notes:       e.Notes,
			Attendees:   nonNilAttendees(byEvent[e.ID]),
		})
	}
	return out, nil
}

func loadBundleAttachments(db *gorm.DB, userID uint) ([]models.AccountBundleAttachment, error) {
	var attachments []models.Attachment
	if err := db.Where("user_id = ?", userID).Order("created_at ASC").Find(&attachments).Error; err != nil { // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		return nil, err // # pragma: no cover — defensive: paired with the query-error guard above
	}
	out := make([]models.AccountBundleAttachment, 0, len(attachments))
	for _, a := range attachments {
		out = append(out, models.AccountBundleAttachment{
			ID:              a.ID,
			ContactVCardUID: a.ContactVCardUID,
			OriginalName:    a.OriginalName,
			ContentType:     a.ContentType,
			SizeBytes:       a.SizeBytes,
			CreatedAt:       a.CreatedAt,
		})
	}
	return out, nil
}

func resolveContactUID(id *uint, uidByContactID map[uint]string) string {
	if id == nil {
		return ""
	}
	return uidByContactID[*id]
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func nonNilHouseholdMembers(in []models.AccountBundleHouseholdMember) []models.AccountBundleHouseholdMember {
	if in == nil {
		return []models.AccountBundleHouseholdMember{}
	}
	return in
}

func nonNilAttendees(in []models.AccountBundleAttendee) []models.AccountBundleAttendee {
	if in == nil {
		return []models.AccountBundleAttendee{}
	}
	return in
}
