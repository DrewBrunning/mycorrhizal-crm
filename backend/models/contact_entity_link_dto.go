package models

import (
	"time"

	"mycorrhizal/contactmodel"
)

// Contact-entity link request/response shapes (issue #2297, ADR 0043
// decisions 2, 3 and 10).

// ContactEntityLinkInput is the body of POST (create) and PUT (replace) on
// /contacts/{id}/entity-links. EntityID is required on create and ignored on
// update (a link's entity is immutable: re-link instead). IsPrimary and
// Sensitivity are kept when omitted on update; Role, RoleLabel and Period are
// replaced. Role is validated against the entity kind's closed vocabulary in
// the handler (the kind is only known once the entity is loaded).
type ContactEntityLinkInput struct {
	EntityID    string                      `json:"entity_id,omitempty"`
	Role        string                      `json:"role" validate:"required"`
	RoleLabel   string                      `json:"role_label,omitempty" validate:"max=100"`
	IsPrimary   *bool                       `json:"is_primary,omitempty"`
	Period      *contactmodel.TemporalRange `json:"period,omitempty"`
	Sensitivity string                      `json:"sensitivity,omitempty" validate:"omitempty,oneof=normal private secret"`
}

// ContactEntityLinkSource names one of the contact's own ORG/ADR values by
// kind ("organization" or "address") and either its Card element ID or its
// zero-based position in the Card collection. Exactly one of ID and Index is
// required: organisations imported from vCard often carry no element ID.
type ContactEntityLinkSource struct {
	Kind  string `json:"kind" validate:"required,oneof=organization address"`
	ID    string `json:"id,omitempty"`
	Index *int   `json:"index,omitempty" validate:"omitempty,min=0"`
}

// ContactEntityLinkFromExistingInput is the body of POST
// /contacts/{id}/entity-links/from-existing. EntityID optionally names an
// existing entity of the matching kind; without it a new entity is created from
// the value. The link fields are those of ContactEntityLinkInput.
type ContactEntityLinkFromExistingInput struct {
	Source      ContactEntityLinkSource     `json:"source" validate:"required"`
	EntityID    string                      `json:"entity_id,omitempty"`
	Role        string                      `json:"role" validate:"required"`
	RoleLabel   string                      `json:"role_label,omitempty" validate:"max=100"`
	IsPrimary   *bool                       `json:"is_primary,omitempty"`
	Period      *contactmodel.TemporalRange `json:"period,omitempty"`
	Sensitivity string                      `json:"sensitivity,omitempty" validate:"omitempty,oneof=normal private secret"`
}

// ContactEntityLinkResponse is a link as the owner's REST surface returns it.
// Current is true when the link's period includes today (the owner's calendar
// day); ProjectsAs is "ORG" or "ADR" while the link is primary, current and of
// a kind that projects, else "" (ADR 0043 decision 3 -- the preview shown next
// to the link).
type ContactEntityLinkResponse struct {
	ID              string                      `json:"id"`
	ContactVCardUID string                      `json:"contact_vcard_uid"`
	EntityID        string                      `json:"entity_id"`
	EntityName      string                      `json:"entity_name"`
	Kind            string                      `json:"kind"`
	Role            string                      `json:"role"`
	RoleLabel       string                      `json:"role_label"`
	IsPrimary       bool                        `json:"is_primary"`
	Period          *contactmodel.TemporalRange `json:"period,omitempty"`
	Sensitivity     string                      `json:"sensitivity"`
	Current         bool                        `json:"current"`
	ProjectsAs      string                      `json:"projects_as"`
	Revision        int64                       `json:"revision"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
}

// ContactEntityLinksResponse is GET /contacts/{id}/entity-links.
type ContactEntityLinksResponse struct {
	Links []ContactEntityLinkResponse `json:"links"`
}

// ContactEntityLinkAdoptResponse is POST .../{linkId}/adopt. Adopted is false
// when the contact already held a value-equal own entry (nothing was written).
// ElementKind is "organization" or "address"; ElementID is the Card element ID
// of the copied (or already-present) value.
type ContactEntityLinkAdoptResponse struct {
	Link        ContactEntityLinkResponse `json:"link"`
	Adopted     bool                      `json:"adopted"`
	ElementKind string                    `json:"element_kind"`
	ElementID   string                    `json:"element_id"`
}

// ContactEntityLinkFromExistingResponse is POST .../from-existing.
type ContactEntityLinkFromExistingResponse struct {
	Link          ContactEntityLinkResponse `json:"link"`
	Entity        EntityResponse            `json:"entity"`
	EntityCreated bool                      `json:"entity_created"`
}
