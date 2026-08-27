package services

// Issue #512: end-to-end hostile-input coverage for the live CardDAV
// reconcile path -- services/contact_sync_service.go's reconcileContactSync
// and SyncSubscription. A CardDAV server is a second, untrusted ingestion
// point for exactly the same hostile vCard bytes the import assistant
// already guards against (issue #375's controllers/hostile_input_e2e_test.go,
// issue #416's services/import_sanitize_test.go); before this file, nothing
// exercised that path end-to-end with hostile content. Uses
// database.InitDB (real migrated schema) rather than AutoMigrate, per the
// issue's "real DB" requirement -- contact_sync_service_test.go's other
// direct-reconcile tests use AutoMigrate for convenience, but a schema-drift
// bug (see CLAUDE.md's ContactSyncLink.ETag/HouseholdMember.MemberVCardUID
// traps) is exactly the kind of thing hostile-input coverage should not be
// able to miss.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// hostileTestCard builds a vcard.Card field-by-field rather than through
// parseTestCard's text-decode round trip, so a payload can carry raw control
// bytes or invalid UTF-8 exactly as given -- reconcileContactSync's own
// vcard.NewEncoder(&buf).Encode(obj.Card) re-serializes it to text before
// adapter.Import parses it back, so the hostile bytes still have to survive
// that encode/decode round trip, just as they would over a real CardDAV
// response.
func hostileTestCard(uid, fn, familyName, givenName, note string) vcard.Card {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	card.SetValue(vcard.FieldUID, uid)
	card.SetValue(vcard.FieldFormattedName, fn)
	card.SetValue(vcard.FieldName, familyName+";"+givenName+";;;")
	if note != "" {
		card.SetValue(vcard.FieldNote, note)
	}
	return card
}

// realMigratedContactSyncDB opens a fresh database.InitDB-migrated database
// (the real hand-written migration SQL, not AutoMigrate) in a per-test temp
// file.
func realMigratedContactSyncDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	return db
}

// --- Control characters / invalid UTF-8: reconcileContactSync must apply
// the same fix-up the VCF/CSV import assistant does (issue #416's
// SanitizeImportedContact), not just the REST/import paths. ---

func TestReconcileContactSync_ControlCharactersAndInvalidUTF8_Sanitized(t *testing.T) {
	db := realMigratedContactSyncDB(t, "sync-hostile-controlchars.db")
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	// 0x01/0x02 are C0 control characters (dropped, not replaced); 0xff is
	// not valid standalone UTF-8 (replaced with U+FFFD) -- the exact same
	// two byte-level hostilities services/import_sanitize_test.go's
	// TestBuildContactFromRow_HostileControlCharactersAndInvalidUTF8_ImportsCleanly
	// pins for the CSV/VCF import path.
	card := hostileTestCard("hostile-uid-1", "Ada\x01Lovelace", "Lovelace\xff", "Ada\x02", "")
	obj := carddav.AddressObject{Path: "/addressbooks/test/hostile1.vcf", ETag: "\"e1\"", Card: card}

	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{obj}, nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{Created: 1}, stats)

	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, obj.Path).First(&link).Error)
	var contact models.Contact
	require.NoError(t, db.First(&contact, link.ContactID).Error)

	assert.Equal(t, "Ada", contact.Firstname, "the C0 control character must be stripped, not stored")
	assert.Equal(t, "Lovelace�", contact.Lastname, "the invalid UTF-8 byte must be replaced with U+FFFD, not stored raw")
	assert.False(t, strings.ContainsRune(contact.Firstname, 0x01))
	assert.False(t, strings.ContainsRune(contact.Lastname, 0xff))
}

// TestReconcileContactSync_ControlCharactersSanitizedOnUpdateToo pins the
// same fix-up on the update branch (a remote edit to an already-synced
// contact), which is a separate code path from the create branch above.
func TestReconcileContactSync_ControlCharactersSanitizedOnUpdateToo(t *testing.T) {
	db := realMigratedContactSyncDB(t, "sync-hostile-controlchars-update.db")
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	href := "/addressbooks/test/hostile2.vcf"
	clean := hostileTestCard("hostile-uid-2", "Bob Builder", "Builder", "Bob", "")
	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{{Path: href, ETag: "\"e1\"", Card: clean}}, nil, false, "")
	require.NoError(t, err)

	hostile := hostileTestCard("hostile-uid-2", "Bob\x00Builder", "Builder\x1b", "Bob", "")
	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{{Path: href, ETag: "\"e2\"", Card: hostile}}, nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats)

	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, href).First(&link).Error)
	var contact models.Contact
	require.NoError(t, db.First(&contact, link.ContactID).Error)
	assert.Equal(t, "Bob", contact.Firstname)
	assert.Equal(t, "Builder", contact.Lastname)
}

