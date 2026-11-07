package services

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/jscontact"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Standing shares (ADR 0042, issue #2290): the single place that authorizes
// and resolves a share grant. Controllers never read share_grants or load an
// entity "for a grant" any other way (docs/security/standing-shares-review.md
// section 3; TestShareGrantReadsOnlyThroughService).
//
// Authorization rule: every lookup carries the caller's role column
// (from_user_id for owner routes, to_user_id for recipient routes) in the same
// WHERE as the id, and the source entity is fetched scoped by the grant's
// owner (from the grant row, never from the request). Every not-party /
// wrong-role / wrong-state / expired / blocked / entity-gone case is the same
// 404.

const (
	// MaxPendingShareGrantsPerPair and MaxPendingShareGrantsPerSender cap the
	// outstanding (pending or blocked, which the sender sees as pending) grants
	// a sender may have (ADR 0042 decision 11). Fixed caps rather than operator
	// config: they are an anti-spam bound, like the other per-request limits.
	MaxPendingShareGrantsPerPair   = 10
	MaxPendingShareGrantsPerSender = 50
	// MaxShareGrantCircleMembers caps the member names a circle grant returns.
	MaxShareGrantCircleMembers = 200
	// ShareGrantPendingTTL is how long a grant may wait for a response.
	ShareGrantPendingTTL = 30 * 24 * time.Hour
)

// ErrShareGrantNotFound is the one not-found every denial returns.
func ErrShareGrantNotFound() *apperrors.AppError { return apperrors.ErrNotFound("Share grant") }

func shareGrantConflict() *apperrors.AppError {
	return apperrors.ErrConflict("").WithReason(apperrors.ReasonShareGrantStateConflict, nil)
}

func dbErr(op string, err error) *apperrors.AppError {
	return apperrors.ErrDatabase(op).WithError(err)
}

// pendingExpired reports whether a never-answered grant is past its TTL.
func pendingExpired(g *models.ShareGrant, now time.Time) bool {
	return g.Status == models.ShareGrantStatusPending && !g.CreatedAt.Add(ShareGrantPendingTTL).After(now)
}

// timeExpired reports whether the grant's own expires_at has passed.
func timeExpired(g *models.ShareGrant, now time.Time) bool {
	return g.ExpiresAt != nil && !g.ExpiresAt.After(now)
}

// EffectiveStatus is the status a party should see: a live grant whose time
// has run out reads as expired even before the job moves the row, and the
// internal blocked status is the sender's pending.
func EffectiveStatus(g *models.ShareGrant, now time.Time, forOwner bool) string {
	switch g.Status {
	case models.ShareGrantStatusBlocked:
		if forOwner {
			if pendingExpiredBlocked(g, now) || timeExpired(g, now) {
				return models.ShareGrantStatusExpired
			}
			return models.ShareGrantStatusPending
		}
		return models.ShareGrantStatusBlocked
	case models.ShareGrantStatusPending, models.ShareGrantStatusActive:
		if timeExpired(g, now) || pendingExpired(g, now) {
			return models.ShareGrantStatusExpired
		}
	}
	return g.Status
}

func pendingExpiredBlocked(g *models.ShareGrant, now time.Time) bool {
	return !g.CreatedAt.Add(ShareGrantPendingTTL).After(now)
}

