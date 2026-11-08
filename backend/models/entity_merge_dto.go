package models

// Entity merge, suggestions and the create-from-contacts helper (issue #2299,
// ADR 0043 decisions 8, 9 and 10). Request structs are explicit allowlists.

// Suggestion reasons (ADR 0043 decision 8), strongest first.
const (
	EntitySuggestionReasonExactName    = "exact_name"
	EntitySuggestionReasonNormalName   = "normalized_name"
	EntitySuggestionReasonAddressParts = "address_components"
)

// Entity suggestion modes: pairs of live entities, or groups of the contacts'
// own ORG/ADR values (the "create entities from my contacts" helper).
const (
	EntitySuggestionModePairs         = "pairs"
	EntitySuggestionModeContactValues = "contact_values"
)

// EntityRestoreScopeEntityOnly is reported by a merge: restoring the loser
// revives only the entity itself; the re-pointed links stay with the survivor.
const EntityRestoreScopeEntityOnly = "entity_only"

// EntityMergeInput is the body of POST /entities/{id}/merge. {id} is the loser;
// Into is the survivor. Resolutions maps a core field of the entity's kind to
// "loser" to fill it from the loser; a field the survivor already has is never
// overwritten (rejected, not merged), and "survivor" is an explicit no-op.
type EntityMergeInput struct {
	Into        string            `json:"into" validate:"required,uuid4"`
	Resolutions map[string]string `json:"resolutions"`
}

// EntityMergeDemotion names a re-pointed primary link that lost its flag.
type EntityMergeDemotion struct {
	LinkID          string `json:"link_id"`
	ContactVCardUID string `json:"contact_vcard_uid"`
}

// EntityMergeResponse is POST /entities/{id}/merge. Every list is always
// present (never omitted) so clients can iterate without a nil check.
type EntityMergeResponse struct {
	Survivor EntityResponse `json:"survivor"`
	LoserID  string         `json:"loser_id"`
	// LinksRepointed counts loser links now attached to the survivor;
	// LinksCollapsed counts loser links soft-deleted as duplicates of a link the
	// survivor's contact already had (same role, start and end).
	LinksRepointed int `json:"links_repointed"`
	LinksCollapsed int `json:"links_collapsed"`
	// PrimariesDemoted lists re-pointed links that lost is_primary because the
	// contact already had another primary of the kind.
	PrimariesDemoted []EntityMergeDemotion `json:"primaries_demoted"`
	// FieldValuesMoved counts custom values re-keyed to the survivor;
	// FieldValuesDropped lists definition ids where the survivor already had a
	// value (the survivor's wins; the loser's stays in the loser's snapshot).
	FieldValuesMoved   int      `json:"field_values_moved"`
	FieldValuesDropped []string `json:"field_values_dropped"`
	// ResolvedFields lists the core fields filled from the loser.
	ResolvedFields []string `json:"resolved_fields"`
	// RestoreScope is always "entity_only": restoring the loser from the trash
	// brings back the entity but not the links, which stay with the survivor.
	RestoreScope string `json:"restore_scope"`
}

// EntitySuggestionEntity is one side of a suggested pair.
type EntitySuggestionEntity struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// EntitySuggestionPair is a candidate duplicate pair; A.ID < B.ID.
type EntitySuggestionPair struct {
	Reason string                 `json:"reason"`
	A      EntitySuggestionEntity `json:"a"`
	B      EntitySuggestionEntity `json:"b"`
}

// EntityValueGroupContact is one contact holding a value of a group.
type EntityValueGroupContact struct {
	ContactID uint   `json:"contact_id"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	// ValueName is the contact's own spelling ("Acme, Inc").
	ValueName string `json:"value_name"`
	// HasPrimary is true when the contact already has a live primary link of
	// the kind (so "set as primary" would be ignored for it).
	HasPrimary bool `json:"has_primary"`
}

// EntityValueGroup is a set of contacts whose ORG (or ADR) values compare equal.
type EntityValueGroup struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Name is the proposed entity name (the most common spelling).
	Name     string                    `json:"name"`
	Reason   string                    `json:"reason"`
	Variants []string                  `json:"variants"`
	Contacts []EntityValueGroupContact `json:"contacts"`
}

// EntitySuggestionsResponse is GET /entities/suggestions. Pairs is set in
// "pairs" mode and Groups in "contact_values" mode; the other is an empty list.
type EntitySuggestionsResponse struct {
	Mode       string                 `json:"mode"`
	Pairs      []EntitySuggestionPair `json:"pairs"`
	Groups     []EntityValueGroup     `json:"groups"`
	NextCursor string                 `json:"next_cursor"`
	Limit      int                    `json:"limit"`
}

// EntityDismissInput is the body of POST /entities/suggestions/dismiss.
type EntityDismissInput struct {
	EntityA string `json:"entity_a" validate:"required,uuid4"`
	EntityB string `json:"entity_b" validate:"required,uuid4"`
}

// EntityDismissResponse reports the canonical (ordered) pair that was hidden.
type EntityDismissResponse struct {
	IDLow  string `json:"id_low"`
	IDHigh string `json:"id_high"`
}

// EntityFromContactsGroup confirms one proposed group: the entity to create
// and the subset of the group's contacts to link.
type EntityFromContactsGroup struct {
	Key        string `json:"key" validate:"required,max=500"`
	Name       string `json:"name,omitempty" validate:"max=200"`
	Role       string `json:"role" validate:"required"`
	SetPrimary bool   `json:"set_primary"`
	ContactIDs []uint `json:"contact_ids" validate:"required,min=1,max=1000"`
}

// EntityFromContactsInput is the body of POST /entities/from-contacts.
type EntityFromContactsInput struct {
	Kind   string                    `json:"kind" validate:"required,oneof=organisation place"`
	Groups []EntityFromContactsGroup `json:"groups" validate:"required,min=1,max=100,dive"`
}

// EntityFromContactsCreated is one entity the helper created with its links.
type EntityFromContactsCreated struct {
	Key    string                      `json:"key"`
	Entity EntityResponse              `json:"entity"`
	Links  []ContactEntityLinkResponse `json:"links"`
}

// EntityFromContactsSkip reports a confirmed group or contact that was not
// applied: group_not_found (stale or already matched), contact_not_in_group.
type EntityFromContactsSkip struct {
	Key       string `json:"key"`
	ContactID uint   `json:"contact_id,omitempty"`
	Reason    string `json:"reason"`
}

// EntityFromContactsResponse is POST /entities/from-contacts.
type EntityFromContactsResponse struct {
	Created []EntityFromContactsCreated `json:"created"`
	Skipped []EntityFromContactsSkip    `json:"skipped"`
}
