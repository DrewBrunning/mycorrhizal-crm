package models

import (
	"fmt"
	"strings"

	"mycorrhizal/contactmodel"

	"gorm.io/gorm"
)

// ProfilePictureURL returns the relative URL to the contact's existing
// profile-picture endpoint (GET /api/v1/contacts/:id/profile_picture) for a
// contact that has a photo, or "" when it has none. It is M6's
// response-shape change: the list/detail photo fields carry this URL instead
// of the raw stored value (a base64 data URI or a legacy disk-file name), so
// a native client can hand it straight to an authenticated image loader
// (M6). The
// endpoint itself is unchanged and still serves raw bytes.
//
// preferThumbnail selects the lightweight ?thumbnail=true variant when a
// base64 data-URL thumbnail is stored. The thumbnail endpoint can only serve
// that base64 thumbnail (legacy filename thumbnails and full-size disk photos
// both live behind the no-param variant), so whenever preferThumbnail is set
// but no decodable thumbnail exists, the full-photo variant is used instead.
// When neither a disk photo nor a decodable thumbnail exists, "" is returned
// and the caller's omitempty drops the field entirely, per the ticket's
// "absent when none does".
func ProfilePictureURL(id uint, photo, photoThumbnail string, preferThumbnail bool) string {
	if preferThumbnail && strings.HasPrefix(photoThumbnail, "data:") {
		return fmt.Sprintf("/api/v1/contacts/%d/profile_picture?thumbnail=true", id)
	}
	if photo != "" {
		return fmt.Sprintf("/api/v1/contacts/%d/profile_picture", id)
	}
	if strings.HasPrefix(photoThumbnail, "data:") {
		return fmt.Sprintf("/api/v1/contacts/%d/profile_picture?thumbnail=true", id)
	}
	return ""
}

// ContactSummaryColumns is the fixed set of columns needed to build a
// ContactSummary (list-view) response. Selecting only these avoids the
// over-fetch (heavy JSON columns like card/emails/phones/addresses/...) that
// the removed fields= param used to exist to let callers opt out of (docs/adrs/0001-neutral-hub-and-spoke-contact-model.md) — now that the list
// endpoint has a fixed slim shape, this is baked in rather than
// caller-configurable.
//
// Defined here (models) rather than in the controllers package so the
// duplicate-scan service (services/duplicate_service.go, which cannot import
// controllers without a cycle) builds its ContactSummaries from the SAME
// column list — a hand-synced mirror is exactly the drift hazard /CLAUDE.md
// trap #4 warns about. Controllers build their cursor variants (adding
// updated_at / sort_name / deleted_at) off this base.
var ContactSummaryColumns = []string{
	"id", "vcard_uid", "firstname", "lastname", "nickname", "fn", "email", "phone", "birthday", "org",
	"photo", "photo_thumbnail", "archived", "is_favorite",
}

