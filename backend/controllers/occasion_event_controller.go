package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OccasionEvent controllers (docs/adrs/0026-occasions-events.md, issue #1228).
// An OccasionEvent is a one-off event the user is hosting; attendees are a
// nested sub-resource (add / set-RSVP / remove), never a bulk-replace field
// (CLAUDE.md backend convention). RSVP is a manually-recorded status, not a
// delivered invitation — this feature sends nothing.

// validateOccasionEventTimes enforces ends_at >= starts_at when an end is
// given — the same cross-field controller check validateOccasionAnchorPair
// uses (this codebase has no cross-field struct-tag validator).
func validateOccasionEventTimes(input *models.OccasionEventInput) *apperrors.AppError {
	if input.EndsAt != nil && input.EndsAt.Before(*input.StartsAt) {
		return apperrors.ErrValidation("Event end time cannot be before its start time")
	}
	return nil
}

// eventContactName mirrors services.contactDisplayName / GetUpcomingBirthdays'
// own name-building (nickname preferred when present) — duplicated locally
// because controllers cannot call the unexported service helper.
func eventContactName(contact *models.Contact) string {
	name := contact.Firstname
	if contact.Nickname != "" {
		name = contact.Nickname
	}
	if contact.Lastname != "" {
		name += " " + contact.Lastname
	}
	if strings.TrimSpace(name) == "" {
		name = contact.VCardUID
	}
	return name
}

// loadOwnedOccasionEvent fetches one event scoped to the caller, aborting the
// request with a 404/500 and returning ok=false on failure.
func loadOwnedOccasionEvent(c *gin.Context, db *gorm.DB, userID uint, id string) (models.OccasionEvent, bool) {
	var event models.OccasionEvent
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Occasion event").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion event").WithError(err))
		}
		return models.OccasionEvent{}, false
	}
	return event, true
}

// attendeeViews loads one event's attendees enriched with their contact's
// display data. A nil result is normalized to an empty slice (CLAUDE.md
// frontend trap #8 — a collection field must never serialize as an absent key).
func attendeeViews(db *gorm.DB, userID uint, eventID string) ([]models.OccasionEventAttendeeView, error) {
	var attendees []models.OccasionEventAttendee
	if err := db.Where("event_id = ? AND user_id = ?", eventID, userID).Order("created_at asc").Find(&attendees).Error; err != nil {
		return nil, err
	}
	views := []models.OccasionEventAttendeeView{}
	if len(attendees) == 0 {
		return views, nil
	}

	uids := make([]string, 0, len(attendees))
	for _, a := range attendees {
		uids = append(uids, a.EntityID)
	}
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
		return nil, err
	}
	byUID := make(map[string]models.Contact, len(contacts))
	for _, c := range contacts {
		byUID[c.VCardUID] = c
	}

	for _, a := range attendees {
		view := models.OccasionEventAttendeeView{
			ID: a.ID, EventID: a.EventID, EntityID: a.EntityID, RSVP: a.RSVP,
		}
		if contact, ok := byUID[a.EntityID]; ok {
			view.ContactID = contact.ID
			view.ContactName = eventContactName(&contact)
		}
		views = append(views, view)
	}
	return views, nil
}

// CreateOccasionEvent creates a new OccasionEvent for the authenticated user.
// Sensitivity defaults to normal.
func CreateOccasionEvent(c *gin.Context) {
	input, err := middleware.GetValidated[models.OccasionEventInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	if vErr := validateOccasionEventTimes(input); vErr != nil {
		apperrors.AbortWithError(c, vErr)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	sensitivity := input.Sensitivity
	if sensitivity == "" {
		sensitivity = models.RelationshipSensitivityNormal
	}

	event := models.OccasionEvent{
		UserID:      userID,
		Title:       input.Title,
		StartsAt:    *input.StartsAt,
		EndsAt:      input.EndsAt,
		Location:    input.Location,
		Sensitivity: sensitivity,
		Notes:       input.Notes,
	}
	if err := db.Create(&event).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save occasion event").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Occasion event created successfully", "occasion_event": event})
}

