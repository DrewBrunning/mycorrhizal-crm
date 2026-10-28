package controllers

import (
	"errors"
	"net/http"
	"strconv"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Contact trash (issue #2002, ADR 0037). Every query is scoped by user_id; a
// contact that is not the caller's, not trashed, or has no snapshot is a 404
// with no side effect (never 403).

func trashContactID(c *gin.Context) (uint, bool) {
	raw, ok := requirePathUintID(c, "id")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseUint(raw, 10, strconv.IntSize)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact"))
		return 0, false
	}
	return uint(id), true
}

func abortTrashError(c *gin.Context, id uint, err error) {
	var inUse *services.ContactUIDInUseError
	switch {
	case errors.Is(err, services.ErrContactNotInTrash):
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
	case errors.As(err, &inUse):
		apperrors.AbortWithError(c, apperrors.ErrConflict("A live contact already uses this contact's UID; merge or delete it first").
			WithDetails("live_contact_id", inUse.LiveContactID).WithReason(apperrors.ReasonTrashUIDInUse, nil))
	case errors.Is(err, services.ErrSnapshotUnsupported):
		apperrors.AbortWithError(c, apperrors.ErrConflict("This contact's deletion snapshot is not supported by this version; it can still be deleted permanently").WithReason(apperrors.ReasonTrashSnapshotUnsupported, nil))
	default:
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Trash operation failed").WithError(err))
	}
}

// ListTrashedContacts handles GET /api/v1/trash/contacts.
func ListTrashedContacts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	p := GetPaginationParams(c)
	entries, total, err := services.ListTrash(db, userID, p.Offset, p.Limit)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to list trashed contacts").WithError(err))
		return
	}
	retention := currentConfig(c).DeleteRetentionDays
	out := make([]models.TrashedContact, 0, len(entries))
	for _, e := range entries {
		item := models.TrashedContact{Contact: models.NewContactSummary(&e.Contact), DeletedAt: e.DeletedAt}
		if retention > 0 {
			purgeAt := e.DeletedAt.AddDate(0, 0, retention)
			item.PurgeAt = &purgeAt
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, models.TrashListResponse{Contacts: out, Total: total, Page: p.Page, Limit: p.Limit, RetentionDays: retention})
}

// RestoreTrashedContact handles POST /api/v1/trash/contacts/:id/restore.
func RestoreTrashedContact(c *gin.Context) {
	id, ok := trashContactID(c)
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	res, err := services.RestoreContact(db, userID, id, clock.FromContext(c).Now())
	if err != nil {
		abortTrashError(c, id, err)
		return
	}
	skipped := make([]models.TrashRestoreSkip, 0, len(res.Skipped))
	for _, s := range res.Skipped {
		skipped = append(skipped, models.TrashRestoreSkip{Table: s.Table, Reason: s.Reason, Count: s.Count})
	}
	services.TriggerWebhooksAsync(c.Request.Context(), db, currentConfig(c), userID, "contact.restored", gin.H{"id": res.Contact.ID})
	c.JSON(http.StatusOK, models.TrashRestoreResponse{Contact: models.NewContactSummary(&res.Contact), Skipped: skipped})
}

// PurgeTrashedContact handles DELETE /api/v1/trash/contacts/:id.
func PurgeTrashedContact(c *gin.Context) {
	id, ok := trashContactID(c)
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	files, err := services.PurgeContact(db, userID, id)
	if err != nil {
		abortTrashError(c, id, err)
		return
	}
	// Only after the purge committed (ADR 0037 decision 5).
	services.RemovePurgedFiles(currentConfig(c), files)
	c.JSON(http.StatusOK, models.TrashPurgeResponse{Message: "Contact permanently deleted", Purged: 1})
}

// EmptyTrash handles DELETE /api/v1/trash/contacts.
func EmptyTrash(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	files, n, err := services.EmptyTrash(db, userID)
	if err != nil {
		abortTrashError(c, 0, err)
		return
	}
	services.RemovePurgedFiles(currentConfig(c), files)
	c.JSON(http.StatusOK, models.TrashPurgeResponse{Message: "Trash emptied", Purged: n})
}
