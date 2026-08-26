package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/jscontact"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateContactShare exports the sender's contact through T9's field
// selection (models.RecordForContactFiltered — this is what gets the
// sensitivity gating enforced server-side, not just by the UI), serializes it
// once as a JSContact "Card set" (matching ExportContactsAsJSContact's own
// shape, so ParseJSContact's array-form path is exercised identically to a
// normal import), and creates a pending ContactShare row.
func CreateContactShare(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)
	photoDir := currentConfig(c).ProfilePhotoDir

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	input, err := middleware.GetValidated[models.ContactShareInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if input.ToUserID == userID {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("to_user_id", "Cannot share a contact with yourself"))
		return
	}

	var toUser models.User
	if err := db.First(&toUser, input.ToUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Recipient user"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve recipient").WithError(err))
		}
		return
	}

	var contact models.Contact
	if err := db.Where("vcard_uid = ? AND user_id = ?", input.VCardUID, userID).First(&contact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("vcard_uid", input.VCardUID))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	sel := models.NewFieldSelection()
	for _, token := range input.Sections {
		if err := sel.Enable(token); err != nil {
			apperrors.AbortWithError(c, apperrors.ErrValidation(err.Error()))
			return
		}
	}
	sel.IncludeSensitive = input.IncludeSensitive

	record := models.RecordForContactFiltered(&contact, photoDir, db, sel)
	adapter := jscontact.Adapter{}
	data, diags, exportErr := adapter.Export(record)
	if exportErr != nil {
		log.Error().Err(exportErr).Uint("contact_id", contact.ID).Msg("Failed to encode contact as JSContact for share")
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to build share"))
		return
	}
	for _, d := range diags {
		log.Debug().Str("severity", d.Severity).Str("concept", d.Concept).Uint("contact_id", contact.ID).Msg(logger.SanitizeLogField(d.Message))
	}

	payload, marshalErr := json.Marshal([]json.RawMessage{json.RawMessage(data)})
	if marshalErr != nil {
		log.Error().Err(marshalErr).Msg("Failed to marshal contact share payload")
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to build share"))
		return
	}

	share := models.ContactShare{
		FromUserID:         userID,
		ToUserID:           input.ToUserID,
		ContactDisplayName: strings.TrimSpace(fmt.Sprintf("%s %s", contact.Firstname, contact.Lastname)),
		Payload:            string(payload),
		Status:             models.ContactShareStatusPending,
	}
	if err := db.Create(&share).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to create contact share").WithError(err))
		return
	}

	log.Info().Str("share_id", share.ID).Uint("from_user_id", userID).Uint("to_user_id", input.ToUserID).Msg("Contact share created")
	c.JSON(http.StatusOK, gin.H{"message": "Share created", "contact_share": share})
}

// usernamesByID batch-resolves usernames for a set of user IDs — used by the
// list handlers below so the inbox/outbox can show the other party's name
// without an extra round trip per row.
func usernamesByID(db *gorm.DB, ids []uint) (map[uint]string, *apperrors.AppError) {
	result := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var users []models.User
	if err := db.Select("id, username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to resolve usernames").WithError(err)
	}
	for _, u := range users {
		result[u.ID] = u.Username
	}
	return result, nil
}

// ListIncomingContactShares returns ContactShares where the authenticated
// user is the recipient, cursor-paginated like ListCircles.
func ListIncomingContactShares(c *gin.Context) {
	listContactShares(c, "to_user_id", func(s models.ContactShare) uint { return s.FromUserID })
}

// ListOutgoingContactShares returns ContactShares the authenticated user has
// sent, cursor-paginated like ListCircles.
func ListOutgoingContactShares(c *gin.Context) {
	listContactShares(c, "from_user_id", func(s models.ContactShare) uint { return s.ToUserID })
}

// listContactShares is the shared implementation behind
// ListIncomingContactShares/ListOutgoingContactShares — scopeColumn is
// "to_user_id" or "from_user_id" (whichever makes the caller the owner of
// this side of the list), and otherParty extracts the OTHER party's user ID
// from a row so their username can be resolved in one batched query.
func listContactShares(c *gin.Context, scopeColumn string, otherParty func(models.ContactShare) uint) {
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

	var shares []models.ContactShare
	var total int64

	baseQuery := db.Model(&models.ContactShare{}).Where(scopeColumn+" = ?", userID)
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count contact shares").WithError(err))
		return
	}

	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("contact_shares", params.Cursor, params.Cursor.ID, desc)
		baseQuery = baseQuery.Where(pred, t, idv)
	}

	if err := cursorOrderBy(baseQuery, "contact_shares", desc).
		Limit(params.Limit + 1).
		Find(&shares).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact shares").WithError(err))
		return
	}
	nextCursor := ""
	if len(shares) > params.Limit {
		shares = shares[:params.Limit]
		nextCursor = EncodeCursor(shares[len(shares)-1].UpdatedAt, shares[len(shares)-1].ID)
	}

	otherIDs := make([]uint, 0, len(shares))
	seen := make(map[uint]bool, len(shares))
	for _, s := range shares {
		id := otherParty(s)
		if !seen[id] {
			seen[id] = true
			otherIDs = append(otherIDs, id)
		}
	}
	usernames, appErr := usernamesByID(db, otherIDs)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"contact_shares": shares,
		"usernames":      usernames,
		"total":          total,
		"next_cursor":    nextCursor,
		"limit":          params.Limit,
	})
}

