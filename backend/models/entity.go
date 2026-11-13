package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"mycorrhizal/contactmodel"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

// Reusable entities (issue #2295, docs/adrs/0043-reusable-entities.md): an
// organisation, place or pet shared by many contacts. One table for all kinds
// (decision 1); a contact attaches through ContactEntityLink (decision 2), not
// the relationship graph.

// Entity kinds -- a closed enum. Widening it later is additive.
const (
	EntityKindOrganisation = "organisation"
	EntityKindPlace        = "place"
	EntityKindPet          = "pet"
)

// EntityKinds lists every kind in a stable order.
func EntityKinds() []string {
	return []string{EntityKindOrganisation, EntityKindPlace, EntityKindPet}
}

// EntityNameMaxLength is the maximum entity name length in characters.
const EntityNameMaxLength = 200

// Entity is one reusable organisation, place or pet.
//
// Soft delete (ADR 0004: user-authored content). UUID primary key generated
// in BeforeCreate. Kind is immutable after create (enforced by a database
// trigger too). Core holds the kind-specific typed JSON, encrypted at rest
// with the same serializer family as contacts.card; read and write it through
// the typed OrganisationCore / PlaceCore / PetCore structs, never a free-form
// map. A place's address sensitivity lives in Sensitivity, not in Core.
type Entity struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint   `gorm:"not null;index:idx_entities_user_kind,priority:1" json:"-"`
	Kind   string `gorm:"not null;index:idx_entities_user_kind,priority:2;<-:create" json:"kind" validate:"required,oneof=organisation place pet"`
	// Name is NFC-normalized on write (ADR 0016), required, at most 200 characters.
	Name string `gorm:"not null" json:"name" validate:"required,max=200"`
	// NameKey is derived from Name by BeforeSave; never client-supplied.
	NameKey string `gorm:"not null" json:"-"`
	// Core is the kind-specific JSON (validated by ValidateCore), encrypted at rest.
	Core        string `gorm:"not null;serializer:encrypted" json:"-"`
	Sensitivity string `gorm:"not null;default:normal;index" json:"sensitivity" validate:"required,oneof=normal private secret"`

	// ETag/Revision: the standard revision token (ADR 0006), see LifeEvent.
	ETag     string `gorm:"column:etag;<-:create" json:"-"`
	Revision int64  `gorm:"column:revision;<-:create;not null;default:1" json:"revision"`

	revisionStampedOnCreate bool
	// priorSensitivity is the stored sensitivity captured by BeforeSave for the
	// share-grant re-confirmation check in AfterSave (issue #2564).
	priorSensitivity string
}

// OrganisationCore is the core schema of an organisation entity. Name mirrors
// the Entity name; Units are the ordered sub-units (ORG components).
type OrganisationCore struct {
	Organization contactmodel.Organization `json:"organization"`
	URL          string                    `json:"url,omitempty" validate:"max=2000"`
	Notes        string                    `json:"notes,omitempty" validate:"max=5000"`
}

// PlaceCore is the core schema of a place entity. The address's own
// Sensitivity is NOT stored here (the entity column is the single source,
// ADR 0043 decision 4); ValidateCore rejects a non-empty one.
type PlaceCore struct {
	Address contactmodel.Address `json:"address"`
	Notes   string               `json:"notes,omitempty" validate:"max=5000"`
}

// PetCore is the core schema of a pet entity. Pets never project (decision 3).
type PetCore struct {
	Species string                    `json:"species,omitempty" validate:"max=100"`
	Breed   string                    `json:"breed,omitempty" validate:"max=100"`
	Birth   *contactmodel.PartialDate `json:"birth,omitempty"`
	Death   *contactmodel.PartialDate `json:"death,omitempty"`
	Notes   string                    `json:"notes,omitempty" validate:"max=5000"`
}

var entityCoreValidator = validator.New()

func newEntityCore(kind string) (any, error) {
	switch kind {
	case EntityKindOrganisation:
		return &OrganisationCore{}, nil
	case EntityKindPlace:
		return &PlaceCore{}, nil
	case EntityKindPet:
		return &PetCore{}, nil
	}
	return nil, fmt.Errorf("unknown entity kind %q", kind)
}

