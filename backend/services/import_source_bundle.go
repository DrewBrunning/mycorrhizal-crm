package services

import (
	"strings"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Account-bundle entity mappings and importers (issue #1260). These extend the
// shared source-import engine to every user-authored entity the Meerkat and
// Monica mappings did not cover, so the `mycorrhizal` source can land a bundle
// through the same ExecuteSourceImportWithActions pass. Existing sources leave
// these slices empty and are unaffected.

// MappedFieldDefinition is one custom-field schema row, keyed by its stable
// UUID.
type MappedFieldDefinition struct {
	Ref         SourceRef
	ID          string
	Label       string
	Key         string
	Target      string
	Type        string
	Constraints models.FieldConstraints
	Projection  string
	Sensitivity string
	Position    int
}

// MappedReminderCompletion is one reminder completion.
type MappedReminderCompletion struct {
	Ref SourceRef
	// UUID preserves the source's stable completion identity.
	UUID string
	// ReminderUUID references MappedReminder.UUID (optional).
	ReminderUUID string
	Contact      SourceRef
	Message      string
	CompletedAt  string
}

// MappedLifeEvent is one life event keyed by its stable UUID.
type MappedLifeEvent struct {
	Ref              SourceRef
	ID               string
	Contact          SourceRef
	Type             string
	Category         string
	Date             *contactmodel.PartialDate
	EndDate          *contactmodel.PartialDate
	Description      string
	Source           string
	RelatedEntityIDs []string
	Remind           bool
}

// MappedConversationAgenda is one agenda item keyed by its stable UUID.
type MappedConversationAgenda struct {
	Ref          SourceRef
	ID           string
	Contact      SourceRef
	Content      string
	ReferenceURL string
	DiscussedAt  *time.Time
	ActivityUUID string
}

// MappedCadencePolicy is one cadence policy keyed by its stable UUID.
type MappedCadencePolicy struct {
	Ref                SourceRef
	ID                 string
	Contact            SourceRef
	TargetIntervalDays int
	QualifyingTypes    []string
}

// MappedDataDecayPolicy is one data-decay policy keyed by its stable UUID.
type MappedDataDecayPolicy struct {
	Ref            SourceRef
	ID             string
	Contact        SourceRef
	IntervalDays   int
	LastVerifiedAt *time.Time
	Active         bool
}

// MappedOccasion is one occasion obligation keyed by its stable UUID.
type MappedOccasion struct {
	Ref               SourceRef
	ID                string
	Contact           SourceRef
	Kind              string
	Label             string
	AnchorMonth       *int
	AnchorDay         *int
	LinkedLifeEventID string
	LeadTimeDays      int
	Active            bool
	Sensitivity       string
	Notes             string
}

// MappedOccasionEvent is one occasion event keyed by its stable UUID.
type MappedOccasionEvent struct {
	Ref         SourceRef
	ID          string
	Title       string
	StartsAt    string
	EndsAt      *time.Time
	Location    string
	Sensitivity string
	Notes       string
	Attendees   []MappedOccasionAttendee
}

// MappedOccasionAttendee is one attendee of a MappedOccasionEvent.
type MappedOccasionAttendee struct {
	Contact SourceRef
	RSVP    string
}

// importFieldDefinitions lands the custom-field schema half before values
// reference it. Existing sources have no definitions here (the old importer
// created one implicitly per value), so this is bundle-only.
func importFieldDefinitions(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	fieldDefRemap map[string]string, skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, d := range plan.FieldDefinitions {
		record := d.Ref.String()
		if skipImported(record, d.Ref) {
			continue
		}
		def := models.FieldDefinition{
			ID:          d.ID,
			UserID:      userID,
			Label:       d.Label,
			Key:         d.Key,
			Target:      d.Target,
			Type:        d.Type,
			Constraints: d.Constraints,
			Projection:  d.Projection,
			Sensitivity: d.Sensitivity,
			Position:    d.Position,
		}
		if err := tx.Create(&def).Error; err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") && def.Key != "" {
				// Definition already present (same user+key) — reuse it.
				var existing models.FieldDefinition
				if findErr := tx.Where("user_id = ? AND key = ?", userID, def.Key).First(&existing).Error; findErr == nil {
					if err := recordSourceLink(tx, userID, plan.System, d.Ref.ExternalID,
						models.ImportSourceLinkKindFieldDefinition, existing.ID); err != nil { // # pragma: no cover — defensive
						return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
					}
					if d.ID != "" {
						fieldDefRemap[d.ID] = existing.ID
					}
					imported[d.Ref.ExternalID] = true
					continue
				}
			}
			report.appendIssue(ImportIssue{Record: record, Field: "field_definition", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, d.Ref.ExternalID,
			models.ImportSourceLinkKindFieldDefinition, def.ID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		if d.ID != "" {
			fieldDefRemap[d.ID] = def.ID
		}
		imported[d.Ref.ExternalID] = true
	}
	return nil
}

func importReminderCompletions(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), refToID map[string]uint, skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, c := range plan.ReminderCompletions {
		record := c.Ref.String()
		if skipImported(record, c.Ref) {
			continue
		}
		contactID, ok := refToID[c.Contact.ExternalID]
		if !ok {
			report.appendIssue(ImportIssue{Record: record, Field: c.Contact.ExternalID, Category: ImportIssueCategoryUnsupported, Message: "references a contact that was not imported"})
			continue
		}
		completedAt, err := parseSourceTime(c.CompletedAt)
		if err != nil {
			report.appendIssue(ImportIssue{Record: record, Field: "reminder_completion.completed_at", Category: ImportIssueCategoryInvalid, Message: "unparseable date: " + c.CompletedAt})
			continue
		}
		var reminderID *uint
		if c.ReminderUUID != "" {
			var reminder models.Reminder
			if err := tx.Where("user_id = ? AND uuid = ?", userID, c.ReminderUUID).First(&reminder).Error; err == nil {
				reminderID = &reminder.ID
			}
		}
		completion := models.ReminderCompletion{
			UUID:        c.UUID,
			UserID:      userID,
			ReminderID:  reminderID,
			ContactID:   contactID,
			Message:     c.Message,
			CompletedAt: completedAt,
		}
		if err := tx.Create(&completion).Error; err != nil { // # pragma: no cover — defensive
			report.appendIssue(ImportIssue{Record: record, Field: "reminder_completion", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, c.Ref.ExternalID,
			models.ImportSourceLinkKindReminderCompletion, completion.UUID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[c.Ref.ExternalID] = true
	}
	return nil
}

func importLifeEvents(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, le := range plan.LifeEvents {
		record := le.Ref.String()
		if skipImported(record, le.Ref) {
			continue
		}
		entityUID, ok := uidOf(record, le.Contact)
		if !ok {
			continue
		}
		event := models.LifeEvent{
			ID:               le.ID,
			UserID:           userID,
			EntityID:         entityUID,
			Type:             le.Type,
			Category:         le.Category,
			Date:             le.Date,
			EndDate:          le.EndDate,
			Description:      le.Description,
			Source:           le.Source,
			RelatedEntityIDs: le.RelatedEntityIDs,
			Remind:           le.Remind,
		}
		if err := tx.Create(&event).Error; err != nil { // # pragma: no cover — defensive
			report.appendIssue(ImportIssue{Record: record, Field: "life_event", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, le.Ref.ExternalID,
			models.ImportSourceLinkKindLifeEvent, event.ID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[le.Ref.ExternalID] = true
	}
	return nil
}

func importConversationAgenda(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, it := range plan.ConversationAgenda {
		record := it.Ref.String()
		if skipImported(record, it.Ref) {
			continue
		}
		entityUID, ok := uidOf(record, it.Contact)
		if !ok {
			continue
		}
		item := models.ConversationAgenda{
			ID:           it.ID,
			UserID:       userID,
			EntityID:     entityUID,
			Content:      it.Content,
			ReferenceURL: it.ReferenceURL,
			DiscussedAt:  it.DiscussedAt,
			ActivityID:   activityIDByUUID(tx, userID, it.ActivityUUID),
		}
		if err := tx.Create(&item).Error; err != nil { // # pragma: no cover — defensive
			report.appendIssue(ImportIssue{Record: record, Field: "conversation_agenda", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, it.Ref.ExternalID,
			models.ImportSourceLinkKindAgendaItem, item.ID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[it.Ref.ExternalID] = true
	}
	return nil
}

func importCadencePolicies(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, p := range plan.CadencePolicies {
		record := p.Ref.String()
		if skipImported(record, p.Ref) {
			continue
		}
		entityUID, ok := uidOf(record, p.Contact)
		if !ok {
			continue
		}
		policy := models.CadencePolicy{
			ID:                 p.ID,
			UserID:             userID,
			EntityID:           entityUID,
			TargetIntervalDays: p.TargetIntervalDays,
			QualifyingTypes:    p.QualifyingTypes,
		}
		if err := tx.Create(&policy).Error; err != nil { // # pragma: no cover — defensive
			report.appendIssue(ImportIssue{Record: record, Field: "cadence_policy", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, p.Ref.ExternalID,
			models.ImportSourceLinkKindCadencePolicy, policy.ID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[p.Ref.ExternalID] = true
	}
	return nil
}

func importDataDecayPolicies(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, p := range plan.DataDecayPolicies {
		record := p.Ref.String()
		if skipImported(record, p.Ref) {
			continue
		}
		entityUID, ok := uidOf(record, p.Contact)
		if !ok {
			continue
		}
		policy := models.DataDecayPolicy{
			ID:             p.ID,
			UserID:         userID,
			EntityID:       entityUID,
			IntervalDays:   p.IntervalDays,
			LastVerifiedAt: p.LastVerifiedAt,
			Active:         p.Active,
		}
		if err := tx.Create(&policy).Error; err != nil { // # pragma: no cover — defensive
			report.appendIssue(ImportIssue{Record: record, Field: "data_decay_policy", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, p.Ref.ExternalID,
			models.ImportSourceLinkKindDataDecayPolicy, policy.ID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[p.Ref.ExternalID] = true
	}
	return nil
}

func importOccasions(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, o := range plan.Occasions {
		record := o.Ref.String()
		if skipImported(record, o.Ref) {
			continue
		}
		entityUID, ok := uidOf(record, o.Contact)
		if !ok {
			continue
		}
		occasion := models.OccasionObligation{
			ID:                o.ID,
			UserID:            userID,
			EntityID:          entityUID,
			Kind:              o.Kind,
			Label:             o.Label,
			AnchorMonth:       o.AnchorMonth,
			AnchorDay:         o.AnchorDay,
			LinkedLifeEventID: o.LinkedLifeEventID,
			LeadTimeDays:      o.LeadTimeDays,
			Active:            o.Active,
			Sensitivity:       o.Sensitivity,
			Notes:             o.Notes,
		}
		if err := tx.Create(&occasion).Error; err != nil { // # pragma: no cover — defensive
			report.appendIssue(ImportIssue{Record: record, Field: "occasion", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		if err := recordSourceLink(tx, userID, plan.System, o.Ref.ExternalID,
			models.ImportSourceLinkKindOccasion, occasion.ID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[o.Ref.ExternalID] = true
	}
	return nil
}

func importOccasionEvents(tx *gorm.DB, userID uint, plan *ImportSourcePlan, imported map[string]bool,
	uidOf func(string, SourceRef) (string, bool), skipImported func(string, SourceRef) bool, report *ImportReport,
) error {
	for _, e := range plan.OccasionEvents {
		record := e.Ref.String()
		if skipImported(record, e.Ref) {
			continue
		}
		startsAt, err := parseSourceTime(e.StartsAt)
		if err != nil {
			report.appendIssue(ImportIssue{Record: record, Field: "occasion_event.starts_at", Category: ImportIssueCategoryInvalid, Message: "unparseable date: " + e.StartsAt})
			continue
		}
		event := models.OccasionEvent{
			ID:          e.ID,
			UserID:      userID,
			Title:       e.Title,
			StartsAt:    startsAt,
			EndsAt:      e.EndsAt,
			Location:    e.Location,
			Sensitivity: e.Sensitivity,
			Notes:       e.Notes,
		}
		if err := tx.Create(&event).Error; err != nil { // # pragma: no cover — defensive
			report.appendIssue(ImportIssue{Record: record, Field: "occasion_event", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			continue
		}
		for _, a := range e.Attendees {
			uid, ok := uidOf(record+" attendee", a.Contact)
			if !ok {
				continue
			}
			if err := tx.Create(&models.OccasionEventAttendee{
				UserID:   userID,
				EventID:  event.ID,
				EntityID: uid,
				RSVP:     a.RSVP,
			}).Error; err != nil { // # pragma: no cover — defensive
				report.appendIssue(ImportIssue{Record: record, Field: "occasion_event.attendee", Category: ImportIssueCategoryInvalid, Message: err.Error()})
			}
		}
		if err := recordSourceLink(tx, userID, plan.System, e.Ref.ExternalID,
			models.ImportSourceLinkKindOccasionEvent, event.ID); err != nil { // # pragma: no cover — defensive
			return err // # pragma: no cover — defensive: recordSourceLink on a healthy migrated schema
		}
		imported[e.Ref.ExternalID] = true
	}
	return nil
}

// activityIDByUUID resolves an activity's uint PK from its stable UUID, or nil
// when the reference is empty/unresolvable.
func activityIDByUUID(tx *gorm.DB, userID uint, uuid string) *uint {
	if uuid == "" {
		return nil
	}
	var activity models.Activity
	if err := tx.Where("user_id = ? AND uuid = ?", userID, uuid).First(&activity).Error; err != nil {
		return nil
	}
	return &activity.ID
}
