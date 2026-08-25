package controllers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPNGBase64ExportCtrl is a 1x1 transparent PNG, the same fixture used by
// photostore/photostore_test.go and services/contact_sync_service_test.go for
// the same reason (small enough to embed inline, decodes as a real image).
// Kept as an unexported copy here since photostore's own const isn't
// exported across package boundaries.
const testPNGBase64ExportCtrl = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// registerVCFRoute wires /export/vcf onto an existing router the same way
// routes.go does: ExportContactsAsVCF takes photoDir as an explicit
// parameter (unlike ExportContactsAsJSContact, which reads it from
// currentConfig(c) -- see export_controller.go's doc comments on both
// handlers), forwarded here via a closure just like routes.go's own
// `func(c *gin.Context) { controllers.ExportContactsAsVCF(c, cfg.ProfilePhotoDir) }`.
func registerVCFRoute(router *gin.Engine, photoDir string) {
	router.GET("/export/vcf", func(c *gin.Context) {
		ExportContactsAsVCF(c, photoDir)
	})
}

// registerJSContactRoute wires /export/jscontact onto an existing router,
// seeding the "cfg" gin-context key ExportContactsAsJSContact reads via
// currentConfig(c) (helpers.go: `val.(config.Config)`, a value type, not a
// pointer -- get this wrong and the photo-bridging test would silently
// no-op instead of testing anything real).
func registerJSContactRoute(router *gin.Engine, photoDir string) {
	router.GET("/export/jscontact", func(c *gin.Context) {
		c.Set("cfg", config.Config{ProfilePhotoDir: photoDir})
		ExportContactsAsJSContact(c)
	})
}

