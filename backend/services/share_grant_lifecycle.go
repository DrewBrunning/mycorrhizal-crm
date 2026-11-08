package services

import (
	"context"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Standing-share lifecycle (ADR 0042 decisions 10 and 11, issue #2291): the
// scheduled job, the per-party audit trail, the recipient's in-app notices and
// the pending-count badge.
//
// Correctness never depends on the job. The read path (EffectiveStatus,
// IncomingShareGrantsQuery, ResolveActiveShareGrant) evaluates expiry itself
// with the injected clock; the job only moves the row to `expired`, writes the
// audit event and, after the retention window, hard-deletes terminal rows.

// shareGrantLifecycleMinInterval is the ADR 0011 de-dup window for the hourly
// job.
var shareGrantLifecycleMinInterval = JobCatchupWindow(time.Hour)

const (
	shareGrantLifecycleBatch = 200
	// MaxShareGrantNotices caps the unread notices one call returns.
	MaxShareGrantNotices = 50
)

// ShareGrantLifecycleResult is what one run changed. Counts only.
type ShareGrantLifecycleResult struct {
	Expired       int64
	Purged        int64
	NoticesPurged int64
}

// Total is the number of rows the run moved or removed.
func (r ShareGrantLifecycleResult) Total() int64 { return r.Expired + r.Purged + r.NoticesPurged }

// auditShareGrant writes g's audit event on the owner's trail and, for the
// transitions that change what the recipient can do (accept, decline, revoke,
// expire), on the recipient's trail too. wasBlocked suppresses the recipient
// row: a recipient never learns of a grant from a blocked sender (decision 8).
// g must be the post-transition row. Audit is fire-and-forget by design and
// can never fail the transition.
func auditShareGrant(tx *gorm.DB, g *models.ShareGrant, op string, wasBlocked bool) {
	models.RecordShareGrantAudit(tx, g, g.FromUserID, op)
	if wasBlocked {
		return
	}
	switch op {
	case models.AuditOpAccept, models.AuditOpDecline, models.AuditOpRevoke, models.AuditOpExpire:
		models.RecordShareGrantAudit(tx, g, g.ToUserID, op)
	}
}

// notifyShareGrant records the recipient's in-app notice. Payload: sender id
// and entity type only. A failure is logged and swallowed: a notification can
// never fail the transition that caused it (acceptance, issue #2291).
func notifyShareGrant(tx *gorm.DB, g *models.ShareGrant, kind string) {
	now := Now().UTC()
	n := models.ShareGrantNotification{
		UserID: g.ToUserID, FromUserID: g.FromUserID, Kind: kind, EntityType: g.EntityType,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Session(&gorm.Session{NewDB: true}).Create(&n).Error; err != nil {
		logger.Warn().Err(err).Str("kind", kind).Msg("share grant: failed to record the recipient notice")
	}
}

// RunShareGrantLifecycle is one pass of the job: expire grants past expires_at
// and unanswered pending grants past ShareGrantPendingTTL, then hard-delete
// terminal rows (and old notices) past SHARE_GRANT_RETENTION_DAYS. Active and
// pending rows are never purged. It is idempotent and reads the injected
// clock (services.Now). A retention of 0 or less disables the purge.
func RunShareGrantLifecycle(ctx context.Context, db *gorm.DB, cfg config.Config) (ShareGrantLifecycleResult, error) {
	var res ShareGrantLifecycleResult
	now := Now().UTC()
	var err error
	if res.Expired, err = expireShareGrants(ctx, db, now); err != nil {
		return res, err
	}
	if cfg.ShareGrantRetentionDays <= 0 {
		return res, nil
	}
	cutoff := now.AddDate(0, 0, -cfg.ShareGrantRetentionDays)
	del := db.WithContext(ctx).Exec(
		"DELETE FROM share_grants WHERE status IN (?, ?, ?) AND updated_at < ?",
		models.ShareGrantStatusDeclined, models.ShareGrantStatusRevoked, models.ShareGrantStatusExpired, cutoff)
	if del.Error != nil {
		logger.Ctx(ctx).Error().Err(del.Error).Msg("share grant lifecycle: failed to purge terminal grants")
		return res, del.Error
	}
	res.Purged = del.RowsAffected
	nd := db.WithContext(ctx).Exec("DELETE FROM share_grant_notifications WHERE created_at < ?", cutoff)
	if nd.Error != nil {
		logger.Ctx(ctx).Error().Err(nd.Error).Msg("share grant lifecycle: failed to purge old notices")
		return res, nd.Error
	}
	res.NoticesPurged = nd.RowsAffected
	return res, nil
}

func expireShareGrants(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	var total int64
	for {
		var rows []models.ShareGrant
		if err := db.WithContext(ctx).
			Where("status IN ? AND ((expires_at IS NOT NULL AND expires_at <= ?) OR (status IN ? AND created_at <= ?))",
				models.ShareGrantLiveStatuses, now,
				[]string{models.ShareGrantStatusPending, models.ShareGrantStatusBlocked}, now.Add(-ShareGrantPendingTTL)).
			Order("id").Limit(shareGrantLifecycleBatch).Find(&rows).Error; err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("share grant lifecycle: failed to find grants to expire")
			return total, err
		}
		moved := 0
		for i := range rows {
			g := rows[i]
			// CAS on the status this pass read: a concurrent accept/revoke wins.
			res := db.WithContext(ctx).Table("share_grants").
				Where("id = ? AND status = ?", g.ID, g.Status).
				UpdateColumns(map[string]any{
					"status": models.ShareGrantStatusExpired, "updated_at": now,
					"revision": gorm.Expr("revision + 1"),
				})
			if res.Error != nil {
				logger.Ctx(ctx).Error().Err(res.Error).Msg("share grant lifecycle: failed to expire a grant")
				return total, res.Error
			}
			if res.RowsAffected != 1 {
				continue
			}
			wasBlocked := g.Status == models.ShareGrantStatusBlocked
			g.Status, g.UpdatedAt, g.Revision = models.ShareGrantStatusExpired, now, g.Revision+1
			auditShareGrant(db.WithContext(ctx), &g, models.AuditOpExpire, wasBlocked)
			moved++
		}
		total += int64(moved)
		if moved == 0 || len(rows) < shareGrantLifecycleBatch {
			return total, nil
		}
	}
}

// RunShareGrantLifecycleScheduled is the scheduled entry point: it takes the
// job lock (ADR 0011), reports the number of rows it moved or removed and
// returns ErrJobSkipped when the lock is held or the job ran too recently.
func RunShareGrantLifecycleScheduled(db *gorm.DB, cfg config.Config) (int, error) {
	ctx := logger.JobContext(models.JobNameShareGrantLifecycle)
	acquired, err := acquireJobLock(db, models.JobNameShareGrantLifecycle, shareGrantLifecycleMinInterval)
	if err != nil {
		return 0, err // # pragma: no cover — acquireJobLock never returns an error (job_lock.go returns err == nil, nil)
	}
	if !acquired {
		return 0, ErrJobSkipped
	}
	res, runErr := RunShareGrantLifecycle(ctx, db, cfg)
	if relErr := releaseJobLock(db, models.JobNameShareGrantLifecycle, runErr == nil); relErr != nil {
		logger.Ctx(ctx).Error().Err(relErr).Msg("share grant lifecycle: failed to release job lock") // # pragma: no cover — defensive: release only fails on a broken job_locks table
	}
	if runErr != nil {
		return 0, runErr
	}
	if res.Total() > 0 {
		logger.Ctx(ctx).Info().Int64("expired", res.Expired).Int64("purged", res.Purged).
			Int64("notices_purged", res.NoticesPurged).Msg("Share grant lifecycle pass completed")
	}
	return int(res.Total()), nil
}

// AuditRecipientDeletion writes, for every live incoming grant of userID, a
// `revoke` event with reason `recipient_deleted` on the OWNER's trail, so the
// owner's list does not silently lose rows when the recipient's account goes
// (ADR 0042 decision 10). Call it inside the delete transaction before the
// grants are hard-deleted; it writes nothing on the departing user's own trail
// (their audit rows are cascade-deleted with them) and does not delete.
func AuditRecipientDeletion(tx *gorm.DB, userID uint, now time.Time) error {
	var rows []models.ShareGrant
	if err := tx.Where("to_user_id = ? AND status IN ?", userID, models.ShareGrantLiveStatuses).Find(&rows).Error; err != nil {
		return err
	}
	now = now.UTC()
	reason := models.ShareGrantReasonRecipientDeleted
	for i := range rows {
		g := rows[i]
		g.Status, g.RevokedReason, g.RevokedAt = models.ShareGrantStatusRevoked, &reason, &now
		g.Revision++
		models.RecordShareGrantAudit(tx, &g, g.FromUserID, models.AuditOpRevoke)
	}
	return nil
}

// --- in-app badge and notices ----------------------------------------------

// PendingShareGrantCount is the recipient's badge: grants awaiting their
// answer that they can see and that time has not expired. A blocked sender's
// grant is never counted (decision 8).
func PendingShareGrantCount(db *gorm.DB, userID uint, now time.Time) (int64, error) {
	now = now.UTC()
	var n int64
	err := db.Model(&models.ShareGrant{}).
		Where("to_user_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?) AND created_at > ?",
			userID, models.ShareGrantStatusPending, now, now.Add(-ShareGrantPendingTTL)).
		Count(&n).Error
	return n, err
}

// ShareGrantNoticeView is one unread notice: sender username and entity type
// only, never a label or content.
type ShareGrantNoticeView struct {
	ID             uint      `json:"id"`
	Kind           string    `json:"kind"`
	EntityType     string    `json:"entity_type"`
	SenderUsername string    `json:"sender_username"`
	CreatedAt      time.Time `json:"created_at"`
}

// ListShareGrantNotices returns the caller's newest unread notices (capped).
func ListShareGrantNotices(db *gorm.DB, userID uint) ([]ShareGrantNoticeView, error) {
	out := []ShareGrantNoticeView{}
	err := db.Table("share_grant_notifications AS n").
		Select("n.id AS id, n.kind AS kind, n.entity_type AS entity_type, u.username AS sender_username, n.created_at AS created_at").
		Joins("JOIN users u ON u.id = n.from_user_id").
		Where("n.user_id = ? AND n.read_at IS NULL", userID).
		Order("n.id DESC").Limit(MaxShareGrantNotices).Scan(&out).Error
	return out, err
}

// MarkShareGrantNoticesRead marks every unread notice of the caller read and
// returns how many it changed. It is scoped by user_id.
func MarkShareGrantNoticesRead(db *gorm.DB, userID uint, now time.Time) (int64, error) {
	now = now.UTC()
	res := db.Table("share_grant_notifications").
		Where("user_id = ? AND read_at IS NULL", userID).
		UpdateColumns(map[string]any{"read_at": now, "updated_at": now})
	return res.RowsAffected, res.Error
}
