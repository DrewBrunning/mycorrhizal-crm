package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type gormDB = gorm.DB

// gallerySpec is everything that differs between the three image owners
// (activity, note, contact); the upload/list/download/delete flow itself lives
// once in the gallery* helpers below (issue #2312). Route registration stays
// per owner.
type gallerySpec[T any, P interface {
	*T
	services.ImageRow
}, O any, D any] struct {
	// service builds the owner's image service from the request config/DB.
	service func(c *gin.Context) *services.ImageService[T, P]
	// load resolves the owner scoped to the caller, aborting with 404/500 itself.
	load func(c *gin.Context) (*O, bool)
	// ownerRef maps a loaded owner to the service's OwnerRef.
	ownerRef func(*O) services.OwnerRef
	// dto maps a row to its wire shape.
	dto func(img T, owner *O) D
	// dedupe makes upload go through UploadDeduped and report `deduplicated`.
	dedupe bool
	// remove overrides the plain soft delete (the contact gallery also
	// clears a main-image pointer).
	remove func(c *gin.Context, svc *services.ImageService[T, P], userID uint, owner *O, id uint) error
	// abort overrides how a delete error is rendered.
	abort func(c *gin.Context, err error)
}

// galleryImageID parses the :image_id path parameter.
func galleryImageID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("image_id"), 10, strconv.IntSize)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Invalid image ID").WithReason(apperrors.ReasonAttachmentInvalidID, nil))
		return 0, false
	}
	return uint(id), true
}

func serveImageUpload[T any, P interface {
	*T
	services.ImageRow
}, O any, D any](c *gin.Context, spec gallerySpec[T, P, O, D]) {
	owner, ok := spec.load(c)
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

	svc := spec.service(c)
	ref := spec.ownerRef(owner)
	if spec.dedupe {
		img, duplicated, err := svc.UploadDeduped(c.Request.Context(), userID, ref, file)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.GetAppError(err))
			return
		}
		status := http.StatusCreated
		if duplicated {
			status = http.StatusOK
		}
		c.JSON(status, gin.H{"image": spec.dto(*img, owner), "deduplicated": duplicated})
		return
	}
	img, err := svc.Upload(c.Request.Context(), userID, ref, file)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"image": spec.dto(*img, owner)})
}

func serveImageList[T any, P interface {
	*T
	services.ImageRow
}, O any, D any](c *gin.Context, spec gallerySpec[T, P, O, D]) {
	owner, ok := spec.load(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	rows, err := spec.service(c).List(c.Request.Context(), userID, spec.ownerRef(owner))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	dtos := make([]D, 0, len(rows))
	for _, img := range rows {
		dtos = append(dtos, spec.dto(img, owner))
	}
	c.JSON(http.StatusOK, gin.H{"images": dtos, "total": len(dtos)})
}

// serveImageDownload serves the full variant or the thumbnail. Ownership is
// checked before the file is opened or an ETag is honoured. The download
// security headers are set here and nowhere else.
func serveImageDownload[T any, P interface {
	*T
	services.ImageRow
}, O any, D any](c *gin.Context, spec gallerySpec[T, P, O, D], thumbnail bool) {
	owner, ok := spec.load(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := galleryImageID(c)
	if !ok {
		return
	}
	svc := spec.service(c)
	img, err := svc.Get(c.Request.Context(), userID, spec.ownerRef(owner), id)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.GetAppError(err))
		return
	}
	f := P(img).Fields()

	variant := "file"
	storedName := f.StoredName
	mimeType := f.MIMEType
	if thumbnail {
		variant = "thumb"
		storedName = f.ThumbName
		mimeType = f.ThumbMIMEType
	}
	etag := `"` + f.UUID + "-" + variant + `"`
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
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="image-%s.%s"`, f.UUID, imageExt(mimeType)))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Data(http.StatusOK, mimeType, data)
}

func serveImageDelete[T any, P interface {
	*T
	services.ImageRow
}, O any, D any](c *gin.Context, spec gallerySpec[T, P, O, D]) {
	owner, ok := spec.load(c)
	if !ok {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := galleryImageID(c)
	if !ok {
		return
	}
	svc := spec.service(c)
	var err error
	if spec.remove != nil {
		err = spec.remove(c, svc, userID, owner, id)
	} else {
		err = svc.Delete(c.Request.Context(), userID, spec.ownerRef(owner), id)
	}
	if err != nil {
		if spec.abort != nil {
			spec.abort(c, err)
		} else {
			apperrors.AbortWithError(c, apperrors.GetAppError(err))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Image deleted"})
}

func imageExt(mimeType string) string {
	if mimeType == "image/png" {
		return "png"
	}
	return "jpg"
}

// galleryService builds an owner's image service from the request config and
// DB; perOwnerLimit is the owner's IMAGE_MAX_PER_* value.
func galleryService[T any, P interface {
	*T
	services.ImageRow
}](c *gin.Context, build func(db *gormDB, store imaging.Store, limit, maxPixels int, maxBytes int64, quota services.UserQuota) *services.ImageService[T, P], perOwnerLimit func(config.Config) int) *services.ImageService[T, P] {
	cfg := currentConfig(c)
	return build(
		c.MustGet("db").(*gormDB),
		imaging.Store{Dir: cfg.ImagesDir},
		perOwnerLimit(cfg),
		cfg.ImageMaxPixels,
		int64(cfg.ImageMaxUploadMB)<<20,
		services.UserQuotaFromConfig(cfg),
	)
}

// ImageUploadBodyLimit is the per-request body cap for an image upload on any
// owner: the configured file cap plus multipart overhead.
func ImageUploadBodyLimit(cfg *config.Config) int64 {
	return int64(cfg.ImageMaxUploadMB)<<20 + 1<<20
}
