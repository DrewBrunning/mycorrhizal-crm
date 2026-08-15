package controllers

import (
	"errors"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetContactDetail returns the M4 contact-detail composite
// (M4): everything
// ContactDetailPage.tsx renders for one contact, in one call — the record
// itself plus every sub-resource block the page currently fetches with
// ~21 separate requests. Read-only, no writes, no cache; follows the N2
// briefing composite's pattern (briefing_controller.go) at a larger scale.
// The profile picture is deliberately NOT included (still its own request —
// a blob doesn't belong in this JSON envelope).
func GetContactDetail(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	contact, ok := resolveOwnedContactByID(c, db, userID, c.Param("id"))
	if !ok {
		return
	}

	detail, err := buildContactDetail(db, userID, contact, currentConfig(c))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compose contact detail").WithError(err))
		return
	}

	c.JSON(http.StatusOK, detail)
}

// buildContactDetail composes a ContactDetailResponse from a contact's
// existing data. Pure — performs no writes. Every sub-query is scoped by
// user_id (CLAUDE.md trap 5); soft-deleted rows are excluded by GORM's
// default deleted_at IS NULL scoping, matching each source query's existing
// per-resource handler.
//
// T66 bounds the six timeline-eligible blocks at timelinePreviewLimit each
// (see the inline comment on the notes query for the reasoning); the
// remaining blocks stay unpaginated by design (M4 design decision 6).
func buildContactDetail(db *gorm.DB, userID uint, contact *models.Contact, cfg config.Config) (*models.ContactDetailResponse, error) {
	detail := &models.ContactDetailResponse{
		Contact: models.NewContactRecordResponse(contact, cfg.ProfilePhotoDir, db),
	}

	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	detail.User = models.ContactDetailUser{EnabledContactFields: user.EnabledContactFields}

	// T66: the six timeline-eligible blocks are bounded at timelinePreviewLimit
	// (5) — enough to build the default 5-item preview correctly regardless of
	// type distribution (any item in the global top 5 must be within its own
	// type's top 5), each ordered by the timeline's event-date key. The other
	// blocks (reminders, agenda, field values, identities, circles, tags) stay
	// unpaginated — they are bounded-small or the screen's complete set.
	if err := db.Where("contact_id = ? AND user_id = ?", contact.ID, userID).
		Order("date DESC").Limit(timelinePreviewLimit).Find(&detail.Notes).Error; err != nil {
		return nil, err
	}

	if err := db.Model(&models.Activity{}).
		Preload("Contacts", func(db *gorm.DB) *gorm.DB {
			return db.Select("ID", "Firstname", "Lastname", "PhotoThumbnail", "Circles").Where("user_id = ?", userID)
		}).
		Joins("JOIN activity_contacts ON activities.id = activity_contacts.activity_id").
		Where("activities.user_id = ? AND activity_contacts.contact_id = ?", userID, contact.ID).
		Order("activities.date DESC").
		Limit(timelinePreviewLimit).
		Find(&detail.Activities).Error; err != nil {
		return nil, err
	}

	if err := db.Where("contact_id = ? AND user_id = ?", contact.ID, userID).
		Order("completed_at DESC").Limit(timelinePreviewLimit).Find(&detail.Completions).Error; err != nil {
		return nil, err
	}

	if err := db.Where("contact_id = ? AND user_id = ?", contact.ID, userID).
		Order("remind_at ASC").Find(&detail.Reminders).Error; err != nil {
		return nil, err
	}

	rels, err := resolveConfirmedRelationships(db, userID, contact)
	if err != nil {
		return nil, err
	}
	detail.RelationshipEdges = rels

	if err := attachContactDetailLifeEvents(db, userID, contact, detail); err != nil {
		return nil, err
	}

	if err := db.Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).
		Order("created_at DESC").Find(&detail.Agenda).Error; err != nil {
		return nil, err
	}

	// Gifts are ordered by date (not created_at) so the bounded block doubles
	// as the timeline preview's gift slice: undated ideas sort last and fall
	// out of the top 5, matching the timeline's "undated ideas are not
	// events" rule while the status filter stays on the timeline endpoint.
	if err := db.Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).
		Order("date DESC").Limit(timelinePreviewLimit).Find(&detail.Gifts).Error; err != nil {
		return nil, err
	}

	if err := db.Where("entity_id = ? AND user_id = ?", contact.VCardUID, userID).
		Find(&detail.FieldValues).Error; err != nil {
		return nil, err
	}

	if err := db.Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).
		Find(&detail.ExternalIdentities).Error; err != nil {
		return nil, err
	}

	if err := db.Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).
		Order("occurred_at DESC").Limit(timelinePreviewLimit).Find(&detail.ExternalActivities).Error; err != nil {
		return nil, err
	}

	if err := attachContactDetailCircles(db, userID, contact, detail); err != nil {
		return nil, err
	}
	if err := attachContactDetailTags(db, userID, contact, detail); err != nil {
		return nil, err
	}

	if err := attachContactDetailImmich(db, cfg, userID, contact, detail); err != nil {
		return nil, err
	}

	normalizeContactDetailSlices(detail)
	return detail, nil
}

