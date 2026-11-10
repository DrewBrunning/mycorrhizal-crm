package controllers

// Issue #2300 (ADR 0043 decision 7, amended 2026-11-10): POST /import/entities/csv
// restores the entity sections of a CSV backup through services.RestoreEntityCSV.
// These tests drive the real handler over a real migrated schema.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const entityCSVRestorePath = "/import/entities/csv"

// entityCSVRestoreRouter serves the restore route as one fixed user.
func entityCSVRestoreRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Set("cfg", config.Config{ProfilePhotoDir: os.TempDir()})
		c.Next()
	})
	router.POST(entityCSVRestorePath, RestoreEntitiesCSV)
	return router
}

type entityCSVRestoreIssueBody struct {
	Record   string `json:"record"`
	Field    string `json:"field"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type entityCSVRestoreBody struct {
	EntitiesCreated    int                         `json:"entities_created"`
	LinksCreated       int                         `json:"links_created"`
	FieldValuesCreated int                         `json:"field_values_created"`
	Issues             []entityCSVRestoreIssueBody `json:"issues"`
}

func postEntityCSV(t *testing.T, router http.Handler, filename, body string) (*httptest.ResponseRecorder, entityCSVRestoreBody) {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, entityCSVRestorePath, filename, []byte(body)))
	var out entityCSVRestoreBody
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	}
	return w, out
}

func newRestoreUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{Username: name, Password: "password123!A", Email: name + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func countUserEntities(t *testing.T, db *gorm.DB, userID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Entity{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

func TestRestoreEntitiesCSV_RestoresAndIsIdempotent(t *testing.T) {
	fx := seedCSVFullFidelity(t)
	seedCSVEntities(t, fx.db, fx.user.ID, fx.ada, fx.bob)
	body := csvEntExport(t, fx.router)

	dst := dbtest.New(t)
	dest := newRestoreUser(t, dst, "csvrestoredest")
	for _, c := range []models.Contact{fx.ada, fx.bob} {
		require.NoError(t, dst.Create(&models.Contact{UserID: dest.ID, VCardUID: c.VCardUID, Firstname: c.Firstname, Lastname: c.Lastname}).Error)
	}
	for _, key := range []string{"industry", "offset"} {
		require.NoError(t, dst.Create(&models.FieldDefinition{UserID: dest.ID, Label: key, Key: key, Target: "organisation", Type: models.FieldTypeText, Projection: "internal-only", Sensitivity: "normal"}).Error)
	}
	router := entityCSVRestoreRouter(dst, dest.ID)

	w, got := postEntityCSV(t, router, "backup.csv", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 3, got.EntitiesCreated)
	assert.Equal(t, 2, got.LinksCreated)
	assert.Equal(t, 2, got.FieldValuesCreated)
	assert.Empty(t, got.Issues)
	assert.Contains(t, w.Body.String(), `"issues":[]`, "issues is always an array")
	assert.EqualValues(t, 3, countUserEntities(t, dst, dest.ID))

	// Re-running is a no-op reported as skipped rows, never a duplicate.
	w, again := postEntityCSV(t, router, "backup.csv", body)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Zero(t, again.EntitiesCreated+again.LinksCreated+again.FieldValuesCreated)
	require.NotEmpty(t, again.Issues)
	for _, i := range again.Issues {
		assert.Equal(t, services.ImportIssueCategorySkipped, i.Category, "%+v", i)
	}
	assert.EqualValues(t, 3, countUserEntities(t, dst, dest.ID))
}

func TestRestoreEntitiesCSV_MissingDependenciesAreReportedNotCreated(t *testing.T) {
	fx := seedCSVFullFidelity(t)
	seedCSVEntities(t, fx.db, fx.user.ID, fx.ada, fx.bob)
	body := csvEntExport(t, fx.router)

	dst := dbtest.New(t)
	dest := newRestoreUser(t, dst, "csvnodeps") // no contacts, no field definitions

	w, got := postEntityCSV(t, entityCSVRestoreRouter(dst, dest.ID), "backup.csv", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 3, got.EntitiesCreated)
	assert.Zero(t, got.LinksCreated)
	assert.Zero(t, got.FieldValuesCreated)

	var contactIssues, defIssues int
	for _, i := range got.Issues {
		assert.Equal(t, services.ImportIssueCategoryUnsupported, i.Category, "%+v", i)
		switch i.Field {
		case "entity_link.contact":
			contactIssues++
		case "entity_field_value.field_definition_id":
			defIssues++
		}
	}
	assert.Equal(t, 2, contactIssues)
	assert.Equal(t, 2, defIssues)

	var contacts, defs int64
	require.NoError(t, dst.Model(&models.Contact{}).Where("user_id = ?", dest.ID).Count(&contacts).Error)
	require.NoError(t, dst.Model(&models.FieldDefinition{}).Where("user_id = ?", dest.ID).Count(&defs).Error)
	assert.Zero(t, contacts, "a missing contact is reported, never created")
	assert.Zero(t, defs, "a missing field definition is reported, never created")
}

// Ownership: the restore lands only in the caller's account. Another user's
// rows are neither overwritten nor reachable, and the caller cannot link to a
// contact that belongs to someone else even though the vCard UID matches.
func TestRestoreEntitiesCSV_IsScopedToTheCaller(t *testing.T) {
	fx := seedCSVFullFidelity(t)
	seedCSVEntities(t, fx.db, fx.user.ID, fx.ada, fx.bob)
	body := csvEntExport(t, fx.router)

	other := newRestoreUser(t, fx.db, "csvintruder")
	var ownerEntitiesBefore []models.Entity
	require.NoError(t, fx.db.Where("user_id = ?", fx.user.ID).Order("id").Find(&ownerEntitiesBefore).Error)

	w, got := postEntityCSV(t, entityCSVRestoreRouter(fx.db, other.ID), "backup.csv", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 3, got.EntitiesCreated)
	assert.Zero(t, got.LinksCreated, "the owner's contacts are not the caller's")
	for _, i := range got.Issues {
		assert.Equal(t, services.ImportIssueCategoryUnsupported, i.Category, "%+v", i)
	}

	var ownerEntitiesAfter []models.Entity
	require.NoError(t, fx.db.Where("user_id = ?", fx.user.ID).Order("id").Find(&ownerEntitiesAfter).Error)
	assert.Equal(t, ownerEntitiesBefore, ownerEntitiesAfter, "the owner's entities are untouched")

	var theirs []models.Entity
	require.NoError(t, fx.db.Where("user_id = ?", other.ID).Find(&theirs).Error)
	require.Len(t, theirs, 3)
	owned := map[string]bool{}
	for _, e := range ownerEntitiesBefore {
		owned[e.ID] = true
	}
	for _, e := range theirs {
		assert.False(t, owned[e.ID], "ids are re-minted, never the owner's")
	}
	var links int64
	require.NoError(t, fx.db.Model(&models.ContactEntityLink{}).Where("user_id = ?", other.ID).Count(&links).Error)
	assert.Zero(t, links)
}

func TestRestoreEntitiesCSV_StoresFormulaCellsAsInertText(t *testing.T) {
	db := dbtest.New(t)
	u := newRestoreUser(t, db, "csvformula")
	csvBody := "=== ENTITIES ===\nID,Kind,Name,Sensitivity,Core (JSON),Notes,Created At,Updated At\n" +
		`e1,organisation,"=HYPERLINK(""http://attacker/"")",normal,{},,,` + "\n"

	w, got := postEntityCSV(t, entityCSVRestoreRouter(db, u.ID), "edited.csv", csvBody)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, got.EntitiesCreated)

	var e models.Entity
	require.NoError(t, db.Where("user_id = ?", u.ID).First(&e).Error)
	assert.Equal(t, `=HYPERLINK("http://attacker/")`, e.Name, "stored verbatim as data")

	// An invalid row is reported, not stored.
	bad := "=== ENTITIES ===\nID,Kind,Name,Sensitivity,Core (JSON),Notes,Created At,Updated At\n" +
		"e2,spaceship,Enterprise,normal,{},,,\n"
	w, got = postEntityCSV(t, entityCSVRestoreRouter(db, u.ID), "bad.csv", bad)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Zero(t, got.EntitiesCreated)
	require.Len(t, got.Issues, 1)
	assert.Equal(t, services.ImportIssueCategoryInvalid, got.Issues[0].Category)
}

func TestRestoreEntitiesCSV_CSVWithoutEntitySectionsIsANoOp(t *testing.T) {
	db := dbtest.New(t)
	u := newRestoreUser(t, db, "csvpre17")
	w, got := postEntityCSV(t, entityCSVRestoreRouter(db, u.ID), "old.csv", "=== CONTACTS ===\nID,First Name\n1,Ada\n")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Zero(t, got.EntitiesCreated+got.LinksCreated+got.FieldValuesCreated)
	assert.Empty(t, got.Issues)
}

func TestRestoreEntitiesCSV_RejectionsCarryReasonCodes(t *testing.T) {
	db := dbtest.New(t)
	u := newRestoreUser(t, db, "csvreject")
	router := entityCSVRestoreRouter(db, u.ID)

	t.Run("no file", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequestNoFile(t, entityCSVRestorePath))
		requireReason(t, w, http.StatusBadRequest, "import_file_missing")
	})
	t.Run("too large", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, entityCSVRestorePath, "big.csv", make([]byte, services.MaxCSVSize+1024)))
		requireReason(t, w, http.StatusBadRequest, "csv_file_too_large")
		_, params := wireReason(t, w)
		assert.EqualValues(t, services.MaxCSVSize/(1024*1024), params["max_mb"])
	})
	t.Run("wrong extension", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, entityCSVRestorePath, "backup.txt", []byte("=== ENTITIES ===\n")))
		requireReason(t, w, http.StatusBadRequest, "csv_file_not_csv")
	})
	t.Run("too many rows", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("=== ENTITIES ===\nID,Kind,Name,Sensitivity,Core (JSON)\n")
		for i := 0; i <= services.MaxEntityCSVRows; i++ {
			fmt.Fprintf(&b, "e%d,place,P,normal,{}\n", i)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, entityCSVRestorePath, "many.csv", []byte(b.String())))
		requireReason(t, w, http.StatusBadRequest, "import_file_unreadable")
		assert.EqualValues(t, 0, countUserEntities(t, db, u.ID), "nothing is written when the file is rejected")
	})
}

// One restore per user at a time: a concurrent request is refused, not queued,
// and another user is unaffected.
func TestRestoreEntitiesCSV_OneInFlightRestorePerUser(t *testing.T) {
	db := dbtest.New(t)
	u := newRestoreUser(t, db, "csvbusy")
	v := newRestoreUser(t, db, "csvfree")

	release, ok := acquireEntityCSVRestore(u.ID)
	require.True(t, ok)
	body := "=== ENTITIES ===\nID,Kind,Name,Sensitivity,Core (JSON)\ne1,place,Cabin,normal,{}\n"

	w, _ := postEntityCSV(t, entityCSVRestoreRouter(db, u.ID), "b.csv", body)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "RATE_LIMIT_EXCEEDED")
	assert.EqualValues(t, 0, countUserEntities(t, db, u.ID))

	w, got := postEntityCSV(t, entityCSVRestoreRouter(db, v.ID), "b.csv", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, got.EntitiesCreated, "another user is not blocked")

	release()
	w, got = postEntityCSV(t, entityCSVRestoreRouter(db, u.ID), "b.csv", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, got.EntitiesCreated, "released: the next restore runs")

	release2, ok := acquireEntityCSVRestore(u.ID)
	require.True(t, ok, "the guard is released after a completed request")
	release2()
}