// validSections returns the tokens that are current models.FieldSections
// members, in grant order, de-duplicated. Unknown tokens are dropped (narrow,
// never widen).
func validSections(tokens []string) []string {
	known := map[string]bool{}
	for _, s := range models.FieldSections() {
		known[s] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, t := range tokens {
		if known[t] && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// resolveOwnedEntity checks that ownerID owns a live entity of the given
// type/identity. A foreign or missing entity is the same not-found.
func resolveOwnedEntity(db *gorm.DB, ownerID uint, entityType, entityID string) *apperrors.AppError {
	var n int64
	var err error
	switch entityType {
	case models.ShareGrantEntityContact:
		err = db.Model(&models.Contact{}).Where("vcard_uid = ? AND user_id = ? AND archived = ?", entityID, ownerID, false).Count(&n).Error
	case models.ShareGrantEntityNote:
		err = db.Model(&models.Note{}).Where("uuid = ? AND user_id = ?", entityID, ownerID).Count(&n).Error
	case models.ShareGrantEntityActivity:
		err = db.Model(&models.Activity{}).Where("uuid = ? AND user_id = ?", entityID, ownerID).Count(&n).Error
	case models.ShareGrantEntityCircle:
		err = db.Model(&models.Circle{}).Where("id = ? AND user_id = ?", entityID, ownerID).Count(&n).Error
	default:
		return apperrors.ErrNotFound("Share grant entity")
	}
	if err != nil {
		return dbErr("Failed to look up the entity to share", err)
	}
	if n == 0 {
		return apperrors.ErrNotFound("Share grant entity")
	}
	return nil
}

func sectionsInvalid(msg string) *apperrors.AppError {
	return apperrors.ErrValidation(msg).WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "sections"})
}

// CreateShareGrant creates a pending grant (or, when the recipient has
// blocked the sender, an internal `blocked` grant the sender sees as pending).
func CreateShareGrant(db *gorm.DB, ownerID uint, in models.ShareGrantInput, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC()
	if in.ExpiresAt != nil {
		exp := in.ExpiresAt.UTC()
		in.ExpiresAt = &exp
	}
	if in.ToUserID == ownerID {
		return nil, apperrors.ErrValidation("cannot share with yourself").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "to_user_id"})
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return nil, apperrors.ErrValidation("expires_at must be in the future").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "expires_at"})
	}
	var sections []string
	if in.EntityType == models.ShareGrantEntityContact {
		if len(in.Sections) == 0 {
			return nil, sectionsInvalid("sections must name at least one section")
		}
		for _, s := range in.Sections {
			if len(validSections([]string{s})) == 0 {
				return nil, apperrors.ErrValidation("unknown export field section \""+s+"\"").
					WithReason(apperrors.ReasonContactShareSectionUnknown, map[string]any{"section": s})
			}
		}
		sections = validSections(in.Sections)
	} else if len(in.Sections) > 0 {
		return nil, sectionsInvalid("sections apply only to contact grants")
	}

	var created models.ShareGrant
	err := db.Transaction(func(tx *gorm.DB) error {
		var recipients int64
		if err := tx.Model(&models.User{}).Where("id = ?", in.ToUserID).Count(&recipients).Error; err != nil {
			return dbErr("Failed to look up the recipient", err)
		}
		if recipients == 0 {
			return apperrors.ErrNotFound("Recipient user")
		}
		if appErr := resolveOwnedEntity(tx, ownerID, in.EntityType, in.EntityID); appErr != nil {
			return appErr
		}

		var live int64
		if err := tx.Model(&models.ShareGrant{}).
			Where("from_user_id = ? AND to_user_id = ? AND entity_type = ? AND entity_id = ? AND status IN ?",
				ownerID, in.ToUserID, in.EntityType, in.EntityID, models.ShareGrantLiveStatuses).Count(&live).Error; err != nil {
			return dbErr("Failed to check for an existing share", err)
		}
		if live > 0 {
			return apperrors.ErrConflict("").WithReason(apperrors.ReasonShareGrantAlreadyExists, nil)
		}

		pendingCount := func(extra string, args ...any) (int64, error) {
			var n int64
			q := tx.Model(&models.ShareGrant{}).Where("from_user_id = ? AND status IN ?", ownerID,
				[]string{models.ShareGrantStatusPending, models.ShareGrantStatusBlocked})
			if extra != "" {
				q = q.Where(extra, args...)
			}
			return n, q.Count(&n).Error
		}
		perSender, err := pendingCount("")
		if err != nil {
			return dbErr("Failed to check the pending share cap", err)
		}
		perPair, err := pendingCount("to_user_id = ?", in.ToUserID)
		if err != nil {
			return dbErr("Failed to check the pending share cap", err)
		}
		if perSender >= MaxPendingShareGrantsPerSender || perPair >= MaxPendingShareGrantsPerPair {
			return apperrors.ErrRateLimitExceeded().WithReason(apperrors.ReasonShareGrantPendingLimit, nil)
		}

		status := models.ShareGrantStatusPending
		var blocked int64
		if err := tx.Model(&models.ShareBlock{}).
			Where("blocker_user_id = ? AND blocked_user_id = ?", in.ToUserID, ownerID).Count(&blocked).Error; err != nil {
			return dbErr("Failed to check the recipient's block list", err)
		}
		if blocked > 0 {
			status = models.ShareGrantStatusBlocked
		}

		created = models.ShareGrant{
			FromUserID: ownerID, ToUserID: in.ToUserID,
			EntityType: in.EntityType, EntityID: in.EntityID,
			Sections: sections, Status: status, ExpiresAt: in.ExpiresAt,
			CreatedAt: now, UpdatedAt: now,
		}
		// Select-list create: only service-owned/allowlisted columns are written.
		if err := tx.Select("ID", "FromUserID", "ToUserID", "EntityType", "EntityID", "Sections",
			"ReconfirmSections", "Status", "ExpiresAt", "Revision", "CreatedAt", "UpdatedAt").Create(&created).Error; err != nil {
			return dbErr("Failed to create the share", err)
		}
		return nil
	})
	if err != nil {
		return nil, asAppError(err)
	}
	return &created, nil
}

