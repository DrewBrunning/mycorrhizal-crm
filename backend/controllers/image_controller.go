package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
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
	cfg := currentConfig(c)
	db := c.MustGet("db").(*gorm.DB)
	return services.ActivityImageService(
		db,
		imaging.Store{Dir: cfg.ImagesDir},
		cfg.ImageMaxPerActivity,
		cfg.ImageMaxPixels,
		int64(cfg.ImageMaxUploadMB)<<20,
		services.UserQuotaFromConfig(cfg),
	)
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

// UploadActivityImage stores one image against an activity.
func UploadActivityImage(c *gin.Context) {
	activity, ok := loadOwnedActivity(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "No file uploaded").WithReason(apperrors.ReasonAttachmentFileMissing, nil))
		return
	}
	if maxBytes := int64(currentConfig(c).ImageMaxUploadMB) << 20; maxBytes > 0 && fileHeader.Size > maxBytes {
		apperrors.AbortWithError(c, apperrors.ErrPayloadTooLarge("image too large").WithReason(apperrors.ReasonImageTooLarge, map[string]any{"max_mb": currentConfig(c).ImageMaxUploadMB}))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to read uploaded file").WithError(err))
		return
	}
	defer file.Close()

	svc := activityImageService(c)
	img, err := svc.Upload(c.Request.Context(), userID, activityOwnerRef(activity), file)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"image": activityImageDTO(*img)})
}

// ListActivityImages returns an activity's live images, newest first.
func ListActivityImages(c *gin.Context) {
	activity, ok := loadOwnedActivity(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	svc := activityImageService(c)
	rows, err := svc.List(c.Request.Context(), userID, activityOwnerRef(activity))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	dtos := make([]ActivityImageDTO, 0, len(rows))
	for _, img := range rows {
		dtos = append(dtos, activityImageDTO(img))
	}
	c.JSON(http.StatusOK, gin.H{"images": dtos, "total": len(dtos)})
}

// DownloadActivityImage serves the full variant or the thumbnail. Ownership is
// checked before the file is opened or an ETag is honoured.
func DownloadActivityImage(c *gin.Context, thumbnail bool) {
	activity, ok := loadOwnedActivity(c)
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
	svc := activityImageService(c)
	img, err := svc.Get(c.Request.Context(), userID, activityOwnerRef(activity), uint(id))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}

	variant := "file"
	storedName := img.StoredName
	mimeType := img.MIMEType
	if thumbnail {
		variant = "thumb"
		storedName = img.ThumbName
		mimeType = img.ThumbMIMEType
	}
	etag := `"` + img.UUID + "-" + variant + `"`
	if match := c.GetHeader("If-None-Match"); match != "" && match == etag {
		c.Status(http.StatusNotModified)
		return
	}

	data, err := svc.Store.Read(userID, storedName)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Image file"))
		return
	}

	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, no-cache")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="image-%s.%s"`, img.UUID, imageExt(mimeType)))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Data(http.StatusOK, mimeType, data)
}

func imageExt(mimeType string) string {
	if mimeType == "image/png" {
		return "png"
	}
	return "jpg"
}

// DeleteActivityImage soft-deletes one image and removes its files after the
// commit.
func DeleteActivityImage(c *gin.Context) {
	activity, ok := loadOwnedActivity(c)
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
	svc := activityImageService(c)
	if err := svc.Delete(c.Request.Context(), userID, activityOwnerRef(activity), uint(id)); err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Image deleted"})
}

// imageUploadBodyLimit is the per-request body cap for an image upload: the
// configured file cap plus multipart overhead.
// ImageUploadBodyLimit exports the per-request upload cap for routes.
func ImageUploadBodyLimit(cfg *config.Config) int64 {
	return imageUploadBodyLimit(cfg)
}

func imageUploadBodyLimit(cfg *config.Config) int64 {
	return int64(cfg.ImageMaxUploadMB)<<20 + 1<<20
}
