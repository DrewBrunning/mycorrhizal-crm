package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/imaging"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Gallery images in the account bundle (ADR 0040 decision 3, issue #2004).

const (
	// bundleImageHeadroom is the JSON slack kept free below the bundle size
	// limit for everything that is not an image.
	bundleImageHeadroom = 4 << 20
	// maxBundleImageBytes is the per-image ceiling on the stored full variant.
	maxBundleImageBytes = 4 << 20
	// bundleImageOverhead estimates one image's JSON metadata around its data.
	bundleImageOverhead = 512
)

// BundleImageExport configures image embedding. A zero Store.Dir skips images
// entirely (no directory configured). MaxBundleBytes is the size limit the
// finished bundle must stay under (the controller passes the same limit it
// enforces on the payload); zero means no budget.
type BundleImageExport struct {
	Store          imaging.Store
	MaxBundleBytes int64
}

type bundleImageCandidate struct {
	row    models.ImageFields
	kind   string
	owner  string
	isMain bool
}

// attachBundleImages embeds gallery images into bundle after every other
// section is loaded. Admission order is main images first, then newest first;
// an image that does not fit the budget, exceeds the per-image ceiling or whose
// file is missing is counted in the omissions, never dropped silently.
func attachBundleImages(db *gorm.DB, userID uint, bundle *models.AccountBundle, opts BundleImageExport, stats *AccountBundleStats) error {
	bundle.Images = []models.AccountBundleImage{}
	if opts.Store.Dir == "" {
		return nil
	}

	contactUIDs := make(map[string]int, len(bundle.Plan.Contacts))
	for i, c := range bundle.Plan.Contacts {
		contactUIDs[c.UID] = i
	}
	noteUUIDs := make(map[string]bool, len(bundle.Plan.Notes))
	for _, n := range bundle.Plan.Notes {
		noteUUIDs[n.UUID] = true
	}
	actUUIDs := make(map[string]bool, len(bundle.Plan.Activities))
	for _, a := range bundle.Plan.Activities {
		actUUIDs[a.UUID] = true
	}

	var (
		mains []struct {
			VCardUID    string
			MainImageID uint
		}
		cimgs []models.ContactImage
		nimgs []models.NoteImage
		aimgs []models.ActivityImage
		notes []models.Note
		acts  []models.Activity
	)
	// One check for all six reads: a failure of any is a failed export.
	if err := errors.Join(
		db.Model(&models.Contact{}).Select("vcard_uid, main_image_id").
			Where("user_id = ? AND main_image_id IS NOT NULL", userID).Scan(&mains).Error,
		db.Where("user_id = ?", userID).Find(&cimgs).Error,
		db.Where("user_id = ?", userID).Find(&nimgs).Error,
		db.Where("user_id = ?", userID).Find(&aimgs).Error,
		db.Select("id, uuid").Where("user_id = ?", userID).Find(&notes).Error,
		db.Select("id, uuid").Where("user_id = ?", userID).Find(&acts).Error,
	); err != nil {
		return err
	}

	mainIDs := map[uint]bool{}
	for _, m := range mains {
		mainIDs[m.MainImageID] = true
	}
	noteUUIDByID := make(map[uint]string, len(notes))
	for _, n := range notes {
		noteUUIDByID[n.ID] = n.UUID
	}
	actUUIDByID := make(map[uint]string, len(acts))
	for _, a := range acts {
		actUUIDByID[a.ID] = a.UUID
	}

	var cands []bundleImageCandidate
	for _, im := range cimgs {
		if _, ok := contactUIDs[im.ContactVCardUID]; ok {
			cands = append(cands, bundleImageCandidate{row: im.ImageFields, kind: "contact", owner: im.ContactVCardUID, isMain: mainIDs[im.ID]})
		}
	}
	for _, im := range nimgs {
		if u := noteUUIDByID[im.NoteID]; u != "" && noteUUIDs[u] {
			cands = append(cands, bundleImageCandidate{row: im.ImageFields, kind: "note", owner: u})
		}
	}
	for _, im := range aimgs {
		if u := actUUIDByID[im.ActivityID]; u != "" && actUUIDs[u] {
			cands = append(cands, bundleImageCandidate{row: im.ImageFields, kind: "activity", owner: u})
		}
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].isMain != cands[j].isMain {
			return cands[i].isMain
		}
		return cands[i].row.CreatedAt.After(cands[j].row.CreatedAt)
	})

	// The budget is what remains after the rest of the document and the JSON
	// headroom. Marshalling once here measures the rest exactly.
	var budget int64 = -1
	if opts.MaxBundleBytes > 0 {
		base, err := json.Marshal(bundle)
		if err != nil { // # pragma: no cover — the same document is marshalled by the controller
			return err // # pragma: no cover
		}
		budget = opts.MaxBundleBytes - bundleImageHeadroom - int64(len(base))
	}

	var used int64
	for _, c := range cands {
		omit := func() {
			bundle.Omitted.ImagesOmitted++
			bundle.Omitted.ImageBytesOmitted += c.row.SizeBytes
			stats.ImagesOmitted++
		}
		if c.row.SizeBytes > maxBundleImageBytes {
			omit()
			continue
		}
		est := int64(base64.StdEncoding.EncodedLen(int(c.row.SizeBytes))) + bundleImageOverhead
		if budget >= 0 && used+est > budget {
			omit()
			continue
		}
		data, err := opts.Store.Read(userID, c.row.StoredName)
		if err != nil {
			logger.Warn().Err(err).Str("image_uuid", c.row.UUID).Msg("account bundle: image file unreadable; counted as omitted")
			omit()
			continue
		}
		used += int64(base64.StdEncoding.EncodedLen(len(data))) + bundleImageOverhead
		bundle.Images = append(bundle.Images, models.AccountBundleImage{
			UUID: c.row.UUID, OwnerKind: c.kind, Owner: c.owner,
			MIMEType: c.row.MIMEType, Width: c.row.Width, Height: c.row.Height,
			TakenAt: c.row.TakenAt, TakenDate: c.row.TakenDate, Sensitivity: c.row.Sensitivity,
			Crop: c.row.StoredCrop(), CreatedAt: c.row.CreatedAt.UTC(),
			Data: base64.StdEncoding.EncodeToString(data),
		})
		stats.ImagesEmbedded++
		if c.isMain {
			if i, ok := contactUIDs[c.owner]; ok {
				bundle.Plan.Contacts[i].MainImage = c.row.UUID
			}
		}
	}
	return nil
}

