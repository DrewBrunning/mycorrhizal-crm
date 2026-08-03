package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateConversationAgenda creates a new ConversationAgenda (conversation_
// agenda.go, §91.11) for the authenticated user, scoped to a Contact they own
// via EntityID. Agenda items are free-text contextual memory — never
// date-scheduled, which is what distinguishes them from Reminders.
func CreateConversationAgenda(c *gin.Context) {
	input, err := middleware.GetValidated[models.ConversationAgendaInput](c)
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

	item := models.ConversationAgenda{
		UserID:       userID,
		EntityID:     input.EntityID,
		Content:      input.Content,
		ReferenceURL: input.ReferenceURL,
	}

	if err := db.Create(&item).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save conversation agenda item").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Conversation agenda item created successfully", "conversation_agenda": item})
}

// GetConversationAgenda returns one ConversationAgenda.
func GetConversationAgenda(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var item models.ConversationAgenda
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Conversation agenda item").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve conversation agenda item").WithError(err))
		}
		return
	}

	c.JSON(http.StatusOK, item)
}

// ListConversationAgenda returns the authenticated user's ConversationAgenda
// items, cursor-paginated (T17), optionally filtered by
// ?entity_id=<Contact.VCardUID> in browse mode. Agenda items soft-delete
// (user-authored content), so the ?since= change feed returns tombstones and
// the collection is incremental. `total` is deliberately gone, matching
// LifeEvents: agenda items accumulate without bound and exact counts are the
// thing cursor pagination gives up.
func ListConversationAgenda(c *gin.Context) {
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

	var items []models.ConversationAgenda

	if params.Since {
		query := db.Unscoped().Model(&models.ConversationAgenda{}).Where("user_id = ?", userID)
		if params.Cursor != nil {
			pred, t, idv := cursorPredicate("conversation_agenda", params.Cursor, params.Cursor.ID, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "conversation_agenda", false).Limit(params.Limit + 1)
		if err := query.Find(&items).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve conversation agenda items").WithError(err))
			return
		}
		nextCursor := ""
		if len(items) > params.Limit {
			items = items[:params.Limit]
			nextCursor = EncodeCursor(items[len(items)-1].UpdatedAt, items[len(items)-1].ID)
		}
		for i := range items {
			items[i].Deleted = items[i].DeletedAt.Valid
		}
		c.JSON(http.StatusOK, gin.H{
			"conversation_agenda": items,
			"next_cursor":         nextCursor,
			"limit":               params.Limit,
			"sync":                buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	entityID := c.Query("entity_id")

	baseQuery := db.Model(&models.ConversationAgenda{}).Where("user_id = ?", userID)
	if entityID != "" {
		baseQuery = baseQuery.Where("entity_id = ?", entityID)
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("conversation_agenda", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "conversation_agenda", desc).
		Limit(params.Limit + 1).
		Find(&items).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve conversation agenda items").WithError(err))
		return
	}
	nextCursor := ""
	if len(items) > params.Limit {
		items = items[:params.Limit]
		nextCursor = EncodeCursor(items[len(items)-1].UpdatedAt, items[len(items)-1].ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation_agenda": items,
		"next_cursor":         nextCursor,
		"limit":               params.Limit,
		"sync":                buildSyncMeta(SyncModeIncremental),
	})
}

// UpdateConversationAgenda updates a ConversationAgenda — full-replace
// semantics for the content fields via the same ConversationAgendaInput as
// create, matching UpdateLifeEvent/UpdatePreference's precedent. The resolved
// state (discussed_at / activity_id) is deliberately left untouched: it is
// owned by PATCH /:id/discuss, so editing an item's text must not silently
// re-open or re-close it.
func UpdateConversationAgenda(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var item models.ConversationAgenda
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Conversation agenda item").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve conversation agenda item").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.ConversationAgendaInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if !verifyOwnedContact(c, db, userID, input.EntityID) {
		return
	}

	item.EntityID = input.EntityID
	item.Content = input.Content
	item.ReferenceURL = input.ReferenceURL

	if err := db.Save(&item).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save conversation agenda item").WithError(err))
		return
	}

	c.JSON(http.StatusOK, item)
}

// DiscussConversationAgenda marks a ConversationAgenda item as discussed —
// the agenda's only "resolution": the item has no due date or completion cron
// (§91.11); it is resolved by the next conversation, surfacing by context, not
// by time. Sets discussed_at to now and, when an activity_id is supplied,
// links the interaction that covered it (feeding the timeline). The item
// stays visible in a resolved state — "we talked about this on the 3rd" is
// half the value. Re-discussing is idempotent (refreshes the date); an
// omitted activity_id leaves any existing link untouched.
func DiscussConversationAgenda(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var item models.ConversationAgenda
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Conversation agenda item").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve conversation agenda item").WithError(err))
		}
		return
	}

	input, err := middleware.GetValidated[models.ConversationAgendaDiscussInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	// The activity must belong to the user — same ownership scoping every
	// cross-entity reference gets (CLAUDE.md trap 5). A soft reference with
	// no FK, so we verify in the query rather than relying on the database.
	if input.ActivityID != nil {
		var count int64
		if err := db.Model(&models.Activity{}).Where("id = ? AND user_id = ?", *input.ActivityID, userID).Count(&count).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to verify activity").WithError(err))
			return
		}
		if count == 0 {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Activity").WithDetails("id", *input.ActivityID))
			return
		}
	}

	now := time.Now()
	item.DiscussedAt = &now
	if input.ActivityID != nil {
		item.ActivityID = input.ActivityID
	}

	if err := db.Save(&item).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to mark conversation agenda item discussed").WithError(err))
		return
	}

	c.JSON(http.StatusOK, item)
}

// DeleteConversationAgenda soft-deletes a ConversationAgenda item
// (user-authored content). The row stays in the change feed as a tombstone
// but leaves the browse list; discussed items remain recoverable.
func DeleteConversationAgenda(c *gin.Context) {
	id := c.Param("id")
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var item models.ConversationAgenda
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Conversation agenda item").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve conversation agenda item").WithError(err))
		}
		return
	}

	if err := db.Delete(&item).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete conversation agenda item").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation agenda item deleted"})
}
