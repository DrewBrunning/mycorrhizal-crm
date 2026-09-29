package services

import (
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
)

// MapAccountBundle turns a parsed account bundle (issues #1259/#1260) into the
// shared ImportSourcePlan, so the `mycorrhizal` source lands through the exact
// same ExecuteSourceImportWithActions pass every other source uses. Every
// entity's stable ID is carried through so re-running is idempotent via the
// import_source_links ledger and a re-export is comparable.
//
// ExternalID namespacing is one space per entity kind ("contact/<uid>",
// "note/<uuid>", ...), so two kinds can never collide in the ledger
// (models.ImportSourceLink's own rule).
func MapAccountBundle(bundle *models.AccountBundle) *ImportSourcePlan {
	plan := &ImportSourcePlan{System: "mycorrhizal"}
	if bundle == nil {
		return plan
	}

	for _, c := range bundle.Plan.Contacts {
		card := c.Card
		if card.UID == "" {
			card.UID = c.UID
		}
		plan.Contacts = append(plan.Contacts, MappedContact{
			Ref:      bundleContactRef(c.UID),
			Record:   &contactmodel.Record{UID: c.UID, Card: card, Envelope: c.CRM, Passthrough: c.Passthrough},
			Archived: c.Archived,
			Favorite: c.IsFavorite,
		})
	}

	for _, r := range bundle.Plan.Relationships {
		plan.Relationships = append(plan.Relationships, MappedRelationship{
			Ref:         bundleRef("relationship", r.ID),
			ID:          r.ID,
			Source:      bundleContactRef(r.SourceID),
			Target:      bundleContactRef(r.TargetID),
			Type:        r.Type,
			Directional: r.Directional,
			Status:      r.Status,
			Sensitivity: r.Sensitivity,
			Provenance:  r.Source,
			Confidence:  r.Confidence,
			Metadata:    r.Metadata,
		})
	}

	for _, n := range bundle.Plan.Notes {
		plan.Notes = append(plan.Notes, MappedNote{
			Ref:     bundleRef("note", n.UUID),
			UUID:    n.UUID,
			Contact: bundleContactRef(n.ContactUID),
			Content: n.Content,
			Date:    formatBundleTime(n.Date),
		})
	}

	for _, r := range bundle.Plan.Reminders {
		plan.Reminders = append(plan.Reminders, MappedReminder{
			Ref:                   bundleRef("reminder", r.UUID),
			UUID:                  r.UUID,
			Contact:               bundleContactRef(r.ContactUID),
			Message:               r.Message,
			RemindAt:              formatBundleTime(r.RemindAt),
			Recurrence:            r.Recurrence,
			ReoccurFromCompletion: r.ReoccurFromCompletion,
			Completed:             r.Completed,
			LastSent:              r.LastSent,
			LifeEventID:           derefString(r.LifeEventID),
			OccasionObligationID:  derefString(r.OccasionObligationID),
		})
	}

	for _, c := range bundle.Plan.ReminderCompletions {
		plan.ReminderCompletions = append(plan.ReminderCompletions, MappedReminderCompletion{
			Ref:          bundleRef("reminder_completion", c.UUID),
			UUID:         c.UUID,
			ReminderUUID: c.ReminderUUID,
			Contact:      bundleContactRef(c.ContactUID),
			Message:      c.Message,
			CompletedAt:  formatBundleTime(c.CompletedAt),
		})
	}

	for _, a := range bundle.Plan.Activities {
		ref := bundleRef("activity", a.UUID)
		contacts := make([]SourceRef, 0, len(a.AttendeeUIDs))
		for _, uid := range a.AttendeeUIDs {
			contacts = append(contacts, bundleContactRef(uid))
		}
		plan.Activities = append(plan.Activities, MappedActivity{
			Ref:         ref,
			UUID:        a.UUID,
			Contacts:    contacts,
			Title:       a.Title,
			Description: a.Description,
			Location:    a.Location,
			Date:        formatBundleTime(a.Date),
			Type:        a.Type,
			ExternalRef: a.ExternalRef,
		})
	}

	for _, e := range bundle.Plan.LifeEvents {
		plan.LifeEvents = append(plan.LifeEvents, MappedLifeEvent{
			Ref:              bundleRef("life_event", e.ID),
			ID:               e.ID,
			Contact:          bundleContactRef(e.EntityID),
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

	for _, g := range bundle.Plan.Gifts {
		plan.Gifts = append(plan.Gifts, MappedGift{
			Ref:          bundleRef("gift", g.ID),
			ID:           g.ID,
			Contact:      bundleContactRef(g.EntityID),
			Status:       g.Status,
			Occasion:     g.Occasion,
			Description:  g.Description,
			URL:          g.URL,
			Notes:        g.Notes,
			Date:         formatBundleTimePtr(g.Date),
			ValueCents:   g.ValueCents,
			Currency:     g.Currency,
			LifeEventID:  g.LifeEventID,
			ActivityUUID: g.ActivityUUID,
		})
	}

	for _, p := range bundle.Plan.Preferences {
		plan.Preferences = append(plan.Preferences, MappedPreference{
			Ref:           bundleRef("preference", p.ID),
			ID:            p.ID,
			Contact:       bundleContactRef(p.EntityID),
			Category:      p.Category,
			Key:           p.Key,
			Value:         p.Value,
			Notes:         p.Notes,
			Level:         p.Level,
			Source:        p.Source,
			Confidence:    p.Confidence,
			LastConfirmed: p.LastConfirmed,
			Sensitivity:   p.Sensitivity,
		})
	}

	for _, it := range bundle.Plan.ConversationAgenda {
		plan.ConversationAgenda = append(plan.ConversationAgenda, MappedConversationAgenda{
			Ref:          bundleRef("conversation_agenda", it.ID),
			ID:           it.ID,
			Contact:      bundleContactRef(it.EntityID),
			Content:      it.Content,
			ReferenceURL: it.ReferenceURL,
			DiscussedAt:  it.DiscussedAt,
			ActivityUUID: it.ActivityUUID,
		})
	}

	for _, p := range bundle.Plan.CadencePolicies {
		plan.CadencePolicies = append(plan.CadencePolicies, MappedCadencePolicy{
			Ref:                bundleRef("cadence_policy", p.ID),
			ID:                 p.ID,
			Contact:            bundleContactRef(p.EntityID),
			TargetIntervalDays: p.TargetIntervalDays,
			QualifyingTypes:    p.QualifyingTypes,
		})
	}

	for _, p := range bundle.Plan.DataDecayPolicies {
		plan.DataDecayPolicies = append(plan.DataDecayPolicies, MappedDataDecayPolicy{
			Ref:            bundleRef("data_decay_policy", p.ID),
			ID:             p.ID,
			Contact:        bundleContactRef(p.EntityID),
			IntervalDays:   p.IntervalDays,
			LastVerifiedAt: p.LastVerifiedAt,
			Active:         p.Active,
		})
	}

	for _, h := range bundle.Plan.Households {
		members := make([]MappedHouseholdMember, 0, len(h.Members))
		for _, m := range h.Members {
			members = append(members, MappedHouseholdMember{
				Contact: bundleContactRef(m.MemberVCardUID),
				Role:    m.Role,
				Since:   m.Since,
				Until:   m.Until,
			})
		}
		plan.Households = append(plan.Households, MappedHousehold{
			Ref:     bundleRef("household", h.ID),
			ID:      h.ID,
			Name:    h.Name,
			Type:    h.Type,
			Address: h.Address,
			Members: members,
		})
	}

	for _, c := range bundle.Plan.Circles {
		members := make([]SourceRef, 0, len(c.MemberUIDs))
		for _, uid := range c.MemberUIDs {
			members = append(members, bundleContactRef(uid))
		}
		plan.Circles = append(plan.Circles, MappedCircle{Ref: bundleRef("circle", c.ID), ID: c.ID, Name: c.Name, Members: members})
	}

	for _, t := range bundle.Plan.Tags {
		contacts := make([]SourceRef, 0, len(t.ContactUIDs))
		for _, uid := range t.ContactUIDs {
			contacts = append(contacts, bundleContactRef(uid))
		}
		plan.Tags = append(plan.Tags, MappedTag{Ref: bundleRef("tag", t.ID), ID: t.ID, Name: t.Name, Contacts: contacts})
	}

	for _, d := range bundle.Plan.CustomFieldDefinitions {
		plan.FieldDefinitions = append(plan.FieldDefinitions, MappedFieldDefinition{
			Ref:         bundleRef("field_definition", d.ID),
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

	for _, v := range bundle.Plan.CustomFieldValues {
		plan.CustomFields = append(plan.CustomFields, MappedCustomField{
			Ref:               bundleRef("field_value", v.FieldDefinitionID+"/"+v.EntityID),
			Contact:           bundleContactRef(v.EntityID),
			RawValue:          v.Value,
			FieldDefinitionID: v.FieldDefinitionID,
		})
	}

	for _, o := range bundle.Plan.Occasions {
		plan.Occasions = append(plan.Occasions, MappedOccasion{
			Ref:               bundleRef("occasion", o.ID),
			ID:                o.ID,
			Contact:           bundleContactRef(o.EntityID),
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

	for _, e := range bundle.Plan.OccasionEvents {
		attendees := make([]MappedOccasionAttendee, 0, len(e.Attendees))
		for _, a := range e.Attendees {
			attendees = append(attendees, MappedOccasionAttendee{Contact: bundleContactRef(a.EntityID), RSVP: a.RSVP})
		}
		plan.OccasionEvents = append(plan.OccasionEvents, MappedOccasionEvent{
			Ref:         bundleRef("occasion_event", e.ID),
			ID:          e.ID,
			Title:       e.Title,
			StartsAt:    formatBundleTime(e.StartsAt),
			EndsAt:      e.EndsAt,
			Location:    e.Location,
			Sensitivity: e.Sensitivity,
			Notes:       e.Notes,
			Attendees:   attendees,
		})
	}

	return plan
}

// bundleContactRef namespaces a contact reference by its stable vcard_uid.
func bundleContactRef(uid string) SourceRef {
	if uid == "" {
		return SourceRef{System: "mycorrhizal", ExternalID: "contact/"}
	}
	return SourceRef{System: "mycorrhizal", ExternalID: "contact/" + uid}
}

func bundleRef(kind, id string) SourceRef {
	return SourceRef{System: "mycorrhizal", ExternalID: kind + "/" + id}
}

func formatBundleTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatBundleTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatBundleTime(*t)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