// ContactSummary is the slim per-item shape for GET /api/v1/contacts (list).
// Per docs/adrs/0001-neutral-hub-and-spoke-contact-model.md ("Mobile-CRUD-real"
// section), it wraps contactmodel.Projection's own fields (Firstname,
// Lastname, FN, PrimaryEmail, PrimaryPhone, Birthday, Org) plus the record's
// identity (UID) and the existing Photo/PhotoThumbnail — deliberately a new
// controller-layer DTO built FROM a Contact, not contactmodel.Projection
// reused verbatim as a wire type (Projection stays an internal
// persistence-projection helper). This is what a mobile contact-list screen
// calls; it must not require fetching every contact's full nested Card.
//
// ID and Archived are practical additions beyond the doc's literal field
// list: ID is required for any client to address a specific contact (link
// to the detail view, issue further requests), and Archived lets a caller
// that requested include_archived=true (mixed archived/active results)
// distinguish which is which without a second round trip. Both are cheap,
// already-loaded scalar columns — no extra query cost.
//
// Nickname was added after the fact (frontend migration pre-work):
// ContactsPage's list view renders it per-row, and the old fields=-based
// flat API used to let it request it directly. Cheap, an already-loaded
// column like ID/Archived above.
//
// T108: Circles lived here too until 2026-08-14, and was removed rather than
// fixed. It was never actually populated -- contactSummaryColumns never
// selected the column, so every response carried a silently empty "circles":
// null -- and nothing consumed it: ContactsPage's circle chips come from a
// separate useCircles() lookup, not this DTO. It also would have been the
// wrong data if selected: Contact.Circles is the legacy flat JSON column
// T2/T3 superseded with circle_members, so populating it verbatim would ship
// stale values for any contact edited since that migration. If a real
// consumer needs circles on the list endpoint, it should read from
// circle_members (a join or a second query), not resurrect this field.
type ContactSummary struct {
	ID           uint   `json:"id"`
	UID          string `json:"uid"`
	Firstname    string `json:"firstname"`
	Lastname     string `json:"lastname"`
	Nickname     string `json:"nickname"`
	FN           string `json:"fn"`
	PrimaryEmail string `json:"primary_email"`
	PrimaryPhone string `json:"primary_phone"`
	Birthday     string `json:"birthday"`
	Org          string `json:"org"`
	Photo        string `json:"photo"`
	// PhotoThumbnail is M6's response-shape change: it carries a relative
	// URL to the profile-picture thumbnail endpoint (ProfilePictureURL) when
	// a photo exists, and is omitted (omitempty) when none does — never the
	// raw stored base64 data URI or legacy disk-file name.
	PhotoThumbnail string `json:"photo_thumbnail,omitempty"`
	Archived       bool   `json:"archived"`
	// IsFavorite is always on the wire (no omitempty): a bool defaulting to
	// false must be present so the TS side can treat it as required — an
	// absent is_favorite would read as undefined and break the star icon
	// (CLAUDE.md frontend trap 8).
	IsFavorite bool `json:"is_favorite"`
	// Deleted is the T17 change-feed tombstone marker, set only by the
	// ?since= feed path (which reads rows with Unscoped()). A plain list
	// request never sets it.
	Deleted bool `json:"deleted,omitempty"`
}

// NewContactSummary builds a ContactSummary directly from a Contact's own
// already-denormalized columns (Firstname/Lastname/FN/Email/Phone/Birthday/
// Org), rather than re-deriving them a third time via
// RecordFromContact+contactmodel.DeriveProjection: BeforeSave
// already keeps those columns in sync with the neutral Card on every
// create/update, so reading them directly here is the single derivation
// path, just read from its cached/persisted destination instead of
// recomputed on every list-row render.
func NewContactSummary(c *Contact) ContactSummary {
	return ContactSummary{
		ID:             c.ID,
		UID:            c.VCardUID,
		Firstname:      c.Firstname,
		Lastname:       c.Lastname,
		Nickname:       c.Nickname,
		FN:             c.FN,
		PrimaryEmail:   c.Email,
		PrimaryPhone:   c.Phone,
		Birthday:       c.Birthday,
		Org:            c.Org,
		Photo:          c.Photo,
		PhotoThumbnail: ProfilePictureURL(c.ID, c.Photo, c.PhotoThumbnail, true),
		Archived:       c.Archived,
		IsFavorite:     c.IsFavorite,
	}
}

// ContactSummaryWithRelations extends ContactSummary with the sub-resource
// arrays requested via includes= (notes/activities/reminders). This is used only when includes= is non-empty; plain
// ContactSummary is used otherwise. It is intentionally NOT a full Card:
// includes= augments the slim list shape, it does not upgrade it to the
// detail shape.
//
// Relationships was removed from this shape alongside the
// includes=relationships removal in contact_controller.go's GetContacts.
type ContactSummaryWithRelations struct {
	ContactSummary
	Notes      []Note     `json:"notes,omitempty"`
	Activities []Activity `json:"activities,omitempty"`
	Reminders  []Reminder `json:"reminders,omitempty"`
}

