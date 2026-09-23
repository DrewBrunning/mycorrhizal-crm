package controllers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetUpcomingOccasions is the "upcoming occasions" dashboard-widget aggregate
// (ADR 0024, issue #387, ticket #1224): every birthday/anniversary/life-event/
// active-obligation occurrence within the next `days` (30 or 90, default 30),
// sorted ascending by days-until.
//
// "Today" is decided via reminderNow(c) — the single reminder-zone clock
// (ADR 0015 Rule 4), not the server's own local zone.
func GetUpcomingOccasions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 30 && parsed != 90) {
			apperrors.AbortWithError(c, apperrors.ErrValidation("days must be 30 or 90"))
			return
		}
		days = parsed
	}
	includeSensitive := c.Query("include_sensitive") == "true"

	now := reminderNow(c)
	occasions, err := services.GetUpcomingOccasions(db, userID, now, days, includeSensitive)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve upcoming occasions").WithError(err))
		return
	}
	// CLAUDE.md frontend trap #8: never let an empty result serialize as an
	// absent key — a nil slice with `omitempty` disappears from the JSON
	// entirely, which a required TS array type cannot guard against.
	if occasions == nil {
		occasions = []models.UpcomingOccasion{}
	}

	c.JSON(http.StatusOK, gin.H{"occasions": occasions, "days": days})
}

// GetOccasionCardListCSV is the sensitivity-filtered "Christmas cards"
// address-list export (ADR 0024 part 4, issue #387, ticket #1225): every
// contact with an active OccasionObligation of the given Kind (default
// "card"), with their display name and first postal address.
//
// This is deliberately NOT routed through GET /api/v1/export (the
// full-fidelity personal-backup exception, issue #861) — that endpoint
// exists precisely because it never leaves the instance in the sense that
// matters; a mailing-label export is exactly the kind of copy meant to leave
// the instance (to a printer, a card-fulfillment service) that the general
// sensitivity rule governs. OccasionObligation rows above `normal`
// sensitivity are excluded by default, the opposite of /export's default,
// re-includable only via ?include_sensitive=true.
//
// A contact with no postal address on file is skipped, not emitted with a
// blank address column.
func GetOccasionCardListCSV(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	kind := c.DefaultQuery("kind", models.OccasionObligationKindCard)
	includeSensitive := c.Query("include_sensitive") == "true"

	query := db.Where("user_id = ? AND active = ? AND kind = ?", userID, true, kind)
	if !includeSensitive {
		query = query.Where("sensitivity = ?", models.RelationshipSensitivityNormal)
	}
	var obligations []models.OccasionObligation
	if err := query.Order("label ASC").Find(&obligations).Error; err != nil {
		abortExportError(c, log, "export:occasion-card-list", "database", "Failed to fetch occasion obligations for card list", err)
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(csvSafeRecord([]string{"Contact Name", "Address", "Obligation"})); err != nil {
		abortExportError(c, log, "export:occasion-card-list", "serialization", "Failed to write CSV header", err)
		return
	}

	for _, obligation := range obligations {
		var contact models.Contact
		if err := db.Where("user_id = ? AND vcard_uid = ?", userID, obligation.EntityID).First(&contact).Error; err != nil {
			continue // the referenced contact no longer exists or isn't owned; skip rather than fail the whole export
		}
		if len(contact.Addresses) == 0 {
			continue
		}
		address := models.FormatAddress(contact.Addresses[0])
		if address == "" {
			continue
		}
		name := contactDisplayNameFlat(&contact)
		if err := writer.Write(csvSafeRecord([]string{name, address, obligation.Label})); err != nil {
			abortExportError(c, log, "export:occasion-card-list", "serialization", "Failed to write CSV row", err)
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		abortExportError(c, log, "export:occasion-card-list", "serialization", "CSV writer error", err)
		return
	}

	filename := fmt.Sprintf("mycorrhizal-%s-list-%s.csv", kind, time.Now().Format("2006-01-02"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Length", fmt.Sprintf("%d", buf.Len()))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// contactDisplayNameFlat mirrors services.contactDisplayName exactly, kept
// local to controllers since services cannot depend on controllers and this
// handler needs it directly against a models.Contact it already loaded.
func contactDisplayNameFlat(contact *models.Contact) string {
	name := contact.Firstname
	if contact.Nickname != "" {
		name = contact.Nickname
	}
	if contact.Lastname != "" {
		name += " " + contact.Lastname
	}
	return name
}

// GetGiftShoppingList is "who still needs a gift" (ADR 0024 part 5, issue
// #387, ticket #1226): every active Kind="gift" obligation due within
// `days`, joined against Gift to show whether this cycle's gift already has
// a linked idea/purchase.
func GetGiftShoppingList(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 30 && parsed != 90) {
			apperrors.AbortWithError(c, apperrors.ErrValidation("days must be 30 or 90"))
			return
		}
		days = parsed
	}
	includeSensitive := c.Query("include_sensitive") == "true"

	now := reminderNow(c)
	items, err := services.GetGiftShoppingList(db, userID, now, days, includeSensitive)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve gift shopping list").WithError(err))
		return
	}
	if items == nil {
		items = []models.GiftShoppingItem{}
	}

	c.JSON(http.StatusOK, gin.H{"gift_shopping_list": items, "days": days})
}
