package models

import (
	"testing"

	"mycorrhizal/contactmodel"
)

// TestNewContactRecordResponse_PreservesPersistedCardOnlyData is the
// regression test for a real, live bug (found while auditing WP-73's work):
// NewContactRecordResponse called RecordFromContact directly, which
// silently drops any Card-only data with no flat-field home (SpeakToAs
// here) from GET /api/v1/contacts/{id} and the POST/PUT response bodies —
// exactly the data a nested REST write would have just set. It must go
// through RecordForContact instead, which prefers the already-persisted
// Card. See models.RecordForContact's doc comment for the full history.
func TestNewContactRecordResponse_PreservesPersistedCardOnlyData(t *testing.T) {
	c := &Contact{
		Firstname: "Ada",
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			SpeakToAs: &contactmodel.SpeakToAs{
				Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}},
			},
		},
	}

	resp := NewContactRecordResponse(c, "", nil)

	if resp.Card.SpeakToAs == nil || len(resp.Card.SpeakToAs.Pronouns) != 1 || resp.Card.SpeakToAs.Pronouns[0].Pronouns != "she/her" {
		t.Errorf("ContactRecordResponse.Card.SpeakToAs = %+v, want the persisted she/her preserved in the API response", resp.Card.SpeakToAs)
	}
}

// TestNewContactSummary_IncludesNickname is a regression test for the
// frontend-migration pre-work gap: ContactsPage's list view renders nickname
// per row, but GET /contacts' slim ContactSummary projection didn't carry it
// until this fix.
//
// T108: this test alone never caught that GET /contacts actually shipped an
// empty nickname on every real request — it exercises NewContactSummary
// directly against a hand-built Contact, never the controller's fixed
// contactSummaryColumns Select that GORM actually runs, which is exactly
// where the real bug lived (nickname was never in that column list, so GORM
// silently left Contact.Nickname at its zero value on every real query, no
// matter how correct NewContactSummary's own mapping was). See
// contact_controller_test.go's raw-JSON pin for the test that actually
// covers the query layer.
//
// Circles was removed from ContactSummary entirely as part of the same fix
// (see the doc comment on ContactSummary.Circles' old field, and
// contact_controller.go's contactSummaryColumns): it was never selected
// either, and even if it had been, Contact.Circles is the legacy flat column
// T2/T3 superseded with circle_members, so populating it would have shipped
// stale data rather than nothing.
func TestNewContactSummary_IncludesNickname(t *testing.T) {
	c := &Contact{
		Firstname: "Ada",
		Lastname:  "Lovelace",
		Nickname:  "Countess",
	}

	summary := NewContactSummary(c)

	if summary.Nickname != "Countess" {
		t.Errorf("ContactSummary.Nickname = %q, want %q", summary.Nickname, "Countess")
	}
}
