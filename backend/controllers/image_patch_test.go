package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ADR 0040 decision 8 lists PATCH .../images/:image_id for every owner; v1.6.0
// shipped it for contacts only, so a note or activity image's sensitivity
// could be read but never changed (found in the v1.6.0 review, #2304).

type imagePatchCase struct {
	name string
	// setup returns the router, the DB, the caller's owner ID, a second owner
	// ID of the caller's, a foreign owner ID, and the URL prefix.
	setup func(t *testing.T) (router *gin.Engine, db *gorm.DB, own, ownOther, foreign uint, prefix string)
	// upload stores one image on owner and returns its ID.
	upload func(t *testing.T, owner uint) uint
	// stored reads back the persisted row's sensitivity and stored name.
	stored func(t *testing.T, db *gorm.DB, id uint) (sensitivity, storedName string)
}

func imagePatchCases() []imagePatchCase {
	var noteEnv noteImageTestEnv
	var actEnv imageTestEnv
	return []imagePatchCase{
		{
			name: "note",
			setup: func(t *testing.T) (*gin.Engine, *gorm.DB, uint, uint, uint, string) {
				noteEnv = setupNoteImageRouter(t)
				noteEnv.router.PATCH("/notes/:id/images/:image_id", PatchNoteImage)
				other := models.User{Username: "patch-other", Password: "password123!A", Email: "patch-other@example.com"}
				require.NoError(t, noteEnv.db.Create(&other).Error)
				own := models.Note{UserID: noteEnv.user.ID, Content: "Mine", Date: testDate()}
				own2 := models.Note{UserID: noteEnv.user.ID, Content: "Also mine", Date: testDate()}
				theirs := models.Note{UserID: other.ID, Content: "Theirs", Date: testDate()}
				require.NoError(t, noteEnv.db.Create(&own).Error)
				require.NoError(t, noteEnv.db.Create(&own2).Error)
				require.NoError(t, noteEnv.db.Create(&theirs).Error)
				return noteEnv.router, noteEnv.db, own.ID, own2.ID, theirs.ID, "/notes/"
			},
			upload: func(t *testing.T, owner uint) uint {
				rec := uploadNoteImageForTest(t, noteEnv, owner, testJPEG(t))
				require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
				var out struct{ Image NoteImageDTO }
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				return out.Image.ID
			},
			stored: func(t *testing.T, db *gorm.DB, id uint) (string, string) {
				var row models.NoteImage
				require.NoError(t, db.First(&row, id).Error)
				return row.Sensitivity, row.StoredName
			},
		},
		{
			name: "activity",
			setup: func(t *testing.T) (*gin.Engine, *gorm.DB, uint, uint, uint, string) {
				actEnv = setupImageRouter(t)
				actEnv.router.PATCH("/activities/:id/images/:image_id", PatchActivityImage)
				other := models.User{Username: "patch-other", Password: "password123!A", Email: "patch-other@example.com"}
				require.NoError(t, actEnv.db.Create(&other).Error)
				own := models.Activity{UserID: actEnv.user.ID, Title: "Mine", Date: testDate()}
				own2 := models.Activity{UserID: actEnv.user.ID, Title: "Also mine", Date: testDate()}
				theirs := models.Activity{UserID: other.ID, Title: "Theirs", Date: testDate()}
				require.NoError(t, actEnv.db.Create(&own).Error)
				require.NoError(t, actEnv.db.Create(&own2).Error)
				require.NoError(t, actEnv.db.Create(&theirs).Error)
				return actEnv.router, actEnv.db, own.ID, own2.ID, theirs.ID, "/activities/"
			},
			upload: func(t *testing.T, owner uint) uint {
				rec := uploadActivityImage(t, actEnv, owner, testJPEG(t))
				require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
				var out struct{ Image ActivityImageDTO }
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				return out.Image.ID
			},
			stored: func(t *testing.T, db *gorm.DB, id uint) (string, string) {
				var row models.ActivityImage
				require.NoError(t, db.First(&row, id).Error)
				return row.Sensitivity, row.StoredName
			},
		},
	}
}

