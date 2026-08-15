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
// (T18, T18). Deliberately updates
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
// The snapshot has two shapes (T82, T82), and undo must handle both:
//
//   - Events written before T82's capture change were marshaled by
//     json.Marshal(&Contact), which omits Card/CRM/Passthrough (all json:"-"),
//     so they carry only flat fields. For those, the T75 stopgap applies:
//     restore the snapshot's flat state onto the live contact and let
//     BeforeSave's merge (contact.go) rebuild the flat-owned Card sub-
//     structures from it while carrying the contact's current Card-only
//     members through (SpeakToAs, PersonalInfo, unprojected address
//     components, CRMEnvelope.Kind, ...). Undo reverts everything the snapshot
//     recorded and preserves what it could not — never deleting it.
//
//   - Events written after T82 capture the nested columns too
//     (ContactAuditSnapshot.HasNested). Those get a full restore via
//     RestoreFullStateFrom: flat state plus the exact before Card/CRM/
//     Passthrough, with the photo (stripped from snapshots) re-derived from
//     the restored Photo path.
//
// HasNested distinguishes the two by "was card/crm/passthrough present in the
// raw snapshot", which is exactly the difference between "absent because the
// event predates the change" and "cleared by the user" — a live contact's
// Card is never empty, so a non-empty nested snapshot can only mean a T82
// event with an authoritative before state.
func undoContact(c *gin.Context, db *gorm.DB, userID uint, event models.AuditEvent) {
	var before models.ContactAuditSnapshot
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

	if before.HasNested() {
		// T82 event: the snapshot carries the full before Card/CRM/Passthrough —
		// restore it verbatim. BeforeSave then leaves the restored nested columns
		// alone (RestoreFullStateFrom sets cardSetDirectly).
		current.RestoreFullStateFrom(&before)
	} else {
		// Pre-T82 event: restore the snapshot's flat state onto the loaded
		// contact; BeforeSave's merge then rebuilds the flat-owned neutral
		// columns from it without touching the Card-only data no snapshot has
		// ever carried (T75 stopgap).
		current.RestoreFlatStateFrom(&before.Contact)
	}

	if err := db.Save(&current).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to undo contact update").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Contact restored to its previous state"})
}
