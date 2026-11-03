package services

import (
	"context"
	"errors"
	"strings"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
	"mycorrhizal/models"
	"mycorrhizal/photostore"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errMainImageGone = errors.New("main image is no longer live")

// SetContactMainImage makes a gallery image the contact's main image and
// derives the legacy avatar from it (ADR 0040 decision 10). The stored full
// variant is re-cropped/resized to the existing 400 px avatar and 48 px
// thumbnail. The contact row is saved in one transaction that first re-checks
// the image is still live and owned, so a concurrent delete cannot leave the
// pointer on a tombstone; the save bumps the revision/ETag for CardDAV.
func SetContactMainImage(ctx context.Context, db *gorm.DB, store imaging.Store, userID uint, contact *models.Contact, image *models.ContactImage, photoDir string) (*models.Contact, error) {
	// The derived avatar ships through CardDAV, vCard export and shares, and the
	// outward photo filter for private/secret images is not built yet, so only
	// normal-sensitivity images may become the main image (ADR 0040 decision 7).
	if image.Sensitivity != models.ImageSensitivityNormal {
		return nil, apperrors.ErrValidation("Only a normal-sensitivity image can be the main image").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "image_id"})
	}
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
		return nil, apperrors.ErrInternal("Failed to derive the profile photo").
			WithReason(apperrors.ReasonPhotoProcessFailed, nil).WithError(err)
	}

	// A legacy photo (no main image) exists nowhere else, so it is never
	// unlinked here; only a previously derived avatar is replaced (ADR 0040
	// decision 10).
	oldPhoto := ""
	if contact.MainImageID != nil {
		oldPhoto = contact.Photo
	}
	prevMain, prevPhoto, prevThumb := contact.MainImageID, contact.Photo, contact.PhotoThumbnail
	contact.MainImageID = &image.ID
	contact.Photo = photoName
	contact.PhotoThumbnail = thumb
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var live int64
		if err := tx.Model(&models.ContactImage{}).
			Where("id = ? AND user_id = ? AND contact_vcard_uid = ?", image.ID, userID, contact.VCardUID).
			Count(&live).Error; err != nil {
			return err
		}
		if live == 0 {
			return errMainImageGone
		}
		return tx.Save(contact).Error
	})
	if err != nil {
		contact.MainImageID, contact.Photo, contact.PhotoThumbnail = prevMain, prevPhoto, prevThumb
		_ = photostore.RemoveContactPhoto(photoDir, photoName)
		if errors.Is(err, errMainImageGone) {
			return nil, apperrors.ErrNotFound("Image")
		}
		return nil, apperrors.ErrDatabase("Failed to set the main image").WithError(err)
	}
	if oldPhoto != "" && oldPhoto != photoName {
		_ = photostore.RemoveContactPhoto(photoDir, oldPhoto)
	}
	return contact, nil
}

// clearMainImage clears the main-image pointer and derived avatar on the
// contact inside tx and returns the avatar filename to remove after commit.
// The in-memory contact is restored if the save fails.
func clearMainImage(tx *gorm.DB, contact *models.Contact) (string, error) {
	prevMain, prevPhoto, prevThumb := contact.MainImageID, contact.Photo, contact.PhotoThumbnail
	contact.MainImageID = nil
	contact.Photo = ""
	contact.PhotoThumbnail = ""
	if err := tx.Save(contact).Error; err != nil {
		contact.MainImageID, contact.Photo, contact.PhotoThumbnail = prevMain, prevPhoto, prevThumb
		return "", err
	}
	return prevPhoto, nil
}

// ClearContactMainImage clears the main-image pointer and the derived avatar.
// A legacy photo with no main image is deliberately left untouched (ADR 0040
// decision 10).
func ClearContactMainImage(ctx context.Context, db *gorm.DB, userID uint, contact *models.Contact, photoDir string) (*models.Contact, error) {
	if contact.MainImageID == nil {
		return contact, nil
	}
	oldPhoto, err := clearMainImage(db.WithContext(ctx), contact)
	if err != nil {
		return nil, apperrors.ErrDatabase("Failed to clear the main image").WithError(err)
	}
	if oldPhoto != "" {
		_ = photostore.RemoveContactPhoto(photoDir, oldPhoto)
	}
	return contact, nil
}

// DeleteContactImageAndClearMain soft-deletes a gallery image and, when it is
// the contact's main image, clears the pointer and derived avatar in the same
// transaction (ADR 0040 decisions 4 and 8). Files are removed after commit.
func DeleteContactImageAndClearMain(ctx context.Context, db *gorm.DB, svc *ImageService[models.ContactImage, *models.ContactImage], userID uint, contact *models.Contact, imageID uint, photoDir string) error {
	var oldPhoto string
	err := svc.DeleteWithin(ctx, userID, OwnerRef{ID: contact.ID, UID: contact.VCardUID}, imageID, func(tx *gorm.DB) error {
		if contact.MainImageID == nil || *contact.MainImageID != imageID {
			return nil
		}
		var err error
		oldPhoto, err = clearMainImage(tx, contact)
		return err
	})
	if err != nil {
		return err
	}
	if oldPhoto != "" {
		_ = photostore.RemoveContactPhoto(photoDir, oldPhoto)
	}
	return nil
}