// NewContactSummaryWithRelations builds the extended list-item shape from a
// Contact whose requested associations have already been Preload()ed by the
// caller (GetContacts in contact_controller.go).
func NewContactSummaryWithRelations(c *Contact) ContactSummaryWithRelations {
	return ContactSummaryWithRelations{
		ContactSummary: NewContactSummary(c),
		Notes:          c.Notes,
		Activities:     c.Activities,
		Reminders:      c.Reminders,
	}
}

// ContactRecordInput is the request body for POST /api/v1/contacts and
// PUT /api/v1/contacts/{id}: the full neutral Card/CRM shape,
// nested — not the old flat ContactInput. Gender rides alongside Card/CRM
// rather than inside either: per RecordFromContact/ApplyRecordToContact's
// own extensive documentation, Gender is a legacy CRM concept with
// deliberately no neutral-model home (it is not vCard GENDER or JSContact
// speakToAs), so it isn't part of contactmodel.Card or contactmodel.
// CRMEnvelope at all — it has to be carried as a sibling field on the
// wire DTO instead.
//
// Validation is deliberately light on Card/CRM/Passthrough: per this WP's
// (graceful, non-strict validation sourced from the neutral model's
// own degradation policy, docs/adrs/0001-neutral-hub-and-spoke-contact-model.md), nothing here
// hard-fails on an unrecognized enum value (e.g. an unexpected
// NameComponent.Kind, Anniversary.Kind, or PersonalInfo.Kind) — contactmodel
// itself (a P0-locked package, out of this WP's scope to modify) carries no
// `validate` struct tags on any of these nested types, so there is nothing
// to loosen here; the previous flat ContactInput's handful of
// `validate:"omitempty,oneof=..."` tags simply have no equivalent
// re-introduced on the new nested shape, by design. Firstname/name
// presence is checked post-conversion in the controller (see CreateContact/
// UpdateContact) rather than via a struct tag, since "at least one name
// component present" is not expressible as a simple field-level validator on
// a nested slice.
type ContactRecordInput struct {
	Gender      string                   `json:"gender" validate:"omitempty,max=100"`
	Card        contactmodel.Card        `json:"card"`
	CRM         contactmodel.CRMEnvelope `json:"crm"`
	Passthrough contactmodel.Passthrough `json:"passthrough"`
}

// ToRecord builds a contactmodel.Record from the input's Card/CRM/
// Passthrough, ready to hand to ApplyRecordToContact.
func (in *ContactRecordInput) ToRecord() *contactmodel.Record {
	return &contactmodel.Record{Card: in.Card, Envelope: in.CRM, Passthrough: in.Passthrough}
}

// ContactRecordResponse is the full neutral detail-view response shape for
// GET /api/v1/contacts/{id}, and the response returned by POST/PUT
// /api/v1/contacts (a symmetric read/write contract,
// "Mobile-CRUD-real" section). Card/CRM/Passthrough are exposed under their
// own top-level names here rather than by giving models.Contact's existing
// `json:"-"` Card/CRM/Passthrough fields real JSON tags — this keeps
// Contact's own default JSON shape (still relied on by GetContactsRandom,
// Archive/UnarchiveContact, etc., all out of this WP's Gap list) completely
// unchanged, and gives this response a clean place to add
// identity/relationship fields the bare Contact struct doesn't carry the
// same way (UID/ETag surfaced as their own fields rather than the
// Card.UID/etag columns).
type ContactRecordResponse struct {
	ID          uint                     `json:"id"`
	UID         string                   `json:"uid"`
	ETag        string                   `json:"etag"`
	Gender      string                   `json:"gender"`
	Card        contactmodel.Card        `json:"card"`
	CRM         contactmodel.CRMEnvelope `json:"crm"`
	Passthrough contactmodel.Passthrough `json:"passthrough,omitempty"`
	Photo       string                   `json:"photo"`
	// PhotoThumbnail is M6's response-shape change, exactly as on
	// ContactSummary: a relative URL to the profile-picture thumbnail
	// endpoint when a photo exists, omitted (omitempty) when none does.
	PhotoThumbnail string `json:"photo_thumbnail,omitempty"`
	Archived       bool   `json:"archived"`
	IsFavorite     bool   `json:"is_favorite"`

	// Preserved from the existing GetContact/GetContacts preload-all
	// behavior (Gap: "keep the existing preload behavior for backward
	// compat" — see GetContact's doc comment in contact_controller.go for
	// the full reasoning on why these are still included here even though
	// dedicated /contacts/:id/notes-style endpoints also exist).
	//
	// Relationships was removed — see GetContact's doc comment.
	Notes      []Note     `json:"notes,omitempty"`
	Activities []Activity `json:"activities,omitempty"`
	Reminders  []Reminder `json:"reminders,omitempty"`
}