// attachContactDetailLifeEvents fetches the contact's life events and
// batch-resolves every RelatedEntityIDs UID across all of them in one query
// (design decision 3 — never a per-event lookup).
func attachContactDetailLifeEvents(db *gorm.DB, userID uint, contact *models.Contact, detail *models.ContactDetailResponse) error {
	var events []models.LifeEvent
	if err := db.Where("user_id = ? AND entity_id = ?", userID, contact.VCardUID).
		Order("created_at DESC").Limit(timelinePreviewLimit).Find(&events).Error; err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	relatedUIDs := make([]string, 0)
	seen := make(map[string]bool)
	for _, e := range events {
		for _, uid := range e.RelatedEntityIDs {
			if uid != "" && !seen[uid] {
				seen[uid] = true
				relatedUIDs = append(relatedUIDs, uid)
			}
		}
	}

	nameByUID := make(map[string]string, len(relatedUIDs))
	if len(relatedUIDs) > 0 {
		var related []models.Contact
		if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, relatedUIDs).Find(&related).Error; err != nil {
			return err
		}
		for _, r := range related {
			nameByUID[r.VCardUID] = strings.TrimSpace(r.Firstname + " " + r.Lastname)
		}
	}

	detail.LifeEvents = make([]models.ContactDetailLifeEvent, len(events))
	for i, e := range events {
		names := make(map[string]string, len(e.RelatedEntityIDs))
		for _, uid := range e.RelatedEntityIDs {
			if name, ok := nameByUID[uid]; ok {
				names[uid] = name
			}
		}
		detail.LifeEvents[i] = models.ContactDetailLifeEvent{LifeEvent: e, RelatedEntityNames: names}
	}
	return nil
}

// attachContactDetailCircles resolves THIS contact's circle memberships
// (not the global per-user circle list GET /circles returns) via the
// circle_members join, scoped by user_id.
func attachContactDetailCircles(db *gorm.DB, userID uint, contact *models.Contact, detail *models.ContactDetailResponse) error {
	return db.Model(&models.Circle{}).
		Joins("JOIN circle_members ON circle_members.circle_id = circles.id").
		Where("circles.user_id = ? AND circle_members.user_id = ? AND circle_members.member_vcard_uid = ?", userID, userID, contact.VCardUID).
		Find(&detail.Circles).Error
}

// attachContactDetailTags resolves THIS contact's tags (not the global
// per-user tag list GET /tags returns) via the contact_tags join, scoped by
// user_id.
func attachContactDetailTags(db *gorm.DB, userID uint, contact *models.Contact, detail *models.ContactDetailResponse) error {
	return db.Model(&models.Tag{}).
		Joins("JOIN contact_tags ON contact_tags.tag_id = tags.id").
		Where("tags.user_id = ? AND contact_tags.user_id = ? AND contact_tags.contact_vcard_uid = ?", userID, userID, contact.VCardUID).
		Find(&detail.Tags).Error
}

// attachContactDetailImmich sets detail.Immich when the user has an Immich
// config at all (design decision 5) — regardless of whether this particular
// contact is linked to an Immich person. Summary stays nil when no link
// exists; a link is resolved into a live summary exactly like
// GetImmichContactSummary (immich_controller.go) does per-contact today.
func attachContactDetailImmich(db *gorm.DB, cfg config.Config, userID uint, contact *models.Contact, detail *models.ContactDetailResponse) error {
	immichCfg, err := services.GetImmichConfigForUser(db, userID)
	if err != nil {
		return err
	}
	if immichCfg == nil {
		return nil
	}

	block := &models.ContactDetailImmich{}
	detail.Immich = block

	var identity models.ExternalIdentity
	err = db.Where("user_id = ? AND system = ? AND entity_id = ?", userID, services.ExternalSystemImmich, contact.VCardUID).First(&identity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	summary := services.FetchImmichPersonSummary(db, cfg, userID, &identity)
	if summary == nil {
		return nil
	}
	block.Summary = &models.ImmichPersonSummary{
		Identity:      summary.Identity,
		PersonName:    summary.PersonName,
		PhotoCount:    summary.PhotoCount,
		LatestAssetID: summary.LatestAssetID,
		LatestAt:      summary.LatestAt,
	}
	return nil
}

// normalizeContactDetailSlices replaces every nil collection block with an
// empty slice so the response always serializes them as `[]`, never `null`/
// absent — the normalizeBriefingSlices discipline (briefing_controller.go),
// applied to the larger M4 set. This is the single most likely regression
// per the ticket's own traps section.
func normalizeContactDetailSlices(d *models.ContactDetailResponse) {
	if d.Notes == nil {
		d.Notes = []models.Note{}
	}
	if d.Activities == nil {
		d.Activities = []models.Activity{}
	}
	if d.Completions == nil {
		d.Completions = []models.ReminderCompletion{}
	}
	if d.Reminders == nil {
		d.Reminders = []models.Reminder{}
	}
	if d.RelationshipEdges == nil {
		d.RelationshipEdges = []models.BriefingRelationship{}
	}
	if d.LifeEvents == nil {
		d.LifeEvents = []models.ContactDetailLifeEvent{}
	}
	if d.Agenda == nil {
		d.Agenda = []models.ConversationAgenda{}
	}
	if d.Gifts == nil {
		d.Gifts = []models.Gift{}
	}
	if d.FieldValues == nil {
		d.FieldValues = []models.FieldValue{}
	}
	if d.ExternalIdentities == nil {
		d.ExternalIdentities = []models.ExternalIdentity{}
	}
	if d.ExternalActivities == nil {
		d.ExternalActivities = []models.ExternalActivity{}
	}
	if d.Circles == nil {
		d.Circles = []models.Circle{}
	}
	if d.Tags == nil {
		d.Tags = []models.Tag{}
	}
}
