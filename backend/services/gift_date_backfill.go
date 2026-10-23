package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"mycorrhizal/logger"
	"mycorrhizal/models"
)

// Gift date -> calendar day startup backfill (ADR 0015 amendment, v1.5.0
// review).
//
// Gift.Date used to be an instant: the web client wrote the picked day's
// local midnight, Android did the same, and Android's "mark given" (plus the
// server's own given-date default, issue #1736) wrote the current time. Since
// #1736/#1823 the score and cadence read a gift date as a UTC calendar day,
// so those instants were off by one day east of UTC (and for an evening
// "mark given" west of UTC). The ADR now classifies Gift.Date as a calendar
// day stored at UTC midnight, and every write path normalizes through
// models.GiftCalendarDay. This job rewrites the rows written before that.
//
// The writer's zone is not stored, so each existing instant is read in the
// reminder zone (REMINDER_TIMEZONE) -- the best single-tenant proxy, and
// exactly how the server interpreted these values before #1823. A value
// already at UTC midnight is kept as is. Soft-deleted rows are included, so an
// undo restores a normalized date. updated_at is bumped so the incremental
// feed (and the Android offline mirror reading it) re-delivers the row.
//
// Runs once per database through migration 000052's data_backfills ledger.
// Re-running after a crash before the ledger write is safe: the normalization
// is idempotent.
const (
	// GiftDateBackfillName is the data_backfills.name ledger key.
	GiftDateBackfillName = "gifts.date_calendar_day"
	// GiftDateBackfillVersion bumps if the normalization rule changes.
	GiftDateBackfillVersion = 1
)

// GiftDateBackfillStats reports what a run did.
type GiftDateBackfillStats struct {
	GiftsScanned int64 `json:"gifts_scanned"`
	GiftsUpdated int64 `json:"gifts_updated"`
	AlreadyDone  bool  `json:"already_done"`
}

// giftDateBackfillBatch bounds how many rows one transaction rewrites.
const giftDateBackfillBatch = 500

// NormalizeGiftDatesToCalendarDays performs the one-shot rewrite described in
// the file comment, reading pre-existing instants in loc. A nil db is a no-op;
// a nil loc means UTC.
func NormalizeGiftDatesToCalendarDays(db *gorm.DB, loc *time.Location) (*GiftDateBackfillStats, error) {
	stats := &GiftDateBackfillStats{}
	if db == nil {
		return stats, nil
	}
	if loc == nil {
		loc = time.UTC
	}

	var n int64
	if err := db.Table("data_backfills").
		Where("name = ? AND version = ?", GiftDateBackfillName, GiftDateBackfillVersion).
		Count(&n).Error; err != nil {
		return stats, fmt.Errorf("gift date backfill (read ledger): %w", err)
	}
	if n > 0 {
		stats.AlreadyDone = true
		return stats, nil
	}

	type giftDateRow struct {
		ID   string
		Date time.Time
	}
	var rows []giftDateRow
	if err := db.Model(&models.Gift{}).Unscoped().
		Select("id, date").
		Where("date IS NOT NULL").
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return stats, fmt.Errorf("gift date backfill (load gifts): %w", err)
	}
	stats.GiftsScanned = int64(len(rows))

	now := Now().UTC()
	for start := 0; start < len(rows); start += giftDateBackfillBatch {
		batch := rows[start:min(start+giftDateBackfillBatch, len(rows))]
		err := db.Transaction(func(tx *gorm.DB) error {
			for _, r := range batch {
				day := models.GiftCalendarDay(r.Date, loc)
				if day.Equal(r.Date) {
					continue
				}
				if err := tx.Model(&models.Gift{}).Unscoped().Where("id = ?", r.ID).
					UpdateColumns(map[string]any{"date": day, "updated_at": now}).Error; err != nil {
					return fmt.Errorf("gift date backfill (update gift %s): %w", r.ID, err)
				}
				stats.GiftsUpdated++
			}
			return nil
		})
		if err != nil {
			return stats, err
		}
	}

	if err := db.Exec(
		`INSERT INTO data_backfills (name, version, completed_at) VALUES (?, ?, ?)`,
		GiftDateBackfillName, GiftDateBackfillVersion, now,
	).Error; err != nil {
		return stats, fmt.Errorf("gift date backfill (record completion): %w", err)
	}

	logger.Info().
		Int64("gifts_scanned", stats.GiftsScanned).
		Int64("gifts_updated", stats.GiftsUpdated).
		Str("zone", loc.String()).
		Msg("gift date backfill complete")
	return stats, nil
}
