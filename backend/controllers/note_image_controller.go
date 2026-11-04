package controllers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NoteImageDTO is the wire shape of one note image. file_url and
// thumbnail_url are ready-made relative URLs so a client never assembles one
// from the owner id (ADR 0040 decision 8).
type NoteImageDTO struct {
	ID           uint       `json:"id"`
	UUID         string     `json:"uuid"`
	NoteID       uint       `json:"note_id"`
	MIMEType     string     `json:"mime_type"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	SizeBytes    int64      `json:"size_bytes"`
	Sensitivity  string     `json:"sensitivity"`
	TakenAt      *time.Time `json:"taken_at"`
	TakenDate    *string    `json:"taken_date"`
	CreatedAt    time.Time  `json:"created_at"`
	FileURL      string     `json:"file_url"`
	ThumbnailURL string     `json:"thumbnail_url"`
}

func noteImageDTO(img models.NoteImage) NoteImageDTO {
	return NoteImageDTO{
		ID:           img.ID,
		UUID:         img.UUID,
		NoteID:       img.NoteID,
		MIMEType:     img.MIMEType,
		Width:        img.Width,
		Height:       img.Height,
		SizeBytes:    img.SizeBytes,
		Sensitivity:  img.Sensitivity,
		TakenAt:      img.TakenAt,
		TakenDate:    img.TakenDate,
		CreatedAt:    img.CreatedAt,
		FileURL:      fmt.Sprintf("/api/v1/notes/%d/images/%d/file", img.NoteID, img.ID),
		ThumbnailURL: fmt.Sprintf("/api/v1/notes/%d/images/%d/thumbnail", img.NoteID, img.ID),
	}
}

// noteImageService builds the note-image service from the request's config
// and DB.
func noteImageService(c *gin.Context) *services.ImageService[models.NoteImage, *models.NoteImage] {
	return galleryService(c, services.NoteImageService, func(cfg config.Config) int { return cfg.ImageMaxPerNote })
}

// loadOwnedNote resolves a note by id, scoped to the caller. Every
// image route uses this first so a foreign activity is a 404 before any image
// row is touched.
func loadOwnedNote(c *gin.Context) (*models.Note, bool) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return nil, false
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return nil, false
	}
	var activity models.Note
	if err := db.Where("user_id = ?", userID).First(&activity, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Note").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve activity").WithError(err))
		}
		return nil, false
	}
	return &activity, true
}

func noteOwnerRef(activity *models.Note) services.OwnerRef {
	return services.OwnerRef{ID: activity.ID, UID: activity.UUID}
}

var noteGallery = gallerySpec[models.NoteImage, *models.NoteImage, models.Note, NoteImageDTO]{
	service:  noteImageService,
	load:     loadOwnedNote,
	ownerRef: noteOwnerRef,
	dto:      func(img models.NoteImage, _ *models.Note) NoteImageDTO { return noteImageDTO(img) },
	// Deleting the last image of an empty-content note would leave an invalid
	// note (ADR 0040 decision 11); the count runs in the delete's transaction.
	remove: func(c *gin.Context, svc *services.ImageService[models.NoteImage, *models.NoteImage], userID uint, note *models.Note, id uint) error {
		return svc.DeleteWithin(c.Request.Context(), userID, noteOwnerRef(note), id, func(tx *gorm.DB) error {
			if strings.TrimSpace(note.Content) != "" {
				return nil
			}
			var live int64
			if err := tx.Model(&models.NoteImage{}).Where("note_id = ? AND user_id = ?", note.ID, userID).Count(&live).Error; err != nil {
				return err
			}
			if live == 0 {
				return errNoteRequiresContentOrImage(true)
			}
			return nil
		})
	},
}

// UploadNoteImage stores one image against a note.
func UploadNoteImage(c *gin.Context) { serveImageUpload(c, noteGallery) }

// ListNoteImages returns a note's live images, newest first.
func ListNoteImages(c *gin.Context) { serveImageList(c, noteGallery) }

// DownloadNoteImage serves the full variant or the thumbnail.
func DownloadNoteImage(c *gin.Context, thumbnail bool) { serveImageDownload(c, noteGallery, thumbnail) }

// DeleteNoteImage soft-deletes one image and removes its files after the
// commit.
func DeleteNoteImage(c *gin.Context) { serveImageDelete(c, noteGallery) }

// PatchNoteImage updates a note image's sensitivity or crop.
func PatchNoteImage(c *gin.Context) { serveImagePatch(c, noteGallery) }

// NoteImageUploadBodyLimit exports the per-request upload cap for routes.
func NoteImageUploadBodyLimit(cfg *config.Config) int64 { return ImageUploadBodyLimit(cfg) }

// errNoteRequiresContentOrImage is the typed refusal for a note that would be
// left with neither text nor a live image. A refused delete is a conflict with
// the note's current state; a refused write is an unprocessable body.
func errNoteRequiresContentOrImage(conflict bool) *apperrors.AppError {
	const msg = "a note needs text or at least one image"
	if conflict {
		return apperrors.ErrConflict(msg).WithReason(apperrors.ReasonNoteRequiresContentOrImage, nil)
	}
	return apperrors.ErrBusinessLogic(msg).WithReason(apperrors.ReasonNoteRequiresContentOrImage, nil)
}
