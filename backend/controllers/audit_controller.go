package controllers

import (
	"encoding/json"
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListAuditEvents returns the caller's audit events, filtered by entity
// (entity_type + entity_id) when supplied. Immutable log — read-only surface.
func ListAuditEvents(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	query := db.Model(&models.AuditEvent{}).Where("user_id = ?", userID)
	if et := c.Query("entity_type"); et != "" {
		query = query.Where("entity_type = ?", et)
	}
	if eid := c.Query("entity_id"); eid != "" {
		query = query.Where("entity_id = ?", eid)
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	var events []models.AuditEvent
	if err := query.Order("created_at desc").Limit(limit).Find(&events).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to list audit events").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"audit_events": events, "total": len(events)})
}

// UndoAuditEvent reverts an update by restoring the event's before snapshot
// (T18, docs/fork-plan/tickets/34-T18-audit-trail.md). Deliberately updates
// only:
//
//   - delete events are rejected — restoring a cascade-deleted row across the
//     dependent tables is a follow-up, not this endpoint.
//   - events past the retention window are rejected (ErrGone): after
//     AUDIT_RETENTION_DAYS the purge has removed them, so a "revertable
//     until" surface would show a dead button; the API refuses instead of
//     silently no-op'ing.
//
// Implemented for Contact via the canonical ApplyRecordToContact path (the
// before snapshot's flat fields are re-derived into a neutral Record and
// applied, so the neutral Card/CRM/Passthrough columns are rebuilt from the
// restored flat state rather than diverging).
func UndoAuditEvent(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	cfg := currentConfig(c)

	id := c.Param("id")
	var event models.AuditEvent
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Audit event").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve audit event").WithError(err))
		}
		return
	}

	if event.Operation != models.AuditOpUpdate {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("operation", "undo supports update events only"))
		return
	}

	// Reject events past the retention window (a purged event is unrecoverable).
	retention := time.Duration(cfg.AuditRetentionDays) * 24 * time.Hour
	if retention > 0 && time.Since(event.CreatedAt) > retention {
		apperrors.AbortWithError(c, apperrors.ErrGone("This audit event is past its retention window and can no longer be undone"))
		return
	}

	switch event.EntityType {
	case models.AuditEntityContact:
		undoContact(c, db, userID, event)
	default:
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("entity_type", "undo is not supported for "+event.EntityType))
	}
}

// undoContact restores a Contact from the event's before snapshot.
//
// T75 (docs/fork-plan/tickets/119-T75-plain-save-destroys-card-only-data.md):
// the before snapshot never contained Card/CRM/Passthrough (all json:"-",
// see T82), so it can only restore the flat fields. The stopgap restores
// exactly those — copying the snapshot's flat state onto the live contact
// and letting BeforeSave's merge (contact.go) rebuild the flat-owned Card
// sub-structures from it while carrying the contact's current Card-only
// members through (SpeakToAs, PersonalInfo, unprojected address components,
// CRMEnvelope.Kind, ...). The result is honest rather than complete: undo
// reverts everything the snapshot actually recorded and leaves everything
// else as it is. Before this change undo overwrote the contact's Card with a
// Record rebuilt from a snapshot that never carried one, deleting what it
// could not restore. T82 (audit snapshots capturing nested data) makes undo
// full-fidelity afterwards.
func undoContact(c *gin.Context, db *gorm.DB, userID uint, event models.AuditEvent) {
	var before models.Contact
	if err := json.Unmarshal([]byte(event.BeforeSnapshot), &before); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("audit snapshot is corrupt").WithError(err))
		return
	}

	var current models.Contact
	if err := db.Where("user_id = ? AND vcard_uid = ?", userID, event.EntityID).First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("vcard_uid", event.EntityID))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	// Restore the snapshot's flat state onto the loaded contact; BeforeSave's
	// merge then rebuilds the flat-owned neutral columns from it without
	// touching the Card-only data no snapshot has ever carried.
	current.RestoreFlatStateFrom(&before)

	if err := db.Save(&current).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to undo contact update").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Contact restored to its previous state"})
}
