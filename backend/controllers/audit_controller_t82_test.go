package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUndoAuditEvent_FullFidelityRestoresNestedData pins T82's headline
// guarantee at the handler level: an edit that changes only nested data
// (pronouns here — no flat field differs) produces an update event whose
// before-snapshot carries the nested Card, and undoing it restores the nested
// data exactly. Before T82 the snapshot never carried Card/CRM/Passthrough, so
// undo could only preserve the current value, not revert a nested edit.
func TestUndoAuditEvent_FullFidelityRestoresNestedData(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	models.AuditFlush()

	contact := &models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(contact, richCardOnlyRecordCtrl(), "")
	require.NoError(t, db.Create(contact).Error)
	models.AuditFlush()

	// Change ONLY nested data: she/her -> they/them. Every flat field is
	// identical before and after.
	var loaded models.Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	loaded.Card.SpeakToAs = &contactmodel.SpeakToAs{
		Pronouns: []contactmodel.Pronouns{{Pronouns: "they/them"}},
	}
	require.NoError(t, db.Save(&loaded).Error)
	models.AuditFlush()

	var persisted models.Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	require.Equal(t, "they/them", persisted.Card.SpeakToAs.Pronouns[0].Pronouns,
		"the edit must have landed before we test undo")

	var event models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpUpdate).First(&event).Error)

	// The before-snapshot must carry the PRE-edit nested data.
	var snap models.ContactAuditSnapshot
	require.NoError(t, json.Unmarshal([]byte(event.BeforeSnapshot), &snap))
	require.True(t, snap.HasNested(), "the T82 before-snapshot must carry the nested columns")
	require.NotNil(t, snap.Card.SpeakToAs)
	assert.Equal(t, "she/her", snap.Card.SpeakToAs.Pronouns[0].Pronouns,
		"the snapshot must capture the pre-edit pronouns")

	req, _ := http.NewRequest("POST", "/audit/"+auditItoa(event.ID)+"/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", contact.VCardUID).First(&reloaded).Error)
	require.NotNil(t, reloaded.Card.SpeakToAs)
	assert.Equal(t, "she/her", reloaded.Card.SpeakToAs.Pronouns[0].Pronouns,
		"undo must restore the pre-edit pronouns from the T82 snapshot (full fidelity)")
	assert.Equal(t, "Ada", reloaded.Firstname, "the flat state must be unchanged by this undo")
}

// TestUndoAuditEvent_PreT82SnapshotPreservesNestedData pins T82's item 5: an
// event written before the capture change has no card/crm/passthrough in its
// snapshot, forever. Undo of such an event must keep the T75 behavior —
// restore the flat state, preserve the Card-only data — and must not mistake
// "absent because the event predates the change" for "the user cleared the
// data." The flat-only snapshot is hand-crafted exactly as json.Marshal
// (&Contact) used to produce it.
func TestUndoAuditEvent_PreT82SnapshotPreservesNestedData(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	models.AuditFlush()

	contact := &models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(contact, richCardOnlyRecordCtrl(), "")
	require.NoError(t, db.Create(contact).Error)
	models.AuditFlush()

	// A pre-T82 event would have captured the flat state at some earlier
	// moment — here, the contact's state right now, minus any nested columns
	// (json.Marshal of the loaded Contact is byte-identical to the old shape).
	var currentState models.Contact
	require.NoError(t, db.First(&currentState, contact.ID).Error)
	flatOnly, err := json.Marshal(currentState)
	require.NoError(t, err)
	require.NotContains(t, string(flatOnly), `"card"`, "the crafted snapshot must be flat-only")

	oldEvent := models.AuditEvent{
		EntityType:     models.AuditEntityContact,
		EntityID:       contact.VCardUID,
		Operation:      models.AuditOpUpdate,
		UserID:         user.ID,
		BeforeSnapshot: string(flatOnly),
	}
	require.NoError(t, db.Create(&oldEvent).Error)

	// Now a real edit changes a flat field (and, underneath it, the card).
	contact.Firstname = "Changed"
	require.NoError(t, db.Save(contact).Error)
	models.AuditFlush()

	req, _ := http.NewRequest("POST", "/audit/"+auditItoa(oldEvent.ID)+"/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", contact.VCardUID).First(&reloaded).Error)
	assert.Equal(t, "Ada", reloaded.Firstname, "undo must restore the flat state the old snapshot recorded")
	assertCardOnlyDataPreserved(t, reloaded)
}
