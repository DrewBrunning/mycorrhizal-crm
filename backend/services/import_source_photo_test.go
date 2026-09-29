package services

import (
	"encoding/base64"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func photoPlanContact(uri string) *MappedContact {
	return &MappedContact{
		Ref: SourceRef{System: "mycorrhizal", ExternalID: "contact/x"},
		Record: &contactmodel.Record{Card: contactmodel.Card{
			Media: []contactmodel.Resource{{Kind: "photo", URI: uri}},
		}},
	}
}

// TestPersistEmbeddedPhoto covers every branch of the issue #1308 helper: the
// success path is exercised end to end by TestAccountBundle_RoundTrip; this
// pins the no-op cases and the named-loss (transformed) cases.
func TestPersistEmbeddedPhoto(t *testing.T) {
	png := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bundleTestPNG(t))

	t.Run("nil record is a no-op", func(t *testing.T) {
		c := &models.Contact{}
		report := &ImportReport{}
		persistEmbeddedPhoto(c, &MappedContact{}, report)
		assert.Empty(t, c.Photo)
		assert.Empty(t, report.Issues)
	})

	t.Run("no photo entry is a no-op", func(t *testing.T) {
		c := &models.Contact{}
		report := &ImportReport{}
		mc := &MappedContact{Record: &contactmodel.Record{}}
		persistEmbeddedPhoto(c, mc, report)
		assert.Empty(t, c.Photo)
		assert.Empty(t, report.Issues)
	})

	t.Run("remote URL is left as the transient reference, no network", func(t *testing.T) {
		c := &models.Contact{}
		report := &ImportReport{}
		persistEmbeddedPhoto(c, photoPlanContact("https://example.invalid/a.png"), report)
		assert.Empty(t, c.Photo)
		assert.Empty(t, report.Issues)
	})

	t.Run("undecodable data URI is reported", func(t *testing.T) {
		useBundlePhotoDir(t)
		c := &models.Contact{}
		report := &ImportReport{}
		persistEmbeddedPhoto(c, photoPlanContact("data:image/png;base64,@@@not-base64@@@"), report)
		assert.Empty(t, c.Photo)
		require.Len(t, report.Issues, 1)
		assert.Equal(t, ImportIssueCategoryTransformed, report.Issues[0].Category)
		assert.Equal(t, "photo", report.Issues[0].Field)
	})

	t.Run("no photo directory is reported", func(t *testing.T) {
		orig := models.DefaultPhotoDir
		models.DefaultPhotoDir = ""
		t.Cleanup(func() { models.DefaultPhotoDir = orig })
		c := &models.Contact{}
		report := &ImportReport{}
		persistEmbeddedPhoto(c, photoPlanContact(png), report)
		assert.Empty(t, c.Photo)
		require.Len(t, report.Issues, 1)
		assert.Contains(t, report.Issues[0].Message, "no profile-photo directory")
	})

	t.Run("bytes that are not an image are reported", func(t *testing.T) {
		useBundlePhotoDir(t)
		c := &models.Contact{}
		report := &ImportReport{}
		junk := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("definitely not a png"))
		persistEmbeddedPhoto(c, photoPlanContact(junk), report)
		assert.Empty(t, c.Photo)
		require.Len(t, report.Issues, 1)
		assert.Contains(t, report.Issues[0].Message, "could not be saved")
	})

	t.Run("a real photo sets the flat columns", func(t *testing.T) {
		useBundlePhotoDir(t)
		c := &models.Contact{}
		report := &ImportReport{}
		persistEmbeddedPhoto(c, photoPlanContact(png), report)
		assert.NotEmpty(t, c.Photo)
		assert.NotEmpty(t, c.PhotoThumbnail)
		assert.Empty(t, report.Issues)
	})
}
