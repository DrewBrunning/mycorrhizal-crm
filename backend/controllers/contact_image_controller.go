package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ContactImageDTO is the wire shape of one contact image. file_url and
// thumbnail_url are ready-made relative URLs so a client never assembles one
// from the owner id (ADR 0040 decision 8).
type ContactImageDTO struct {
	ID              uint       `json:"id"`
	UUID            string     `json:"uuid"`
	ContactID       uint       `json:"contact_id"`
	ContactVCardUID string     `json:"contact_vcard_uid"`
	MIMEType        string     `json:"mime_type"`
	Width           int        `json:"width"`
	Height          int        `json:"height"`
	SizeBytes       int64      `json:"size_bytes"`
	Sensitivity     string     `json:"sensitivity"`
	TakenAt         *time.Time `json:"taken_at"`
	TakenDate       *string    `json:"taken_date"`
	CreatedAt       time.Time  `json:"created_at"`
	FileURL         string     `json:"file_url"`
	ThumbnailURL    string     `json:"thumbnail_url"`
	// Crop is the stored crop rectangle (fractions of the full variant), or
	// null. Always present so a client can open its editor at the stored
	// crop (issue #2004).
	Crop *models.ImageCrop `json:"crop"`
}

func contactImageDTO(img models.ContactImage, contactID uint) ContactImageDTO {
	return ContactImageDTO{
		ID:              img.ID,
		UUID:            img.UUID,
		ContactID:       contactID,
		ContactVCardUID: img.ContactVCardUID,
		MIMEType:        img.MIMEType,
		Width:           img.Width,
		Height:          img.Height,
		SizeBytes:       img.SizeBytes,
		Sensitivity:     img.Sensitivity,
		TakenAt:         img.TakenAt,
		TakenDate:       img.TakenDate,
		CreatedAt:       img.CreatedAt,
		FileURL:         fmt.Sprintf("/api/v1/contacts/%d/images/%d/file", contactID, img.ID),
		ThumbnailURL:    fmt.Sprintf("/api/v1/contacts/%d/images/%d/thumbnail", contactID, img.ID),
		Crop:            img.StoredCrop(),
	}
}

// contactImageService builds the contact-image service from the request's
// config and DB.
func contactImageService(c *gin.Context) *services.ImageService[models.ContactImage, *models.ContactImage] {
	return galleryService(c, services.ContactImageService, func(cfg config.Config) int { return cfg.ImageMaxPerContact })
}

// loadOwnedContact resolves a contact by id, scoped to the caller. Every
// image route uses this first so a foreign contact is a 404 before any image
// row is touched.
func loadOwnedContact(c *gin.Context) (*models.Contact, bool) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return nil, false
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return nil, false
	}
	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return nil, false
	}
	return &contact, true
}

// abortContactImageError maps a service error to a response. A concurrent
// contact write surfaces as a revision conflict (412/404), exactly like the
// other contact writers, instead of a generic 500.
func abortContactImageError(c *gin.Context, err error) {
	if handleRevisionConflict(c, "Contact", err) {
		return
	}
	apperrors.AbortWithError(c, apperrors.GetAppError(err))
}

func contactOwnerRef(contact *models.Contact) services.OwnerRef {
	return services.OwnerRef{ID: contact.ID, UID: contact.VCardUID}
}

var contactGallery = gallerySpec[models.ContactImage, *models.ContactImage, models.Contact, ContactImageDTO]{
	service:  contactImageService,
	load:     loadOwnedContact,
	ownerRef: contactOwnerRef,
	dto: func(img models.ContactImage, contact *models.Contact) ContactImageDTO {
		return contactImageDTO(img, contact.ID)
	},
	dedupe: true,
	remove: func(c *gin.Context, svc *services.ImageService[models.ContactImage, *models.ContactImage], userID uint, contact *models.Contact, id uint) error {
		db := c.MustGet("db").(*gorm.DB)
		return services.DeleteContactImageAndClearMain(c.Request.Context(), db, svc, userID, contact, id, currentConfig(c).ProfilePhotoDir)
	},
	abort: abortContactImageError,
}

// UploadContactImage stores one image against a contact.
func UploadContactImage(c *gin.Context) { serveImageUpload(c, contactGallery) }

// ListContactImages returns a contact's live images, newest first.
func ListContactImages(c *gin.Context) { serveImageList(c, contactGallery) }

// DownloadContactImage serves the full variant or the thumbnail.
func DownloadContactImage(c *gin.Context, thumbnail bool) {
	serveImageDownload(c, contactGallery, thumbnail)
}

// DeleteContactImage soft-deletes one image and removes its files after the
// commit.
func DeleteContactImage(c *gin.Context) { serveImageDelete(c, contactGallery) }

// ContactPhotoDTO is one aggregated Photos-tab item.
// PhotoSource names where a Photos-tab image lives (ADR 0040 decision 8):
// the contact itself, one of its notes, or one of its activities.
type PhotoSource struct {
	Kind  string `json:"kind"`
	ID    uint   `json:"id"`
	Label string `json:"label"`
}

type ContactPhotoDTO struct {
	ID           uint        `json:"id"`
	Source       PhotoSource `json:"source"`
	MIMEType     string      `json:"mime_type"`
	Width        int         `json:"width"`
	Height       int         `json:"height"`
	SizeBytes    int64       `json:"size_bytes"`
	Sensitivity  string      `json:"sensitivity"`
	TakenAt      *time.Time  `json:"taken_at"`
	TakenDate    *string     `json:"taken_date"`
	CreatedAt    time.Time   `json:"created_at"`
	FileURL      string      `json:"file_url"`
	ThumbnailURL string      `json:"thumbnail_url"`
	// Crop is set for contact-gallery images only (null otherwise).
	Crop *models.ImageCrop `json:"crop"`
}

