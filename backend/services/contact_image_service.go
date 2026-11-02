package services

import (
	"context"
	"strings"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
	"mycorrhizal/models"
	"mycorrhizal/photostore"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SetContactMainImage makes a gallery image the contact's main image and
// derives the legacy avatar from it (ADR 0040 decision 10). The stored full
// variant is re-cropped/resized to the existing 400 px avatar and 48 px
// thumbnail; the contact row is saved in one transaction so the revision/ETag
// bump is visible to CardDAV.
func SetContactMainImage(ctx context.Context, db *gorm.DB, store imaging.Store, userID uint, contact *models.Contact, image *models.ContactImage, photoDir string) (*models.Contact, error) {
	full, err := store.Read(userID, image.StoredName)
	if err != nil {
		return nil, apperrors.ErrNotFound("Image file")
	}
	var crop *photostore.CropRect
	if image.CropX != nil && image.CropY != nil && image.CropW != nil && image.CropH != nil {
		crop = &photostore.CropRect{
			X: *image.CropX,
			Y: *image.CropY,
			W: *image.CropW,
			H: *image.CropH,
		}
	}
	photoName, thumb, err := photostore.SaveContactPhotoCropped(full, image.MIMEType, crop, photoDir)
	if err != nil {
		return nil, apperrors.ErrValidation("Failed to derive the profile photo").
			WithReason(apperrors.ReasonPhotoProcessFailed, nil).WithError(err)
	}

	oldPhoto := contact.Photo
	contact.MainImageID = &image.ID
	contact.Photo = photoName
	contact.PhotoThumbnail = thumb
	if err := db.WithContext(ctx).Save(contact).Error; err != nil {
		_ = photostore.RemoveContactPhoto(photoDir, photoName)
		return nil, apperrors.ErrDatabase("Failed to set the main image").WithError(err)
	}
	if oldPhoto != "" && oldPhoto != photoName {
		_ = photostore.RemoveContactPhoto(photoDir, oldPhoto)
	}
	return contact, nil
}

// ClearContactMainImage clears the main-image pointer and the derived avatar.
// A legacy photo with no main image is deliberately left untouched (ADR 0040
// decision 10).
func ClearContactMainImage(ctx context.Context, db *gorm.DB, userID uint, contact *models.Contact, photoDir string) (*models.Contact, error) {
	if contact.MainImageID == nil {
		return contact, nil
	}
	oldPhoto := contact.Photo
	contact.MainImageID = nil
	contact.Photo = ""
	contact.PhotoThumbnail = ""
	if err := db.WithContext(ctx).Save(contact).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to clear the main image").WithError(err)
	}
	if oldPhoto != "" {
		_ = photostore.RemoveContactPhoto(photoDir, oldPhoto)
	}
	return contact, nil
}

// AdoptImageIntoContact copies a note's or activity's stored full variant into
// the contact gallery without re-decoding it (ADR 0040 decision 10). The
// source must belong to the same user and to this contact (the note is filed
// under it, or the activity is linked to it). Sensitivity is copied verbatim
// and never lowered.
func AdoptImageIntoContact(ctx context.Context, db *gorm.DB, store imaging.Store, userID uint, contact *models.Contact, kind string, sourceID uint, imageID uint, limit int) (*models.ContactImage, error) {
	if limit > 0 {
		var count int64
		if err := db.WithContext(ctx).Model(&models.ContactImage{}).
			Where("user_id = ? AND contact_vcard_uid = ?", userID, contact.VCardUID).
			Count(&count).Error; err != nil {
			return nil, apperrors.ErrDatabase("Failed to check the image limit").WithError(err)
		}
		if count >= int64(limit) {
			return nil, apperrors.ErrConflict("Image limit reached").
				WithReason(apperrors.ReasonImageLimitReached, map[string]any{"limit": limit})
		}
	}

	type sourceImage struct {
		storedName  string
		thumbName   string
		mime        string
		thumbMIME   string
		width       int
		height      int
		sizeBytes   int64
		thumbBytes  int64
		sensitivity string
		dhash       *int64
		takenAt     *time.Time
		takenDate   *string
	}

	var src sourceImage
	switch kind {
	case "note":
		var note models.Note
		if err := db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, sourceID).First(&note).Error; err != nil {
			return nil, apperrors.ErrNotFound("Image")
		}
		if note.ContactID == nil || *note.ContactID != contact.ID {
			return nil, apperrors.ErrNotFound("Image")
		}
		var img models.NoteImage
		if err := db.WithContext(ctx).Where("user_id = ? AND note_id = ? AND id = ?", userID, note.ID, imageID).First(&img).Error; err != nil {
			return nil, apperrors.ErrNotFound("Image")
		}
		src = sourceImage{img.StoredName, img.ThumbName, img.MIMEType, img.ThumbMIMEType, img.Width, img.Height, img.SizeBytes, img.ThumbSizeBytes, img.Sensitivity, img.DHash, img.TakenAt, img.TakenDate}
	case "activity":
		var linked int64
		if err := db.WithContext(ctx).Table("activity_contacts").
			Where("activity_id = ? AND contact_id = ?", sourceID, contact.ID).Count(&linked).Error; err != nil {
			return nil, apperrors.ErrDatabase("Failed to check the activity").WithError(err)
		}
		if linked == 0 {
			return nil, apperrors.ErrNotFound("Image")
		}
		var img models.ActivityImage
		if err := db.WithContext(ctx).Where("user_id = ? AND activity_id = ? AND id = ?", userID, sourceID, imageID).First(&img).Error; err != nil {
			return nil, apperrors.ErrNotFound("Image")
		}
		src = sourceImage{img.StoredName, img.ThumbName, img.MIMEType, img.ThumbMIMEType, img.Width, img.Height, img.SizeBytes, img.ThumbSizeBytes, img.Sensitivity, img.DHash, img.TakenAt, img.TakenDate}
	default:
		return nil, apperrors.ErrValidation("source_kind must be note or activity").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "source_kind"})
	}

	full, err := store.Read(userID, src.storedName)
	if err != nil {
		return nil, apperrors.ErrNotFound("Image file")
	}
	thumb, err := store.Read(userID, src.thumbName)
	if err != nil {
		return nil, apperrors.ErrNotFound("Image file")
	}
	ext, thumbExt := imageExtension(src.mime), imageExtension(src.thumbMIME)
	newFull, newThumb, err := store.Save(userID, full, thumb, ext, thumbExt)
	if err != nil {
		return nil, apperrors.ErrInternal("Failed to copy image").WithError(err)
	}

	row := models.ContactImage{
		ImageFields: models.ImageFields{
			UserID:         userID,
			UUID:           uuid.New().String(),
			StoredName:     newFull,
			ThumbName:      newThumb,
			MIMEType:       src.mime,
			ThumbMIMEType:  src.thumbMIME,
			Width:          src.width,
			Height:         src.height,
			SizeBytes:      src.sizeBytes,
			ThumbSizeBytes: src.thumbBytes,
			Sensitivity:    src.sensitivity,
			DHash:          src.dhash,
			TakenAt:        src.takenAt,
			TakenDate:      src.takenDate,
		},
		ContactVCardUID: contact.VCardUID,
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		_ = store.Remove(userID, newFull)
		_ = store.Remove(userID, newThumb)
		return nil, apperrors.ErrDatabase("Failed to adopt image").WithError(err)
	}
	models.RecordAuditEvent(db, models.AuditEntityContact, contact.VCardUID, models.AuditOpCreate, userID)
	return &row, nil
}

func imageExtension(mime string) string {
	switch strings.ToLower(mime) {
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	default:
		return "jpg"
	}
}
