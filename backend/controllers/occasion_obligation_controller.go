package controllers

import (
	"errors"
	"fmt"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// validateOccasionAnchorPair enforces ADR 0024's "both or neither" rule for
// AnchorMonth/AnchorDay — a partial anchor (only a month, or only a day) has
// no meaningful annual occurrence, mirroring validateGiftValueCurrency's
// cross-field pattern (this codebase has no cross-field struct-tag
// validator).
func validateOccasionAnchorPair(input *models.OccasionObligationInput) *apperrors.AppError {
	if (input.AnchorMonth == nil) != (input.AnchorDay == nil) {
		return apperrors.ErrValidation("An occasion anchor date requires both a month and a day, or neither")
	}
	return nil
}

// syncOccasionObligationReminder keeps a materialized Reminder row in sync
// with an OccasionObligation (docs/adrs/0024-occasions.md, issue #387, ticket
// #1223) — the same pattern syncLifeEventReminder (life_event_controller.go)
// uses, with one difference: the reminder fires LeadTimeDays before the
// anchor date, not on it. Hard-deletes any existing reminder for this
// obligation (machine-synthesized, not user-authored — no soft-delete),
// then creates a new yearly one if Active and the obligation has a valid
// anchor month/day.
//
// Recurrence "yearly" (not "once") is deliberate: services.CalculateNextReminderTime
// already regenerates any yearly reminder's next occurrence generically on
// completion, so — unlike a one-off reminder — this single row keeps itself
// current year over year with no separate regeneration job. The lead-time
// offset survives every future regeneration automatically, since AddDate
// preserves the fixed day-of-year gap between the reminder and the (implicit)
// anchor.
//
// A RemindAt that lands in the past (the next occurrence's anchor is closer
// than LeadTimeDays) is valid, not an error: the reminder is simply
// immediately due, which is the correct behavior for an obligation just
// created close to its own anchor date — the same "overdue is a real state"
// convention every other reminder in this codebase already follows.
func syncOccasionObligationReminder(tx *gorm.DB, userID uint, obligation *models.OccasionObligation, now time.Time, loc *time.Location) error {
	if err := tx.Where("reminder_id IN (SELECT id FROM reminders WHERE occasion_obligation_id = ?)", obligation.ID).Delete(&models.NotificationDelivery{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("occasion_obligation_id = ?", obligation.ID).Delete(&models.Reminder{}).Error; err != nil {
		return err
	}

	if !obligation.Active {
		return nil
	}
	if obligation.AnchorMonth == nil || obligation.AnchorDay == nil {
		return nil
	}

	contactID, err := getContactIDByVCardUID(tx, userID, obligation.EntityID)
	if err != nil {
		return err
	}
	if contactID == nil {
		return nil
	}

	remindAt := nextRemindAt(*obligation.AnchorMonth, *obligation.AnchorDay, now, loc).AddDate(0, 0, -obligation.LeadTimeDays)

	contactName := obligation.EntityID // fallback: show VCardUID in message if name lookup fails
	var contact models.Contact
	if tx.Where("vcard_uid = ? AND user_id = ?", obligation.EntityID, userID).First(&contact).Error == nil {
		contactName = contact.Firstname + " " + contact.Lastname
	}

	reminder := models.Reminder{
		UserID:               userID,
		Message:              fmt.Sprintf("Occasion — %s: %s", obligation.Label, contactName),
		RemindAt:             remindAt,
		Recurrence:           "yearly",
		ContactID:            contactID,
		OccasionObligationID: &obligation.ID,
	}
	return tx.Create(&reminder).Error
}

// CreateOccasionObligation creates a new OccasionObligation
// (occasion_obligation.go) for the authenticated user, scoped to a Contact
// they own via EntityID. Active defaults to true when omitted; Sensitivity
// defaults to normal.
func CreateOccasionObligation(c *gin.Context) {
	input, err := middleware.GetValidated[models.OccasionObligationInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}
	if vErr := validateOccasionAnchorPair(input); vErr != nil {
		apperrors.AbortWithError(c, vErr)
		return
	}
	if !verifyOwnedLifeEvent(c, db, userID, input.LinkedLifeEventID) {
		return
	}

	active := true
	if input.Active != nil {
		active = *input.Active
	}
	sensitivity := input.Sensitivity
	if sensitivity == "" {
		sensitivity = models.RelationshipSensitivityNormal
	}

	cfg := currentConfig(c)
	loc := cfg.GetReminderLocation()
	now := time.Now().In(loc)

	var obligation models.OccasionObligation
	txErr := db.Transaction(func(tx *gorm.DB) error {
		obligation = models.OccasionObligation{
			UserID:            userID,
			EntityID:          input.EntityID,
			Kind:              input.Kind,
			Label:             input.Label,
			AnchorMonth:       input.AnchorMonth,
			AnchorDay:         input.AnchorDay,
			LinkedLifeEventID: input.LinkedLifeEventID,
			LeadTimeDays:      input.LeadTimeDays,
			Active:            active,
			Sensitivity:       sensitivity,
			Notes:             input.Notes,
		}
		if err := tx.Create(&obligation).Error; err != nil {
			return err
		}
		return syncOccasionObligationReminder(tx, userID, &obligation, now, loc)
	})
	if txErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save occasion obligation").WithError(txErr))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Occasion obligation created successfully", "occasion_obligation": obligation})
}

// GetOccasionObligation returns one OccasionObligation.
func GetOccasionObligation(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var obligation models.OccasionObligation
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&obligation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Occasion obligation").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion obligation").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, obligation)
}

