package controllers

// Issue #555: the cross-user contact-share matrix. #444 (DATA-04) proved the
// sensitivity gating in models.RecordForContactFiltered holds for a
// single-user export; ContactShare runs the exact same FieldSelection/
// projection machinery (contact_share_controller.go:69-78) but the payload
// leaves the sender's account, so this file asserts the same guarantees
// hold when a second, real user is the intended reader — inspecting the
// stored Payload column directly (decodeSharePayloadCard, from
// contact_share_controller_test.go), never the API response shape.
//
// Three surfaces carry a sensitivity dimension above "normal"
// (models.IsSensitiveSection / models.sensitiveSections): RelationshipEdge
// (-> Card.RelatedTo), Preference of category hobby (-> Card.PersonalInfo),
// and a FieldDefinition/FieldValue projected via "vcard:X-..." (->
// Passthrough.VCard, exported as vCardProps in JSContact). Each is exercised
// at sensitivity private and secret, with IncludeSensitive off and on, with
// its own section selected -- the same shape as export_controller_test.go's
// TestExportContactsAsVCF_SecretCustomField_ExcludedByDefault_IncludedWithOptIn,
// just against the share pipeline instead of the export endpoint.

import (
	"encoding/json"
	"net/http"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// sensitiveSurfaceFixture seeds one of the three sensitivity-bearing
// surfaces at the given sensitivity, touching contact (the contact being
// shared) and, where the surface needs a second party, other. Returns the
// JSContact key the payload carries the data under, so the caller can
// Contains/NotContains against decodeSharePayloadCard's result.
type sensitiveSurfaceFixture struct {
	name         string
	section      string
	jscontactKey string
	seed         func(t *testing.T, db *gorm.DB, userID uint, contact, other models.Contact, sensitivity string)
}

var sensitiveSurfaceFixtures = []sensitiveSurfaceFixture{
	{
		name:         "relationship_edge",
		section:      models.SectionRelatedTo,
		jscontactKey: "relatedTo",
		seed: func(t *testing.T, db *gorm.DB, userID uint, contact, other models.Contact, sensitivity string) {
			t.Helper()
			require.NoError(t, db.Create(&models.RelationshipEdge{
				UserID: userID, SourceID: contact.VCardUID, TargetID: other.VCardUID, Type: "friend_of",
				Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
				Sensitivity: sensitivity,
			}).Error)
		},
	},
	{
		name:         "hobby_preference",
		section:      models.SectionPersonalInfo,
		jscontactKey: "personalInfo",
		seed: func(t *testing.T, db *gorm.DB, userID uint, contact, other models.Contact, sensitivity string) {
			t.Helper()
			require.NoError(t, db.Create(&models.Preference{
				UserID: userID, EntityID: contact.VCardUID, Category: models.PreferenceCategoryHobby,
				Value: "underwater basket weaving", Sensitivity: sensitivity,
			}).Error)
		},
	},
	{
		name:         "custom_field",
		section:      models.SectionCustomFields,
		jscontactKey: "vCardProps",
		seed: func(t *testing.T, db *gorm.DB, userID uint, contact, other models.Contact, sensitivity string) {
			t.Helper()
			def := models.FieldDefinition{
				UserID: userID, Label: "Secret Field", Key: "secret_field_" + sensitivity, Target: "contact",
				Type: "string", Projection: "vcard:X-SECRET-FIELD", Sensitivity: sensitivity,
			}
			require.NoError(t, db.Create(&def).Error)
			require.NoError(t, db.Create(&models.FieldValue{
				FieldDefinitionID: def.ID, UserID: userID, EntityID: contact.VCardUID,
				Value: json.RawMessage(`"top secret value"`),
			}).Error)
		},
	},
}

// TestCreateContactShare_SensitivityMatrix is the ticket's core matrix:
// surface x sensitivity x IncludeSensitive, asserted against the raw stored
// payload. A cell where the sensitive item leaks with IncludeSensitive=false
// is a real cross-user data leak, not just a single-account export bug.
func TestCreateContactShare_SensitivityMatrix(t *testing.T) {
	for _, surface := range sensitiveSurfaceFixtures {
		for _, sensitivity := range []string{models.RelationshipSensitivityPrivate, models.RelationshipSensitivitySecret} {
			for _, includeSensitive := range []bool{false, true} {
				t.Run(surface.name+"_"+sensitivity+"_includeSensitive="+boolLabel(includeSensitive), func(t *testing.T) {
					db, cfg, sender, recipient, _ := newContactShareFixtures(t)

					contact := models.Contact{UserID: sender.ID, Firstname: "Alice", Lastname: "Anderson"}
					require.NoError(t, db.Create(&contact).Error)
					other := models.Contact{UserID: sender.ID, Firstname: "Bob", Lastname: "Brown"}
					require.NoError(t, db.Create(&other).Error)

					surface.seed(t, db, sender.ID, contact, other, sensitivity)

					router := contactShareRouterFor(db, cfg, sender.ID)
					w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
						ToUserID: recipient.ID, VCardUID: contact.VCardUID,
						Sections: []string{surface.section}, IncludeSensitive: includeSensitive,
					})
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())

					var share models.ContactShare
					require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", sender.ID, recipient.ID).First(&share).Error)
					card := decodeSharePayloadCard(t, share.Payload)

					if includeSensitive {
						assert.Contains(t, card, surface.jscontactKey,
							"%s at sensitivity=%s must be present in the stored payload with IncludeSensitive=true", surface.name, sensitivity)
					} else {
						assert.NotContains(t, card, surface.jscontactKey,
							"%s at sensitivity=%s must NOT be present in the stored payload without explicit opt-in -- a cross-user leak, not just a single-account one", surface.name, sensitivity)
					}
				})
			}
		}
	}
}