// --- HTML/script in a free-text field: this repo's established bar
// (services/import_sanitize_test.go's
// TestBuildContactFromRow_HTMLScriptInFreeTextField_StoredLiterallyNotStripped)
// is "stored verbatim, not stripped" -- the frontend never renders free text
// via dangerouslySetInnerHTML or a markdown pass, so the payload is inert on
// display. This proves the CardDAV sync path holds to the same bar rather
// than mangling it, crashing, or (worse) somehow unescaping it further. ---

func TestReconcileContactSync_HTMLScriptInFreeTextField_StoredLiterallyNotStripped(t *testing.T) {
	db := realMigratedContactSyncDB(t, "sync-hostile-html.db")
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	const scriptPayload = "<script>alert(1)</script>"
	const imgPayload = "<img src=x onerror=alert(1)>"
	card := hostileTestCard("hostile-uid-html", scriptPayload, scriptPayload, "X", imgPayload)
	obj := carddav.AddressObject{Path: "/addressbooks/test/hostile-html.vcf", ETag: "\"e1\"", Card: card}

	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{obj}, nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{Created: 1}, stats)

	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, obj.Path).First(&link).Error)
	var contact models.Contact
	require.NoError(t, db.First(&contact, link.ContactID).Error)

	assert.Equal(t, scriptPayload, contact.Lastname, "HTML/script in free text is stored verbatim, matching the import-assistant bar (issue #416) -- the frontend renders it inert")
	require.Len(t, contact.Card.Notes, 1)
	assert.Equal(t, imgPayload, contact.Card.Notes[0].Note, "NOTE has no flat-field home and is never sanitized on any import path -- it must still round-trip byte-for-byte, not be corrupted or double-escaped")
}

// --- Duplicate/colliding UID: a malicious or careless remote CardDAV
// server offers a *new* address-object path whose UID collides with a
// contact this user already owns (created outside this subscription). The
// database's own idx_contacts_vcard_uid_user unique index
// (database/migrations/000001_initial_schema.up.sql) is the actual guard
// here; this test pins that reconcileContactSync surfaces that as a
// contained sync error rather than silently corrupting the existing
// contact or the rest of the batch. ---

func TestReconcileContactSync_DuplicateUIDCollidesWithExistingContact_RejectedWithoutCorruption(t *testing.T) {
	db := realMigratedContactSyncDB(t, "sync-hostile-dupuid.db")
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	existing := models.Contact{UserID: user.ID, VCardUID: "collided-uid", Firstname: "Original", Lastname: "Owner"}
	require.NoError(t, db.Create(&existing).Error)

	hostile := hostileTestCard("collided-uid", "Impostor", "Impostor", "Evil", "")
	obj := carddav.AddressObject{Path: "/addressbooks/test/impostor.vcf", ETag: "\"e1\"", Card: hostile}

	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{obj}, nil, false, "")
	require.Error(t, err, "a colliding UID must not silently create a second contact")

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "only the original contact should exist")

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	assert.Equal(t, "Original", reloaded.Firstname, "the pre-existing contact must be untouched")
	assert.Equal(t, "Owner", reloaded.Lastname)

	var linkCount int64
	require.NoError(t, db.Model(&models.ContactSyncLink{}).Where("subscription_id = ?", sub.ID).Count(&linkCount).Error)
	assert.EqualValues(t, 0, linkCount, "the failed create must not leave a dangling sync link")
}

// --- Sensitivity: reconcileContactSync's full-replace of the flat Contact
// fields must not touch the separate FieldValue/RelationshipEdge tables
// that carry the normal/private/secret sensitivity marker (models/
// field_definition.go, models/relationship_edge.go) -- a hostile remote
// update must not be able to wipe, downgrade, or otherwise disturb data
// that's gated above "normal" sensitivity, the same guarantee the export
// path already gives (models/contact_record.go's RecordForContact family
// filters to sensitivity=normal by default). ---