func TestExportData(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/export", ExportData)

	// Create test data
	contact1 := models.Contact{
		UserID:             user.ID,
		Firstname:          "Alice",
		Lastname:           "Johnson",
		Email:              "alice@example.com",
		Phone:              "123-456-7890",
		Birthday:           "1990-01-15",
		Address:            "123 Main St",
		HowWeMet:           "Work conference",
		WorkInformation:    "Software Engineer",
		ContactInformation: "Prefers email",
		// Deliberately left on the legacy flat column and NOT mirrored into
		// Circle entities: the CSV exporter must ignore it. See the
		// "must not export the legacy flat column" assertion below.
		Circles: []string{"StaleLegacyCircle"},
	}
	db.Create(&contact1)

	// T3: the "Circles"/"Tags" export columns source from the real
	// Circle/Tag entities, not the flat Contact.Circles JSON column. The
	// exporter used to read that column, so it emitted stale legacy strings
	// while omitting every membership the user had actually created.
	friends := models.Circle{UserID: user.ID, Name: "Friends"}
	work := models.Circle{UserID: user.ID, Name: "Work"}
	db.Create(&friends)
	db.Create(&work)
	db.Create(&models.CircleMember{CircleID: friends.ID, UserID: user.ID, MemberVCardUID: contact1.VCardUID})
	db.Create(&models.CircleMember{CircleID: work.ID, UserID: user.ID, MemberVCardUID: contact1.VCardUID})

	vegan := models.Tag{UserID: user.ID, Name: "vegan"}
	db.Create(&vegan)
	db.Create(&models.ContactTag{TagID: vegan.ID, UserID: user.ID, ContactVCardUID: contact1.VCardUID})

	// T20a: the "Food Preference" export column sources from the structured
	// preferences table now, not the retired Contact.FoodPreference field.
	foodConfidence := 1.0
	db.Create(&models.Preference{
		UserID:      user.ID,
		EntityID:    contact1.VCardUID,
		Category:    models.PreferenceCategoryFood,
		Value:       "Vegetarian",
		Source:      models.PreferenceSourceUser,
		Confidence:  &foodConfidence,
		Sensitivity: models.RelationshipSensitivityNormal,
	})

	contact2 := models.Contact{
		UserID:    user.ID,
		Firstname: "Bob",
		Lastname:  "Smith",
		Email:     "bob@example.com",
		Circles:   []string{"Family"},
	}
	db.Create(&contact2)

	// Create a relationship edge (RELATIONSHIPS now reads
	// RelationshipEdge, not the legacy models.Relationship table)
	edge := models.RelationshipEdge{
		UserID:      user.ID,
		SourceID:    contact1.VCardUID,
		TargetID:    contact2.VCardUID,
		Type:        "friend_of",
		Directional: false,
		Source:      models.RelationshipSourceUserConfirmed,
		Confidence:  1.0,
		Status:      models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&edge).Error)

	// Create an activity
	activityDate := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	activity := models.Activity{
		UserID:      user.ID,
		Title:       "Coffee Meeting",
		Description: "Catch up over coffee",
		Location:    "Local Cafe",
		Date:        activityDate,
		Contacts:    []models.Contact{contact1},
	}
	db.Create(&activity)

	// Create a note
	noteDate := time.Date(2024, 7, 10, 10, 0, 0, 0, time.UTC)
	note := models.Note{
		UserID:    user.ID,
		ContactID: &contact1.ID,
		Content:   "Remember to follow up about the project",
		Date:      noteDate,
	}
	db.Create(&note)

	// Create a reminder
	byMail := false
	reoccur := true
	reminderDate := time.Date(2024, 8, 1, 9, 0, 0, 0, time.UTC)
	reminder := models.Reminder{
		UserID:                user.ID,
		ContactID:             &contact1.ID,
		Message:               "Birthday reminder",
		RemindAt:              reminderDate,
		Recurrence:            "yearly",
		ByMail:                &byMail,
		ReoccurFromCompletion: &reoccur,
		Completed:             false,
	}
	db.Create(&reminder)

	// Make the request
	req, _ := http.NewRequest("GET", "/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check headers
	contentType := w.Header().Get("Content-Type")
	assert.Contains(t, contentType, "text/csv")

	contentDisposition := w.Header().Get("Content-Disposition")
	assert.Contains(t, contentDisposition, "attachment")
	assert.Contains(t, contentDisposition, "mycorrhizal-export")
	assert.Contains(t, contentDisposition, ".csv")

	// Check body content
	body := w.Body.String()

	// Verify contacts section
	assert.Contains(t, body, "=== CONTACTS ===")
	assert.Contains(t, body, "Alice")
	assert.Contains(t, body, "Johnson")
	assert.Contains(t, body, "alice@example.com")
	assert.Contains(t, body, "Bob")
	assert.Contains(t, body, "Smith")
	assert.Contains(t, body, "Friends; Work", "the Circles column must source from the Circle entities")
	assert.Contains(t, body, "vegan", "the Tags column must source from the Tag entities")
	assert.NotContains(t, body, "StaleLegacyCircle",
		"the exporter must not fall back to the legacy flat Contact.Circles column")
	assert.Contains(t, body, "Vegetarian", "the Food Preference column must source from the preferences table")

	// Verify relationships section
	assert.Contains(t, body, "=== RELATIONSHIPS ===")
	assert.Contains(t, body, "friend_of")
	assert.Contains(t, body, "confirmed")

	// Verify activities section
	assert.Contains(t, body, "=== ACTIVITIES ===")
	assert.Contains(t, body, "Coffee Meeting")
	assert.Contains(t, body, "Catch up over coffee")
	assert.Contains(t, body, "Local Cafe")

	// Verify notes section
	assert.Contains(t, body, "=== NOTES ===")
	assert.Contains(t, body, "Remember to follow up about the project")

	// Verify reminders section
	assert.Contains(t, body, "=== REMINDERS ===")
	assert.Contains(t, body, "Birthday reminder")
	assert.Contains(t, body, "yearly")
}

