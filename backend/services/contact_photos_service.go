package services

import (
	"context"
	"sort"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ContactPhoto is one item in the aggregated contact Photos tab: the contact's
// own gallery plus images from its notes and activities (ADR 0040 decision 8).
// Source labels are computed per request; no labels are stored.
type ContactPhoto struct {
	Kind        string
	SourceID    uint
	SourceLabel string
	ImageID     uint
	UUID        string
	MIMEType    string
	Width       int
	Height      int
	SizeBytes   int64
	Sensitivity string
	TakenAt     *time.Time
	TakenDate   *string
	CreatedAt   time.Time
}

func (p ContactPhoto) sortTime() time.Time {
	if p.TakenAt != nil {
		return *p.TakenAt
	}
	return p.CreatedAt
}

// ListContactPhotos returns the aggregated, newest-first photo list for one
// contact. It is a read-only in-app view: every image the owner has is
// returned regardless of sensitivity (the owner's own surface, like the contact
// map). Pagination is a simple cap for the WIP slice; the ADR's keyset cursor
// is a follow-up.
func ListContactPhotos(ctx context.Context, db *gorm.DB, userID uint, contact *models.Contact, limit int) ([]ContactPhoto, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var photos []ContactPhoto

	var own []models.ContactImage
	if err := db.WithContext(ctx).
		Where("user_id = ? AND contact_vcard_uid = ?", userID, contact.VCardUID).
		Find(&own).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to retrieve photos").WithError(err)
	}
	for _, img := range own {
		photos = append(photos, contactPhoto("contact", contact.ID, "Contact", img.ID, img.UUID, img.MIMEType, img.Width, img.Height, img.SizeBytes, img.Sensitivity, img.TakenAt, img.TakenDate, img.CreatedAt))
	}

	var notes []models.NoteImage
	if err := db.WithContext(ctx).
		Joins("JOIN notes ON notes.id = note_images.note_id").
		Where("note_images.user_id = ? AND notes.user_id = ? AND notes.contact_id = ? AND notes.deleted_at IS NULL", userID, userID, contact.ID).
		Find(&notes).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to retrieve note photos").WithError(err)
	}
	for _, img := range notes {
		photos = append(photos, contactPhoto("note", img.NoteID, "Note", img.ID, img.UUID, img.MIMEType, img.Width, img.Height, img.SizeBytes, img.Sensitivity, img.TakenAt, img.TakenDate, img.CreatedAt))
	}

	var activities []models.ActivityImage
	if err := db.WithContext(ctx).
		Joins("JOIN activities ON activities.id = activity_images.activity_id AND activities.deleted_at IS NULL").
		Joins("JOIN activity_contacts ON activity_contacts.activity_id = activity_images.activity_id").
		Where("activity_images.user_id = ? AND activity_contacts.contact_id = ?", userID, contact.ID).
		Find(&activities).Error; err != nil {
		return nil, apperrors.ErrDatabase("Failed to retrieve activity photos").WithError(err)
	}
	labels, err := activityLabels(ctx, db, userID, activities)
	if err != nil {
		return nil, apperrors.ErrDatabase("Failed to retrieve activity labels").WithError(err)
	}
	for _, img := range activities {
		label := labels[img.ActivityID]
		if label == "" {
			label = "Activity"
		}
		photos = append(photos, contactPhoto("activity", img.ActivityID, label, img.ID, img.UUID, img.MIMEType, img.Width, img.Height, img.SizeBytes, img.Sensitivity, img.TakenAt, img.TakenDate, img.CreatedAt))
	}

	// Newest first; kind then image id break ties so the order is stable
	// across requests regardless of query row order.
	sort.Slice(photos, func(i, j int) bool {
		ti, tj := photos[i].sortTime(), photos[j].sortTime()
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		if photos[i].Kind != photos[j].Kind {
			return photos[i].Kind < photos[j].Kind
		}
		return photos[i].ImageID < photos[j].ImageID
	})
	if len(photos) > limit {
		photos = photos[:limit]
	}
	return photos, nil
}

func contactPhoto(kind string, sourceID uint, label string, imageID uint, uuid, mime string, width, height int, size int64, sensitivity string, takenAt *time.Time, takenDate *string, createdAt time.Time) ContactPhoto {
	return ContactPhoto{
		Kind:        kind,
		SourceID:    sourceID,
		SourceLabel: label,
		ImageID:     imageID,
		UUID:        uuid,
		MIMEType:    mime,
		Width:       width,
		Height:      height,
		SizeBytes:   size,
		Sensitivity: sensitivity,
		TakenAt:     takenAt,
		TakenDate:   takenDate,
		CreatedAt:   createdAt,
	}
}

func activityLabels(ctx context.Context, db *gorm.DB, userID uint, images []models.ActivityImage) (map[uint]string, error) {
	ids := make([]uint, 0, len(images))
	for _, img := range images {
		ids = append(ids, img.ActivityID)
	}
	if len(ids) == 0 {
		return map[uint]string{}, nil
	}
	var activities []models.Activity
	if err := db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, ids).Find(&activities).Error; err != nil {
		return nil, err
	}
	labels := make(map[uint]string, len(activities))
	for _, a := range activities {
		labels[a.ID] = a.Title
	}
	return labels, nil
}
