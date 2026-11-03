package services

import (
	"context"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ImageKindUsage is the row count and on-disk bytes (full plus thumbnail) of
// one owner kind's images.
type ImageKindUsage struct {
	Count int64 `json:"count"`
	Bytes int64 `json:"bytes"`
}

// ImageByKind splits ImageUsage by owner kind.
type ImageByKind struct {
	Activity ImageKindUsage `json:"activity"`
	Note     ImageKindUsage `json:"note"`
	Contact  ImageKindUsage `json:"contact"`
}

// ImageUsage is the admin storage view of gallery images (ADR 0040 decision
// 6): summed from the tables over live and not-yet-purged soft-deleted rows,
// because both occupy disk until the purge or sweeper removes the files.
type ImageUsage struct {
	Count  int64       `json:"count"`
	Bytes  int64       `json:"bytes"`
	ByKind ImageByKind `json:"by_kind"`
}

// ImageStorageUsage sums the three image tables. Best-effort like the rest of
// the status block: a failed read leaves that kind at zero and logs, so the
// diagnostic endpoint never turns into a 500.
func ImageStorageUsage(ctx context.Context, db *gorm.DB) ImageUsage {
	sum := func(model any, name string) ImageKindUsage {
		var out ImageKindUsage
		err := db.WithContext(ctx).Unscoped().Model(model).
			Select("COUNT(*) AS count, COALESCE(SUM(size_bytes + thumb_size_bytes), 0) AS bytes").
			Scan(&out).Error
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).Str("table", name).Msg("image storage usage: sum failed")
		}
		return out
	}
	u := ImageUsage{ByKind: ImageByKind{
		Activity: sum(&models.ActivityImage{}, "activity_images"),
		Note:     sum(&models.NoteImage{}, "note_images"),
		Contact:  sum(&models.ContactImage{}, "contact_images"),
	}}
	for _, k := range []ImageKindUsage{u.ByKind.Activity, u.ByKind.Note, u.ByKind.Contact} {
		u.Count += k.Count
		u.Bytes += k.Bytes
	}
	return u
}
