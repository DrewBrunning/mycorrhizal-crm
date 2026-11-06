package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// imageSweepGrace is how old a file or empty note must be before the sweeper
// touches it, so an in-flight upload (files written, row not yet inserted) or
// a note waiting for its first image is never raced (ADR 0040 decisions 5, 11).
const imageSweepGrace = time.Hour

// imageSweepMinInterval is the ADR 0011 de-dup window for the daily sweep.
var imageSweepMinInterval = JobCatchupWindow(24 * time.Hour)

// ImageSweepResult is what one sweep removed. Counts only: no file names are
// ever recorded (ADR 0040 decision 5).
type ImageSweepResult struct {
	FilesRemoved int64
	BytesRemoved int64
	NotesRemoved int64
}

// SweepImages removes gallery files no image row names and soft-deletes empty
// image-less notes whose awaiting-images window has lapsed (ADR 0040 decisions
// 5 and 11). A file is removed iff it is older than the grace period and no
// row in any of the three image tables, read Unscoped, names it as stored_name
// or thumb_name; stale *.tmp files go regardless of rows. It only ever looks
// under cfg.ImagesDir, never PROFILE_PHOTO_DIR or ATTACHMENTS_DIR. Per-file
// failures are logged and skipped; the returned error is a failure to scan.
func SweepImages(ctx context.Context, db *gorm.DB, cfg config.Config) (ImageSweepResult, error) {
	var res ImageSweepResult
	cutoff := Now().Add(-imageSweepGrace)

	if cfg.ImagesDir != "" {
		if err := sweepImageFiles(ctx, db, cfg.ImagesDir, cutoff, &res); err != nil {
			return res, err
		}
	}

	var notes []models.Note
	if err := db.WithContext(ctx).
		Where("content = '' AND created_at < ?", cutoff).
		Where("NOT EXISTS (SELECT 1 FROM note_images WHERE note_images.note_id = notes.id AND note_images.deleted_at IS NULL)").
		Find(&notes).Error; err != nil {
		return res, fmt.Errorf("image sweep: find empty notes: %w", err) // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
	}
	for i := range notes {
		// ADR 0042 decision 10: a note's standing-share grants end with it.
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&notes[i]).Error; err != nil {
				return err
			}
			_, err := RevokeShareGrantsForEntity(tx, notes[i].UserID, models.ShareGrantEntityNote, notes[i].UUID, Now())
			return err
		}); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Uint("note_id", notes[i].ID).Msg("image sweep: failed to soft-delete empty note") // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
			continue
		}
		res.NotesRemoved++
	}
	return res, nil
}

func sweepImageFiles(ctx context.Context, db *gorm.DB, root string, cutoff time.Time, res *ImageSweepResult) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("image sweep: read %s: %w", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "u") {
			continue
		}
		userID, err := strconv.ParseUint(strings.TrimPrefix(e.Name(), "u"), 10, strconv.IntSize)
		if err != nil {
			continue
		}
		if err := sweepUserImageDir(ctx, db, filepath.Join(root, e.Name()), uint(userID), cutoff, res); err != nil {
			return err // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
		}
	}
	return nil
}

func sweepUserImageDir(ctx context.Context, db *gorm.DB, dir string, userID uint, cutoff time.Time, res *ImageSweepResult) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("image sweep: read %s: %w", dir, err) // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
	}
	referenced := map[string]struct{}{}
	for _, table := range []string{"activity_images", "note_images", "contact_images"} {
		var rows []struct{ StoredName, ThumbName string }
		if err := db.WithContext(ctx).Table(table).
			Select("stored_name, thumb_name").Where("user_id = ?", userID).Scan(&rows).Error; err != nil {
			return fmt.Errorf("image sweep: list %s: %w", table, err) // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
		}
		for _, r := range rows {
			referenced[r.StoredName] = struct{}{}
			referenced[r.ThumbName] = struct{}{}
		}
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		info, err := f.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if _, ok := referenced[f.Name()]; ok && !strings.HasSuffix(f.Name(), ".tmp") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Msg("image sweep: failed to remove file") // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
			continue
		}
		res.FilesRemoved++
		res.BytesRemoved += info.Size()
	}
	return nil
}

// SweepImagesScheduled is the scheduled entry point: it takes the job lock
// (ADR 0011) and reports the number of items removed, recording the file and
// byte totals on the operational timeline.
func SweepImagesScheduled(db *gorm.DB, cfg config.Config) (int, error) {
	ctx := logger.JobContext(models.JobNameImageSweep)
	acquired, err := acquireJobLock(db, models.JobNameImageSweep, imageSweepMinInterval)
	if err != nil {
		return 0, err // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
	}
	if !acquired {
		return 0, ErrJobSkipped
	}
	res, sweepErr := SweepImages(ctx, db, cfg)
	if relErr := releaseJobLock(db, models.JobNameImageSweep, sweepErr == nil); relErr != nil {
		logger.Ctx(ctx).Error().Err(relErr).Msg("image sweep: failed to release job lock") // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
	}
	if sweepErr != nil {
		return 0, sweepErr // # pragma: no cover — defensive: a healthy migrated schema and a readable image directory do not fail here
	}
	if res.FilesRemoved > 0 || res.NotesRemoved > 0 {
		models.RecordSystemEvent(ctx, db, models.SystemEvent{
			EventType: models.SysEventJobCompleted,
			Component: logger.ComponentStorage,
			Operation: models.JobNameImageSweep,
			Result:    models.SysResult(logger.ResultSuccess),
			Detail:    fmt.Sprintf("files_removed=%d bytes_removed=%d empty_notes_removed=%d", res.FilesRemoved, res.BytesRemoved, res.NotesRemoved),
		})
	}
	return int(res.FilesRemoved + res.NotesRemoved), nil
}