// GetOccasionEvent returns one event plus its attendee/RSVP list.
func GetOccasionEvent(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	event, ok := loadOwnedOccasionEvent(c, db, userID, id)
	if !ok {
		return
	}
	views, err := attendeeViews(db, userID, event.ID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion event attendees").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"occasion_event": event, "attendees": views})
}

// ListOccasionEvents returns the authenticated user's events, cursor-paginated
// (T17), optionally filtered by a ?from=/?to= RFC 3339 window on starts_at.
// Mirrors ListOccasionObligations' exact shape, including the ?since= change
// feed with soft-delete tombstones.
func ListOccasionEvents(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	params, err := GetCursorParams(c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	if err := CheckFeedCursorAge(c, params); err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	from, to, windowErr := parseEventWindow(c)
	if windowErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrValidation("from and to must be RFC 3339 timestamps"))
		return
	}

	var events []models.OccasionEvent

	if params.Since {
		query := db.Unscoped().Model(&models.OccasionEvent{}).Where("user_id = ?", userID)
		if params.Cursor != nil {
			pred, t, idv := cursorPredicate("occasion_events", params.Cursor, params.Cursor.ID, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "occasion_events", false).Limit(params.Limit + 1)
		if err := query.Find(&events).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion events").WithError(err))
			return
		}
		nextCursor := ""
		if len(events) > params.Limit {
			events = events[:params.Limit]
			nextCursor = EncodeCursor(events[len(events)-1].UpdatedAt, events[len(events)-1].ID)
		}
		for i := range events {
			events[i].Deleted = events[i].DeletedAt.Valid
		}
		c.JSON(http.StatusOK, gin.H{
			"occasion_events": events,
			"next_cursor":     nextCursor,
			"limit":           params.Limit,
			"sync":            buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	baseQuery := db.Model(&models.OccasionEvent{}).Where("user_id = ?", userID)
	if from != nil {
		baseQuery = baseQuery.Where("starts_at >= ?", *from)
	}
	if to != nil {
		baseQuery = baseQuery.Where("starts_at <= ?", *to)
	}

	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count occasion events").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("occasion_events", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "occasion_events", desc).
		Limit(params.Limit + 1).
		Find(&events).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion events").WithError(err))
		return
	}
	nextCursor := ""
	if len(events) > params.Limit {
		events = events[:params.Limit]
		nextCursor = EncodeCursor(events[len(events)-1].UpdatedAt, events[len(events)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"occasion_events": events,
		"next_cursor":     nextCursor,
		"limit":           params.Limit,
		"total":           total,
		"sync":            buildSyncMeta(SyncModeIncremental),
	})
}

// parseEventWindow reads the optional ?from=/?to= RFC 3339 bounds on starts_at.
func parseEventWindow(c *gin.Context) (from, to *time.Time, err error) {
	if raw := c.Query("from"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		from = &parsed
	}
	if raw := c.Query("to"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		to = &parsed
	}
	return from, to, nil
}

// UpdateOccasionEvent updates an OccasionEvent — full-replace semantics via the
// same OccasionEventInput as create, matching UpdateOccasionObligation's own
// precedent. Attendees are not touched here (nested sub-resources own them).
func UpdateOccasionEvent(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	event, ok := loadOwnedOccasionEvent(c, db, userID, id)
	if !ok {
		return
	}

	input, err := middleware.GetValidated[models.OccasionEventInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	if vErr := validateOccasionEventTimes(input); vErr != nil {
		apperrors.AbortWithError(c, vErr)
		return
	}

	sensitivity := input.Sensitivity
	if sensitivity == "" {
		sensitivity = models.RelationshipSensitivityNormal
	}

	event.Title = input.Title
	event.StartsAt = *input.StartsAt
	event.EndsAt = input.EndsAt
	event.Location = input.Location
	event.Sensitivity = sensitivity
	event.Notes = input.Notes

	if err := db.Save(&event).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save occasion event").WithError(err))
		return
	}

	c.JSON(http.StatusOK, event)
}

// DeleteOccasionEvent soft-deletes an event (user-authored content) and
// hard-deletes its attendees in the same transaction — attendees are
// join-shaped rows, so they leave with their parent (mirrors
// DeleteOccasionObligation's hard-delete of the materialized reminder).
func DeleteOccasionEvent(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	event, ok := loadOwnedOccasionEvent(c, db, userID, id)
	if !ok {
		return
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("event_id = ? AND user_id = ?", event.ID, userID).Delete(&models.OccasionEventAttendee{}).Error; err != nil {
			return err
		}
		return tx.Delete(&event).Error
	})
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete occasion event").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Occasion event deleted"})
}

// AddOccasionEventAttendee invites one owned contact to an event. A duplicate
// add is a checked 409 ErrAlreadyExists (not a sniffed constraint error),
// mirroring AddCircleMember.
func AddOccasionEventAttendee(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	event, ok := loadOwnedOccasionEvent(c, db, userID, id)
	if !ok {
		return
	}

	input, err := middleware.GetValidated[models.OccasionEventAttendeeInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	var existing models.OccasionEventAttendee
	lookupErr := db.Where("event_id = ? AND entity_id = ? AND user_id = ?", event.ID, input.EntityID, userID).First(&existing).Error
	if lookupErr == nil {
		apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Event attendee"))
		return
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to check existing event attendee").WithError(lookupErr))
		return
	}

	rsvp := input.RSVP
	if rsvp == "" {
		rsvp = models.OccasionEventRSVPPending
	}

	attendee := models.OccasionEventAttendee{
		UserID:   userID,
		EventID:  event.ID,
		EntityID: input.EntityID,
		RSVP:     rsvp,
	}
	if err := db.Create(&attendee).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to add event attendee").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Attendee added", "attendee": attendee})
}

