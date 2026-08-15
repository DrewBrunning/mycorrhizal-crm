package controllers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// photoURLTestRouter builds a real-migrated-schema router (database.InitDB,
// not AutoMigrate — CLAUDE.md backend trap 1) with the contacts list, detail,
// and profile-picture endpoints wired, for M6's response-shape tests
// (M6).
func photoURLTestRouter(t *testing.T) (*gorm.DB, *gin.Engine, *config.Config, models.User) {
	t.Helper()
	photoDir := t.TempDir()
	cfg := &config.Config{ProfilePhotoDir: photoDir}

	db, err := database.InitDB(filepath.Join(t.TempDir(), "photo-url.db"))
	require.NoError(t, err)

	user := models.User{Username: "photo-tester", Password: "password123!A", Email: "photo-tester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", *cfg)
		c.Next()
	})
	router.GET("/contacts", GetContacts)
	router.GET("/contacts/:id", GetContact)
	router.GET("/contacts/:id/profile_picture", func(c *gin.Context) { GetProfilePicture(c, cfg) })
	return db, router, cfg, user
}

// listContactItem fetches one GET /contacts page and returns the items as
// raw maps so "absent" vs "empty" on the wire is distinguishable (CLAUDE.md
// frontend trap 8's discipline applied server-side).
func listContactItem(t *testing.T, router *gin.Engine, firstname string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest("GET", "/contacts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	for _, raw := range body["contacts"].([]any) {
		item := raw.(map[string]any)
		if item["firstname"] == firstname {
			return item
		}
	}
	t.Fatalf("contact %q not found in list: %s", firstname, w.Body.String())
	return nil
}

func photoDataURL(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestContactsListDetail_PhotoURLIsServed pins M6's response-shape change
// against the real migrated schema: the list/detail photo_thumbnail carries a
// relative profile-picture URL (never the raw base64 thumbnail or a legacy
// disk-file name), a photo-less contact omits the field entirely, the
// Card.Media photo entry carries the URL too, and the exposed URL actually
// serves bytes over the unchanged profile-picture endpoint.
func TestContactsListDetail_PhotoURLIsServed(t *testing.T) {
	db, router, cfg, user := photoURLTestRouter(t)

	thumbURL := photoDataURL(t)

	thumbOnly := models.Contact{UserID: user.ID, Firstname: "Thumb", PhotoThumbnail: thumbURL}
	require.NoError(t, db.Create(&thumbOnly).Error)

	// A disk-backed photo with only a legacy filename thumbnail: the
	// thumbnail endpoint cannot serve that, so the exposed URL must be the
	// full-photo variant.
	diskFile := filepath.Join(cfg.ProfilePhotoDir, "disk_photo.jpg")
	require.NoError(t, os.WriteFile(diskFile, []byte("not-really-a-jpeg-but-a-file"), 0o644))
	diskPhoto := models.Contact{UserID: user.ID, Firstname: "Disk", Photo: "disk_photo.jpg", PhotoThumbnail: "legacy.jpg"}
	require.NoError(t, db.Create(&diskPhoto).Error)

	photoLess := models.Contact{UserID: user.ID, Firstname: "None"}
	require.NoError(t, db.Create(&photoLess).Error)

	// List: URL (not data:/disk path), omitted when there is no photo.
	thumbItem := listContactItem(t, router, "Thumb")
	wantThumbURL := "/api/v1/contacts/" + strconv.Itoa(int(thumbOnly.ID)) + "/profile_picture?thumbnail=true"
	assert.Equal(t, wantThumbURL, thumbItem["photo_thumbnail"])
	assert.NotContains(t, thumbItem["photo_thumbnail"].(string), "data:image")

	diskItem := listContactItem(t, router, "Disk")
	wantDiskURL := "/api/v1/contacts/" + strconv.Itoa(int(diskPhoto.ID)) + "/profile_picture"
	assert.Equal(t, wantDiskURL, diskItem["photo_thumbnail"], "a legacy filename thumbnail cannot be served; the full-photo URL must be exposed")

	noneItem := listContactItem(t, router, "None")
	_, present := noneItem["photo_thumbnail"]
	assert.False(t, present, "a photo-less contact must omit photo_thumbnail entirely")

	// Detail: photo_thumbnail URL + Card.Media photo entry URL.
	req, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(thumbOnly.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	assert.Equal(t, wantThumbURL, detail["photo_thumbnail"])

	card := detail["card"].(map[string]any)
	media := card["media"].([]any)
	require.Len(t, media, 1, "the thumbnail-only contact's Card.Media must carry its bridged photo entry")
	photo := media[0].(map[string]any)
	assert.Equal(t, "photo", photo["kind"])
	assert.Equal(t, wantThumbURL, photo["uri"], "Card.Media photo uri must be the profile-picture URL, not a data URI")

	// The exposed thumbnail URL actually serves bytes over the unchanged
	// profile-picture endpoint.
	ppReq, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(thumbOnly.ID))+"/profile_picture?thumbnail=true", nil)
	ppW := httptest.NewRecorder()
	router.ServeHTTP(ppW, ppReq)
	require.Equal(t, http.StatusOK, ppW.Code, ppW.Body.String())
	decoded, err := base64.StdEncoding.DecodeString(thumbURL[len("data:image/jpeg;base64,"):])
	require.NoError(t, err)
	assert.Equal(t, decoded, ppW.Body.Bytes())
	assert.Equal(t, "image/jpeg", ppW.Header().Get("Content-Type"))

	// And the disk-backed full-photo URL serves the file too.
	diskReq, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(diskPhoto.ID))+"/profile_picture", nil)
	diskW := httptest.NewRecorder()
	router.ServeHTTP(diskW, diskReq)
	require.Equal(t, http.StatusOK, diskW.Code, diskW.Body.String())
	assert.Equal(t, []byte("not-really-a-jpeg-but-a-file"), diskW.Body.Bytes())
}