// ListOccasionObligations returns the authenticated user's
// OccasionObligations, cursor-paginated (T17), optionally filtered by
// ?entity_id=<Contact.VCardUID> in browse mode. Mirrors ListPreferences'
// exact shape (small, bounded-per-contact collection, so `total` stays in
// browse mode — unlike the unbounded Gift/LifeEvent timelines).
func ListOccasionObligations(c *gin.Context) {
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

	var obligations []models.OccasionObligation

	if params.Since {
		query := db.Unscoped().Model(&models.OccasionObligation{}).Where("user_id = ?", userID)
		if params.Cursor != nil {
			pred, t, idv := cursorPredicate("occasion_obligations", params.Cursor, params.Cursor.ID, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "occasion_obligations", false).Limit(params.Limit + 1)
		if err := query.Find(&obligations).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion obligations").WithError(err))
			return
		}
		nextCursor := ""
		if len(obligations) > params.Limit {
			obligations = obligations[:params.Limit]
			nextCursor = EncodeCursor(obligations[len(obligations)-1].UpdatedAt, obligations[len(obligations)-1].ID)
		}
		for i := range obligations {
			obligations[i].Deleted = obligations[i].DeletedAt.Valid
		}
		c.JSON(http.StatusOK, gin.H{
			"occasion_obligations": obligations,
			"next_cursor":          nextCursor,
			"limit":                params.Limit,
			"sync":                 buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	entityID := c.Query("entity_id")

	baseQuery := db.Model(&models.OccasionObligation{}).Where("user_id = ?", userID)
	if entityID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", entityID)
	}

	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count occasion obligations").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("occasion_obligations", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "occasion_obligations", desc).
		Limit(params.Limit + 1).
		Find(&obligations).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion obligations").WithError(err))
		return
	}
	nextCursor := ""
	if len(obligations) > params.Limit {
		obligations = obligations[:params.Limit]
		nextCursor = EncodeCursor(obligations[len(obligations)-1].UpdatedAt, obligations[len(obligations)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"occasion_obligations": obligations,
		"next_cursor":          nextCursor,
		"limit":                params.Limit,
		"total":                total,
		"sync":                 buildSyncMeta(SyncModeIncremental),
	})
}

// UpdateOccasionObligation updates an OccasionObligation — full-replace
// semantics via the same OccasionObligationInput as create, matching
// UpdatePreference's own precedent.
func UpdateOccasionObligation(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var obligation models.OccasionObligation
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&obligation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Occasion obligation").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion obligation").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.OccasionObligationInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}
	if vErr := validateOccasionAnchorPair(input); vErr != nil {
		apperrors.AbortWithError(c, vErr)
		return
	}
	if !verifyOwnedLifeEvent(c, db, userID, input.LinkedLifeEventID) {
		return
	}

	active := true
	if input.Active != nil {
		active = *input.Active
	}
	sensitivity := input.Sensitivity
	if sensitivity == "" {
		sensitivity = models.RelationshipSensitivityNormal
	}

	obligation.EntityID = input.EntityID
	obligation.Kind = input.Kind
	obligation.Label = input.Label
	obligation.AnchorMonth = input.AnchorMonth
	obligation.AnchorDay = input.AnchorDay
	obligation.LinkedLifeEventID = input.LinkedLifeEventID
	obligation.LeadTimeDays = input.LeadTimeDays
	obligation.Active = active
	obligation.Sensitivity = sensitivity
	obligation.Notes = input.Notes

	cfg := currentConfig(c)
	loc := cfg.GetReminderLocation()
	now := time.Now().In(loc)

	txErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&obligation).Error; err != nil {
			return err
		}
		return syncOccasionObligationReminder(tx, userID, &obligation, now, loc)
	})
	if txErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save occasion obligation").WithError(txErr))
		return
	}

	c.JSON(http.StatusOK, obligation)
}

// DeleteOccasionObligation soft-deletes an OccasionObligation (user-authored
// content).
func DeleteOccasionObligation(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var obligation models.OccasionObligation
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&obligation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Occasion obligation").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve occasion obligation").WithError(err))
		}
		return
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("reminder_id IN (SELECT id FROM reminders WHERE occasion_obligation_id = ?)", obligation.ID).Delete(&models.NotificationDelivery{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("occasion_obligation_id = ?", obligation.ID).Delete(&models.Reminder{}).Error; err != nil {
			return err
		}
		return tx.Delete(&obligation).Error
	})
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete occasion obligation").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Occasion obligation deleted"})
}