// TestCreateContactShare_NormalSensitivityAlwaysCrossesWithoutOptIn is the
// matrix's control cell: normal-sensitivity data in all three surfaces must
// reach the recipient's payload even with IncludeSensitive=false, since
// "normal" is the default, always-shareable classification. Without this
// case, an over-broad fix to the leak tests above (e.g. gating the whole
// section on IncludeSensitive) would pass every leak test while silently
// breaking ordinary sharing.
func TestCreateContactShare_NormalSensitivityAlwaysCrossesWithoutOptIn(t *testing.T) {
	for _, surface := range sensitiveSurfaceFixtures {
		t.Run(surface.name, func(t *testing.T) {
			db, cfg, sender, recipient, _ := newContactShareFixtures(t)

			contact := models.Contact{UserID: sender.ID, Firstname: "Alice", Lastname: "Anderson"}
			require.NoError(t, db.Create(&contact).Error)
			other := models.Contact{UserID: sender.ID, Firstname: "Bob", Lastname: "Brown"}
			require.NoError(t, db.Create(&other).Error)

			surface.seed(t, db, sender.ID, contact, other, models.RelationshipSensitivityNormal)

			router := contactShareRouterFor(db, cfg, sender.ID)
			w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
				ToUserID: recipient.ID, VCardUID: contact.VCardUID,
				Sections: []string{surface.section}, IncludeSensitive: false,
			})
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var share models.ContactShare
			require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", sender.ID, recipient.ID).First(&share).Error)
			card := decodeSharePayloadCard(t, share.Payload)
			assert.Contains(t, card, surface.jscontactKey, "%s at sensitivity=normal must cross without any opt-in", surface.name)
		})
	}
}

// TestCreateContactShare_SectionSelectionAloneCannotImplyOptIn is the
// foot-gun guard #444 built for single-user export, asserted here across
// all three sensitive surfaces at once: selecting a section that CAN carry
// sensitive data is not the same act as opting into IncludeSensitive, and
// nothing about section selection can imply it. Matters more for sharing
// than for export because the data leaves the account entirely.
func TestCreateContactShare_SectionSelectionAloneCannotImplyOptIn(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)

	contact := models.Contact{UserID: sender.ID, Firstname: "Alice", Lastname: "Anderson"}
	require.NoError(t, db.Create(&contact).Error)
	other := models.Contact{UserID: sender.ID, Firstname: "Bob", Lastname: "Brown"}
	require.NoError(t, db.Create(&other).Error)

	allSections := make([]string, 0, len(sensitiveSurfaceFixtures))
	for _, surface := range sensitiveSurfaceFixtures {
		surface.seed(t, db, sender.ID, contact, other, models.RelationshipSensitivitySecret)
		allSections = append(allSections, surface.section)
	}

	router := contactShareRouterFor(db, cfg, sender.ID)
	// Every sensitive section explicitly selected, but IncludeSensitive left
	// false -- must still exclude every secret item.
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID, VCardUID: contact.VCardUID, Sections: allSections, IncludeSensitive: false,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var share models.ContactShare
	require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", sender.ID, recipient.ID).First(&share).Error)
	card := decodeSharePayloadCard(t, share.Payload)
	for _, surface := range sensitiveSurfaceFixtures {
		assert.NotContains(t, card, surface.jscontactKey,
			"selecting section %q must not itself enable sensitive data (%s) -- IncludeSensitive is a separate, deliberate flag", surface.section, surface.name)
	}
}

// TestCreateContactShare_PayloadFrozenAtCreation pins property 2 from the
// ticket: the Payload is serialized once and never re-derived. Adding new
// (even normal-sensitivity) data to the source contact after the share is
// created must not retroactively appear in the already-created share --
// documented behavior (models/contact_share.go's Payload doc comment), now
// pinned by a real test rather than only asserted in a comment.
func TestCreateContactShare_PayloadFrozenAtCreation(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)

	contact := models.Contact{UserID: sender.ID, Firstname: "Alice", Lastname: "Anderson", Email: "alice@example.com"}
	require.NoError(t, db.Create(&contact).Error)

	router := contactShareRouterFor(db, cfg, sender.ID)
	w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: recipient.ID, VCardUID: contact.VCardUID, Sections: []string{models.SectionEmails, models.SectionPhones},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var share models.ContactShare
	require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", sender.ID, recipient.ID).First(&share).Error)
	originalPayload := share.Payload

	// Mutate the source contact after the share already exists: add a phone
	// number (a selected section) and reclassify nothing -- either way, the
	// already-created share must not change.
	contact.Phone = "555-9999"
	require.NoError(t, db.Save(&contact).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: sender.ID, SourceID: contact.VCardUID, TargetID: contact.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	var reloaded models.ContactShare
	require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
	assert.Equal(t, originalPayload, reloaded.Payload, "Payload must be frozen at creation time and never re-derived")

	card := decodeSharePayloadCard(t, reloaded.Payload)
	assert.NotContains(t, card, "phones", "a phone added after share-creation must not retroactively appear")
}