// TestContactsDetail_PUTRoundTripDoesNotPersistPhotoURL pins the write-path
// half of M6's response-shape change end to end (real migrated schema):
// the web client PUTs the loaded Card back verbatim on every edit, so the
// now-relative Card.Media photo uri would round-trip into storage — and a
// persisted relative URL would break VCF/JSContact export and CardDAV, whose
// consumers cannot fetch a path. applyMedia must recognize the URL as this
// contact's own photo pointer and re-derive the entry from the flat thumbnail
// instead. This is the regression the model-level test
// (TestApplyRecordToContact_ReDerivesRelativePhotoURL) pins through the real
// controller path.
func TestContactsDetail_PUTRoundTripDoesNotPersistPhotoURL(t *testing.T) {
	db, router, _, user := photoURLTestRouter(t)

	contact := models.Contact{UserID: user.ID, Firstname: "Roundtrip", PhotoThumbnail: photoDataURL(t)}
	require.NoError(t, db.Create(&contact).Error)
	contactURL := "/api/v1/contacts/" + strconv.Itoa(int(contact.ID)) + "/profile_picture"

	// GET detail: the read path exposes the relative URL in Card.Media.
	getReq, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID)), nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	require.Equal(t, http.StatusOK, getW.Code, getW.Body.String())
	var detail map[string]any
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &detail))
	card := detail["card"].(map[string]any)
	media := card["media"].([]any)
	require.Len(t, media, 1, "the thumbnail-only contact's Card.Media must carry its bridged photo entry")
	assert.Equal(t, contactURL+"?thumbnail=true", media[0].(map[string]any)["uri"])

	// PUT the loaded card back verbatim with a name edit, exactly as
	// ContactDetailPage.tsx's updateContactRecord sends it.
	card["name"] = map[string]any{"components": []any{map[string]any{"kind": "given", "value": "Roundtrip-Edited"}}}
	putBody, err := json.Marshal(map[string]any{"gender": "", "card": card, "crm": detail["crm"]})
	require.NoError(t, err)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
	putReq, _ := http.NewRequest("PUT", "/contacts/"+strconv.Itoa(int(contact.ID)), bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	router.ServeHTTP(putW, putReq)
	require.Equal(t, http.StatusOK, putW.Code, putW.Body.String())

	// The persisted Card must NOT carry the relative URL.
	var persisted models.Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	require.Len(t, persisted.Card.Media, 1)
	uri := persisted.Card.Media[0].URI
	if strings.HasPrefix(uri, "/api/v1/") {
		t.Errorf("persisted Card.Media[0].URI = %q — the read-path URL round-tripped into storage; exports/CardDAV would serve a dead relative path", uri)
	}
	if !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
		t.Errorf("persisted Card.Media[0].URI = %q, want a data URI re-derived from the flat thumbnail", uri)
	}
	require.NotEmpty(t, persisted.PhotoThumbnail, "the flat photo must be untouched by the URL round-trip")
}
