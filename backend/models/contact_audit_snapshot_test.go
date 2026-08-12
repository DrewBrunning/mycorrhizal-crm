package models

import (
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/contactmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAudit_ContactUpdateSnapshotCapturesNestedData pins T82 at the model
// layer: a contact update that changes only nested data (pronouns here) must
// produce an update event whose before-snapshot contains the pre-edit
// Card/CRM/Passthrough — before T82, Card/CRM/Passthrough were json:"-" on
// Contact, so json.Marshal omitted them and the snapshot showed nothing.
func TestAudit_ContactUpdateSnapshotCapturesNestedData(t *testing.T) {
	db := newAuditTestDB(t)
	user := User{Username: "auditnested", Password: "password123!A", Email: "auditnested@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, richCardOnlyRecord(), "")
	require.NoError(t, db.Create(contact).Error)
	AuditFlush()

	// Edit ONLY nested data: change pronouns; every flat field stays identical.
	var loaded Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	loaded.Card.SpeakToAs = &contactmodel.SpeakToAs{
		Pronouns: []contactmodel.Pronouns{{Pronouns: "they/them"}},
	}
	require.NoError(t, db.Save(&loaded).Error)
	AuditFlush()

	var event AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		AuditEntityContact, contact.VCardUID, AuditOpUpdate).First(&event).Error)
	require.NotEmpty(t, event.BeforeSnapshot)

	var snap ContactAuditSnapshot
	require.NoError(t, json.Unmarshal([]byte(event.BeforeSnapshot), &snap))
	require.True(t, snap.HasNested(), "the before snapshot must carry the nested columns")
	require.NotNil(t, snap.Card.SpeakToAs, "the before snapshot must carry the pre-edit SpeakToAs")
	assert.Equal(t, "she/her", snap.Card.SpeakToAs.Pronouns[0].Pronouns,
		"the before snapshot must capture the PRE-edit pronouns, not the post-edit they/them")
	assert.Equal(t, "pet", snap.CRM.Kind, "the before snapshot must carry the CRM envelope kind")
	assert.Equal(t, "X-CUSTOM", snap.Passthrough.VCard[0].Name,
		"the before snapshot must carry the imported passthrough property")

	// The persisted row, by contrast, has the post-edit pronouns.
	var persisted Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	require.Equal(t, "they/them", persisted.Card.SpeakToAs.Pronouns[0].Pronouns)
}

// TestAudit_ContactSnapshotStripsPhotoMedia pins the T82 storage decision: the
// snapshot's nested Card has photo media entries removed (the photo is flat-
// owned — Contact.Photo/PhotoThumbnail and the on-disk file — and embedding a
// multi-KB base64 image in every event would dominate audit_events growth),
// while non-photo media (logo/sound, which have no flat home) survive, and the
// persisted Card is untouched.
func TestAudit_ContactSnapshotStripsPhotoMedia(t *testing.T) {
	c := &Contact{
		Card: contactmodel.Card{Media: []contactmodel.Resource{
			{Kind: "photo", URI: "data:image/jpeg;base64,AAAA", MediaType: "image/jpeg"},
			{Kind: "logo", URI: "data:image/png;base64,BBBB", MediaType: "image/png"},
			{Kind: "sound", URI: "data:audio/ogg;base64,CCCC", MediaType: "audio/ogg"},
		}},
	}

	snap, ok := c.auditSnapshot().(ContactAuditSnapshot)
	require.True(t, ok)
	require.Len(t, snap.Card.Media, 2, "photo must be stripped from the snapshot card")
	assert.Equal(t, "logo", snap.Card.Media[0].Kind)
	assert.Equal(t, "sound", snap.Card.Media[1].Kind)

	require.Len(t, c.Card.Media, 3, "the persisted card must be untouched by snapshotting")
	assert.Equal(t, "photo", c.Card.Media[0].Kind)
}

// TestAudit_SnapshotRedactionReachesNestedData pins the ticket's item 2: the
// deny-list redaction recurses into the nested card/crm/passthrough objects
// once they are present. A deny-listed key at any depth of the nested Card is
// stripped; innocent sibling keys survive.
func TestAudit_SnapshotRedactionReachesNestedData(t *testing.T) {
	snap := ContactAuditSnapshot{
		Contact: Contact{Firstname: "Ada"},
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			Localizations: map[string]json.RawMessage{
				"en": json.RawMessage(`{"password":"hunter2","ok":1}`),
			},
		},
	}

	raw, err := redactedJSONForAudit(&snap)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	assert.Equal(t, "Ada", m["firstname"], "the flat half must be intact")
	card, ok := m["card"].(map[string]interface{})
	require.True(t, ok, "the nested card must be present in the marshaled snapshot")
	l10n, ok := card["localizations"].(map[string]interface{})
	require.True(t, ok)
	en, ok := l10n["en"].(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, en, "password", "deny-listed keys at nested depth must be stripped")
	assert.Equal(t, float64(1), en["ok"])
}

