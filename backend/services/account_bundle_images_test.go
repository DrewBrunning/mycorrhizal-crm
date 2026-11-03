package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/imaging"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ADR 0040 decision 3 (issue #2004): gallery images travel in the account
// bundle (full variant only, main images first then newest first, within the
// size budget, counted when omitted) and import re-validates every image.

type bundleImageFixture struct {
	db      *gorm.DB
	user    models.User
	dir     string
	contact models.Contact
	note    models.Note
	act     models.Activity
}

func newBundleImageFixture(t *testing.T) bundleImageFixture {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "bundle-img", Password: "password123!A", Email: "bundle-img@example.com"}
	require.NoError(t, db.Create(&user).Error)
	f := bundleImageFixture{db: db, user: user, dir: t.TempDir()}
	f.contact = models.Contact{UserID: user.ID, Firstname: "Dana", VCardUID: "11111111-1111-4111-8111-111111111111"}
	require.NoError(t, db.Create(&f.contact).Error)
	f.note = models.Note{UserID: user.ID, Content: "", Date: svcT0, ContactID: &f.contact.ID}
	require.NoError(t, db.Create(&f.note).Error)
	f.act = models.Activity{UserID: user.ID, Title: "Trip", Date: svcT0}
	require.NoError(t, db.Create(&f.act).Error)
	return f
}

func (f bundleImageFixture) store() imaging.Store { return imaging.Store{Dir: f.dir} }

func (f bundleImageFixture) uploadAll(t *testing.T) (contactImg *models.ContactImage, noteImg *models.NoteImage, actImg *models.ActivityImage) {
	t.Helper()
	ctx := context.Background()
	var err error
	contactImg, err = ContactImageService(f.db, f.store(), 10, 50_000_000, 20<<20, UserQuota{}).
		Upload(ctx, f.user.ID, OwnerRef{ID: f.contact.ID, UID: f.contact.VCardUID}, bytes.NewReader(imageTestJPEGVariant(t, 1)))
	require.NoError(t, err)
	noteImg, err = NoteImageService(f.db, f.store(), 10, 50_000_000, 20<<20, UserQuota{}).
		Upload(ctx, f.user.ID, OwnerRef{ID: f.note.ID, UID: f.note.UUID}, bytes.NewReader(imageTestJPEGVariant(t, 2)))
	require.NoError(t, err)
	actImg, err = ActivityImageService(f.db, f.store(), 10, 50_000_000, 20<<20, UserQuota{}).
		Upload(ctx, f.user.ID, OwnerRef{ID: f.act.ID, UID: f.act.UUID}, bytes.NewReader(imageTestJPEGVariant(t, 3)))
	require.NoError(t, err)
	return
}