// PatchContactImage applies the PATCH allowlist and keeps the derived avatar
// in step when the patched image is the contact's main image: a crop change
// regenerates the avatar, and a sensitivity that is no longer normal clears it
// (ADR 0040 decisions 7 and 10). The row update commits first; a failed
// regeneration is reported and an identical retry converges.
func PatchContactImage(ctx context.Context, db *gorm.DB, svc *ImageService[models.ContactImage, *models.ContactImage], store imaging.Store, userID uint, contact *models.Contact, imageID uint, in models.ContactImagePatchInput, photoDir string) (*models.ContactImage, error) {
	img, err := svc.Update(ctx, userID, OwnerRef{ID: contact.ID, UID: contact.VCardUID}, imageID, in)
	if err != nil {
		return nil, err
	}
	if contact.MainImageID == nil || *contact.MainImageID != imageID {
		return img, nil
	}
	switch {
	case img.Sensitivity != models.ImageSensitivityNormal:
		if _, err := ClearContactMainImage(ctx, db, userID, contact, photoDir); err != nil {
			return nil, err
		}
	case in.Crop.Set:
		if _, err := SetContactMainImage(ctx, db, store, userID, contact, img, photoDir); err != nil {
			return nil, err
		}
	}
	return img, nil
}

// AdoptImageIntoContact copies a note's or activity's stored full variant into
// the contact gallery without re-decoding it (ADR 0040 decision 10). The
// source must belong to the same user and to this contact (the note is filed
// under it, or the activity is live and linked to it). Sensitivity is copied
// verbatim and never lowered. Adopt is an ingress like upload: a near-duplicate
// of a live gallery image returns that image (the bool reports it) before the
// cap applies, and a new copy counts against the cap, the storage quota and
// the free-disk preflight.
func AdoptImageIntoContact(ctx context.Context, svc *ImageService[models.ContactImage, *models.ContactImage], userID uint, contact *models.Contact, kind string, sourceID uint, imageID uint) (*models.ContactImage, bool, error) {
	db, store := svc.DB, svc.Store
	var src models.ImageFields
	switch kind {
	case "note":
		var note models.Note
		if err := db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, sourceID).First(&note).Error; err != nil {
			return nil, false, apperrors.ErrNotFound("Image")
		}
		if note.ContactID == nil || *note.ContactID != contact.ID {
			return nil, false, apperrors.ErrNotFound("Image")
		}
		var img models.NoteImage
		if err := db.WithContext(ctx).Where("user_id = ? AND note_id = ? AND id = ?", userID, note.ID, imageID).First(&img).Error; err != nil {
			return nil, false, apperrors.ErrNotFound("Image")
		}
		src = img.ImageFields
	case "activity":
		var linked int64
		if err := db.WithContext(ctx).Table("activity_contacts").
			Joins("JOIN activities ON activities.id = activity_contacts.activity_id AND activities.user_id = ? AND activities.deleted_at IS NULL", userID).
			Where("activity_contacts.activity_id = ? AND activity_contacts.contact_id = ?", sourceID, contact.ID).Count(&linked).Error; err != nil {
			return nil, false, apperrors.ErrDatabase("Failed to check the activity").WithError(err)
		}
		if linked == 0 {
			return nil, false, apperrors.ErrNotFound("Image")
		}
		var img models.ActivityImage
		if err := db.WithContext(ctx).Where("user_id = ? AND activity_id = ? AND id = ?", userID, sourceID, imageID).First(&img).Error; err != nil {
			return nil, false, apperrors.ErrNotFound("Image")
		}
		src = img.ImageFields
	default:
		return nil, false, apperrors.ErrValidation("source_kind must be note or activity").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "source_kind"})
	}

	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	if src.DHash != nil {
		dup, err := svc.findDuplicate(ctx, userID, owner, *src.DHash)
		if err != nil {
			return nil, false, err
		}
		if dup != nil {
			return dup, true, nil
		}
	}
	if err := svc.checkLimit(ctx, userID, owner); err != nil {
		return nil, false, err
	}

	full, err := store.Read(userID, src.StoredName)
	if err != nil {
		return nil, false, apperrors.ErrNotFound("Image file")
	}
	thumb, err := store.Read(userID, src.ThumbName)
	if err != nil {
		return nil, false, apperrors.ErrNotFound("Image file")
	}
	if err := svc.checkCapacity(userID, len(full)+len(thumb)); err != nil {
		return nil, false, err
	}
	newFull, newThumb, err := store.Save(userID, full, thumb, imageExtension(src.MIMEType), imageExtension(src.ThumbMIMEType))
	if err != nil {
		return nil, false, apperrors.ErrInternal("Failed to copy image").WithError(err)
	}

	// The copy gets a new identity and file names; everything else, including
	// sensitivity, carries over verbatim.
	fields := src
	fields.ID, fields.CreatedAt, fields.UpdatedAt, fields.DeletedAt = 0, time.Time{}, time.Time{}, gorm.DeletedAt{}
	fields.UUID = uuid.New().String()
	fields.CropX, fields.CropY, fields.CropW, fields.CropH = nil, nil, nil, nil
	fields.StoredName, fields.ThumbName = newFull, newThumb
	row := models.ContactImage{ImageFields: fields, ContactVCardUID: contact.VCardUID}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		_ = store.Remove(userID, newFull)
		_ = store.Remove(userID, newThumb)
		return nil, false, apperrors.ErrDatabase("Failed to adopt image").WithError(err)
	}
	models.RecordAuditEvent(db, models.AuditEntityContact, contact.VCardUID, models.AuditOpCreate, userID)
	return &row, false, nil
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
