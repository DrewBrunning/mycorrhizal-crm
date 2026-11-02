package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
	"mycorrhizal/internal/diskspace"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OwnerKind labels an image owner for limits, metrics and audit. It is not a
// polymorphic discriminator on disk: each owner kind has its own table (ADR
// 0040 decision 1).
type OwnerKind string

const (
	OwnerKindActivity OwnerKind = "activity"
	OwnerKindNote     OwnerKind = "note"
	OwnerKindContact  OwnerKind = "contact"
)

// OwnerRef identifies the parent an image belongs to. ID is the numeric parent
// id for activities and notes; UID is the contact vcard_uid (and, for an
// activity, also its stable UUID used in the audit event).
type OwnerRef = models.OwnerRef

// ImageRow is implemented, by pointer, by every owner-specific image model.
type ImageRow interface {
	Fields() *models.ImageFields
	OwnerColumn() string
	SetOwner(OwnerRef)
}

// ImageService is the one repository/service for image upload, list, fetch and
// delete. It is parameterized by owner kind so the per-owner code is only the
// model's OwnerColumn/SetOwner pair (ADR 0040 decision 1).
type ImageService[T any, P interface {
	*T
	ImageRow
}] struct {
	DB       *gorm.DB
	Store    imaging.Store
	Kind     OwnerKind
	Limit    int
	Limits   imaging.Limits
	MaxBytes int64
	Quota    UserQuota
}

// ActivityImageService builds the activity-image service from config.
func ActivityImageService(db *gorm.DB, store imaging.Store, limit, maxPixels int, maxBytes int64, quota UserQuota) *ImageService[models.ActivityImage, *models.ActivityImage] {
	if maxPixels <= 0 {
		maxPixels = imaging.DefaultLimits.MaxPixels
	}
	if maxBytes <= 0 {
		maxBytes = 20 << 20
	}
	return &ImageService[models.ActivityImage, *models.ActivityImage]{
		DB:       db,
		Store:    store,
		Kind:     OwnerKindActivity,
		Limit:    limit,
		Limits:   imaging.Limits{MaxPixels: maxPixels},
		MaxBytes: maxBytes,
		Quota:    quota,
	}
}

func (s *ImageService[T, P]) ownerValue(owner OwnerRef) any {
	if s.Kind == OwnerKindContact {
		return owner.UID
	}
	return owner.ID
}

func (s *ImageService[T, P]) baseQuery(ctx context.Context, userID uint, owner OwnerRef) *gorm.DB {
	return s.DB.WithContext(ctx).Where("user_id = ? AND "+s.newRow().OwnerColumn()+" = ?", userID, s.ownerValue(owner))
}

func (s *ImageService[T, P]) newRow() ImageRow {
	var row T
	return P(&row)
}

// List returns an owner's live images, newest first.
func (s *ImageService[T, P]) List(ctx context.Context, userID uint, owner OwnerRef) ([]T, error) {
	var rows []T
	if err := s.baseQuery(ctx, userID, owner).Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to retrieve images").WithError(err)
	}
	return rows, nil
}

// Get loads one image scoped to the user and owner. A foreign/other-owner row
// is indistinguishable from a missing one (404, never 403).
func (s *ImageService[T, P]) Get(ctx context.Context, userID uint, owner OwnerRef, id uint) (*T, error) {
	var row T
	q := s.baseQuery(ctx, userID, owner)
	if err := q.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound("Image")
		}
		return nil, apperrors.ErrDatabase("Failed to retrieve image").WithError(err)
	}
	return &row, nil
}

