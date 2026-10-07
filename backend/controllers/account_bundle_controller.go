package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
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

	// Issue #1313: refuse up front rather than hand back a bundle the
	// destination's import (MaxMycorrhizalBundleSize) is guaranteed to reject —
	// the attach wizard and local-profile restore would otherwise dead-end at
	// the Upload step with no remedy.
	if int64(len(payload)) > accountBundleMaxBytes {
		log.Warn().
			Int("bytes", len(payload)).
			Int64("limit", accountBundleMaxBytes).
			Str("operation", exportOpAccount).
			Msg("Account bundle exceeds the import size limit; export refused")
		apperrors.AbortWithError(c, apperrors.ErrInsufficientStorage(fmt.Sprintf(
			"The account bundle is %.1f MiB, which is over the %d MiB limit the import accepts. "+
				"Remove contacts or profile photos you no longer need and try again, or use the CSV/vCard export instead.",
			float64(len(payload))/(1<<20), accountBundleMaxBytes>>20,
		)).WithDetails("operation", exportOpAccount).
			WithDetails("category", exportCatValidation).
			WithDetails("bundle_bytes", len(payload)).
			WithDetails("limit_bytes", accountBundleMaxBytes))
		return
	}

	filename := fmt.Sprintf("mycorrhizal-account-%s.json", clock.FromContext(c).Now().Format("2006-01-02"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Length", strconv.Itoa(len(payload)))
	c.Header("X-Mycorrhizal-Bundle-Max-Bytes", strconv.FormatInt(accountBundleMaxBytes, 10))
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

// accountBundleMaxBytes is the mutable seam over services.MaxMycorrhizalBundleSize
// (issue #1313): the export refuses a payload the import would refuse. Tests
// lower it to prove the boundary without building a 64 MiB bundle. Keep equal
// to the constant in production.
var accountBundleMaxBytes int64 = services.MaxMycorrhizalBundleSize
