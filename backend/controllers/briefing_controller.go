package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetContactBriefing returns the N2 "prep view" composition for one contact
// (docs/fork-plan/tickets/22-N2-prep-view.md): everything the user needs to
// remember about this person before seeing them, in one response.
//
// It is read-only — a pure aggregation of existing data (activities, notes,
// cadence health, agenda items, relationship edges, life events, reminders,
// upcoming dates). No writes, no new tables, no cache; each block degrades to
// its zero value when the source is empty or the feature isn't built yet.
//
// Ownership: the contact must belong to the caller (user_id scoping, the
// standing CLAUDE.md trap 5). Sensitivity (91.13): relationship edges with
// `secret` sensitivity are excluded in the query — a secret relationship must
// not surface on a screen likely to be open in front of the person it
// concerns (private relationships stay: the briefing is the user's own
// screen, and private gates sharing/exposure, which this endpoint is not).
// Preference/custom-field sensitivity filtering is handled by those features'
// own projection paths; this endpoint composes the non-sensitive contact data
// the briefing is built from.
func GetContactBriefing(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	briefing, err := buildContactBriefing(db, userID, &contact, time.Now())
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compose contact briefing").WithError(err))
		return
	}

	c.JSON(http.StatusOK, briefing)
}

// buildContactBriefing composes a ContactBriefing from a contact's existing
// data. Pure — performs no writes. `now` anchors "today" for upcoming dates
// and cadence health (callers pass the reminder location).
func buildContactBriefing(db *gorm.DB, userID uint, contact *models.Contact, now time.Time) (*models.ContactBriefing, error) {
	briefing := &models.ContactBriefing{
		ContactID:      contact.ID,
		UID:            contact.VCardUID,
		Name:           strings.TrimSpace(contact.Firstname + " " + contact.Lastname),
		PhotoThumbnail: contact.PhotoThumbnail,
		Kind:           contact.CRM.Kind,
	}

	// --- Last activity ("what happened between us" anchor). The most recent
	// activity linked to this contact via the join table, scoped to the
	// user on both sides (same double-sided scoping the cadence derivation
	// uses — the join table is where a cross-user link could lurk).
	var lastActivity models.Activity
	if err := db.Model(&models.Activity{}).
		Joins("JOIN activity_contacts ac ON ac.activity_id = activities.id").
		Joins("JOIN contacts c ON c.id = ac.contact_id").
		Where("c.vcard_uid = ? AND c.user_id = ? AND activities.user_id = ? AND activities.deleted_at IS NULL",
			contact.VCardUID, userID, userID).
		Order("activities.date DESC").
		First(&lastActivity).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if lastActivity.ID != 0 {
		briefing.LastActivity = &models.BriefingActivity{
			ID:          lastActivity.ID,
			UUID:        lastActivity.UUID,
			Title:       lastActivity.Title,
			Description: lastActivity.Description,
			Type:        lastActivity.Type,
			Location:    lastActivity.Location,
			Date:        lastActivity.Date,
		}
	}

	// --- Recent notes ("what was discussed"): the contact's most recent
	// notes, newest first, capped.
	var notes []models.Note
	if err := db.Where("contact_id = ? AND user_id = ?", contact.ID, userID).
		Order("date DESC").
		Limit(models.BriefingNotesLimit).
		Find(&notes).Error; err != nil {
		return nil, err
	}
	briefing.RecentNotes = notes

	// --- Cadence health (T19, derived, never stored). Only when the contact
	// has a policy; otherwise the block degrades to absent.
	var policies []models.CadencePolicy
	if err := db.Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).Find(&policies).Error; err != nil {
		return nil, err
	}
	if len(policies) > 0 {
		health, err := services.ComputeCadenceHealth(db, userID, &policies[0], now)
		if err != nil {
			return nil, err
		}
		briefing.Cadence = &models.BriefingCadence{
			Policy: policies[0],
			Health: models.BriefingCadenceHealth{
				HasQualifyingInteraction: health.HasQualifyingInteraction,
				LastInteraction:          health.LastInteraction,
				NextDue:                  health.NextDue,
				OverdueBy:                health.OverdueBy,
			},
		}
	}

	// --- Open agenda items (T21): not-yet-discussed items for this contact,
	// newest first.
	var agenda []models.ConversationAgenda
	if err := db.Where("user_id = ? AND entity_id = ? AND discussed_at IS NULL", userID, contact.VCardUID).
		Order("created_at DESC").
		Limit(50).
		Find(&agenda).Error; err != nil {
		return nil, err
	}
	briefing.OpenAgendaItems = agenda

	// --- Relationships: confirmed edges involving this contact, excluding
	// sensitive ones (91.13) in the query, resolved with the other party's
	// name and the display token read from this contact's perspective.
	if err := attachBriefingRelationships(db, userID, contact, briefing); err != nil {
		return nil, err
	}

	// --- Life events: most recent first, capped.
	var lifeEvents []models.LifeEvent
	if err := db.Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).
		Order("created_at DESC").
		Limit(models.BriefingLifeEventsLimit).
		Find(&lifeEvents).Error; err != nil {
		return nil, err
	}
	briefing.LifeEvents = lifeEvents

	// --- Upcoming reminders: incomplete reminders due within the window,
	// soonest first.
	window := now.AddDate(0, 0, models.BriefingReminderWindowDays)
	var reminders []models.Reminder
	if err := db.Where("contact_id = ? AND user_id = ? AND completed = ? AND remind_at <= ?",
		contact.ID, userID, false, window).
		Order("remind_at ASC").
		Find(&reminders).Error; err != nil {
		return nil, err
	}
	briefing.UpcomingReminders = reminders

	// --- Upcoming dates: birthday and anniversary, each with days-until.
	briefing.UpcomingDates = buildUpcomingDates(contact, now)

	return briefing, nil
}