// T7: the CSV export's custom-field columns now source from the v2 system
// (FieldDefinition + FieldValue), not the retired untyped v1. Every definition becomes a header column (the header
// row is user-authored, so csvSafe still neutralizes it), and each contact's
// row carries its value for that definition -- including the Multi join and
// the empty-when-absent case.
func TestExportData_CustomFieldsV2(t *testing.T) {
	db, router := setupRouter()
	router.GET("/export", ExportData)

	var user models.User
	db.First(&user)

	contact := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Johnson"}
	db.Create(&contact)

	enumDef := models.FieldDefinition{
		UserID:      user.ID,
		Label:       "Pronouns",
		Key:         "pronouns",
		Target:      models.FieldDefinitionTargetContact,
		Type:        models.FieldTypeEnum,
		Constraints: models.FieldConstraints{Values: []string{"she/her", "he/him", "they/them"}, Multi: true},
		Projection:  "internal-only",
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	stringDef := models.FieldDefinition{
		UserID:      user.ID,
		Label:       "Internal Note",
		Key:         "internal_note",
		Target:      models.FieldDefinitionTargetContact,
		Type:        models.FieldTypeString,
		Projection:  "internal-only",
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	db.Create(&enumDef)
	db.Create(&stringDef)

	// Multi value serializes as "; "-joined; the absent string value stays empty.
	db.Create(&models.FieldValue{
		FieldDefinitionID: enumDef.ID,
		UserID:            user.ID,
		EntityID:          contact.VCardUID,
		Value:             json.RawMessage(`["she/her","they/them"]`),
	})

	req, _ := http.NewRequest("GET", "/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Both definition labels appear as header columns, and the value appears
	// on the contact's row (Multi joined with "; ").
	assert.Contains(t, body, "Pronouns")
	assert.Contains(t, body, "Internal Note")
	assert.Contains(t, body, "she/her; they/them")
}

func TestSerializeFieldValueForCSV(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"empty bytes", json.RawMessage{}, ""},
		{"json null", json.RawMessage(`null`), ""},
		{"plain string", json.RawMessage(`"hello"`), "hello"},
		{"empty string", json.RawMessage(`""`), ""},
		{"integer number", json.RawMessage(`42`), "42"},
		{"float number", json.RawMessage(`3.14`), "3.14"},
		{"boolean true", json.RawMessage(`true`), "true"},
		{"boolean false", json.RawMessage(`false`), "false"},
		{"multi empty array", json.RawMessage(`[]`), ""},
		{"multi string array", json.RawMessage(`["she/her","they/them"]`), "she/her; they/them"},
		{"multi nested mixed", json.RawMessage(`["ok",42,true]`), "ok; 42; true"},
		{"unexpected object falls back to raw", json.RawMessage(`{"x":1}`), `{"x":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := serializeFieldValueForCSV(tc.raw)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExportDataEmpty(t *testing.T) {
	_, router := setupRouter()

	router.GET("/export", ExportData)

	// Make the request with no data
	req, _ := http.NewRequest("GET", "/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check headers
	contentType := w.Header().Get("Content-Type")
	assert.Contains(t, contentType, "text/csv")

	// Check body still contains section headers
	body := w.Body.String()
	assert.Contains(t, body, "=== CONTACTS ===")
	assert.Contains(t, body, "=== RELATIONSHIPS ===")
	assert.Contains(t, body, "=== ACTIVITIES ===")
	assert.Contains(t, body, "=== NOTES ===")
	assert.Contains(t, body, "=== REMINDERS ===")
}

func TestExportDataUserScoping(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	// Create a second user
	otherUser := models.User{Username: "other", Password: "password456", Email: "other@example.com"}
	db.Create(&otherUser)

	router.GET("/export", ExportData)

	// Create contact for the first user
	contact1 := models.Contact{
		UserID:    user.ID,
		Firstname: "UserContact",
		Lastname:  "One",
	}
	db.Create(&contact1)

	// Create contact for the second user (should NOT appear in export)
	contact2 := models.Contact{
		UserID:    otherUser.ID,
		Firstname: "OtherUserContact",
		Lastname:  "Two",
	}
	db.Create(&contact2)

	// Make the request
	req, _ := http.NewRequest("GET", "/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()

	// Verify only the first user's contact is in the export
	assert.Contains(t, body, "UserContact")
	assert.True(t, strings.Contains(body, "UserContact"))
	assert.False(t, strings.Contains(body, "OtherUserContact"))
}

// --- ExportContactsAsVCF ---

func TestExportContactsAsVCF_DefaultVersion4(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Johnson", Email: "alice@example.com"})

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/vcard")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".vcf")

	body := w.Body.String()
	assert.Contains(t, body, "BEGIN:VCARD")
	assert.Contains(t, body, "VERSION:4.0")
	assert.NotContains(t, body, "VERSION:3.0")
	assert.Contains(t, body, "Alice")
	assert.Contains(t, body, "END:VCARD")
}

func TestExportContactsAsVCF_Version3QueryParam(t *testing.T) {
	// Exercises the exact query-param parsing in ExportContactsAsVCF:
	// `strings.TrimPrefix(c.Query("version"), "v")` compared against "3" or
	// "3.0" -- both "3" and "v3.0" must select vCard 3.0.
	for _, version := range []string{"3", "v3.0"} {
		t.Run(version, func(t *testing.T) {
			db, router := setupRouter()
			registerVCFRoute(router, "")

			var user models.User
			db.First(&user)
			db.Create(&models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Smith"})

			req, _ := http.NewRequest("GET", "/export/vcf?version="+version, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			body := w.Body.String()
			assert.Contains(t, body, "VERSION:3.0")
			assert.NotContains(t, body, "VERSION:4.0")
			assert.Contains(t, body, "Bob")
		})
	}
}

// TestExportContactsAsVCF_NoAuth_Unauthorized exercises the early
// currentUserID(c) !ok return branch.
func TestExportContactsAsVCF_NoAuth_Unauthorized(t *testing.T) {
	db, _ := setupRouter()
	router := routerWithoutAuth(db)
	router.GET("/export/vcf", func(c *gin.Context) {
		ExportContactsAsVCF(c, "")
	})

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

// TestExportContactsAsVCF_DBError exercises the db.Find error branch by
// closing the underlying *sql.DB out from under gorm before the request.
func TestExportContactsAsVCF_DBError(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportContactsAsVCF_MultipleContacts(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Johnson"})
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Smith"})
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Carol", Lastname: "Davis"})

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	assert.Equal(t, 3, strings.Count(body, "BEGIN:VCARD"))
	assert.Equal(t, 3, strings.Count(body, "END:VCARD"))
	assert.Contains(t, body, "Alice")
	assert.Contains(t, body, "Bob")
	assert.Contains(t, body, "Carol")
}

func TestExportContactsAsVCF_UserScoping(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "password456", Email: "other@example.com"}
	db.Create(&otherUser)

	db.Create(&models.Contact{UserID: user.ID, Firstname: "UserContact", Lastname: "One"})
	db.Create(&models.Contact{UserID: otherUser.ID, Firstname: "OtherUserContact", Lastname: "Two"})

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "UserContact")
	assert.NotContains(t, body, "OtherUserContact")
	assert.Equal(t, 1, strings.Count(body, "BEGIN:VCARD"))
}

func TestExportContactsAsVCF_Empty(t *testing.T) {
	_, router := setupRouter()
	registerVCFRoute(router, "")

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/vcard")
	assert.NotContains(t, w.Body.String(), "BEGIN:VCARD")
}

// TestExportContactsAsVCF_DiagnosticSanitization exercises the per-contact
// diagnostic loop in ExportContactsAsVCF (export_controller.go) — the
// message-position log-forgery vector logger.SanitizeLogField was introduced
// to close. A contact with no name at all makes the vcard4 exporter emit its
// "no Name.Full..." warn diagnostic rather than failing, so the loop runs and
// the sanitized message line is produced. Capturing the global logger and
// asserting the diagnostic appears is what proves the loop executes.
func TestExportContactsAsVCF_DiagnosticSanitization(t *testing.T) {
	buf := &bytes.Buffer{}
	oldLogger := logger.Logger
	oldLevel := zerolog.GlobalLevel()
	logger.Logger = zerolog.New(buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		logger.Logger = oldLogger
		zerolog.SetGlobalLevel(oldLevel)
	})

	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID})

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// The nameless contact still exports (as an empty FN) rather than
	// failing the whole export.
	assert.Contains(t, w.Body.String(), "BEGIN:VCARD")
	assert.Contains(t, buf.String(), "no Name.Full and no components to derive FN from",
		"the per-contact diagnostic loop must have logged the nameless contact")
}

// TestExportContactsAsVCF_PhotoBridging is a regression test for the exact
// bug class fixed 3x while auditing (see export_controller.go's and
// models/contact_record.go's RecordForContact doc comments):
// ExportContactsAsVCF must build its contactmodel.Record via
// models.RecordForContact, which -- once the persisted Contact.Card is
// non-zero -- returns the Card exactly as BeforeSave last derived it,
// regardless of the photoDir passed to the handler at export time. Calling
// RecordFromContact fresh here instead would re-derive Media from
// Contact.Photo/PhotoThumbnail using the handler's own photoDir, which this
// test deliberately points at an empty directory that does not contain the
// photo file -- so a regression back to RecordFromContact would silently
// drop the PHOTO property.
func TestExportContactsAsVCF_PhotoBridging(t *testing.T) {
	saveDir := t.TempDir() // where the photo actually lives at save time
	photoBytes, err := base64.StdEncoding.DecodeString(testPNGBase64ExportCtrl)
	if err != nil {
		t.Fatalf("failed to decode test PNG fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "avatar.png"), photoBytes, 0o600); err != nil {
		t.Fatalf("failed to write test photo to disk: %v", err)
	}

	// BeforeSave (models/contact.go) derives Card via models.DefaultPhotoDir,
	// not any handler-supplied photoDir -- point it at saveDir so the photo
	// is actually found and embedded into Card.Media at save time.
	origDefaultPhotoDir := models.DefaultPhotoDir
	models.DefaultPhotoDir = saveDir
	defer func() { models.DefaultPhotoDir = origDefaultPhotoDir }()

	db, router := setupRouter()
	// Deliberately mismatched: the export handler's photoDir points at an
	// empty directory with no "avatar.png" and no PhotoThumbnail fallback on
	// the contact row, so only the pre-baked Card.Media survives.
	registerVCFRoute(router, t.TempDir())

	var user models.User
	db.First(&user)
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Photo",
		Lastname:  "Person",
		Photo:     "avatar.png",
	}
	if err := db.Create(&contact).Error; err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "PHOTO")
	assert.Contains(t, body, testPNGBase64ExportCtrl, "expected the actual photo bytes to be embedded, not dropped")
}

// --- ExportContactsAsJSContact ---

func TestExportContactsAsJSContact_Basic(t *testing.T) {
	db, router := setupRouter()
	registerJSContactRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Johnson"})
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Smith"})

	req, _ := http.NewRequest("GET", "/export/jscontact", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/jscontact+json")

	var cards []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &cards); err != nil {
		t.Fatalf("export body did not parse as a JSON array of Cards: %v\nbody: %s", err, w.Body.String())
	}
	assert.Len(t, cards, 2)

	// Each element must be independently parseable/round-trippable (re-marshal
	// + re-parse each one on its own), and carry recognizable name data
	// (spot-check, per RFC 9553's Card "name" shape).
	var joined strings.Builder
	for _, card := range cards {
		raw, err := json.Marshal(card)
		assert.NoError(t, err)
		var reparsed map[string]any
		assert.NoError(t, json.Unmarshal(raw, &reparsed))
		joined.Write(raw)
		joined.WriteByte(' ')
	}
	assert.Contains(t, joined.String(), "Alice")
	assert.Contains(t, joined.String(), "Bob")
}

// TestExportContactsAsJSContact_NoAuth_Unauthorized exercises the early
// currentUserID(c) !ok return branch (mirrors the equivalent test in
// import_controller_test.go, reusing its routerWithoutAuth helper).
func TestExportContactsAsJSContact_NoAuth_Unauthorized(t *testing.T) {
	db, _ := setupRouter()
	router := routerWithoutAuth(db)
	router.GET("/export/jscontact", func(c *gin.Context) {
		c.Set("cfg", config.Config{})
		ExportContactsAsJSContact(c)
	})

	req, _ := http.NewRequest("GET", "/export/jscontact", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

// TestExportContactsAsJSContact_DBError exercises the db.Find error branch
// by closing the underlying *sql.DB out from under gorm before the request.
func TestExportContactsAsJSContact_DBError(t *testing.T) {
	db, router := setupRouter()
	registerJSContactRoute(router, "")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	req, _ := http.NewRequest("GET", "/export/jscontact", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestExportContactsAsJSContact_UserScoping(t *testing.T) {
	db, router := setupRouter()
	registerJSContactRoute(router, "")

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "password456", Email: "other@example.com"}
	db.Create(&otherUser)

	db.Create(&models.Contact{UserID: user.ID, Firstname: "UserContact", Lastname: "One"})
	db.Create(&models.Contact{UserID: otherUser.ID, Firstname: "OtherUserContact", Lastname: "Two"})

	req, _ := http.NewRequest("GET", "/export/jscontact", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var cards []map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	assert.Len(t, cards, 1)

	body := w.Body.String()
	assert.Contains(t, body, "UserContact")
	assert.NotContains(t, body, "OtherUserContact")
}

func TestExportContactsAsJSContact_Empty(t *testing.T) {
	_, router := setupRouter()
	registerJSContactRoute(router, "")

	req, _ := http.NewRequest("GET", "/export/jscontact", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", strings.TrimSpace(w.Body.String()), "empty export must be a JSON array, not null or an error body")

	var cards []map[string]any
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	assert.Len(t, cards, 0)
}

// TestExportContactsAsJSContact_PhotoBridging mirrors
// TestExportContactsAsVCF_PhotoBridging for the JSContact export path: same
// RecordForContact-vs-RecordFromContact regression, same deliberately
// mismatched photoDir-at-export-time vs photoDir-at-save-time setup.
func TestExportContactsAsJSContact_PhotoBridging(t *testing.T) {
	saveDir := t.TempDir()
	photoBytes, err := base64.StdEncoding.DecodeString(testPNGBase64ExportCtrl)
	if err != nil {
		t.Fatalf("failed to decode test PNG fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "avatar.png"), photoBytes, 0o600); err != nil {
		t.Fatalf("failed to write test photo to disk: %v", err)
	}

	origDefaultPhotoDir := models.DefaultPhotoDir
	models.DefaultPhotoDir = saveDir
	defer func() { models.DefaultPhotoDir = origDefaultPhotoDir }()

	db, router := setupRouter()
	// Mismatched on purpose: currentConfig(c).ProfilePhotoDir at export time
	// points at an empty directory with no avatar.png and no
	// PhotoThumbnail fallback, so only the pre-baked Card.Media survives.
	registerJSContactRoute(router, t.TempDir())

	var user models.User
	db.First(&user)
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Photo",
		Lastname:  "Person",
		Photo:     "avatar.png",
	}
	if err := db.Create(&contact).Error; err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	req, _ := http.NewRequest("GET", "/export/jscontact", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var cards []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &cards); err != nil {
		t.Fatalf("export body did not parse as a JSON array of Cards: %v", err)
	}
	assert.Len(t, cards, 1)

	media, ok := cards[0]["media"].(map[string]any)
	if !ok {
		t.Fatalf("expected a non-empty \"media\" object on the exported Card, got: %v", cards[0]["media"])
	}
	found := false
	for _, v := range media {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		uri, _ := entry["uri"].(string)
		if entry["kind"] == "photo" && strings.HasPrefix(uri, "data:image/") && strings.Contains(uri, testPNGBase64ExportCtrl) {
			found = true
		}
	}
	assert.True(t, found, "expected a media entry with kind=photo and the actual embedded photo data, got media=%v", media)
}

// --- T9 selective export (sections= + include_sensitive=) ---

// The ?sections= field picker must actually narrow a vCard export, and an
// absent param must preserve the pre-T9 all-sections behavior.
func TestExportContactsAsVCF_SectionsFilter(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson", Email: "alice@example.com", Phone: "555-0100"})

	// No sections param -> everything (backward compat).
	req, _ := http.NewRequest("GET", "/export/vcf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "EMAIL")
	assert.Contains(t, w.Body.String(), "TEL")

	// sections=emails -> EMAIL kept, TEL dropped.
	req, _ = http.NewRequest("GET", "/export/vcf?sections=emails", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "EMAIL")
	assert.NotContains(t, body, "TEL", "deselected phones must be omitted from the vCard")
	assert.Contains(t, body, "Alice", "identity data is always exported")
}

// An unknown section token is an explicit 400, not a silent narrowing.
func TestExportContactsAsVCF_UnknownSection_BadRequest(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson"})

	req, _ := http.NewRequest("GET", "/export/vcf?sections=emails,bogus_section", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// The opt-in override flows through the HTTP surface: a secret edge is
// absent by default and present only with ?include_sensitive=true.
func TestExportContactsAsVCF_IncludeSensitiveOptIn(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Brown"}
	db.Create(&alice)
	db.Create(&bob)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID:      user.ID,
		SourceID:    alice.VCardUID,
		TargetID:    bob.VCardUID,
		Type:        "spouse_of",
		Source:      models.RelationshipSourceUserConfirmed,
		Confidence:  1.0,
		Status:      models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)

	// Default (even with related_to selected): secret edge stays out.
	req, _ := http.NewRequest("GET", "/export/vcf?sections=related_to", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "RELATED", "checking a section is not enough to export a sensitive edge")

	// Explicit opt-in: the edge projects.
	req, _ = http.NewRequest("GET", "/export/vcf?sections=related_to&include_sensitive=true", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "RELATED", "include_sensitive=true must project the secret edge")
}

// TestExportContactsAsVCF_SecretCustomField_ExcludedByDefault_IncludedWithOptIn
// is issue #416's pinning test for the custom-field half of field-sensitivity
// filtering. models/contact_record.go's projectCustomFields already applies
// the same `sensitivity = 'normal'` query-level filter as
// projectRelationshipEdges (which TestExportContactsAsVCF_IncludeSensitiveOptIn
// above pins for RelationshipEdge) -- this is the missing equivalent for a
// secret-sensitivity FieldDefinition/FieldValue, modeled directly on that
// test. Only a definition with a "vcard:X-..." Projection ever reaches an
// export at all (default "internal-only" definitions never do).
func TestExportContactsAsVCF_SecretCustomField_ExcludedByDefault_IncludedWithOptIn(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson"}
	require.NoError(t, db.Create(&alice).Error)

	def := models.FieldDefinition{
		UserID:      user.ID,
		Label:       "Secret Field",
		Key:         "secret_field",
		Target:      "contact",
		Type:        "string",
		Projection:  "vcard:X-SECRET-FIELD",
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: def.ID,
		UserID:            user.ID,
		EntityID:          alice.VCardUID,
		Value:             json.RawMessage(`"top secret value"`),
	}).Error)

	// Default (even with custom_fields selected): the secret field stays out.
	req, _ := http.NewRequest("GET", "/export/vcf?sections=custom_fields", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "X-SECRET-FIELD", "checking a section is not enough to export a secret custom field")

	// Explicit opt-in: the field projects.
	req, _ = http.NewRequest("GET", "/export/vcf?sections=custom_fields&include_sensitive=true", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "X-SECRET-FIELD", "include_sensitive=true must project the secret custom field")
}

// The JSContact equivalent of the VCF test above.
func TestExportContactsAsJSContact_SecretCustomField_ExcludedByDefault_IncludedWithOptIn(t *testing.T) {
	db, router := setupRouter()
	registerJSContactRoute(router, "")

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson"}
	require.NoError(t, db.Create(&alice).Error)

	def := models.FieldDefinition{
		UserID:      user.ID,
		Label:       "Secret Field",
		Key:         "secret_field",
		Target:      "contact",
		Type:        "string",
		Projection:  "vcard:X-SECRET-FIELD",
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: def.ID,
		UserID:            user.ID,
		EntityID:          alice.VCardUID,
		Value:             json.RawMessage(`"top secret value"`),
	}).Error)

	req, _ := http.NewRequest("GET", "/export/jscontact?sections=custom_fields", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "X-SECRET-FIELD")

	req, _ = http.NewRequest("GET", "/export/jscontact?sections=custom_fields&include_sensitive=true", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "X-SECRET-FIELD")
}

// The same picker applies to the JSContact export handler.
func TestExportContactsAsJSContact_SectionsFilter(t *testing.T) {
	db, router := setupRouter()
	registerJSContactRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson", Email: "alice@example.com", Phone: "555-0100"})

	req, _ := http.NewRequest("GET", "/export/jscontact?sections=emails", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var cards []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	require.Len(t, cards, 1)
	assert.Contains(t, cards[0], "emails")
	_, phonesPresent := cards[0]["phones"]
	assert.False(t, phonesPresent, "deselected phones must be omitted from the JSContact export")
	_, namePresent := cards[0]["name"]
	assert.True(t, namePresent, "identity name is always exported")
}

// parseExportFieldSelection accepts include_sensitive=1 as the numeric
// equivalent of true — a single-character concession that matches the
// "true"/"1" pair already used by the boolean-flag endpoints elsewhere in
// the codebase (e.g. reminder "by_mail").
func TestExportContactsAsVCF_IncludeSensitive_OneIsTrue(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Brown"}
	db.Create(&alice)
	db.Create(&bob)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID:      user.ID,
		SourceID:    alice.VCardUID,
		TargetID:    bob.VCardUID,
		Type:        "spouse_of",
		Source:      models.RelationshipSourceUserConfirmed,
		Confidence:  1.0,
		Status:      models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)

	req, _ := http.NewRequest("GET", "/export/vcf?sections=related_to&include_sensitive=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "RELATED", "include_sensitive=1 must project the secret edge")
}

// include_sensitive=true on its own (no ?sections=) implies all sections, and
// the sensitive opt-in is still respected.
func TestExportContactsAsVCF_IncludeSensitive_WithoutSectionsParam(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson", Email: "alice@example.com", Phone: "555-0100"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob", Lastname: "Brown"}
	db.Create(&alice)
	db.Create(&bob)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID:      user.ID,
		SourceID:    alice.VCardUID,
		TargetID:    bob.VCardUID,
		Type:        "spouse_of",
		Source:      models.RelationshipSourceUserConfirmed,
		Confidence:  1.0,
		Status:      models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)

	req, _ := http.NewRequest("GET", "/export/vcf?include_sensitive=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "RELATED", "include_sensitive=true without sections= must project the secret edge")
	assert.Contains(t, w.Body.String(), "EMAIL", "absent sections= must default to all sections")
	assert.Contains(t, w.Body.String(), "TEL")
}

// An empty ?sections= value is treated identically to the absent param,
// preserving the pre-T9 all-sections default.
func TestExportContactsAsVCF_EmptySectionsParam_AllFields(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")

	var user models.User
	db.First(&user)
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson", Email: "alice@example.com", Phone: "555-0100"})

	req, _ := http.NewRequest("GET", "/export/vcf?sections=", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "EMAIL", "empty sections= must default to all sections")
	assert.Contains(t, w.Body.String(), "TEL")
	assert.Contains(t, w.Body.String(), "Alice", "identity data is always exported")
}