func TestReconcileContactSync_SensitiveFieldValueAndRelationshipEdgeSurviveHostileRemoteUpdate(t *testing.T) {
	db := realMigratedContactSyncDB(t, "sync-hostile-sensitivity.db")
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	href := "/addressbooks/test/secretive.vcf"
	original := hostileTestCard("secretive-uid", "Grace Hopper", "Hopper", "Grace", "")
	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{{Path: href, ETag: "\"e1\"", Card: original}}, nil, false, "")
	require.NoError(t, err)

	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, href).First(&link).Error)
	var contact models.Contact
	require.NoError(t, db.First(&contact, link.ContactID).Error)

	friend := models.Contact{UserID: user.ID, Firstname: "Friend"}
	require.NoError(t, db.Create(&friend).Error)

	def := models.FieldDefinition{
		UserID:      user.ID,
		Label:       "Secret note",
		Key:         "secret_note_512",
		Target:      models.FieldDefinitionTargetContact,
		Type:        models.FieldTypeString,
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&def).Error)
	fv := models.FieldValue{
		FieldDefinitionID: def.ID,
		UserID:            user.ID,
		EntityID:          contact.VCardUID,
		Value:             json.RawMessage(`"do not leak this"`),
	}
	require.NoError(t, db.Create(&fv).Error)

	edge := models.RelationshipEdge{
		UserID:      user.ID,
		SourceID:    contact.VCardUID,
		TargetID:    friend.VCardUID,
		Type:        "friend_of",
		Directional: false,
		Source:      "user-confirmed",
		Confidence:  1,
		Status:      models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&edge).Error)

	// A remote update to an unrelated field -- exactly the shape a
	// compromised/malicious CardDAV server would send to try to ride along
	// with a legitimate-looking sync.
	updated := hostileTestCard("secretive-uid", "Grace Hopper", "Hopper", "Grace M.", "")
	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{{Path: href, ETag: "\"e2\"", Card: updated}}, nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats)

	var reloadedFV models.FieldValue
	require.NoError(t, db.First(&reloadedFV, fv.ID).Error)
	assert.JSONEq(t, `"do not leak this"`, string(reloadedFV.Value))

	var reloadedDef models.FieldDefinition
	require.NoError(t, db.First(&reloadedDef, "id = ?", def.ID).Error)
	assert.Equal(t, models.RelationshipSensitivitySecret, reloadedDef.Sensitivity)

	var reloadedEdge models.RelationshipEdge
	require.NoError(t, db.First(&reloadedEdge, "id = ?", edge.ID).Error)
	assert.Equal(t, models.RelationshipSensitivitySecret, reloadedEdge.Sensitivity)
	assert.Equal(t, models.RelationshipStatusConfirmed, reloadedEdge.Status)
}

// --- Size limit: pin that maxContactResponseBytes (contact_sync_service.go)
// is actually wired into the live HTTP path, not just present as a constant.
// A CardDAV response far larger than any legitimate address book is
// refused rather than fully buffered/parsed. ---

func TestSyncSubscription_OversizedResponse_RejectedNotSilentlyAccepted(t *testing.T) {
	// One property value alone exceeds maxContactResponseBytes (20MiB) --
	// large enough that only the size guard, not coincidence, can be
	// refusing it.
	huge := strings.Repeat("A", 21<<20)
	cardText := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:huge-uid\r\nFN:Huge\r\nNOTE:" + huge + "\r\nEND:VCARD\r\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		writeOversizedMultistatus(w, cardText)
	}))
	defer server.Close()

	db := realMigratedContactSyncDB(t, "sync-hostile-oversized.db")
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, server.URL+"/addressbooks/test/", "", "")

	service := NewContactSyncService(false)
	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.Error(t, err, "an oversized response must not be silently accepted")

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.EqualValues(t, 0, count, "no contact must be created from a response the size guard refused")

	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, models.ContactSyncStatusError, sub.LastSyncStatus)
}

// writeOversizedMultistatus writes a minimal multistatus response embedding a
// single address object -- a free function (not addressMultistatusResponse,
// whose XML-escaping pass over a 21MiB string would be wasteful and is
// unneeded here since the payload has no XML-special characters).
func writeOversizedMultistatus(w http.ResponseWriter, cardText string) {
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>` + "\n" +
		`<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">` + "\n" +
		`<d:response><d:href>/addressbooks/test/huge.vcf</d:href>` +
		`<d:propstat><d:prop><card:address-data>` + cardText + `</card:address-data>` +
		`<d:getetag>&quot;huge-etag&quot;</d:getetag></d:prop>` +
		`<d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>` +
		`</d:multistatus>`))
}