// attachBriefingRelationships resolves the briefing's relationship block.
// Only status:confirmed edges are included; suggested edges are never fact.
// Sensitivity: only `secret` edges are excluded (91.13) — a secret
// relationship must not surface on a screen likely to be open in front of the
// person it concerns. `private` relationships stay: the briefing is the
// user's own screen, and private means "only you should see it" — the
// sensitivity tiers gate sharing/exposure (exports, sync, shared views), none
// of which this endpoint is.
func attachBriefingRelationships(db *gorm.DB, userID uint, contact *models.Contact, briefing *models.ContactBriefing) error {
	var edges []models.RelationshipEdge
	if err := db.Where(
		"user_id = ? AND status = ? AND sensitivity != ? AND (source_id = ? OR target_id = ?)",
		userID, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret,
		contact.VCardUID, contact.VCardUID,
	).Find(&edges).Error; err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}

	// Resolve the other-party display names in one query. The "other party"
	// is whichever endpoint isn't the viewed contact.
	otherUIDs := make([]string, 0, len(edges))
	seen := make(map[string]bool)
	for _, e := range edges {
		other := otherPartyUID(e, contact.VCardUID)
		if other != "" && !seen[other] {
			seen[other] = true
			otherUIDs = append(otherUIDs, other)
		}
	}
	var others []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, otherUIDs).Find(&others).Error; err != nil {
		return err
	}
	nameByUID := make(map[string]string, len(others))
	idByUID := make(map[string]uint, len(others))
	for _, o := range others {
		nameByUID[o.VCardUID] = strings.TrimSpace(o.Firstname + " " + o.Lastname)
		idByUID[o.VCardUID] = o.ID
	}

	briefing.Relationships = make([]models.BriefingRelationship, 0, len(edges))
	for _, e := range edges {
		otherUID := otherPartyUID(e, contact.VCardUID)
		briefing.Relationships = append(briefing.Relationships, models.BriefingRelationship{
			Edge:                e,
			OtherPartyContactID: idByUID[otherUID],
			OtherPartyName:      nameByUID[otherUID],
			OtherPartyUID:       otherUID,
			DisplayToken:        displayTokenFor(e, contact.VCardUID),
		})
	}
	return nil
}

// otherPartyUID returns whichever of SourceID/TargetID is not the viewed
// contact ("" when both endpoints are the viewed contact — a self-loop, which
// should not normally exist but is safe to ignore).
func otherPartyUID(edge models.RelationshipEdge, viewedUID string) string {
	if edge.SourceID == viewedUID {
		return edge.TargetID
	}
	return edge.SourceID
}

// displayTokenFor resolves the relationship_type_registry token describing the
// OTHER party from the viewed contact's perspective. The registry stores
// `type` as "SourceID's role relative to TargetID"; when the viewed contact is
// the source, the other (target) party's role is the inverse token.
func displayTokenFor(edge models.RelationshipEdge, viewedUID string) string {
	if edge.SourceID == viewedUID {
		return models.InverseRelationType(edge.Type)
	}
	return edge.Type
}

// buildUpcomingDates derives the briefing's upcoming-dates block from the
// flat Birthday/Anniversary columns, using services.DaysUntilBirthday for the
// days-until computation (its Dec 31 → Jan 1 wrap is the pinned, tested
// behavior). Dates in the past are excluded — "upcoming" only.
func buildUpcomingDates(contact *models.Contact, now time.Time) []models.BriefingUpcomingDate {
	var dates []models.BriefingUpcomingDate
	for _, item := range []struct {
		label string
		value string
	}{
		{"birthday", contact.Birthday},
		{"anniversary", contact.Anniversary},
	} {
		if item.value == "" {
			continue
		}
		days := services.DaysUntilBirthday(item.value, now)
		if days < 0 || days >= 366 {
			continue
		}
		dates = append(dates, models.BriefingUpcomingDate{
			Label:     item.label,
			Date:      item.value,
			DaysUntil: days,
		})
	}
	return dates
}
