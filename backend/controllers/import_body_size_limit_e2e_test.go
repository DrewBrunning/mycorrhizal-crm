package controllers

// Issue #416: real-pipeline coverage for the import body-size-limit fix.
//
// middleware.DefaultBodySizeLimitMiddleware (10MB) used to run engine-wide
// in front of every route, including the VCF/JSContact import uploads —
// which meant services.MaxVCFSize (50MB) and services.MaxCSVSize (20MB)
// were dead code: nothing could ever reach a size between 10MB and their
// declared limits, since anything over 10MB was already rejected before the
// handler's own check ran. middleware/body_limit_test.go pins the
// middleware's exemption logic in isolation; this file drives the real
// route registration — routes.go's exact two-layer middleware stack (the
// engine-wide default, which exempts this path, plus the route-specific
// override) — against the real VCF upload handler, so a regression in
// either layer shows up here even if the other layer's own test stays
// green.

import (
	"net/http"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tinyVCard3Block is a minimal, valid vCard 3.0 block. Repeating it is used
// below to build a file that is large on the wire (bytes matter for the
// body-size middleware) but trivial to parse (so the test's outcome is
// governed by services.MaxVCFContacts, not by parser quirks).
const tinyVCard3Block = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:A B\r\nN:B;A;;;\r\nEND:VCARD\r\n"

// vcfUploadRoutePath must be registered and requested at this exact path —
// middleware.DefaultBodySizeLimitMiddleware's exemption allowlist is keyed
// on the full "/api/v1/..." path routes.go actually registers. Any other
// path (e.g. omitting the "/api/v1" prefix, as several other controller
// tests in this package do for brevity) would silently fall through to the
// strict 10MB default and defeat the point of this test.
const vcfUploadRoutePath = "/api/v1/contacts/import/vcf/upload"

func TestUploadVCFForImport_RealMiddlewareStack_OverDefaultLimitReachesHandler(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "vcfbodylimit", Password: "password123!A", Email: "vcfbodylimit@example.com"}
	require.NoError(t, db.Create(&user).Error)
	cfg := &config.Config{}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.MaxMultipartMemory = 10 << 20
	// The exact production stack (routes.go): the engine-wide default first,
	// then the route-specific override on this one path.
	router.Use(middleware.DefaultBodySizeLimitMiddleware())
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST(vcfUploadRoutePath, middleware.BodySizeLimitMiddleware(services.MaxVCFSize), func(c *gin.Context) {
		UploadVCFForImport(c, cfg)
	})

	// Repeat the block enough times to both comfortably exceed the old 10MB
	// default AND exceed services.MaxVCFContacts, so ParseVCF has a single,
	// deterministic reason to reject the file (the row-count cap) rather
	// than depending on parse-content specifics.
	repeats := (11 << 20) / len(tinyVCard3Block)
	if repeats <= services.MaxVCFContacts {
		repeats = services.MaxVCFContacts + 1000
	}
	vcf := strings.Repeat(tinyVCard3Block, repeats)
	require.Greater(t, len(vcf), 10<<20, "sanity: the payload must exceed the old 10MB default")
	require.Less(t, int64(len(vcf)), int64(services.MaxVCFSize), "sanity: the payload must stay under the route's own 50MB cap")

	rec := postMultipartFile(t, router, vcfUploadRoutePath, "file", "big.vcf", "text/vcard", []byte(vcf))

	// Before the fix, the engine-wide 10MB default would abort the multipart
	// read partway through, before UploadVCFForImport's own logic ever ran.
	assert.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code,
		"an %d-byte VCF upload must not be capped by the engine-wide 10MB default: %s", len(vcf), rec.Body.String())
	// The handler was reached and ran its own logic: ParseVCF's row-count
	// cap rejects a file this size with a 400, not a crash or a silent
	// truncation -- proving the request made it past the body-size layer
	// intact rather than being silently truncated mid-multipart-part.
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "too many contacts")
}

// TestUploadVCFForImport_RealMiddlewareStack_OrdinaryUploadStillSucceeds is
// the control case for the test above: an ordinary small VCF file through
// the identical real-middleware route still succeeds end-to-end.
func TestUploadVCFForImport_RealMiddlewareStack_OrdinaryUploadStillSucceeds(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "vcfbodylimitok", Password: "password123!A", Email: "vcfbodylimitok@example.com"}
	require.NoError(t, db.Create(&user).Error)
	cfg := &config.Config{}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.MaxMultipartMemory = 10 << 20
	router.Use(middleware.DefaultBodySizeLimitMiddleware())
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST(vcfUploadRoutePath, middleware.BodySizeLimitMiddleware(services.MaxVCFSize), func(c *gin.Context) {
		UploadVCFForImport(c, cfg)
	})

	rec := postMultipartFile(t, router, vcfUploadRoutePath, "file", "small.vcf", "text/vcard", []byte(tinyVCard3Block))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