// BundleImageImport carries what an image import needs from the server config.
type BundleImageImport struct {
	Config config.Config
}

// BundleImageImportResult counts what the image pass did: Created images
// landed, Skipped already existed for this user or had no owner in the
// destination, Rejected failed validation (corrupt, oversized, over a cap).
type BundleImageImportResult struct {
	Created  int
	Skipped  int
	Rejected int
}

// ImportBundleImages lands a bundle's images after the entity import. Every
// image is re-decoded and re-encoded through the normal pipeline under the
// same pixel limit, concurrency bound and per-owner cap as an upload; the
// declared mime/dimensions are never trusted. An image whose uuid already exists
// for the user is skipped (idempotent re-import). Owners resolve by the
// bundle's portable ids: contacts through refToID (the entity import's
// source-ref map), notes and activities by their preserved uuid; an unresolved
// owner skips the image. mainByContact (contact uid -> image uuid) is applied
// after every image has landed.
func ImportBundleImages(ctx context.Context, db *gorm.DB, opts BundleImageImport, userID uint, images []models.AccountBundleImage, mainByContact map[string]string, refToID map[string]uint) BundleImageImportResult {
	var res BundleImageImportResult
	cfg := opts.Config
	if cfg.ImagesDir == "" || len(images) == 0 {
		return res
	}
	store := imaging.Store{Dir: cfg.ImagesDir}
	maxBytes := int64(cfg.ImageMaxUploadMB) << 20
	quota := UserQuotaFromConfig(cfg)
	contactSvc := ContactImageService(db, store, cfg.ImageMaxPerContact, cfg.ImageMaxPixels, maxBytes, quota)
	noteSvc := NoteImageService(db, store, cfg.ImageMaxPerNote, cfg.ImageMaxPixels, maxBytes, quota)
	actSvc := ActivityImageService(db, store, cfg.ImageMaxPerActivity, cfg.ImageMaxPixels, maxBytes, quota)

	landed := map[string]uint{} // source uuid -> local contact image id
	for _, im := range images {
		if ctx.Err() != nil {
			return res
		}
		// The source uuid is kept when free. When another account already holds
		// it (the column is globally unique) a uuid derived from the user and the
		// source uuid is used, so a re-import into this account still finds it.
		localUUID := im.UUID
		if imageUUIDTaken(ctx, db, userID, im.UUID) {
			localUUID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%d/%s", userID, im.UUID))).String()
		}
		if existing := existingImageForUser(ctx, db, userID, localUUID); existing != nil {
			res.Skipped++
			if existing.kind == "contact" {
				landed[im.UUID] = existing.id
			}
			continue
		}
		data, err := base64.StdEncoding.DecodeString(im.Data)
		if err != nil {
			res.Rejected++
			continue
		}
		meta := ImageRestoreMeta{UUID: localUUID, Sensitivity: im.Sensitivity, TakenAt: im.TakenAt, TakenDate: im.TakenDate, Crop: im.Crop, CreatedAt: im.CreatedAt}
		switch im.OwnerKind {
		case "contact":
			id, ok := refToID["contact/"+im.Owner]
			var contact models.Contact
			if !ok || db.WithContext(ctx).Where("user_id = ?", userID).First(&contact, id).Error != nil {
				res.Skipped++
				continue
			}
			row, err := contactSvc.Restore(ctx, userID, OwnerRef{ID: contact.ID, UID: contact.VCardUID}, bytes.NewReader(data), meta)
			if err != nil {
				res.Rejected++
				continue
			}
			landed[im.UUID] = row.ID
			res.Created++
		case "note":
			var note models.Note
			if db.WithContext(ctx).Where("user_id = ? AND uuid = ?", userID, im.Owner).Order("id ASC").First(&note).Error != nil {
				res.Skipped++
				continue
			}
			if _, err := noteSvc.Restore(ctx, userID, OwnerRef{ID: note.ID, UID: note.UUID}, bytes.NewReader(data), meta); err != nil {
				res.Rejected++
				continue
			}
			res.Created++
		case "activity":
			var act models.Activity
			if db.WithContext(ctx).Where("user_id = ? AND uuid = ?", userID, im.Owner).Order("created_at ASC").First(&act).Error != nil {
				res.Skipped++
				continue
			}
			if _, err := actSvc.Restore(ctx, userID, OwnerRef{ID: act.ID, UID: act.UUID}, bytes.NewReader(data), meta); err != nil {
				res.Rejected++
				continue
			}
			res.Created++
		default:
			res.Rejected++
		}
	}

	for uid, imgUUID := range mainByContact {
		if ctx.Err() != nil {
			break // # pragma: no cover — defensive: reached only by a cancelled context mid-loop or a row deleted between two reads
		}
		applyBundleMainImage(ctx, db, store, cfg, userID, refToID["contact/"+uid], landed[imgUUID])
	}
	return res
}