// ValidateEntityCore strictly decodes raw as the core struct of kind (unknown
// keys are rejected: no free-form core) and runs its validate tags.
func ValidateEntityCore(kind, raw string) error {
	core, err := newEntityCore(kind)
	if err != nil {
		return err
	}
	if raw == "" {
		raw = "{}"
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(core); err != nil {
		return fmt.Errorf("invalid %s core: %w", kind, err)
	}
	if err := entityCoreValidator.Struct(core); err != nil {
		return fmt.Errorf("invalid %s core: %w", kind, err)
	}
	if p, ok := core.(*PlaceCore); ok && p.Address.Sensitivity != "" {
		return fmt.Errorf("invalid place core: address sensitivity belongs in the entity sensitivity column")
	}
	return nil
}

// SetCore marshals a typed core struct (*OrganisationCore, *PlaceCore or
// *PetCore, matching the entity's kind) into Core.
func (e *Entity) SetCore(core any) error {
	want, err := newEntityCore(e.Kind)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%T", core) != fmt.Sprintf("%T", want) {
		return fmt.Errorf("core %T does not match entity kind %q", core, e.Kind)
	}
	raw, err := json.Marshal(core)
	if err != nil {
		return err
	}
	if err := ValidateEntityCore(e.Kind, string(raw)); err != nil {
		return err
	}
	e.Core = string(raw)
	return nil
}

// DecodeCore returns the typed core struct for the entity's kind.
func (e *Entity) DecodeCore() (any, error) {
	core, err := newEntityCore(e.Kind)
	if err != nil {
		return nil, err
	}
	raw := e.Core
	if raw == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), core); err != nil {
		return nil, err
	}
	return core, nil
}

// EntityNameKey derives the normalized comparison key of a name (ADR 0043
// decision 9): NFC, Unicode lower-casing, runs of whitespace collapsed, leading
// and trailing punctuation trimmed. No diacritic folding (ADR 0016).
func EntityNameKey(name string) string {
	s := strings.ToLower(norm.NFC.String(name))
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r) })
}

// BeforeCreate generates the UUID, like FieldDefinition.
func (e *Entity) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

// BeforeSave normalizes the name (NFC), derives NameKey, and rejects an
// unknown kind or an invalid core before anything is written.
func (e *Entity) BeforeSave(tx *gorm.DB) error {
	if e.ID == "" && e.Name == "" && e.Kind == "" {
		return nil // bulk Model(&Entity{}).Where(...).Update(s): nothing to derive
	}
	e.Name = norm.NFC.String(e.Name)
	e.NameKey = EntityNameKey(e.Name)
	if e.Core == "" {
		e.Core = "{}"
	}
	if e.Kind != "" {
		if err := ValidateEntityCore(e.Kind, e.Core); err != nil {
			return err
		}
	}
	return e.captureSensitivity(tx)
}

// AfterCreate stamps revision 1 and the ETag (ADR 0006).
func (e *Entity) AfterCreate(tx *gorm.DB) error {
	e.Revision = 1
	e.ETag = fmt.Sprintf("e-%s-%d", e.ID, e.Revision)
	e.revisionStampedOnCreate = true
	return tx.Table("entities").Where("id = ?", e.ID).UpdateColumns(map[string]any{"revision": e.Revision, "etag": e.ETag}).Error
}

// AfterSave bumps the revision with the CON-01 compare-and-swap.
func (e *Entity) AfterSave(tx *gorm.DB) error {
	if e.ID == "" {
		return nil
	}
	// Issue #2564: withhold the projected section on every linked contact.
	if err := e.markSensitivityReclassification(tx); err != nil {
		return err
	}
	if e.revisionStampedOnCreate {
		e.revisionStampedOnCreate = false
		return nil
	}
	newRevision, err := bumpRevisionCAS(tx, "Entity", "entities", e.ID, e.Revision, func(nr int64) string {
		return fmt.Sprintf("e-%s-%d", e.ID, nr)
	})
	if err != nil {
		return err
	}
	e.Revision = newRevision
	e.ETag = fmt.Sprintf("e-%s-%d", e.ID, newRevision)
	return nil
}
