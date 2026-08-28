package models

import "time"

// StorageSample is one daily snapshot of the on-disk footprint behind the
// storage-growth trend on /admin/system-status (issue #652). The point-in-time
// block (issue #388) answers "how full is the disk right now"; the accumulated
// samples answer "how fast is the data growing, and when does the disk run
// out". ~180 days are retained (STORAGE_SAMPLE_RETENTION_DAYS).
//
// System-generated operational bookkeeping, hard-delete (no DeletedAt) — rows
// are removed only by the sampler's own retention prune, mirroring JobRun /
// SystemEvent's lifecycle (CLAUDE.md backend trap 7). Not user-scoped, no
// user_id. Read only via the admin system-status surface.
//
// Every field carries an explicit gorm:"column:..." tag: GORM's name
// derivation disagrees with hand-written migration SQL for acronyms/IDs and
// AutoMigrate-based tests cannot see it (CLAUDE.md backend trap #1).
type StorageSample struct {
	ID uint `gorm:"column:id;primarykey" json:"id"`

	// TakenAt is when the sample was measured (defaulted to now by the
	// sampler); the windowed trend queries index on it.
	TakenAt time.Time `gorm:"column:taken_at;not null;index" json:"taken_at"`

	// DatabaseBytes is metrics.DatabaseBytes(cfg.DBPath) — the main DB file
	// plus its -wal / -shm siblings, so the trend tracks the same total the
	// point-in-time block reports.
	DatabaseBytes int64 `gorm:"column:database_bytes;not null" json:"database_bytes"`

	// FSUsedBytes / FSTotalBytes are the filesystem holding the database
	// (used = total - free, from metrics.FilesystemBytes). The capacity
	// projection extrapolates FSUsedBytes to FSTotalBytes.
	FSUsedBytes  int64 `gorm:"column:fs_used_bytes;not null" json:"fs_used_bytes"`
	FSTotalBytes int64 `gorm:"column:fs_total_bytes;not null" json:"fs_total_bytes"`

	// PhotoDirBytes / AttachmentDirBytes are services.StorageUsage totals for
	// the two configured storage directories, persisted so the per-directory
	// growth is part of the history too.
	PhotoDirBytes      int64 `gorm:"column:photo_dir_bytes;not null;default:0" json:"photo_dir_bytes"`
	AttachmentDirBytes int64 `gorm:"column:attachment_dir_bytes;not null;default:0" json:"attachment_dir_bytes"`
}

// TableName pins the table name so it never drifts from the migration.
func (StorageSample) TableName() string {
	return "storage_samples"
}
