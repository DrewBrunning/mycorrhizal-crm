package controllers

import (
	"errors"
	"fmt"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ActivityImageDTO is the wire shape of one activity image. file_url and
// thumbnail_url are ready-made relative URLs so a client never assembles one
// from the owner id (ADR 0040 decision 8).
type ActivityImageDTO struct {
	ID           uint       `json:"id"`
	UUID         string     `json:"uuid"`
	ActivityID   uint       `json:"activity_id"`
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

func activityImageDTO(img models.ActivityImage) ActivityImageDTO {
	return ActivityImageDTO{
		ID:           img.ID,
		UUID:         img.UUID,
		ActivityID:   img.ActivityID,
		MIMEType:     img.MIMEType,
		Width:        img.Width,
		Height:       img.Height,
		SizeBytes:    img.SizeBytes,
		Sensitivity:  img.Sensitivity,
		TakenAt:      img.TakenAt,
		TakenDate:    img.TakenDate,
		CreatedAt:    img.CreatedAt,
		FileURL:      fmt.Sprintf("/api/v1/activities/%d/images/%d/file", img.ActivityID, img.ID),
		ThumbnailURL: fmt.Sprintf("/api/v1/activities/%d/images/%d/thumbnail", img.ActivityID, img.ID),
	}
}

// activityImageService builds the activity-image service from the request's
// config and DB.
func activityImageService(c *gin.Context) *services.ImageService[models.ActivityImage, *models.ActivityImage] {
	return galleryService(c, services.ActivityImageService, func(cfg config.Config) int { return cfg.ImageMaxPerActivity })
}

// loadOwnedActivity resolves an activity by id, scoped to the caller. Every
// image route uses this first so a foreign activity is a 404 before any image
// row is touched.
func loadOwnedActivity(c *gin.Context) (*models.Activity, bool) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return nil, false
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return nil, false
	}
	var activity models.Activity
	if err := db.Where("user_id = ?", userID).First(&activity, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Activity").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve activity").WithError(err))
		}
		return nil, false
	}
	return &activity, true
}

func activityOwnerRef(activity *models.Activity) services.OwnerRef {
	return services.OwnerRef{ID: activity.ID, UID: activity.UUID}
}

var activityGallery = gallerySpec[models.ActivityImage, *models.ActivityImage, models.Activity, ActivityImageDTO]{
	service:  activityImageService,
	load:     loadOwnedActivity,
	ownerRef: activityOwnerRef,
	dto:      func(img models.ActivityImage, _ *models.Activity) ActivityImageDTO { return activityImageDTO(img) },
}

// UploadActivityImage stores one image against an activity.
func UploadActivityImage(c *gin.Context) { serveImageUpload(c, activityGallery) }

// ListActivityImages returns an activity's live images, newest first.
func ListActivityImages(c *gin.Context) { serveImageList(c, activityGallery) }

// DownloadActivityImage serves the full variant or the thumbnail.
func DownloadActivityImage(c *gin.Context, thumbnail bool) {
	serveImageDownload(c, activityGallery, thumbnail)
}

// DeleteActivityImage soft-deletes one image and removes its files after the
// commit.
func DeleteActivityImage(c *gin.Context) { serveImageDelete(c, activityGallery) }

// PatchActivityImage updates an activity image's sensitivity or crop.
func PatchActivityImage(c *gin.Context) { serveImagePatch(c, activityGallery) }