func TestBuildAccountBundle_CarriesImagesAndMainImage(t *testing.T) {
	useFakeClock(t, svcT0)
	f := newBundleImageFixture(t)
	contactImg, noteImg, actImg := f.uploadAll(t)
	// Sensitivity and crop travel (the bundle is the owner's full-fidelity backup).
	require.NoError(t, f.db.Model(contactImg).Updates(map[string]any{"sensitivity": "private", "crop_x": 0.1, "crop_y": 0.2, "crop_w": 0.5, "crop_h": 0.6}).Error)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", f.contact.ID).Update("main_image_id", contactImg.ID).Error)

	bundle, stats, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store(), MaxBundleBytes: MaxMycorrhizalBundleSize})
	require.NoError(t, err)

	require.Len(t, bundle.Images, 3)
	byOwnerKind := map[string]models.AccountBundleImage{}
	for _, im := range bundle.Images {
		byOwnerKind[im.OwnerKind] = im
		raw, err := base64.StdEncoding.DecodeString(im.Data)
		require.NoError(t, err)
		onDisk, err := os.ReadFile(filepath.Join(f.dir, "u"+itoaU(f.user.ID), storedNameFor(t, f, im.UUID)))
		require.NoError(t, err)
		assert.Equal(t, onDisk, raw, "the embedded bytes are the stored full variant")
	}
	c := byOwnerKind["contact"]
	assert.Equal(t, contactImg.UUID, c.UUID)
	assert.Equal(t, f.contact.VCardUID, c.Owner)
	assert.Equal(t, "private", c.Sensitivity)
	require.NotNil(t, c.Crop)
	assert.InDelta(t, 0.5, c.Crop.W, 1e-9)
	assert.Equal(t, f.note.UUID, byOwnerKind["note"].Owner)
	assert.Equal(t, noteImg.UUID, byOwnerKind["note"].UUID)
	assert.Equal(t, f.act.UUID, byOwnerKind["activity"].Owner)
	assert.Equal(t, actImg.UUID, byOwnerKind["activity"].UUID)

	require.Len(t, bundle.Plan.Contacts, 1)
	assert.Equal(t, contactImg.UUID, bundle.Plan.Contacts[0].MainImage)
	assert.Zero(t, bundle.Omitted.ImagesOmitted)
	assert.Equal(t, 3, stats.ImagesEmbedded)

	raw, err := json.Marshal(bundle)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"images":[`)
}

func TestBuildAccountBundle_ImageBudgetOmitsAndCounts(t *testing.T) {
	useFakeClock(t, svcT0)
	f := newBundleImageFixture(t)
	contactImg, _, _ := f.uploadAll(t)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", f.contact.ID).Update("main_image_id", contactImg.ID).Error)

	// Headroom (4 MiB) plus a base document leaves almost nothing: only the
	// main image may fit, then every other image is counted, never silently dropped.
	oneImage := int64(len(base64.StdEncoding.EncodeToString(mustRead(t, filepath.Join(f.dir, "u"+itoaU(f.user.ID), storedNameFor(t, f, contactImg.UUID))))) + bundleImageOverhead)
	base, _, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{})
	require.NoError(t, err)
	baseJSON, err := json.Marshal(base)
	require.NoError(t, err)
	budget := int64(len(baseJSON)) + bundleImageHeadroom + oneImage + 10

	bundle, stats, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store(), MaxBundleBytes: budget})
	require.NoError(t, err)
	require.Len(t, bundle.Images, 1)
	assert.Equal(t, contactImg.UUID, bundle.Images[0].UUID, "the main image is admitted first")
	assert.Equal(t, 2, bundle.Omitted.ImagesOmitted)
	assert.Positive(t, bundle.Omitted.ImageBytesOmitted)
	assert.Equal(t, 2, stats.ImagesOmitted)
}

func TestImportBundleImages_RestoresRevalidatesAndIsIdempotent(t *testing.T) {
	useFakeClock(t, svcT0)
	src := newBundleImageFixture(t)
	contactImg, _, _ := src.uploadAll(t)
	require.NoError(t, src.db.Model(contactImg).Update("sensitivity", "private").Error)
	bundle, _, err := BuildAccountBundleWithImages(src.db, src.user.ID, t.TempDir(), BundleImageExport{Store: src.store(), MaxBundleBytes: MaxMycorrhizalBundleSize})
	require.NoError(t, err)

	// A second user on the same instance imports the bundle: owners resolve by
	// the bundle's portable ids, and the source's image uuids (globally unique
	// columns) are re-minted when they already exist for another account.
	other := models.User{Username: "bundle-img-2", Password: "password123!A", Email: "bundle-img-2@example.com"}
	require.NoError(t, src.db.Create(&other).Error)
	contact := models.Contact{UserID: other.ID, Firstname: "Dana", VCardUID: "22222222-2222-4222-8222-222222222222"}
	require.NoError(t, src.db.Create(&contact).Error)
	note := models.Note{UUID: src.note.UUID, UserID: other.ID, Content: "", Date: svcT0, ContactID: &contact.ID}
	require.NoError(t, src.db.Create(&note).Error)
	act := models.Activity{UUID: src.act.UUID, UserID: other.ID, Title: "Trip", Date: svcT0}
	require.NoError(t, src.db.Create(&act).Error)

	dest := t.TempDir()
	opts := BundleImageImport{Config: config.Config{ImagesDir: dest, ProfilePhotoDir: t.TempDir(), ImageMaxPixels: 50_000_000, ImageMaxUploadMB: 20, ImageMaxPerNote: 10, ImageMaxPerActivity: 10, ImageMaxPerContact: 50}}
	refToID := map[string]uint{"contact/" + src.contact.VCardUID: contact.ID}

	// A corrupt image is rejected and counted; the import continues.
	images := append([]models.AccountBundleImage{}, bundle.Images...)
	images = append(images, models.AccountBundleImage{UUID: "bad", OwnerKind: "note", Owner: src.note.UUID, Data: base64.StdEncoding.EncodeToString([]byte("not an image"))})

	res := ImportBundleImages(context.Background(), src.db, opts, other.ID, images, map[string]string{src.contact.VCardUID: contactImg.UUID}, refToID)
	assert.Equal(t, 3, res.Created)
	assert.Equal(t, 1, res.Rejected)

	var ci models.ContactImage
	require.NoError(t, src.db.Where("user_id = ?", other.ID).First(&ci).Error)
	assert.Equal(t, contact.VCardUID, ci.ContactVCardUID)
	assert.Equal(t, "private", ci.Sensitivity)
	assert.NotEqual(t, contactImg.UUID, ci.UUID, "uuid is re-minted when another account already holds it")
	assert.FileExists(t, filepath.Join(dest, "u"+itoaU(other.ID), ci.StoredName))

	var ni models.NoteImage
	require.NoError(t, src.db.Where("user_id = ? AND note_id = ?", other.ID, note.ID).First(&ni).Error)

	// Re-running into the same account is idempotent.
	again := ImportBundleImages(context.Background(), src.db, opts, other.ID, bundle.Images, nil, refToID)
	assert.Equal(t, 0, again.Created)
	assert.Equal(t, 3, again.Skipped)
}

func TestImportBundleImages_SetsMainImageAndSkipsUnresolvedOwner(t *testing.T) {
	useFakeClock(t, svcT0)
	f := newBundleImageFixture(t)
	contactImg, _, _ := f.uploadAll(t)
	bundle, _, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store(), MaxBundleBytes: MaxMycorrhizalBundleSize})
	require.NoError(t, err)

	other := models.User{Username: "bundle-img-3", Password: "password123!A", Email: "bundle-img-3@example.com"}
	require.NoError(t, f.db.Create(&other).Error)
	contact := models.Contact{UserID: other.ID, Firstname: "Dana", VCardUID: "33333333-3333-4333-8333-333333333333"}
	require.NoError(t, f.db.Create(&contact).Error)
	opts := BundleImageImport{Config: config.Config{ImagesDir: t.TempDir(), ProfilePhotoDir: t.TempDir(), ImageMaxPixels: 50_000_000, ImageMaxUploadMB: 20, ImageMaxPerNote: 10, ImageMaxPerActivity: 10, ImageMaxPerContact: 50}}

	res := ImportBundleImages(context.Background(), f.db, opts, other.ID, bundle.Images,
		map[string]string{f.contact.VCardUID: contactImg.UUID},
		map[string]uint{"contact/" + f.contact.VCardUID: contact.ID})

	// Note and activity owners do not exist in the destination: skipped, not failed.
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 2, res.Skipped)

	var got models.Contact
	require.NoError(t, f.db.First(&got, contact.ID).Error)
	require.NotNil(t, got.MainImageID, "main_image applied after all images land")
	assert.NotEmpty(t, got.Photo, "no photo of its own: the avatar is derived from the main image")
}

func storedNameFor(t *testing.T, f bundleImageFixture, uuid string) string {
	t.Helper()
	for _, table := range []string{"contact_images", "note_images", "activity_images"} {
		var name string
		if err := f.db.Table(table).Select("stored_name").Where("uuid = ?", uuid).Scan(&name).Error; err == nil && name != "" {
			return name
		}
	}
	t.Fatalf("no image row for %s", uuid)
	return ""
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return b
}

func itoaU(u uint) string {
	b, _ := json.Marshal(u)
	return string(b)
}