// TestAudit_ContactAuditSnapshotRoundTrip pins the undo-side contract: a
// snapshot marshaled by auditSnapshot unmarshals back into ContactAuditSnapshot
// with the nested columns populated, and the flat fields land on the embedded
// Contact (what undoContact reads). This is the shape the /audit API serves and
// POST /audit/:id/undo consumes.
func TestAudit_ContactAuditSnapshotRoundTrip(t *testing.T) {
	c := &Contact{Firstname: "Ada", Lastname: "Lovelace"}
	c.Card = contactmodel.Card{
		Name:         &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		SpeakToAs:    &contactmodel.SpeakToAs{Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}}},
		PersonalInfo: []contactmodel.PersonalInfo{{Kind: "hobby", Value: "sailing"}},
	}
	c.CRM = contactmodel.CRMEnvelope{Kind: "pet", HowWeMet: "conference"}

	raw, err := redactedJSONForAudit(c)
	require.NoError(t, err)
	require.Contains(t, raw, `"card"`, "the marshaled snapshot must carry the nested card")
	require.Contains(t, raw, `"crm"`)

	var snap ContactAuditSnapshot
	require.NoError(t, json.Unmarshal([]byte(raw), &snap))
	assert.Equal(t, "Ada", snap.Firstname, "flat fields must land on the embedded Contact")
	assert.Equal(t, "she/her", snap.Card.SpeakToAs.Pronouns[0].Pronouns)
	assert.Equal(t, "pet", snap.CRM.Kind)
	require.True(t, snap.HasNested())
}

// TestAudit_ContactDeleteSnapshotCapturesNestedData pins the delete path: a
// contact's delete event also snapshots the nested columns (it is the last
// record of the row after it is gone), not just its flat fields.
func TestAudit_ContactDeleteSnapshotCapturesNestedData(t *testing.T) {
	db := newAuditTestDB(t)
	user := User{Username: "auditdel", Password: "password123!A", Email: "auditdel@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, richCardOnlyRecord(), "")
	require.NoError(t, db.Create(contact).Error)
	require.NoError(t, db.Delete(contact).Error)
	AuditFlush()

	var event AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		AuditEntityContact, contact.VCardUID, AuditOpDelete).First(&event).Error)
	require.NotEmpty(t, event.BeforeSnapshot)

	var snap ContactAuditSnapshot
	require.NoError(t, json.Unmarshal([]byte(event.BeforeSnapshot), &snap))
	require.True(t, snap.HasNested(), "a delete event must carry the nested columns too")
	assert.Equal(t, "she/her", snap.Card.SpeakToAs.Pronouns[0].Pronouns)
	assert.Equal(t, "pet", snap.CRM.Kind)
}

// TestRestoreFullStateFrom_ReDerivesPhotoFromRestoredFlat pins the one
// non-obvious move in the full-restore undo path: snapshots strip photo media
// (see TestAudit_ContactSnapshotStripsPhotoMedia), so restoring one must
// re-derive the photo from the restored flat Photo path via RecordFromContact +
// mergeMedia — the restored card stays consistent with the restored Photo for
// CardDAV/REST readers instead of silently losing the photo.
func TestRestoreFullStateFrom_ReDerivesPhotoFromRestoredFlat(t *testing.T) {
	photoDir := t.TempDir()
	writeTestJPEG(t, filepath.Join(photoDir, "old.jpg"))
	writeTestJPEG(t, filepath.Join(photoDir, "new.jpg"))
	prev := DefaultPhotoDir
	DefaultPhotoDir = photoDir
	t.Cleanup(func() { DefaultPhotoDir = prev })

	// A contact whose persisted card carries the OLD photo, snapshotted the
	// way the audit does (photo stripped).
	before := &Contact{Photo: "old.jpg"}
	snap, ok := before.auditSnapshot().(ContactAuditSnapshot)
	require.True(t, ok)
	require.Len(t, snap.Card.Media, 0, "the snapshot must not carry the photo")

	// The live contact points at the NEW photo path (post-edit).
	current := &Contact{Photo: "new.jpg"}
	current.RestoreFullStateFrom(&snap)

	assert.True(t, current.cardSetDirectly, "full restore must keep the restored nested columns verbatim")
	require.Len(t, current.Card.Media, 1, "the restored card must re-derive the photo from the restored Photo path")
	assert.Equal(t, "photo", current.Card.Media[0].Kind)
	assert.Contains(t, current.Card.Media[0].URI, "data:image/jpeg;base64,",
		"the re-derived media must be the OLD photo (read via the restored path), not the new one")
}

func writeTestJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for x := 0; x < 16; x++ {
		for y := 0; y < 16; y++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, jpeg.Encode(f, img, &jpeg.Options{Quality: 85}))
}