// imageUUIDTaken reports whether any account other than userID holds an image
// with this uuid (live or soft-deleted). The user's own rows are handled by the
// idempotency check, not here.
func imageUUIDTaken(ctx context.Context, db *gorm.DB, userID uint, uuid string) bool {
	for _, table := range []string{"contact_images", "note_images", "activity_images"} {
		var n int64
		if err := db.WithContext(ctx).Table(table).Where("uuid = ? AND user_id <> ?", uuid, userID).Count(&n).Error; err == nil && n > 0 {
			return true
		}
	}
	return false
}

type existingImage struct {
	kind string
	id   uint
}

// existingImageForUser reports whether the user already holds an image (live
// or soft-deleted) with this uuid, in any of the three tables.
func existingImageForUser(ctx context.Context, db *gorm.DB, userID uint, uuid string) *existingImage {
	for _, t := range []struct{ kind, table string }{{"contact", "contact_images"}, {"note", "note_images"}, {"activity", "activity_images"}} {
		var id uint
		if err := db.WithContext(ctx).Table(t.table).Select("id").Where("user_id = ? AND uuid = ?", userID, uuid).Limit(1).Scan(&id).Error; err == nil && id != 0 {
			return &existingImage{kind: t.kind, id: id}
		}
	}
	return nil
}

// applyBundleMainImage points the contact at its main image. The avatar is
// derived from it only when the card carries no photo of its own (the bundle
// already embedded that photo); otherwise only the pointer is set.
func applyBundleMainImage(ctx context.Context, db *gorm.DB, store imaging.Store, cfg config.Config, userID, contactID, imageID uint) {
	if contactID == 0 || imageID == 0 {
		return // # pragma: no cover — defensive: reached only by a cancelled context mid-loop or a row deleted between two reads
	}
	var contact models.Contact
	var image models.ContactImage
	if db.WithContext(ctx).Where("user_id = ?", userID).First(&contact, contactID).Error != nil ||
		db.WithContext(ctx).Where("user_id = ? AND contact_vcard_uid = ?", userID, contact.VCardUID).First(&image, imageID).Error != nil {
		return // # pragma: no cover — defensive: reached only by a cancelled context mid-loop or a row deleted between two reads
	}
	if contact.Photo == "" {
		if _, err := SetContactMainImage(ctx, db, store, userID, &contact, &image, cfg.ProfilePhotoDir); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Msg("account bundle import: could not derive the avatar from the main image")
		}
		return
	}
	if err := db.WithContext(ctx).Model(&models.Contact{}).Where("id = ? AND user_id = ?", contact.ID, userID).
		UpdateColumn("main_image_id", image.ID).Error; err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("account bundle import: could not set the main image pointer") // # pragma: no cover — defensive: reached only by a cancelled context mid-loop or a row deleted between two reads
	}
}

// ImageRestoreMeta is the bundle metadata re-applied to a freshly processed
// image: the pipeline recomputes the pixels, but sensitivity, capture time,
// crop and creation time are the owner's data.
type ImageRestoreMeta struct {
	UUID        string
	Sensitivity string
	TakenAt     *time.Time
	TakenDate   *string
	Crop        *models.ImageCrop
	CreatedAt   time.Time
}