// NewContactRecordResponse builds the full detail-view response from a
// Contact, deriving Card/CRM/Passthrough/UID via RecordForContact (not
// RecordFromContact directly) so the response reflects what's actually
// persisted, including data with no flat-field home (SpeakToAs,
// PersonalInfo, ...) — calling RecordFromContact fresh here was a real,
// live bug (found while auditing work): it silently dropped exactly
// that data from GET/POST/PUT responses. See RecordForContact's doc
// comment. photoDir (config.Config.ProfilePhotoDir) is forwarded through it
// so the response's Card.Media carries the contact's photo (
// photo-bridging prerequisite) in addition to the existing top-level
// Photo/PhotoThumbnail fields below. db is forwarded to RecordForContact for
// relationship-graph projection; pass nil to skip it.
func NewContactRecordResponse(c *Contact, photoDir string, db *gorm.DB) ContactRecordResponse {
	record := RecordForContact(c, photoDir, db)
	resp := ContactRecordResponse{
		ID:             c.ID,
		UID:            record.UID,
		ETag:           record.ETag,
		Gender:         c.Gender,
		Card:           record.Card,
		CRM:            record.Envelope,
		Passthrough:    record.Passthrough,
		Photo:          c.Photo,
		PhotoThumbnail: ProfilePictureURL(c.ID, c.Photo, c.PhotoThumbnail, true),
		Archived:       c.Archived,
		IsFavorite:     c.IsFavorite,
		Notes:          c.Notes,
		Activities:     c.Activities,
		Reminders:      c.Reminders,
	}

	// M6: the Card.Media photo entry's URI carries the relative
	// profile-picture URL too, so a client rendering the detail avatar from
	// Card.Media can hand it to an image loader. Only the READ response is
	// rewritten — the persisted Card (which feeds CardDAV and the VCF/
	// JSContact exporters) keeps its self-contained data URI. The full-photo
	// variant (no ?thumbnail=true) is preferred so a detail avatar gets a
	// real photo rather than the 48×48 thumbnail.
	//
	// An entry is rewritten ONLY when a URL actually exists: a photo entry
	// that is not backed by a flat Photo/PhotoThumbnail (e.g. one imported
	// into Card.Media directly while photoDir was unavailable) has no
	// profile-picture endpoint to point at — its URI is left untouched rather
	// than blanked, so the imported photo stays visible to clients.
	//
	// The slice is COPIED before rewriting: RecordForContact returns a struct
	// copy whose Media slice still shares its backing array with the stored
	// Card, so an in-place rewrite would silently corrupt the persisted card
	// (exports would then carry a relative URL no external consumer can
	// fetch, and the next web PUT would round-trip it). The copy detaches the
	// response's media from the stored card.
	media := make([]contactmodel.Resource, len(resp.Card.Media))
	copy(media, resp.Card.Media)
	for i := range media {
		if media[i].Kind == "photo" {
			if u := ProfilePictureURL(c.ID, c.Photo, c.PhotoThumbnail, false); u != "" {
				media[i].URI = u
			}
		}
	}
	resp.Card.Media = media

	return resp
}
