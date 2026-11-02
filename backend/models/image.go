package models

import (
	"time"

	"gorm.io/gorm"
)

// Image sensitivity values. These mirror the normal|private|secret set used by
// relationship edges and field definitions (ADR 0040 decision 7), not a new
// vocabulary.
const (
	ImageSensitivityNormal  = "normal"
	ImageSensitivityPrivate = "private"
	ImageSensitivitySecret  = "secret"
)

// ImageFields is every column an image row has except its owner key. The three
// image tables (activity_images, and later note_images/contact_images) embed
// it so the storage, validation, sensitivity and soft-delete rules are decided
// once (ADR 0040 decision 1).
//
// User-authored content -> soft delete (ADR 0004, backend trap 7). The on-disk
// files are removed by the service after the DB transaction commits; the
// tombstone keeps the T17 change feed convergent.
type ImageFields struct {
	gorm.Model
	UserID uint   `gorm:"not null;index" json:"-"`
	UUID   string `gorm:"column:uuid;not null;uniqueIndex" json:"uuid"`

	// StoredName / ThumbName are server-generated UUID filenames on disk. The
	// original filename is deliberately not stored (ADR 0040 decision 1); no
	// client input ever reaches a filesystem path.
	StoredName string `gorm:"column:stored_name;not null" json:"-"`
	ThumbName  string `gorm:"column:thumb_name;not null" json:"-"`

	MIMEType       string `gorm:"column:mime_type;not null" json:"mime_type"`
	ThumbMIMEType  string `gorm:"column:thumb_mime_type;not null" json:"-"`
	Width          int    `gorm:"not null" json:"width"`
	Height         int    `gorm:"not null" json:"height"`
	SizeBytes      int64  `gorm:"column:size_bytes;not null" json:"size_bytes"`
	ThumbSizeBytes int64  `gorm:"column:thumb_size_bytes;not null" json:"-"`

	// Sensitivity decides which copies may leave the instance (ADR 0040
	// decision 7). Each image carries its own value; it is never inherited
	// from the parent.
	Sensitivity string `gorm:"column:sensitivity;not null;default:normal;index" json:"sensitivity"`

	// DHash is the 64-bit difference hash (ADR 0040 decision 9). Activity
	// images are not deduplicated in slice (a), but the hash is stored so the
	// rule can widen later.
	DHash *int64 `gorm:"column:dhash" json:"-"`

	// TakenAt is set only when EXIF carried a zone offset; TakenDate is the
	// camera's local wall-clock day exactly as written in EXIF (ADR 0015).
	TakenAt   *time.Time `gorm:"column:taken_at" json:"taken_at"`
	TakenDate *string    `gorm:"column:taken_date" json:"taken_date"`

	// CropX/Y/W/H are optional crop fractions of the full variant. Reserved
	// for the contact gallery (slice c); activity images do not expose crop
	// yet.
	CropX *float64 `gorm:"column:crop_x" json:"-"`
	CropY *float64 `gorm:"column:crop_y" json:"-"`
	CropW *float64 `gorm:"column:crop_w" json:"-"`
	CropH *float64 `gorm:"column:crop_h" json:"-"`
}

// Fields returns the embedded image columns. It implements the services
// ImageRow interface so the generic image service can operate on any owner.
func (i *ImageFields) Fields() *ImageFields { return i }

// ActivityImage is one image owned by an activity. The owner key is NOT NULL
// so the ownership join is a plain indexed comparison, not a polymorphic
// nullable lookup (ADR 0040 decision 1).
type ActivityImage struct {
	ImageFields
	ActivityID uint `gorm:"column:activity_id;not null;index" json:"activity_id"`
}

// TableName pins the table name so it never drifts from the migration.
func (ActivityImage) TableName() string { return "activity_images" }

// OwnerColumn is the column that names this image's owning activity.
func (a *ActivityImage) OwnerColumn() string { return "activity_id" }

// SetOwner assigns the owner key. The generic service uses this instead of
// reflection so each per-owner model declares its own key once.
func (a *ActivityImage) SetOwner(owner OwnerRef) { a.ActivityID = owner.ID }

// OwnerRef is the minimal portable identity of an image owner: the numeric id
// for activity/note owners, or the vCard UID for a contact owner.
type OwnerRef struct {
	ID  uint
	UID string
}

// NoteImage is one image owned by a note (ADR 0040 slice b).
type NoteImage struct {
	ImageFields
	NoteID uint `gorm:"column:note_id;not null;index" json:"note_id"`
}

// TableName pins the table name so it never drifts from the migration.
func (NoteImage) TableName() string { return "note_images" }

// OwnerColumn is the column that names this image's owning note.
func (n *NoteImage) OwnerColumn() string { return "note_id" }

// SetOwner assigns the owner key.
func (n *NoteImage) SetOwner(owner OwnerRef) { n.NoteID = owner.ID }
