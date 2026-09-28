package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ExportAccountBundle serves GET /api/v1/export/account (issue #1259, ADR 0028
// Decision 3): the full-fidelity, re-importable, versioned JSON account
// bundle. It is the user's own data going to the user's own destination, so —
// like the flat CSV export (issue #861) — it includes every sensitivity level
// and `status: suggested` rows with no include_sensitive opt-in.
//
// The response carries the version and the photo counts in headers so a client
// can size/verify the transfer without parsing the body first; the body is the
// services.BuildAccountBundle document.
func ExportAccountBundle(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// Issue #498: reuse the contact-count ceiling. The bundle is bounded by
	// the same contact count (every other section hangs off a contact), and a
	// pathological instance should be refused rather than buffered whole.
	if !guardExportContactCount(c, db, userID, exportOpAccount) {
		return
	}

	photoDir := currentConfig(c).ProfilePhotoDir
	bundle, stats, err := services.BuildAccountBundle(db, userID, photoDir)
	if err != nil {
		abortExportError(c, log, exportOpAccount, exportCatDatabase, "Failed to build account bundle", err)
		return
	}

	payload, err := json.Marshal(bundle)
	if err != nil {
		abortExportError(c, log, exportOpAccount, exportCatSerialization, "Failed to marshal account bundle", err)
		return
	}

	filename := fmt.Sprintf("mycorrhizal-account-%s.json", time.Now().Format("2006-01-02"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Length", strconv.Itoa(len(payload)))
	c.Header("X-Mycorrhizal-Bundle-Version", strconv.Itoa(bundle.Version))
	c.Header("X-Mycorrhizal-Bundle-Contacts", strconv.Itoa(stats.Contacts))
	c.Header("X-Mycorrhizal-Bundle-Photos", strconv.Itoa(stats.PhotosEmbedded))
	c.Header("X-Mycorrhizal-Bundle-Photos-Omitted", strconv.Itoa(stats.PhotosOmitted))

	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)

	log.Info().
		Int("contacts", stats.Contacts).
		Int("photos", stats.PhotosEmbedded).
		Int("photos_omitted", stats.PhotosOmitted).
		Msg("Account bundle export completed successfully")
}

// exportOpAccount is the structured-failure operation token for the account
// bundle (mirroring exportOpCSV etc.).
const exportOpAccount = "export:account"