func contactPhotoDTO(photo services.ContactPhoto, contactID uint) ContactPhotoDTO {
	var base string
	switch photo.Kind {
	case "note":
		base = fmt.Sprintf("/api/v1/notes/%d/images/%d", photo.SourceID, photo.ImageID)
	case "activity":
		base = fmt.Sprintf("/api/v1/activities/%d/images/%d", photo.SourceID, photo.ImageID)
	default:
		base = fmt.Sprintf("/api/v1/contacts/%d/images/%d", contactID, photo.ImageID)
	}
	return ContactPhotoDTO{
		ID:           photo.ImageID,
		Source:       PhotoSource{Kind: photo.Kind, ID: photo.SourceID, Label: photo.SourceLabel},
		MIMEType:     photo.MIMEType,
		Width:        photo.Width,
		Height:       photo.Height,
		SizeBytes:    photo.SizeBytes,
		Sensitivity:  photo.Sensitivity,
		TakenAt:      photo.TakenAt,
		TakenDate:    photo.TakenDate,
		CreatedAt:    photo.CreatedAt,
		FileURL:      base + "/file",
		ThumbnailURL: base + "/thumbnail",
		Crop:         photo.Crop,
	}
}

// PatchContactImage updates the image's sensitivity and/or crop (ADR 0040
// decision 8). When the image is the contact's main image, a crop change
// regenerates the avatar and a non-normal sensitivity clears it, which bumps
// the contact revision.
func PatchContactImage(c *gin.Context) {
	contact, ok := loadOwnedContact(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("image_id"), 10, strconv.IntSize)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Invalid image ID").WithReason(apperrors.ReasonAttachmentInvalidID, nil))
		return
	}
	var input models.ContactImagePatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("body", "invalid request body").WithReason(apperrors.ReasonValidationInvalidRequestBody, nil))
		return
	}
	svc := contactImageService(c)
	db := c.MustGet("db").(*gorm.DB)
	img, err := services.PatchContactImage(c.Request.Context(), db, svc, svc.Store, userID, contact, uint(id), input, currentConfig(c).ProfilePhotoDir)
	if err != nil {
		abortContactImageError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"image": contactImageDTO(*img, contact.ID)})
}

// PutContactMainImage sets the contact's main image and derives its avatar.
func PutContactMainImage(c *gin.Context) {
	contact, ok := loadOwnedContact(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if !checkIfMatch(c, contact.Revision) {
		return
	}
	var input struct {
		ImageID uint `json:"image_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.ImageID == 0 {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("image_id", "required").WithReason(apperrors.ReasonValidationMissingField, map[string]any{"field": "image_id"}))
		return
	}
	cfg := currentConfig(c)
	svc := contactImageService(c)
	img, err := svc.Get(c.Request.Context(), userID, contactOwnerRef(contact), input.ImageID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	if _, err := services.SetContactMainImage(c.Request.Context(), db, svc.Store, userID, contact, img, cfg.ProfilePhotoDir); err != nil {
		abortContactImageError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Main image updated", "main_image_id": input.ImageID, "revision": contact.Revision})
}

// DeleteContactMainImage clears the pointer and the derived avatar.
func DeleteContactMainImage(c *gin.Context) {
	contact, ok := loadOwnedContact(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if !checkIfMatch(c, contact.Revision) {
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	if _, err := services.ClearContactMainImage(c.Request.Context(), db, userID, contact, currentConfig(c).ProfilePhotoDir); err != nil {
		abortContactImageError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Main image cleared", "revision": contact.Revision})
}

// AdoptContactImage copies a note/activity image into the contact gallery.
func AdoptContactImage(c *gin.Context) {
	contact, ok := loadOwnedContact(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var input struct {
		SourceKind string `json:"source_kind" binding:"required"`
		SourceID   uint   `json:"source_id" binding:"required"`
		ImageID    uint   `json:"image_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("body", "invalid request body").WithReason(apperrors.ReasonValidationInvalidRequestBody, nil))
		return
	}
	svc := contactImageService(c)
	img, duplicated, err := services.AdoptImageIntoContact(c.Request.Context(), svc, userID, contact, input.SourceKind, input.SourceID, input.ImageID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	status := http.StatusCreated
	if duplicated {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"image": contactImageDTO(*img, contact.ID), "deduplicated": duplicated})
}

// GetContactPhotos returns the aggregated Photos tab (own + notes + activities).
func GetContactPhotos(c *gin.Context) {
	contact, ok := loadOwnedContact(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	photos, err := services.ListContactPhotos(c.Request.Context(), db, userID, contact, 200)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	dtos := make([]ContactPhotoDTO, 0, len(photos))
	for _, photo := range photos {
		dtos = append(dtos, contactPhotoDTO(photo, contact.ID))
	}
	c.JSON(http.StatusOK, gin.H{"photos": dtos, "total": len(dtos)})
}

// ContactImageUploadBodyLimit exports the per-request upload cap for routes.
func ContactImageUploadBodyLimit(cfg *config.Config) int64 { return ImageUploadBodyLimit(cfg) }