// Upload validates and stores one image, then inserts its row. The on-disk
// files are removed if the row insert fails, so a failed request leaves no
// orphan behind (ADR 0040 decision 5).
func (s *ImageService[T, P]) Upload(ctx context.Context, userID uint, owner OwnerRef, src io.Reader) (*T, error) {
	if s.Limit > 0 {
		var count int64
		var model T
		if err := s.baseQuery(ctx, userID, owner).Model(&model).Count(&count).Error; err != nil {
			return nil, apperrors.ErrDatabase("Failed to check the image limit").WithError(err)
		}
		if count >= int64(s.Limit) {
			return nil, apperrors.ErrConflict("Image limit reached").WithReason(apperrors.ReasonImageLimitReached, map[string]any{"limit": s.Limit})
		}
	}

	data, err := io.ReadAll(src)
	if err != nil {
		return nil, imageError(imaging.ErrCorrupt, s.Limits.MaxPixels, s.MaxBytes)
	}
	if s.MaxBytes > 0 && int64(len(data)) > s.MaxBytes {
		return nil, imageError(imaging.ErrTooLarge, s.Limits.MaxPixels, s.MaxBytes)
	}
	result, err := imaging.Process(bytes.NewReader(data), s.Limits)
	if err != nil {
		return nil, imageError(err, s.Limits.MaxPixels, s.MaxBytes)
	}

	if s.Quota.Enabled() {
		if qerr := s.Quota.CheckAttachmentUpload(s.DB, userID, int64(len(result.Full)+len(result.Thumb))); qerr != nil {
			return nil, qerr
		}
	}
	if s.Store.Dir != "" {
		if err := diskspace.Require(s.Store.Dir, uint64(len(result.Full)+len(result.Thumb))); err != nil {
			return nil, apperrors.ErrInsufficientStorage(err.Error()).WithReason(apperrors.ReasonImageInsufficientStorage, nil)
		}
	}

	storedName, thumbName, err := s.Store.Save(userID, result.Full, result.Thumb, result.Ext, result.Ext)
	if err != nil {
		return nil, apperrors.ErrInternal("Failed to store image").WithError(err)
	}

	var row T
	p := P(&row)
	p.SetOwner(owner)
	f := p.Fields()
	f.UserID = userID
	f.UUID = uuid.New().String()
	f.StoredName = storedName
	f.ThumbName = thumbName
	f.MIMEType = result.MIME
	f.ThumbMIMEType = result.ThumbMIME
	f.Width = result.Width
	f.Height = result.Height
	f.SizeBytes = int64(len(result.Full))
	f.ThumbSizeBytes = int64(len(result.Thumb))
	f.Sensitivity = models.ImageSensitivityNormal
	f.DHash = &result.DHash
	f.TakenAt = result.TakenAt
	f.TakenDate = result.TakenDate

	if err := s.DB.WithContext(ctx).Create(p).Error; err != nil {
		_ = s.Store.Remove(userID, storedName)
		_ = s.Store.Remove(userID, thumbName)
		return nil, apperrors.ErrDatabase("Failed to save image").WithError(err)
	}
	models.RecordAuditEvent(s.DB, models.AuditEntityActivity, imageAuditID(owner), models.AuditOpCreate, userID)
	return p, nil
}

// Delete soft-deletes one image and, after the transaction commits, removes
// both on-disk files. A failed file removal is logged, not rolled back (ADR
// 0040 decision 5).
func (s *ImageService[T, P]) Delete(ctx context.Context, userID uint, owner OwnerRef, id uint) error {
	row, err := s.Get(ctx, userID, owner, id)
	if err != nil {
		return err
	}
	p := P(row)
	f := p.Fields()

	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(row).Error; err != nil {
			return err
		}
		models.RecordAuditEvent(tx, models.AuditEntityActivity, imageAuditID(owner), models.AuditOpDelete, userID)
		return nil
	}); err != nil {
		return apperrors.ErrDatabase("Failed to delete image").WithError(err)
	}

	if err := s.Store.Remove(userID, f.StoredName); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("stored_name", f.StoredName).Msg("Failed to remove image file")
	}
	if err := s.Store.Remove(userID, f.ThumbName); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("stored_name", f.ThumbName).Msg("Failed to remove image thumbnail")
	}
	return nil
}

func (s *ImageService[T, P]) Count(ctx context.Context, userID uint, owner OwnerRef) (int64, error) {
	var count int64
	var model T
	if err := s.baseQuery(ctx, userID, owner).Model(&model).Count(&count).Error; err != nil {
		return 0, apperrors.ErrDatabase("Failed to count images").WithError(err)
	}
	return count, nil
}

// imageError maps the imaging package's sentinels to stable reason codes.
func imageError(err error, maxPixels int, maxBytes int64) *apperrors.AppError {
	switch {
	case errors.Is(err, imaging.ErrTooLarge):
		return apperrors.ErrPayloadTooLarge("image too large").WithReason(apperrors.ReasonImageTooLarge, map[string]any{"max_mb": maxBytes >> 20})
	case errors.Is(err, imaging.ErrBusy):
		return apperrors.NewError(apperrors.ErrCodeInternal, "image processing is busy", http.StatusServiceUnavailable).WithReason(apperrors.ReasonImageProcessingBusy, nil)
	case errors.Is(err, imaging.ErrPixelsExceeded):
		return apperrors.ErrInvalidInput("file", "image dimensions exceed the pixel limit").
			WithReason(apperrors.ReasonImagePixelsExceeded, map[string]any{"max_pixels": maxPixels})
	case errors.Is(err, imaging.ErrHEICUnsupported):
		return apperrors.ErrInvalidInput("file", "HEIC images are not supported").
			WithReason(apperrors.ReasonImageHEICUnsupported, nil)
	case errors.Is(err, imaging.ErrUnsupportedFormat):
		return apperrors.ErrInvalidInput("file", "unsupported image format").
			WithReason(apperrors.ReasonImageUnsupportedFormat, nil)
	default:
		return apperrors.ErrInvalidInput("file", "the image is corrupt or truncated").
			WithReason(apperrors.ReasonImageCorrupt, nil)
	}
}

func imageAuditID(owner OwnerRef) string {
	if owner.UID != "" {
		return owner.UID
	}
	return strconv.FormatUint(uint64(owner.ID), 10)
}