func asAppError(err error) *apperrors.AppError {
	var ae *apperrors.AppError
	if errors.As(err, &ae) {
		return ae
	}
	return apperrors.ErrInternal("Share grant operation failed").WithError(err)
}

// loadForRole loads a grant by id with the caller's role column in the same
// WHERE. roleCol is "from_user_id" or "to_user_id" (never request-supplied).
// Blocked rows are invisible to the recipient. A miss is the identical 404.
func loadForRole(db *gorm.DB, roleCol string, userID uint, id string) (*models.ShareGrant, *apperrors.AppError) {
	var g models.ShareGrant
	q := db.Where("id = ? AND "+roleCol+" = ?", id, userID)
	if roleCol == "to_user_id" {
		q = q.Where("status <> ?", models.ShareGrantStatusBlocked)
	}
	if err := q.First(&g).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareGrantNotFound()
		}
		return nil, dbErr("Failed to retrieve share", err)
	}
	return &g, nil
}

func isLive(status string) bool {
	return status == models.ShareGrantStatusPending || status == models.ShareGrantStatusActive || status == models.ShareGrantStatusBlocked
}

// transition does a conditional update (ADR 0018 CAS): only a row still in one
// of fromStatuses moves. The caller has already authorized the role.
func transition(tx *gorm.DB, g *models.ShareGrant, roleCol string, userID uint, fromStatuses []string, set map[string]any, now time.Time) (bool, *apperrors.AppError) {
	set["updated_at"] = now
	set["revision"] = gorm.Expr("revision + 1")
	res := tx.Table("share_grants").
		Where("id = ? AND "+roleCol+" = ? AND status IN ?", g.ID, userID, fromStatuses).
		UpdateColumns(set)
	if res.Error != nil {
		return false, dbErr("Failed to update share", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func reload(db *gorm.DB, id string) (*models.ShareGrant, *apperrors.AppError) {
	var g models.ShareGrant
	if err := db.First(&g, "id = ?", id).Error; err != nil {
		return nil, dbErr("Failed to retrieve share", err)
	}
	return &g, nil
}

// AcceptShareGrant (recipient): pending -> active, idempotent on active,
// 409 on a terminal grant, 404 when time has expired it.
func AcceptShareGrant(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC()
	g, appErr := loadForRole(db, "to_user_id", userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if isLive(g.Status) && (timeExpired(g, now) || pendingExpired(g, now)) {
		return nil, ErrShareGrantNotFound()
	}
	switch g.Status {
	case models.ShareGrantStatusActive:
		return g, nil
	case models.ShareGrantStatusPending:
	default:
		return nil, shareGrantConflict()
	}
	ok, appErr := transition(db, g, "to_user_id", userID, []string{models.ShareGrantStatusPending},
		map[string]any{"status": models.ShareGrantStatusActive, "responded_at": now}, now)
	if appErr != nil {
		return nil, appErr
	}
	out, appErr := reload(db, g.ID)
	if appErr != nil {
		return nil, appErr
	}
	if !ok && out.Status != models.ShareGrantStatusActive {
		return nil, shareGrantConflict()
	}
	return out, nil
}

// DeclineShareGrant (recipient): pending -> declined; with block it also
// records the block and ends every pending/active grant from that sender.
func DeclineShareGrant(db *gorm.DB, userID uint, id string, block bool, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC()
	g, appErr := loadForRole(db, "to_user_id", userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if g.Status != models.ShareGrantStatusPending {
		return nil, shareGrantConflict()
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		ok, appErr := transition(tx, g, "to_user_id", userID, []string{models.ShareGrantStatusPending},
			map[string]any{"status": models.ShareGrantStatusDeclined, "responded_at": now}, now)
		if appErr != nil {
			return appErr
		}
		if !ok {
			return shareGrantConflict()
		}
		if block {
			return applyBlock(tx, userID, g.FromUserID, now)
		}
		return nil
	})
	if err != nil {
		return nil, asAppError(err)
	}
	return reload(db, g.ID)
}

// applyBlock inserts the block (idempotent) and declines every pending or
// active grant from that sender to this recipient. Rows already in the
// internal blocked status are left alone.
func applyBlock(tx *gorm.DB, blockerID, blockedID uint, now time.Time) error {
	block := models.ShareBlock{BlockerUserID: blockerID, BlockedUserID: blockedID, CreatedAt: now}
	if err := tx.Where("blocker_user_id = ? AND blocked_user_id = ?", blockerID, blockedID).
		FirstOrCreate(&block).Error; err != nil {
		return dbErr("Failed to record the block", err)
	}
	res := tx.Table("share_grants").
		Where("from_user_id = ? AND to_user_id = ? AND status IN ?", blockedID, blockerID,
			[]string{models.ShareGrantStatusPending, models.ShareGrantStatusActive}).
		UpdateColumns(map[string]any{
			"status": models.ShareGrantStatusDeclined, "responded_at": now,
			"updated_at": now, "revision": gorm.Expr("revision + 1"),
		})
	if res.Error != nil {
		return dbErr("Failed to end the blocked sender's shares", res.Error)
	}
	return nil
}

// LeaveShareGrant (recipient): pending or active -> declined.
func LeaveShareGrant(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC()
	g, appErr := loadForRole(db, "to_user_id", userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if g.Status != models.ShareGrantStatusPending && g.Status != models.ShareGrantStatusActive {
		return nil, shareGrantConflict()
	}
	ok, appErr := transition(db, g, "to_user_id", userID,
		[]string{models.ShareGrantStatusPending, models.ShareGrantStatusActive},
		map[string]any{"status": models.ShareGrantStatusDeclined, "responded_at": now}, now)
	if appErr != nil {
		return nil, appErr
	}
	out, appErr := reload(db, g.ID)
	if appErr != nil {
		return nil, appErr
	}
	if !ok && out.Status != models.ShareGrantStatusDeclined {
		return nil, shareGrantConflict()
	}
	return out, nil
}

// RevokeShareGrant (owner): live -> revoked, idempotent on an owner-revoked
// grant, 409 on declined/expired.
func RevokeShareGrant(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC()
	g, appErr := loadForRole(db, "from_user_id", userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if g.Status == models.ShareGrantStatusRevoked {
		return g, nil
	}
	if !isLive(g.Status) {
		return nil, shareGrantConflict()
	}
	ok, appErr := transition(db, g, "from_user_id", userID, models.ShareGrantLiveStatuses,
		map[string]any{"status": models.ShareGrantStatusRevoked, "revoked_at": now,
			"revoked_reason": models.ShareGrantReasonOwnerRevoked}, now)
	if appErr != nil {
		return nil, appErr
	}
	out, appErr := reload(db, g.ID)
	if appErr != nil {
		return nil, appErr
	}
	if !ok && out.Status != models.ShareGrantStatusRevoked {
		return nil, shareGrantConflict()
	}
	return out, nil
}

// ReconfirmShareGrant (owner) clears the re-confirmation markers so the
// withheld sections are served again (ADR 0042 decision 5).
func ReconfirmShareGrant(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC()
	g, appErr := loadForRole(db, "from_user_id", userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if !isLive(g.Status) {
		return nil, shareGrantConflict()
	}
	ok, appErr := transition(db, g, "from_user_id", userID, models.ShareGrantLiveStatuses,
		map[string]any{"reconfirm_sections": "[]"}, now)
	if appErr != nil {
		return nil, appErr
	}
	if !ok {
		return nil, shareGrantConflict()
	}
	return reload(db, g.ID)
}

// UpdateShareGrant (owner) changes expires_at and/or narrows sections.
// Widening is 409; ifMatch (the raw header, "" = unconditional) is checked
// against revision by checkRevision.
func UpdateShareGrant(db *gorm.DB, userID uint, id string, in models.ShareGrantPatchInput, checkRevision func(current int64) bool, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC()
	g, appErr := loadForRole(db, "from_user_id", userID, id)
	if appErr != nil {
		return nil, appErr
	}
	if !isLive(g.Status) {
		return nil, shareGrantConflict()
	}
	set := map[string]any{}
	if in.ExpiresAt.Set {
		if in.ExpiresAt.Value != nil && !in.ExpiresAt.Value.After(now) {
			return nil, apperrors.ErrValidation("expires_at must be in the future").
				WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "expires_at"})
		}
		if in.ExpiresAt.Value == nil {
			set["expires_at"] = gorm.Expr("NULL")
		} else {
			set["expires_at"] = in.ExpiresAt.Value.UTC()
		}
	}
	if in.Sections != nil {
		if g.EntityType != models.ShareGrantEntityContact {
			return nil, sectionsInvalid("sections apply only to contact grants")
		}
		if len(*in.Sections) == 0 {
			return nil, sectionsInvalid("sections must name at least one section")
		}
		for _, s := range *in.Sections {
			if len(validSections([]string{s})) == 0 {
				return nil, apperrors.ErrValidation("unknown export field section \""+s+"\"").
					WithReason(apperrors.ReasonContactShareSectionUnknown, map[string]any{"section": s})
			}
		}
		next := validSections(*in.Sections)
		current := map[string]bool{}
		for _, s := range g.Sections {
			current[s] = true
		}
		for _, s := range next {
			if !current[s] {
				return nil, apperrors.ErrConflict("").WithReason(apperrors.ReasonShareGrantWideningRefused, nil)
			}
		}
		raw, err := json.Marshal(next)
		if err != nil {
			return nil, apperrors.ErrInternal("Failed to encode sections").WithError(err)
		}
		set["sections"] = string(raw)
		// A section dropped from the grant no longer needs a marker.
		kept := []string{}
		for _, s := range g.ReconfirmSections {
			if containsStr(next, s) {
				kept = append(kept, s)
			}
		}
		rk, _ := json.Marshal(kept)
		set["reconfirm_sections"] = string(rk)
	}
	if len(set) == 0 {
		return nil, apperrors.ErrValidation("nothing to update").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "body"})
	}
	if checkRevision != nil && !checkRevision(g.Revision) {
		return nil, apperrors.ErrPreconditionFailed("").WithReason(apperrors.ReasonCommonPreconditionFailed, nil).
			WithDetails("expected_revision", g.Revision)
	}
	// CAS on the revision this request loaded: a concurrent writer yields 412.
	set["updated_at"] = now
	set["revision"] = gorm.Expr("revision + 1")
	res := db.Table("share_grants").
		Where("id = ? AND from_user_id = ? AND revision = ? AND status IN ?", g.ID, userID, g.Revision, models.ShareGrantLiveStatuses).
		UpdateColumns(set)
	if res.Error != nil {
		return nil, dbErr("Failed to update share", res.Error)
	}
	if res.RowsAffected != 1 {
		return nil, apperrors.ErrPreconditionFailed("").WithReason(apperrors.ReasonCommonPreconditionFailed, nil)
	}
	return reload(db, g.ID)
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ListShareBlocks returns the users the caller has blocked, with usernames.
func ListShareBlocks(db *gorm.DB, userID uint) ([]ShareBlockView, *apperrors.AppError) {
	var rows []models.ShareBlock
	if err := db.Where("blocker_user_id = ?", userID).Order("created_at ASC, blocked_user_id ASC").Find(&rows).Error; err != nil {
		return nil, dbErr("Failed to list blocks", err)
	}
	out := make([]ShareBlockView, 0, len(rows))
	for _, r := range rows {
		var name string
		_ = db.Model(&models.User{}).Select("username").Where("id = ?", r.BlockedUserID).Limit(1).Scan(&name).Error
		out = append(out, ShareBlockView{UserID: r.BlockedUserID, Username: name, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// DeleteShareBlock removes the caller's own block of blockedUserID. Scoped by
// blocker, so it can never remove another user's row; absent is a no-op.
func DeleteShareBlock(db *gorm.DB, userID, blockedUserID uint) *apperrors.AppError {
	if err := db.Where("blocker_user_id = ? AND blocked_user_id = ?", userID, blockedUserID).
		Delete(&models.ShareBlock{}).Error; err != nil {
		return dbErr("Failed to remove the block", err)
	}
	return nil
}

// ShareBlockView is one entry of the recipient's block list.
type ShareBlockView struct {
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// --- views --------------------------------------------------------------

// ShareGrantOwnerView is a grant as its sender sees it. blocked reads as
// pending; sections and reconfirm markers are the owner's own configuration.
type ShareGrantOwnerView struct {
	ID                string     `json:"id"`
	ToUserID          uint       `json:"to_user_id"`
	ToUsername        string     `json:"to_username"`
	EntityType        string     `json:"entity_type"`
	EntityID          string     `json:"entity_id"`
	Label             string     `json:"label"`
	Sections          []string   `json:"sections"`
	ReconfirmSections []string   `json:"reconfirm_sections"`
	Status            string     `json:"status"`
	ExpiresAt         *time.Time `json:"expires_at"`
	RespondedAt       *time.Time `json:"responded_at"`
	RevokedAt         *time.Time `json:"revoked_at"`
	RevokedReason     *string    `json:"revoked_reason"`
	MemberCount       *int       `json:"member_count"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Revision          int64      `json:"revision"`
}

// ShareGrantRecipientView is a grant as its recipient sees it. It never
// carries the entity id or the owner's configuration beyond the granted
// sections.
type ShareGrantRecipientView struct {
	ID           string     `json:"id"`
	FromUsername string     `json:"from_username"`
	EntityType   string     `json:"entity_type"`
	Label        string     `json:"label"`
	Sections     []string   `json:"sections"`
	Status       string     `json:"status"`
	ExpiresAt    *time.Time `json:"expires_at"`
	RespondedAt  *time.Time `json:"responded_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func usernameOf(db *gorm.DB, id uint) string {
	var n string
	_ = db.Model(&models.User{}).Select("username").Where("id = ?", id).Limit(1).Scan(&n).Error
	return n
}

// entityLabel computes the short per-request label for a grant (never
// snapshotted): contact display name, circle name, activity title; a note is
// just "Note". It reads the owner's own rows, scoped by grant.FromUserID.
func entityLabel(db *gorm.DB, g *models.ShareGrant) string {
	var s string
	switch g.EntityType {
	case models.ShareGrantEntityContact:
		var c models.Contact
		if err := db.Select("firstname", "lastname").Where("vcard_uid = ? AND user_id = ?", g.EntityID, g.FromUserID).Limit(1).Find(&c).Error; err == nil {
			s = strings.TrimSpace(c.Firstname + " " + c.Lastname)
		}
	case models.ShareGrantEntityNote:
		return "Note"
	case models.ShareGrantEntityActivity:
		_ = db.Model(&models.Activity{}).Select("title").Where("uuid = ? AND user_id = ?", g.EntityID, g.FromUserID).Limit(1).Scan(&s).Error
	case models.ShareGrantEntityCircle:
		_ = db.Model(&models.Circle{}).Select("name").Where("id = ? AND user_id = ?", g.EntityID, g.FromUserID).Limit(1).Scan(&s).Error
	}
	return s
}

func liveCircleMembers(db *gorm.DB, ownerID uint, circleID string) *gorm.DB {
	return db.Table("circle_members").
		Joins("JOIN contacts ON contacts.vcard_uid = circle_members.member_vcard_uid AND contacts.user_id = circle_members.user_id").
		Where("circle_members.circle_id = ? AND circle_members.user_id = ? AND contacts.user_id = ? AND contacts.deleted_at IS NULL AND contacts.archived = ?",
			circleID, ownerID, ownerID, false)
}

func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// OwnerView renders g for its sender.
func OwnerView(db *gorm.DB, g *models.ShareGrant, now time.Time) ShareGrantOwnerView {
	v := ShareGrantOwnerView{
		ID: g.ID, ToUserID: g.ToUserID, ToUsername: usernameOf(db, g.ToUserID),
		EntityType: g.EntityType, EntityID: g.EntityID, Label: entityLabel(db, g),
		Sections: nz(g.Sections), ReconfirmSections: nz(g.ReconfirmSections),
		Status: EffectiveStatus(g, now, true), ExpiresAt: g.ExpiresAt,
		RevokedAt: g.RevokedAt, RevokedReason: g.RevokedReason,
		CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt, Revision: g.Revision,
	}
	if g.Status != models.ShareGrantStatusBlocked {
		v.RespondedAt = g.RespondedAt
	}
	if g.EntityType == models.ShareGrantEntityCircle {
		var n int64
		if err := liveCircleMembers(db, g.FromUserID, g.EntityID).Count(&n).Error; err == nil {
			c := int(n)
			v.MemberCount = &c
		}
	}
	return v
}

// RecipientView renders g for its recipient.
func RecipientView(db *gorm.DB, g *models.ShareGrant, now time.Time) ShareGrantRecipientView {
	return ShareGrantRecipientView{
		ID: g.ID, FromUsername: usernameOf(db, g.FromUserID), EntityType: g.EntityType,
		Label: entityLabel(db, g), Sections: nz(g.Sections),
		Status: EffectiveStatus(g, now, false), ExpiresAt: g.ExpiresAt,
		RespondedAt: g.RespondedAt, CreatedAt: g.CreatedAt,
	}
}

// OutgoingShareGrantsQuery is the owner's list scope: their own rows only.
func OutgoingShareGrantsQuery(db *gorm.DB, userID uint) *gorm.DB {
	return db.Model(&models.ShareGrant{}).Where("from_user_id = ?", userID)
}

// IncomingShareGrantsQuery is the recipient's list scope: pending and active
// only, never blocked, and not past either expiry.
func IncomingShareGrantsQuery(db *gorm.DB, userID uint, now time.Time) *gorm.DB {
	now = now.UTC()
	return db.Model(&models.ShareGrant{}).
		Where("to_user_id = ? AND status IN ? AND (expires_at IS NULL OR expires_at > ?) AND (status <> ? OR created_at > ?)",
			userID, []string{models.ShareGrantStatusPending, models.ShareGrantStatusActive}, now,
			models.ShareGrantStatusPending, now.Add(-ShareGrantPendingTTL))
}

// --- content ------------------------------------------------------------

// ShareNoteView is the recipient's note projection: text and calendar date
// only, never the contact link, ids or image data (ADR 0042 decision 4).
type ShareNoteView struct {
	Content string    `json:"content"`
	Date    time.Time `json:"date"`
}

// ShareActivityView omits participants, external ref and GeoPulse data.
type ShareActivityView struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Location    string     `json:"location"`
	Date        time.Time  `json:"date"`
	EndDate     *time.Time `json:"end_date"`
	Type        string     `json:"type"`
}

// ShareCircleMemberView is a member's display name only.
type ShareCircleMemberView struct {
	DisplayName string `json:"display_name"`
}

// ShareCircleView is circle metadata plus live member names, capped.
type ShareCircleView struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Color       string                  `json:"color"`
	MemberCount int                     `json:"member_count"`
	Members     []ShareCircleMemberView `json:"members"`
	Truncated   bool                    `json:"members_truncated"`
}

// ShareGrantContent is the single recipient read payload. Exactly one of
// Contact/Note/Activity/Circle is set.
type ShareGrantContent struct {
	GrantID       string             `json:"grant_id"`
	EntityType    string             `json:"entity_type"`
	OwnerUsername string             `json:"owner_username"`
	Contact       json.RawMessage    `json:"contact,omitempty"`
	Note          *ShareNoteView     `json:"note,omitempty"`
	Activity      *ShareActivityView `json:"activity,omitempty"`
	Circle        *ShareCircleView   `json:"circle,omitempty"`
}

// ResolveActiveShareGrant is the one chokepoint for a recipient read: it
// loads the grant with every condition in a single statement. No row is the
// identical 404.
func ResolveActiveShareGrant(db *gorm.DB, userID uint, id string, now time.Time) (*models.ShareGrant, *apperrors.AppError) {
	now = now.UTC() // stored instants are UTC text; compare like with like
	var g models.ShareGrant
	err := db.Where("id = ? AND to_user_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)",
		id, userID, models.ShareGrantStatusActive, now).First(&g).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareGrantNotFound()
		}
		return nil, dbErr("Failed to retrieve share", err)
	}
	return &g, nil
}

// ReadShareGrantContent resolves the grant for the recipient and then fetches
// the source entity scoped by the grant's owner (taken from the grant row).
func ReadShareGrantContent(db *gorm.DB, userID uint, id, photoDir string, now time.Time) (*ShareGrantContent, *apperrors.AppError) {
	g, appErr := ResolveActiveShareGrant(db, userID, id, now)
	if appErr != nil {
		return nil, appErr
	}
	out := &ShareGrantContent{GrantID: g.ID, EntityType: g.EntityType, OwnerUsername: usernameOf(db, g.FromUserID)}
	switch g.EntityType {
	case models.ShareGrantEntityContact:
		raw, appErr := contactContent(db, g, photoDir)
		if appErr != nil {
			return nil, appErr
		}
		out.Contact = raw
	case models.ShareGrantEntityNote:
		var n models.Note
		if err := db.Where("uuid = ? AND user_id = ?", g.EntityID, g.FromUserID).First(&n).Error; err != nil {
			return nil, entityMiss(err)
		}
		out.Note = &ShareNoteView{Content: n.Content, Date: n.Date}
	case models.ShareGrantEntityActivity:
		var a models.Activity
		if err := db.Where("uuid = ? AND user_id = ?", g.EntityID, g.FromUserID).First(&a).Error; err != nil {
			return nil, entityMiss(err)
		}
		out.Activity = &ShareActivityView{Title: a.Title, Description: a.Description, Location: a.Location,
			Date: a.Date, EndDate: a.EndDate, Type: a.Type}
	case models.ShareGrantEntityCircle:
		view, appErr := circleContent(db, g)
		if appErr != nil {
			return nil, appErr
		}
		out.Circle = view
	default:
		return nil, ErrShareGrantNotFound()
	}
	return out, nil
}

func entityMiss(err error) *apperrors.AppError {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrShareGrantNotFound()
	}
	return dbErr("Failed to retrieve shared content", err)
}

// grantSelection turns the stored sections into a selection: only current
// FieldSections tokens count (unknown tokens narrow), sections awaiting
// re-confirmation are withheld whole, and IncludeSensitive is forced false.
func grantSelection(g *models.ShareGrant) *models.FieldSelection {
	sel := models.NewFieldSelection()
	withheld := map[string]bool{}
	for _, s := range g.ReconfirmSections {
		withheld[s] = true
	}
	for _, s := range validSections(g.Sections) {
		if withheld[s] {
			continue
		}
		_ = sel.Enable(s)
	}
	sel.IncludeSensitive = false
	return sel
}

func contactContent(db *gorm.DB, g *models.ShareGrant, photoDir string) (json.RawMessage, *apperrors.AppError) {
	var c models.Contact
	if err := db.Where("vcard_uid = ? AND user_id = ? AND archived = ?", g.EntityID, g.FromUserID, false).First(&c).Error; err != nil {
		return nil, entityMiss(err)
	}
	record := models.RecordForContactFiltered(&c, photoDir, db, grantSelection(g))
	data, _, err := jscontact.Adapter{}.Export(record)
	if err != nil {
		return nil, apperrors.ErrInternal("Failed to build shared contact").WithError(err)
	}
	return json.RawMessage(data), nil
}

func circleContent(db *gorm.DB, g *models.ShareGrant) (*ShareCircleView, *apperrors.AppError) {
	var c models.Circle
	if err := db.Where("id = ? AND user_id = ?", g.EntityID, g.FromUserID).First(&c).Error; err != nil {
		return nil, entityMiss(err)
	}
	var total int64
	if err := liveCircleMembers(db, g.FromUserID, g.EntityID).Count(&total).Error; err != nil {
		return nil, dbErr("Failed to count circle members", err)
	}
	var rows []struct{ Firstname, Lastname string }
	if err := liveCircleMembers(db, g.FromUserID, g.EntityID).
		Select("contacts.firstname AS firstname, contacts.lastname AS lastname").
		Order("contacts.sort_name ASC, contacts.id ASC").Limit(MaxShareGrantCircleMembers).Scan(&rows).Error; err != nil {
		return nil, dbErr("Failed to list circle members", err)
	}
	members := make([]ShareCircleMemberView, 0, len(rows))
	for _, r := range rows {
		members = append(members, ShareCircleMemberView{DisplayName: strings.TrimSpace(r.Firstname + " " + r.Lastname)})
	}
	return &ShareCircleView{Name: c.Name, Description: c.Description, Color: c.Color,
		MemberCount: int(total), Members: members, Truncated: total > int64(len(rows))}, nil
}