// getPendingShareForRecipient loads a ContactShare owned (as recipient) by
// the authenticated user and still pending a response. Ownership is checked
// with a AND-scoped WHERE, not a fetch-then-403, so a non-party gets the same
// 404 a non-owner would get anywhere else in this codebase — never a 403
// that would confirm the row exists.
func getPendingShareForRecipient(c *gin.Context, db *gorm.DB, userID uint) (*models.ContactShare, bool) {
	shareID := c.Param("id")
	var share models.ContactShare
	if err := db.Where("id = ? AND to_user_id = ?", shareID, userID).First(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact share").WithDetails("id", shareID))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact share").WithError(err))
		}
		return nil, false
	}
	if share.Status != models.ContactShareStatusPending {
		apperrors.AbortWithError(c, apperrors.ErrConflict("Share already responded to"))
		return nil, false
	}
	return &share, true
}

// AcceptContactShare parses the share's stored payload through the existing
// VCF/JSContact import pipeline (services.ParseJSContact — the same
// duplicate-detection + preview machinery UploadJSContactForImport uses) and
// returns an ImportPreviewResponse, so the recipient sees exactly what they
// are about to get before anything lands. This step is preview-only and does
// NOT change the share's Status — ConfirmContactShare does that once the
// recipient has actually chosen per-row actions and confirmed.
func AcceptContactShare(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	share, ok := getPendingShareForRecipient(c, db, userID)
	if !ok {
		return
	}

	// Issue #415: an accept is a session creation like any other import — a
	// burst of accepts (or accepts stacked on ordinary imports) must not push
	// the in-memory session store without bound.
	if importSessions.CountActive(userID) >= services.MaxImportSessionsPerUser {
		apperrors.AbortWithError(c, apperrors.NewError(
			apperrors.ErrCodeRateLimitExceeded,
			fmt.Sprintf("Too many in-progress imports. Maximum is %d concurrent import sessions; confirm or wait for an existing one to expire before starting another.", services.MaxImportSessionsPerUser),
			http.StatusTooManyRequests,
		))
		return
	}

	contacts, previews, stats, err := services.ParseJSContact(strings.NewReader(share.Payload), db, userID)
	if err != nil {
		log.Warn().Err(err).Str("share_id", share.ID).Msg("Failed to parse contact share payload")
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("payload", err.Error()))
		return
	}

	sessionID := importSessions.CreateVCFSessionForShare(userID, share.ID, contacts, previews)

	c.JSON(http.StatusOK, models.ImportPreviewResponse{
		SessionID:      sessionID,
		Rows:           previews,
		TotalRows:      len(previews),
		ValidRows:      stats.ValidCount,
		DuplicateCount: stats.DuplicateCount,
		ErrorCount:     stats.ErrorCount,
	})
}

// ConfirmContactShare finalizes an accepted share: it delegates straight to
// importSessions.ConfirmVCF — the exact same service method ConfirmVCFImport
// calls for an ordinary VCF/JSContact import — so add/skip/update per-row
// actions (and photo handling) are fully reused, not reimplemented. This is
// also where the "does accepting overwrite what I already have on this
// person" decision lives: the recipient explicitly picks "add" (create new)
// or "update" (merge via the existing, already-tested MergeImportedContact
// policy) per row via the returned preview's duplicate_match — never
// automatic. Only on success does the share flip to accepted.
//
// Issue #555 recipient-capability rule: the landed Contact (add) or merged
// Contact (update) is, from this point on, an ORDINARY contact the
// recipient owns outright — identical in every respect to one they created
// or imported themselves. There is no provenance flag, no residual
// restriction, and no trace of "this arrived via a share" carried forward:
// the recipient's own sensitivity classifications govern it from here, and
// they may re-share it onward, export it, or edit it exactly as they could
// any other contact (TestConfirmContactShare_AcceptedContactBecomesOrdinaryAndCanBeReShared,
// contact_share_matrix_test.go). This is deliberate, not an oversight:
// ContactShare's whole design point is that the payload becomes the
// recipient's data at accept-time, not a live, sender-controlled reference.
func ConfirmContactShare(c *gin.Context, cfg *config.Config) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	share, ok := getPendingShareForRecipient(c, db, userID)
	if !ok {
		return
	}

	request, err := middleware.GetValidated[models.ImportConfirmRequest](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	// The session must be the one AcceptContactShare minted for THIS share —
	// otherwise a session belonging to a different pending share (or an
	// unrelated CSV/VCF/JSContact import in the same account) could be used
	// to flip this share's status to accepted without its own data being
	// what actually landed. Checked before ConfirmVCF touches anything.
	if !importSessions.SessionBelongsToShare(request.SessionID, userID, share.ID) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("session_id", "Session does not belong to this share"))
		return
	}

	result, appErr := importSessions.ConfirmVCF(db, userID, *request, cfg, log)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	now := time.Now()
	share.Status = models.ContactShareStatusAccepted
	share.RespondedAt = &now
	if err := db.Save(share).Error; err != nil {
		// The import already committed — logging rather than aborting
		// matches import_session.go's own precedent for a secondary,
		// non-critical failure (CreateMergeNote).
		log.Warn().Err(err).Str("share_id", share.ID).Msg("Failed to mark contact share accepted after successful import")
	}

	c.JSON(http.StatusOK, result)
}

// DeclineContactShare flips a pending share to declined. Nothing on the
// sender's side is touched — the sender's original contact and their record
// of having offered it both survive, per the ticket's own trap ("declining
// should not silently destroy the sender's copy of what they offered").
func DeclineContactShare(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	share, ok := getPendingShareForRecipient(c, db, userID)
	if !ok {
		return
	}

	now := time.Now()
	share.Status = models.ContactShareStatusDeclined
	share.RespondedAt = &now
	if err := db.Save(share).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to decline contact share").WithError(err))
		return
	}

	log.Info().Str("share_id", share.ID).Uint("user_id", userID).Msg("Contact share declined")
	c.JSON(http.StatusOK, gin.H{"message": "Share declined"})
}
