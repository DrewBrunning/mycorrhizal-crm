package models

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"

	"gorm.io/gorm"
)

// TestNewContactRecordResponse_PreservesPersistedCardOnlyData is the
// regression test for a real, live bug (found while auditing work):
// NewContactRecordResponse called RecordFromContact directly, which
// silently drops any Card-only data with no flat-field home (SpeakToAs
// here) from GET /api/v1/contacts/{id} and the POST/PUT response bodies —
// exactly the data a nested REST write would have just set. It must go
// through RecordForContact instead, which prefers the already-persisted
// Card. See models.RecordForContact's doc comment for the full history.
func TestNewContactRecordResponse_PreservesPersistedCardOnlyData(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestProfilePictureURL pins M6's URL derivation (the response-shape
// change): a base64 thumbnail yields the lightweight ?thumbnail=true variant,
// a disk photo without a decodable thumbnail yields the full-photo variant,
// and a contact with neither (or only a legacy filename thumbnail, which the
// endpoint can no longer serve) yields "" — which the DTOs' omitempty turns
// into an absent field.
func TestProfilePictureURL(t *testing.T) {
	t.Parallel()
	const thumb = "data:image/jpeg;base64,Zm9v"
	tests := []struct {
		name            string
		photo           string
		photoThumbnail  string
		preferThumbnail bool
		want            string
	}{
		{"data thumb preferred", "", thumb, true, "/api/v1/contacts/7/profile_picture?thumbnail=true"},
		{"data thumb, full variant", "", thumb, false, "/api/v1/contacts/7/profile_picture?thumbnail=true"},
		{"disk photo preferred", "uuid_photo.jpg", "", true, "/api/v1/contacts/7/profile_picture"},
		{"disk photo beats legacy thumb", "uuid_photo.jpg", "legacy.jpg", true, "/api/v1/contacts/7/profile_picture"},
		{"legacy thumb only", "", "legacy.jpg", true, ""},
		{"nothing", "", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProfilePictureURL(7, tt.photo, tt.photoThumbnail, tt.preferThumbnail); got != tt.want {
				t.Errorf("ProfilePictureURL(%q, %q, %v) = %q, want %q", tt.photo, tt.photoThumbnail, tt.preferThumbnail, got, tt.want)
			}
		})
	}
}

// TestNewContactSummary_PhotoThumbnailIsURL pins that the list DTO carries the
// M6 profile-picture URL — never the raw base64 thumbnail it is derived
// from (the query layer's side of this is pinned against the real migrated
// schema in contact_photo_url_test.go, where GORM's column Select actually
// runs).
func TestNewContactSummary_PhotoThumbnailIsURL(t *testing.T) {
	t.Parallel()
	const thumb = "data:image/jpeg;base64,Zm9v"
	c := &Contact{Model: gorm.Model{ID: 3}, Firstname: "Ada", PhotoThumbnail: thumb}

	summary := NewContactSummary(c)

	want := "/api/v1/contacts/3/profile_picture?thumbnail=true"
	if summary.PhotoThumbnail != want {
		t.Errorf("ContactSummary.PhotoThumbnail = %q, want %q", summary.PhotoThumbnail, want)
	}
	if strings.Contains(summary.PhotoThumbnail, "data:image") {
		t.Errorf("ContactSummary.PhotoThumbnail still leaks the raw base64 thumbnail: %q", summary.PhotoThumbnail)
	}
}

// TestNewContactRecordResponse_MediaPhotoURIIsURL pins the detail DTO's M6
// half: the top-level photo_thumbnail carries the thumbnail URL, and the
// Card.Media photo entry's URI is rewritten to the relative profile-picture
// URL (full-photo variant when a disk photo exists) rather than the data URI
// the persisted Card carries.
func TestNewContactRecordResponse_MediaPhotoURIIsURL(t *testing.T) {
	t.Parallel()
	const thumb = "data:image/jpeg;base64,Zm9v"
	c := &Contact{
		Model:          gorm.Model{ID: 9},
		Firstname:      "Grace",
		Photo:          "uuid_photo.jpg",
		PhotoThumbnail: thumb,
		Card: contactmodel.Card{
			Name:  &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Grace"}}},
			Media: []contactmodel.Resource{{Kind: "photo", URI: thumb, MediaType: "image/jpeg"}},
		},
	}

	resp := NewContactRecordResponse(c, "", nil)

	if resp.PhotoThumbnail != "/api/v1/contacts/9/profile_picture?thumbnail=true" {
		t.Errorf("ContactRecordResponse.PhotoThumbnail = %q, want the thumbnail URL", resp.PhotoThumbnail)
	}
	if len(resp.Card.Media) != 1 || resp.Card.Media[0].Kind != "photo" {
		t.Fatalf("ContactRecordResponse.Card.Media = %+v, want 1 photo entry", resp.Card.Media)
	}
	wantURI := "/api/v1/contacts/9/profile_picture"
	if resp.Card.Media[0].URI != wantURI {
		t.Errorf("Card.Media[0].URI = %q, want %q (full-photo variant, never the raw data URI)", resp.Card.Media[0].URI, wantURI)
	}

	// The persisted Card must be untouched: only the READ response rewrites
	// the media URI (exporters/CardDAV still need the self-contained data URI).
	if c.Card.Media[0].URI != thumb {
		t.Errorf("persisted Card.Media[0].URI was mutated to %q; the read-path rewrite must not write back", c.Card.Media[0].URI)
	}
}

// TestNewContactRecordResponse_UnbackedMediaPhotoKept is the regression test
// for a review-pass finding: a Card.Media photo entry that is NOT backed by a
// flat Photo/PhotoThumbnail (e.g. one imported directly into Card.Media while
// photoDir was unavailable, so applyMedia never bridged it to flat) has no
// profile-picture endpoint to point at. Its URI must be left untouched — the
// original imported data URI — rather than blanked to the empty string by the
// M6 rewrite.
func TestNewContactRecordResponse_UnbackedMediaPhotoKept(t *testing.T) {
	t.Parallel()
	const importedPhoto = "data:image/png;base64,QUJDRA=="
	c := &Contact{
		Model:     gorm.Model{ID: 11},
		Firstname: "Imported",
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Imported"}}},
			Media: []contactmodel.Resource{
				{Kind: "photo", URI: importedPhoto, MediaType: "image/png"},
				{Kind: "logo", URI: "data:image/png;base64,TE9HTw==", MediaType: "image/png"},
			},
		},
	}

	resp := NewContactRecordResponse(c, "", nil)

	if len(resp.Card.Media) != 2 {
		t.Fatalf("ContactRecordResponse.Card.Media = %+v, want both entries preserved", resp.Card.Media)
	}
	if resp.Card.Media[0].Kind != "photo" || resp.Card.Media[0].URI != importedPhoto {
		t.Errorf("unbacked photo entry = %+v, want its imported URI %q left untouched", resp.Card.Media[0], importedPhoto)
	}
	if resp.Card.Media[1].Kind != "logo" || resp.Card.Media[1].URI != "data:image/png;base64,TE9HTw==" {
		t.Errorf("non-photo media entry was damaged: %+v", resp.Card.Media[1])
	}
}
