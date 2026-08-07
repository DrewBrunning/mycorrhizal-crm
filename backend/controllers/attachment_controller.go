package controllers

import (
	"bytes"
	"errors"
	"io"
	"mycorrhizal/attachments"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// maxAttachmentSize caps a single uploaded attachment (N7's per-file size cap).
const maxAttachmentSize = 25 * 1024 * 1024 // 25MB

// inlineContentTypes are the attachment types served with
// Content-Disposition: inline (rendered in the browser) rather than forced
// download. Deliberately passive formats only — anything scriptable
// (SVG, HTML) stays download-only regardless, because serving it from this
// API's origin is an XSS vector (the photo/image-proxy precedent).
var inlineContentTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
}

// isRenderableVulnerableType reports whether an upload's content type is
// scriptable in a browser origin (SVG/HTML). Such uploads are rejected
// outright at upload time; the serve side keeps them download-only as defense
// in depth.
func isRenderableVulnerableType(contentType string) bool {
	lower := strings.ToLower(contentType)
	return strings.Contains(lower, "svg") ||
		strings.Contains(lower, "html") ||
		strings.HasPrefix(lower, "text/xml") ||
		strings.HasPrefix(lower, "application/xml")
}

// hasMarkupSignature detects markup that http.DetectContentType misses:
// an SVG or HTML document whose leading bytes don't match any sniffed
// signature (e.g. "<svg ...>", which DetectContentType classifies as
// text/plain). These are the in-origin XSS vectors the ticket calls out.
func hasMarkupSignature(data []byte) bool {
	trimmed := bytes.TrimLeft(data, " \t\r\n\xef\xbb\xbf")
	lower := strings.ToLower(string(trimmed))
	for _, prefix := range []string{"<svg", "<html", "<!doctype", "<script", "<style"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// UploadAttachment stores an uploaded file against a contact (N7). The file is
// written under a server-generated UUID name; the original name is display-only
// and never reaches a filesystem path.
func UploadAttachment(c *gin.Context, cfg *config.Config) {
	idParam := c.Param("id")
	contactID, err := strconv.Atoi(idParam)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Invalid contact ID"))
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, contactID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", idParam))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "No file uploaded"))
		return
	}
	if fileHeader.Size > maxAttachmentSize {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "File too large. Maximum size is 25MB"))
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to read uploaded file").WithError(err))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAttachmentSize+1))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to read uploaded file").WithError(err))
		return
	}
	if len(data) > maxAttachmentSize {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "File too large. Maximum size is 25MB"))
		return
	}

	// Sniff the real content, not the client-declared type: the browser's
	// multipart part may be mislabeled, and the serve-side XSS risk depends on
	// what the bytes actually are.
	contentType := http.DetectContentType(data)
	if isRenderableVulnerableType(contentType) || hasMarkupSignature(data) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("file", "SVG and HTML files are not supported"))
		return
	}

	storedName, err := attachments.Save(data, cfg.AttachmentsDir)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to store attachment").WithError(err))
		return
	}

	attachment := models.Attachment{
		UserID:          userID,
		ContactVCardUID: contact.VCardUID,
		StoredName:      storedName,
		OriginalName:    filepath.Base(fileHeader.Filename),
		ContentType:     contentType,
		SizeBytes:       int64(len(data)),
	}
	if err := db.Create(&attachment).Error; err != nil {
		// Roll back the on-disk write; the record failed.
		_ = attachments.Remove(cfg.AttachmentsDir, storedName)
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save attachment").WithError(err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"attachment": attachment})
}

// ListContactAttachments returns a contact's non-deleted attachments, newest
// first.
func ListContactAttachments(c *gin.Context) {
	idParam := c.Param("id")
	contactID, err := strconv.Atoi(idParam)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Invalid contact ID"))
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, contactID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", idParam))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	var list []models.Attachment
	if err := db.Where("contact_vcard_uid = ? AND user_id = ?", contact.VCardUID, userID).
		Order("created_at desc").Find(&list).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve attachments").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"attachments": list, "total": len(list)})
}

// loadOwnedAttachment loads an attachment scoped to the caller, or aborts.
func loadOwnedAttachment(c *gin.Context, idStr string) (*models.Attachment, bool) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return nil, false
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Invalid attachment ID"))
		return nil, false
	}
	var attachment models.Attachment
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Attachment").WithDetails("id", idStr))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve attachment").WithError(err))
		}
		return nil, false
	}
	return &attachment, true
}

// DownloadAttachment serves an attachment's bytes. Default disposition is
// attachment (download); only the inlineContentTypes allow-list is rendered
// inline. SVG/HTML are never inline. X-Content-Type-Options: nosniff keeps a
// browser from sniffing a stored file into a renderable document.
func DownloadAttachment(c *gin.Context, cfg *config.Config) {
	attachment, ok := loadOwnedAttachment(c, c.Param("id"))
	if !ok {
		return
	}

	path, err := attachments.StoredPath(cfg.AttachmentsDir, attachment.StoredName)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Invalid attachment storage path").WithError(err))
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Attachment file"))
		return
	}

	disposition := "attachment"
	if inlineContentTypes[attachment.ContentType] && !isRenderableVulnerableType(attachment.ContentType) {
		disposition = "inline"
	}
	// RFC 6266: a non-ASCII filename needs filename*; ASCII base is enough for
	// the common case and never breaks the header. Escape quotes.
	displayName := strings.ReplaceAll(attachment.OriginalName, `"`, `'`)
	c.Header("Content-Disposition", disposition+`; filename="`+displayName+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(path)
}

// DeleteAttachment soft-deletes an attachment record and removes its file from
// disk. The file is removed at delete time (N7: an attachment's content IS the
// file; a tombstone that outlives a vanished file is acceptable for the change
// feed, an orphaned file is a leak).
func DeleteAttachment(c *gin.Context, cfg *config.Config) {
	attachment, ok := loadOwnedAttachment(c, c.Param("id"))
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)

	if err := db.Delete(attachment).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete attachment").WithError(err))
		return
	}
	if err := attachments.Remove(cfg.AttachmentsDir, attachment.StoredName); err != nil {
		// The record is gone; a failed file removal must not 500 a successful
		// delete, but it is a real leak — log it.
		logger.FromContext(c).Error().Err(err).Uint("attachment_id", attachment.ID).Str("stored_name", attachment.StoredName).
			Msg("Failed to remove attachment file from disk")
	}

	c.JSON(http.StatusOK, gin.H{"message": "Attachment deleted"})
}