func patchImage(router *gin.Engine, url, body string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("PATCH", url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func imageURL(prefix string, owner, image uint) string {
	return prefix + strconv.FormatUint(uint64(owner), 10) + "/images/" + strconv.FormatUint(uint64(image), 10)
}

func TestImagePatch_SetsSensitivityThroughTheAllowlist(t *testing.T) {
	for _, tc := range imagePatchCases() {
		t.Run(tc.name, func(t *testing.T) {
			router, db, own, _, _, prefix := tc.setup(t)
			id := tc.upload(t, own)
			_, storedBefore := tc.stored(t, db, id)

			// stored_name is not writable: an unknown field is ignored, never mass-assigned.
			rec := patchImage(router, imageURL(prefix, own, id), `{"sensitivity":"private","stored_name":"../evil.jpg"}`)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var out struct {
				Image struct {
					ID          uint   `json:"id"`
					Sensitivity string `json:"sensitivity"`
				} `json:"image"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, id, out.Image.ID)
			assert.Equal(t, "private", out.Image.Sensitivity)

			sens, stored := tc.stored(t, db, id)
			assert.Equal(t, "private", sens)
			assert.Equal(t, storedBefore, stored)

			// An empty body is a no-op that still returns the image.
			rec = patchImage(router, imageURL(prefix, own, id), `{}`)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			sens, _ = tc.stored(t, db, id)
			assert.Equal(t, "private", sens)
		})
	}
}

func TestImagePatch_RejectsInvalidInput(t *testing.T) {
	for _, tc := range imagePatchCases() {
		t.Run(tc.name, func(t *testing.T) {
			router, db, own, _, _, prefix := tc.setup(t)
			id := tc.upload(t, own)
			for _, body := range []string{`{"sensitivity":"public"}`, `{"crop":{"x":0.5,"y":0,"w":0.9,"h":1}}`, `not json`} {
				rec := patchImage(router, imageURL(prefix, own, id), body)
				assert.Equal(t, http.StatusBadRequest, rec.Code, body)
			}
			rec := patchImage(router, prefix+strconv.FormatUint(uint64(own), 10)+"/images/abc", `{"sensitivity":"secret"}`)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			sens, _ := tc.stored(t, db, id)
			assert.Equal(t, "normal", sens, "a rejected PATCH must not change the row")
		})
	}
}

func TestImagePatch_OwnershipIs404(t *testing.T) {
	for _, tc := range imagePatchCases() {
		t.Run(tc.name, func(t *testing.T) {
			router, db, own, ownOther, foreign, prefix := tc.setup(t)
			id := tc.upload(t, own)

			// The image exists, but under a different owner of the caller's: the
			// image is resolved by owner, not by ID alone.
			rec := patchImage(router, imageURL(prefix, ownOther, id), `{"sensitivity":"secret"}`)
			assert.Equal(t, http.StatusNotFound, rec.Code)
			// Another user's owner.
			rec = patchImage(router, imageURL(prefix, foreign, id), `{"sensitivity":"secret"}`)
			assert.Equal(t, http.StatusNotFound, rec.Code)
			// An image ID that doesn't exist.
			rec = patchImage(router, imageURL(prefix, own, id+999), `{"sensitivity":"secret"}`)
			assert.Equal(t, http.StatusNotFound, rec.Code)

			sens, _ := tc.stored(t, db, id)
			assert.Equal(t, "normal", sens)
		})
	}
}

func TestImagePatch_RequiresAnAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	router.PATCH("/notes/:id/images/:image_id", PatchNoteImage)
	router.PATCH("/activities/:id/images/:image_id", PatchActivityImage)
	for _, url := range []string{"/notes/1/images/1", "/activities/1/images/1"} {
		rec := patchImage(router, url, `{"sensitivity":"secret"}`)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, url)
	}
}