// TestContactShare_PayloadUnchangedAcrossLifecycleTransitions covers the
// pending -> accepted and pending -> declined transitions: neither
// AcceptContactShare/ConfirmContactShare nor DeclineContactShare ever
// rewrites Payload, so what the recipient reviewed at accept-time is
// provably what was recorded at creation-time.
func TestContactShare_PayloadUnchangedAcrossLifecycleTransitions(t *testing.T) {
	t.Run("declined", func(t *testing.T) {
		db, cfg, sender, recipient, _ := newContactShareFixtures(t)
		share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)
		original := share.Payload

		recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
		w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/decline", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var reloaded models.ContactShare
		require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
		assert.Equal(t, original, reloaded.Payload)
	})

	t.Run("accepted", func(t *testing.T) {
		db, cfg, sender, recipient, _ := newContactShareFixtures(t)
		share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)
		original := share.Payload

		recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
		w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var preview models.ImportPreviewResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))

		w = doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/confirm", models.ImportConfirmRequest{
			SessionID: preview.SessionID,
			Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var reloaded models.ContactShare
		require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
		assert.Equal(t, original, reloaded.Payload)
	})
}

// TestAcceptDeclineConfirmContactShare_SenderCannotActOnOwnOutgoingShare is
// the sender/recipient asymmetry the ticket calls out explicitly: the
// getPendingShareForRecipient helper AND-scopes on to_user_id, so the
// SENDER of their own outgoing share must get the same 404 an uninvolved
// third party gets -- a sender has no more standing to accept/decline/
// confirm their own offer than anyone else does, and (per the same 404, not
// 403, discipline) cannot use the error status to learn anything about the
// row's existence beyond what the outbox listing already tells them.
func TestAcceptDeclineConfirmContactShare_SenderCannotActOnOwnOutgoingShare(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)

	senderRouter := contactShareRouterFor(db, cfg, sender.ID)

	w := doJSON(t, senderRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "sender must not be able to accept their own outgoing share")

	w = doJSON(t, senderRouter, "POST", "/contact-shares/"+share.ID+"/decline", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "sender must not be able to decline their own outgoing share")

	w = doJSON(t, senderRouter, "POST", "/contact-shares/"+share.ID+"/confirm", models.ImportConfirmRequest{
		SessionID: "nonexistent-session",
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	})
	assert.Equal(t, http.StatusNotFound, w.Code, "sender must not be able to confirm their own outgoing share")

	// The share itself must still be untouched and pending.
	var reloaded models.ContactShare
	require.NoError(t, db.First(&reloaded, "id = ?", share.ID).Error)
	assert.Equal(t, models.ContactShareStatusPending, reloaded.Status)
}

// TestConfirmContactShare_AcceptedContactBecomesOrdinaryAndCanBeReShared
// establishes and pins the rule the ticket asks for in its recommended
// action 6: once a share is accepted, the recipient's copy is an ordinary
// Contact in their own account -- subject to their OWN sensitivity
// classifications, not the sender's, and freely usable like any other
// contact they own, including re-sharing it onward to a third party. There
// is no provenance flag or restriction carried over from the original
// share; the payload becoming a Contact IS the handoff of ownership.
func TestConfirmContactShare_AcceptedContactBecomesOrdinaryAndCanBeReShared(t *testing.T) {
	db, cfg, sender, recipient, third := newContactShareFixtures(t)
	share := createPendingShareFixture(t, db, cfg, sender.ID, recipient.ID, "Alice", []string{models.SectionEmails}, false)

	recipientRouter := contactShareRouterFor(db, cfg, recipient.ID)
	w := doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/accept", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var preview models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))

	w = doJSON(t, recipientRouter, "POST", "/contact-shares/"+share.ID+"/confirm", models.ImportConfirmRequest{
		SessionID: preview.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var landedContact models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", recipient.ID, "Alice").First(&landedContact).Error)

	// The recipient re-shares their new contact onward to a third user --
	// exactly the same CreateContactShare path as any other contact they own.
	w = doJSON(t, recipientRouter, "POST", "/contact-shares", models.ContactShareInput{
		ToUserID: third.ID, VCardUID: landedContact.VCardUID, Sections: []string{models.SectionEmails},
	})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String(), "an accepted share's contact must be re-shareable like any other owned contact")

	var reShare models.ContactShare
	require.NoError(t, db.Where("from_user_id = ? AND to_user_id = ?", recipient.ID, third.ID).First(&reShare).Error)
	assert.Equal(t, models.ContactShareStatusPending, reShare.Status)
}

func boolLabel(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
