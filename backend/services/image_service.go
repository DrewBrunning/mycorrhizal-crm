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

// ContactImageService builds the contact-gallery service from config (ADR 0040 slice c).
func ContactImageService(db *gorm.DB, store imaging.Store, limit, maxPixels int, maxBytes int64, quota UserQuota) *ImageService[models.ContactImage, *models.ContactImage] {
	if maxPixels <= 0 {
		maxPixels = imaging.DefaultLimits.MaxPixels
	}
	if maxBytes <= 0 {
		maxBytes = 20 << 20
	}
	return &ImageService[models.ContactImage, *models.ContactImage]{
		DB:       db,
		Store:    store,
		Kind:     OwnerKindContact,
		Limit:    limit,
		Limits:   imaging.Limits{MaxPixels: maxPixels},
		MaxBytes: maxBytes,
		Quota:    quota,
	}
}

// NoteImageService builds the note-image service from config (ADR 0040 slice b).
func NoteImageService(db *gorm.DB, store imaging.Store, limit, maxPixels int, maxBytes int64, quota UserQuota) *ImageService[models.NoteImage, *models.NoteImage] {
	if maxPixels <= 0 {
		maxPixels = imaging.DefaultLimits.MaxPixels
	}
	if maxBytes <= 0 {
		maxBytes = 20 << 20
	}
	return &ImageService[models.NoteImage, *models.NoteImage]{
		DB:       db,
		Store:    store,
		Kind:     OwnerKindNote,
		Limit:    limit,
		Limits:   imaging.Limits{MaxPixels: maxPixels},
		MaxBytes: maxBytes,
		Quota:    quota,
	}
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

func (s *ImageService[T, P]) auditEntity() string {
	switch s.Kind {
	case OwnerKindNote:
		return models.AuditEntityNote
	case OwnerKindContact:
		return models.AuditEntityContact
	case OwnerKindActivity:
		return models.AuditEntityActivity
	default:
		return models.AuditEntityActivity
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

// processUpload validates and processes one image without touching disk or the
// database, so dedupe can decide before storing anything.
func (s *ImageService[T, P]) processUpload(ctx context.Context, userID uint, owner OwnerRef, src io.Reader) (*imaging.Result, error) {
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

	reader := src
	if s.MaxBytes > 0 {
		reader = io.LimitReader(src, s.MaxBytes+1)
	}
	data, err := io.ReadAll(reader)
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
	return result, nil
}

// storeUpload writes the processed variants and inserts the row. A failed
// insert removes the files so no orphan is left behind (ADR 0040 decision 5).
func (s *ImageService[T, P]) storeUpload(ctx context.Context, userID uint, owner OwnerRef, result *imaging.Result) (*T, error) {
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
	models.RecordAuditEvent(s.DB, s.auditEntity(), imageAuditID(owner), models.AuditOpCreate, userID)
	return p, nil
}

// Upload validates and stores one image, then inserts its row. The on-disk
// files are removed if the row insert fails, so a failed request leaves no
// orphan behind (ADR 0040 decision 5).
func (s *ImageService[T, P]) Upload(ctx context.Context, userID uint, owner OwnerRef, src io.Reader) (*T, error) {
	result, err := s.processUpload(ctx, userID, owner, src)
	if err != nil {
		return nil, err
	}
	return s.storeUpload(ctx, userID, owner, result)
}

// UploadDeduped is Upload for the contact gallery: a live image on the same
// contact whose dHash is within imaging.DedupeMaxHamming is returned instead
// of inserting a re-encoded copy (ADR 0040 decision 9). Other owner kinds
// never dedupe. The bool reports whether an existing row was returned.
func (s *ImageService[T, P]) UploadDeduped(ctx context.Context, userID uint, owner OwnerRef, src io.Reader) (*T, bool, error) {
	result, err := s.processUpload(ctx, userID, owner, src)
	if err != nil {
		return nil, false, err
	}
	if s.Kind == OwnerKindContact {
		var rows []T
		if err := s.baseQuery(ctx, userID, owner).Where("dhash IS NOT NULL").Find(&rows).Error; err != nil {
			return nil, false, apperrors.ErrDatabase("Failed to check for a duplicate image").WithError(err)
		}
		for i := range rows {
			hash := P(&rows[i]).Fields().DHash
			if hash != nil && imaging.HammingDistance(*hash, result.DHash) <= imaging.DedupeMaxHamming {
				return &rows[i], true, nil
			}
		}
	}
	row, err := s.storeUpload(ctx, userID, owner, result)
	return row, false, err
}

// Update applies the PATCH allowlist (sensitivity and crop) to one image.
func (s *ImageService[T, P]) Update(ctx context.Context, userID uint, owner OwnerRef, id uint, in models.ContactImagePatchInput) (*T, error) {
	row, err := s.Get(ctx, userID, owner, id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if in.Sensitivity != nil {
		switch *in.Sensitivity {
		case models.ImageSensitivityNormal, models.ImageSensitivityPrivate, models.ImageSensitivitySecret:
			updates["sensitivity"] = *in.Sensitivity
		default:
			return nil, apperrors.ErrInvalidInput("sensitivity", "must be normal, private or secret").
				WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "sensitivity"})
		}
	}
	if in.ResetCrop {
		updates["crop_x"] = nil
		updates["crop_y"] = nil
		updates["crop_w"] = nil
		updates["crop_h"] = nil
	} else if in.Crop != nil {
		c := *in.Crop
		// Positive-form checks so NaN (every comparison false) is rejected.
		const eps = 1e-9
		if !(c.X >= 0 && c.Y >= 0 && c.W > 0 && c.H > 0 && c.X+c.W <= 1+eps && c.Y+c.H <= 1+eps) {
			return nil, apperrors.ErrInvalidInput("crop", "crop fractions must be inside the image").
				WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "crop"})
		}
		updates["crop_x"] = c.X
		updates["crop_y"] = c.Y
		updates["crop_w"] = c.W
		updates["crop_h"] = c.H
	}
	if len(updates) == 0 {
		return row, nil
	}
	if err := s.DB.WithContext(ctx).Model(row).Updates(updates).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to update image").WithError(err)
	}
	models.RecordAuditEvent(s.DB, s.auditEntity(), imageAuditID(owner), models.AuditOpUpdate, userID)
	return s.Get(ctx, userID, owner, id)
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
		models.RecordAuditEvent(tx, s.auditEntity(), imageAuditID(owner), models.AuditOpDelete, userID)
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