// UpdateOccasionEventAttendee records an attendee's RSVP status (docs/adrs/
// 0026-occasions-events.md part 2 — the user records what the contact told
// them; this sends nothing).
func UpdateOccasionEventAttendee(c *gin.Context) {
	id := c.Param("id")
	vcardUID := c.Param("vcard_uid")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	event, ok := loadOwnedOccasionEvent(c, db, userID, id)
	if !ok {
		return
	}

	var attendee models.OccasionEventAttendee
	if err := db.Where("event_id = ? AND entity_id = ? AND user_id = ?", event.ID, vcardUID, userID).First(&attendee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Event attendee").WithDetails("vcard_uid", vcardUID))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve event attendee").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.OccasionEventAttendeeUpdateInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	attendee.RSVP = input.RSVP
	if err := db.Save(&attendee).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save event attendee").WithError(err))
		return
	}

	c.JSON(http.StatusOK, attendee)
}

// RemoveOccasionEventAttendee removes one contact from an event's attendee list.
func RemoveOccasionEventAttendee(c *gin.Context) {
	id := c.Param("id")
	vcardUID := c.Param("vcard_uid")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	event, ok := loadOwnedOccasionEvent(c, db, userID, id)
	if !ok {
		return
	}

	result := db.Where("event_id = ? AND entity_id = ? AND user_id = ?", event.ID, vcardUID, userID).
		Delete(&models.OccasionEventAttendee{})
	if result.Error != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to remove event attendee").WithError(result.Error))
		return
	}
	if result.RowsAffected == 0 {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Event attendee").WithDetails("vcard_uid", vcardUID))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Attendee removed"})
}

// GetInviteeSuggestions expands one or more owned circles into candidate
// invitees (docs/adrs/0026-occasions-events.md part 3) — every member of the
// named circles, de-duplicated across circles and (when ?event_id= names an
// owned event) excluding contacts already invited. Read-only: it mints nothing.
func GetInviteeSuggestions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	raw := strings.TrimSpace(c.Query("circle_ids"))
	if raw == "" {
		apperrors.AbortWithError(c, apperrors.ErrValidation("circle_ids is required"))
		return
	}
	requested := map[string]bool{}
	var circleIDs []string
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" || requested[id] {
			continue
		}
		requested[id] = true
		circleIDs = append(circleIDs, id)
	}
	if len(circleIDs) == 0 {
		apperrors.AbortWithError(c, apperrors.ErrValidation("circle_ids is required"))
		return
	}

	var circles []models.Circle
	if err := db.Where("user_id = ? AND id IN ?", userID, circleIDs).Find(&circles).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve circles").WithError(err))
		return
	}
	if len(circles) != len(circleIDs) {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Circle").WithDetails("circle_ids", raw))
		return
	}

	exclude := map[string]bool{}
	if eventID := c.Query("event_id"); eventID != "" {
		event, ok := loadOwnedOccasionEvent(c, db, userID, eventID)
		if !ok {
			return
		}
		var existing []models.OccasionEventAttendee
		if err := db.Where("event_id = ? AND user_id = ?", event.ID, userID).Find(&existing).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve event attendees").WithError(err))
			return
		}
		for _, a := range existing {
			exclude[a.EntityID] = true
		}
	}

	var members []models.CircleMember
	if err := db.Where("user_id = ? AND circle_id IN ?", userID, circleIDs).Find(&members).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve circle members").WithError(err))
		return
	}
	seen := map[string]bool{}
	var uids []string
	for _, m := range members {
		if m.MemberVCardUID == "" || seen[m.MemberVCardUID] || exclude[m.MemberVCardUID] {
			continue
		}
		seen[m.MemberVCardUID] = true
		uids = append(uids, m.MemberVCardUID)
	}

	suggestions := []models.InviteeSuggestion{}
	if len(uids) > 0 {
		var contacts []models.Contact
		if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve suggested contacts").WithError(err))
			return
		}
		for _, contact := range contacts {
			suggestions = append(suggestions, models.InviteeSuggestion{
				ContactID: contact.ID, ContactName: eventContactName(&contact), EntityID: contact.VCardUID,
			})
		}
	}
	sort.Slice(suggestions, func(i, j int) bool { return suggestions[i].ContactName < suggestions[j].ContactName })

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions, "circle_ids": circleIDs})
}
