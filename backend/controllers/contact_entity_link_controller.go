package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Contact-entity links (issue #2297, ADR 0043 decisions 2, 3, 6 and 10).
//
// A link attaches a contact to an Entity. Every query is scoped by user_id and
// the link's contact_vcard_uid; a foreign or unknown contact, entity or link
// is a 404 and writes nothing. Sensitivity is shown to the owner (these are
// owner surfaces; nothing here leaves the instance).
//
// Adopt and link-from-existing are explicit user actions. Neither is ever
// reached from an import, a sync or a merge: they exist only as the two routes
// registered in routes.go.

func abortEntityLinkNotFound(c *gin.Context, id string) {
	apperrors.AbortWithError(c, apperrors.ErrNotFound("Entity link").WithDetails("id", id))
}

// loadLinkContact loads the live contact :id owned by userID or aborts. When
// full is false only the identity columns are read.
func loadLinkContact(c *gin.Context, db *gorm.DB, userID uint, full bool) (*models.Contact, bool) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return nil, false
	}
	q := db.Where("user_id = ?", userID)
	if !full {
		q = q.Select("id", "user_id", "vcard_uid")
	}
	var contact models.Contact
	if err := q.First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return nil, false
	}
	return &contact, true
}

// loadOwnedLink loads the live link linkID that belongs to userID AND this
// contact, or aborts. A link of another contact (even the caller's own) is a
// 404 through this contact's path.
func loadOwnedLink(c *gin.Context, db *gorm.DB, userID uint, contact *models.Contact) (*models.ContactEntityLink, bool) {
	linkID := c.Param("linkId")
	var link models.ContactEntityLink
	if err := db.Where("id = ? AND user_id = ? AND contact_vcard_uid = ?", linkID, userID, contact.VCardUID).First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			abortEntityLinkNotFound(c, linkID)
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve entity link").WithError(err))
		}
		return nil, false
	}
	return &link, true
}

// todayPartial is the owner's calendar day (reminder zone, ADR 0015 rule 4) at
// the injected clock's instant.
func todayPartial(c *gin.Context) *contactmodel.PartialDate {
	t := reminderNow(c)
	y, m, d := t.Year(), int(t.Month()), t.Day()
	return &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}
}

func newContactEntityLinkResponse(l *models.ContactEntityLink, entityName string, today *contactmodel.PartialDate) models.ContactEntityLinkResponse {
	current := periodIncludes(l.Period, today)
	projects := ""
	if l.IsPrimary && current {
		switch l.Kind {
		case models.EntityKindOrganisation:
			projects = "ORG"
		case models.EntityKindPlace:
			projects = "ADR"
		}
	}
	return models.ContactEntityLinkResponse{
		ID: l.ID, ContactVCardUID: l.ContactVCardUID, EntityID: l.EntityID, EntityName: entityName,
		Kind: l.Kind, Role: l.Role, RoleLabel: l.RoleLabel, IsPrimary: l.IsPrimary, Period: l.Period,
		Sensitivity: l.Sensitivity, Current: current, ProjectsAs: projects,
		Revision: l.Revision, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

func abortEntityLinkRole(c *gin.Context, kind, role string) {
	apperrors.AbortWithError(c, apperrors.ErrValidation("role is not valid for this kind of entity").
		WithReason(apperrors.ReasonEntityLinkRoleInvalid, nil).WithDetails("role", role).WithDetails("kind", kind))
}

// normalizeLinkPeriod validates the optional range: an all-empty range is no
// range, and an end before the start (at shared precision) is rejected.
func normalizeLinkPeriod(c *gin.Context, p *contactmodel.TemporalRange) (*contactmodel.TemporalRange, bool) {
	if p == nil || p.IsEmpty() {
		return nil, true
	}
	if p.Reversed() {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("period", "the end cannot be before the start").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "period"}))
		return nil, false
	}
	cp := *p
	return &cp, true
}

// abortLinkWrite maps a link write error onto an API error.
func abortLinkWrite(c *gin.Context, err error) {
	var appErr *apperrors.AppError
	switch {
	case handleRevisionConflict(c, "Entity link", err):
	case errors.As(err, &appErr):
		apperrors.AbortWithError(c, appErr)
	case strings.Contains(err.Error(), "UNIQUE constraint failed"):
		apperrors.AbortWithError(c, apperrors.ErrConflict("Another change to this contact's primary link happened at the same time").
			WithReason(apperrors.ReasonEntityLinkPrimaryConflict, nil))
	default:
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save entity link").WithError(err))
	}
}

// ListContactEntityLinks handles GET /contacts/:id/entity-links.
func ListContactEntityLinks(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := loadLinkContact(c, db, userID, false)
	if !ok {
		return
	}
	var links []models.ContactEntityLink
	if err := db.Where("user_id = ? AND contact_vcard_uid = ?", userID, contact.VCardUID).Order("created_at, id").Find(&links).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve entity links").WithError(err))
		return
	}
	names := map[string]string{}
	if len(links) > 0 {
		ids := make([]string, 0, len(links))
		for _, l := range links {
			ids = append(ids, l.EntityID)
		}
		var entities []models.Entity
		if err := db.Select("id", "name").Where("user_id = ? AND id IN ?", userID, ids).Find(&entities).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve entities").WithError(err))
			return
		}
		for _, e := range entities {
			names[e.ID] = e.Name
		}
	}
	today := todayPartial(c)
	out := make([]models.ContactEntityLinkResponse, 0, len(links))
	for i := range links {
		name, live := names[links[i].EntityID]
		if !live {
			continue // entity trashed: its links were cascade-tombstoned, skip a stray
		}
		out = append(out, newContactEntityLinkResponse(&links[i], name, today))
	}
	c.JSON(http.StatusOK, models.ContactEntityLinksResponse{Links: out})
}

// CreateContactEntityLink handles POST /contacts/:id/entity-links.
func CreateContactEntityLink(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := loadLinkContact(c, db, userID, false)
	if !ok {
		return
	}
	input, verr := middleware.GetValidated[models.ContactEntityLinkInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	if input.EntityID == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("entity_id", "entity_id is required").WithReason(apperrors.ReasonValidationRequired, map[string]any{"field": "entity_id"}))
		return
	}
	entity, ok := loadOwnedEntity(c, db, userID, input.EntityID)
	if !ok {
		return
	}
	if !models.ValidEntityLinkRole(entity.Kind, input.Role) {
		abortEntityLinkRole(c, entity.Kind, input.Role)
		return
	}
	period, ok := normalizeLinkPeriod(c, input.Period)
	if !ok {
		return
	}
	link := models.ContactEntityLink{
		UserID: userID, ContactVCardUID: contact.VCardUID, EntityID: entity.ID, Kind: entity.Kind,
		Role: input.Role, RoleLabel: input.RoleLabel, Period: period,
		IsPrimary:   input.IsPrimary != nil && *input.IsPrimary,
		Sensitivity: orDefault(input.Sensitivity, models.RelationshipSensitivityNormal),
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return services.CreateContactEntityLink(tx, &link)
	}); err != nil {
		abortLinkWrite(c, err)
		return
	}
	c.JSON(http.StatusCreated, newContactEntityLinkResponse(&link, entity.Name, todayPartial(c)))
}

// UpdateContactEntityLink handles PUT /contacts/:id/entity-links/:linkId. Only
// role, role_label, period, is_primary and sensitivity are writable; the
// contact, entity and kind of a link never change.
func UpdateContactEntityLink(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := loadLinkContact(c, db, userID, false)
	if !ok {
		return
	}
	link, ok := loadOwnedLink(c, db, userID, contact)
	if !ok {
		return
	}
	if !checkIfMatch(c, link.Revision) {
		return
	}
	input, verr := middleware.GetValidated[models.ContactEntityLinkInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	entity, ok := loadOwnedEntity(c, db, userID, link.EntityID)
	if !ok {
		return
	}
	if !models.ValidEntityLinkRole(link.Kind, input.Role) {
		abortEntityLinkRole(c, link.Kind, input.Role)
		return
	}
	period, ok := normalizeLinkPeriod(c, input.Period)
	if !ok {
		return
	}
	link.Role, link.RoleLabel, link.Period = input.Role, input.RoleLabel, period
	if input.Sensitivity != "" {
		link.Sensitivity = input.Sensitivity
	}
	if input.IsPrimary != nil {
		link.IsPrimary = *input.IsPrimary
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return services.SaveContactEntityLink(tx, link)
	}); err != nil {
		abortLinkWrite(c, err)
		return
	}
	c.JSON(http.StatusOK, newContactEntityLinkResponse(link, entity.Name, todayPartial(c)))
}

// DeleteContactEntityLink handles DELETE /contacts/:id/entity-links/:linkId: a
// soft delete (ADR 0043 decision 6). It carries no deletion_batch_id -- only a
// contact or entity delete stamps a batch, so a trash restore never revives a
// link the user removed by hand.
func DeleteContactEntityLink(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := loadLinkContact(c, db, userID, false)
	if !ok {
		return
	}
	link, ok := loadOwnedLink(c, db, userID, contact)
	if !ok {
		return
	}
	if !checkIfMatch(c, link.Revision) {
		return
	}
	if err := db.Delete(link).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete entity link").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Entity link deleted"})
}

// jsonEqual compares two values by their JSON form (omitempty makes a nil and
// an empty slice equal, which is what "the same value" means for a Card entry).
func jsonEqual(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ja) == string(jb)
}

// AdoptContactEntityLink handles POST /contacts/:id/entity-links/:linkId/adopt
// (ADR 0043 decision 3): copy the entity's current organisation/address into
// the contact's own Card as an ordinary contact value with a fresh element ID.
// The Card is written through RecordForContact -> ApplyRecordToContact ->
// SaveContact (backend traps 2 and 3, never by setting Contact.Card), so the
// flat columns, revision, audit trail and webhook all follow. The link stays.
// A value-equal own entry makes this a no-op (Adopted false).
func AdoptContactEntityLink(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := loadLinkContact(c, db, userID, true)
	if !ok {
		return
	}
	link, ok := loadOwnedLink(c, db, userID, contact)
	if !ok {
		return
	}
	entity, ok := loadOwnedEntity(c, db, userID, link.EntityID)
	if !ok {
		return
	}
	core, err := entity.DecodeCore()
	if err != nil { // # pragma: no cover — core is validated JSON on every write, so a stored core always decodes
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to read entity").WithError(err))
		return
	}

	cfg := currentConfig(c)
	// A nil db keeps the read-time projections (tags, edges, custom fields) out
	// of the record: only the persisted Card is edited and written back.
	record := models.RecordForContact(contact, cfg.ProfilePhotoDir, nil)
	var elementKind, elementID string
	adopted := false

	switch v := core.(type) {
	case *models.OrganisationCore:
		elementKind = "organization"
		org := v.Organization
		if strings.TrimSpace(org.Name) == "" {
			org.Name = entity.Name
		}
		org.ID = ""
		present := false
		for _, own := range record.Card.Organizations {
			cmp := own
			cmp.ID = ""
			if jsonEqual(cmp, org) {
				present, elementID = true, own.ID
				break
			}
		}
		if !present {
			org.ID = uuid.NewString()
			elementID = org.ID
			record.Card.Organizations = append(append([]contactmodel.Organization(nil), record.Card.Organizations...), org)
			adopted = true
		}
	case *models.PlaceCore:
		elementKind = "address"
		addr := v.Address
		if len(addr.Components) == 0 && strings.TrimSpace(addr.Full) == "" {
			apperrors.AbortWithError(c, apperrors.ErrBusinessLogic("This place has no address to adopt").WithReason(apperrors.ReasonEntityLinkKindNotAdoptable, nil))
			return
		}
		addr.ID = ""
		if entity.Sensitivity != models.RelationshipSensitivityNormal {
			addr.Sensitivity = entity.Sensitivity
		}
		present := false
		want := addr
		want.Sensitivity = ""
		for _, own := range record.Card.Addresses {
			cmp := own
			cmp.ID, cmp.Sensitivity = "", ""
			if jsonEqual(cmp, want) {
				present, elementID = true, own.ID
				break
			}
		}
		if !present {
			addr.ID = uuid.NewString()
			elementID = addr.ID
			record.Card.Addresses = append(append([]contactmodel.Address(nil), record.Card.Addresses...), addr)
			adopted = true
		}
	default:
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic("Only an organisation or a place has a value to adopt").WithReason(apperrors.ReasonEntityLinkKindNotAdoptable, nil))
		return
	}

	if adopted {
		ambientBefore := contact.Ambient
		models.ApplyRecordToContact(contact, record, cfg.ProfilePhotoDir)
		if err := services.SaveContact(c.Request.Context(), db, cfg, userID, contact, ambientBefore); err != nil {
			if handleRevisionConflict(c, "Contact", err) {
				return
			}
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to update contact").WithError(err))
			return
		}
	}
	c.JSON(http.StatusOK, models.ContactEntityLinkAdoptResponse{
		Link:    newContactEntityLinkResponse(link, entity.Name, todayPartial(c)),
		Adopted: adopted, ElementKind: elementKind, ElementID: elementID,
	})
}

// addressLabel derives a display name for a place created from a contact's own
// address: the full label, else the components joined, at most 200 characters.
func addressLabel(a contactmodel.Address) string {
	label := strings.TrimSpace(a.Full)
	if label == "" {
		parts := make([]string, 0, len(a.Components))
		for _, comp := range a.Components {
			if v := strings.TrimSpace(comp.Value); v != "" {
				parts = append(parts, v)
			}
		}
		label = strings.Join(parts, ", ")
	}
	if label == "" {
		label = "Address"
	}
	for utf8.RuneCountInString(label) > models.EntityNameMaxLength {
		_, size := utf8.DecodeLastRuneInString(label)
		label = label[:len(label)-size]
	}
	return label
}

// LinkContactEntityFromExisting handles POST
// /contacts/:id/entity-links/from-existing (ADR 0043 decision 3): take one of
// the contact's own ORG/ADR values, link it to the given entity of the
// matching kind or to a new entity created from it. The contact row is only
// read -- its own value is never removed or rewritten.
func LinkContactEntityFromExisting(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	contact, ok := loadLinkContact(c, db, userID, true)
	if !ok {
		return
	}
	input, verr := middleware.GetValidated[models.ContactEntityLinkFromExistingInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	src := input.Source
	if (src.ID == "") == (src.Index == nil) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("source", "exactly one of id and index is required").
			WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "source"}))
		return
	}

	wantKind := models.EntityKindOrganisation
	if src.Kind == "address" {
		wantKind = models.EntityKindPlace
	}
	if !models.ValidEntityLinkRole(wantKind, input.Role) {
		abortEntityLinkRole(c, wantKind, input.Role)
		return
	}
	period, ok := normalizeLinkPeriod(c, input.Period)
	if !ok {
		return
	}

	var newEntity *models.Entity // built from the contact's own value when no entity_id is given
	var matched *models.Entity
	if input.EntityID != "" {
		matched, ok = loadOwnedEntity(c, db, userID, input.EntityID)
		if !ok {
			return
		}
		if matched.Kind != wantKind {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("entity_id", "the entity is not of the kind of the contact's value").
				WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "entity_id"}))
			return
		}
	}

	sourceFound := false
	switch src.Kind {
	case "organization":
		orgs := contact.Card.Organizations
		for i := range orgs {
			if (src.ID != "" && orgs[i].ID == src.ID) || (src.Index != nil && *src.Index == i) {
				sourceFound = true
				if matched != nil {
					break
				}
				org := orgs[i]
				org.ID = ""
				name := strings.TrimSpace(org.Name)
				if name == "" || utf8.RuneCountInString(name) > models.EntityNameMaxLength {
					apperrors.AbortWithError(c, apperrors.ErrInvalidInput("source", "the organisation needs a name of 1 to 200 characters to create an entity from").
						WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "source"}))
					return
				}
				e := &models.Entity{UserID: userID, Kind: models.EntityKindOrganisation, Name: name, Sensitivity: models.RelationshipSensitivityNormal}
				if err := e.SetCore(&models.OrganisationCore{Organization: org}); err != nil {
					apperrors.AbortWithError(c, apperrors.ErrInvalidInput("source", err.Error()).WithReason(apperrors.ReasonEntityCoreInvalid, map[string]any{"reason": err.Error()}))
					return
				}
				newEntity = e
				break
			}
		}
	case "address":
		addrs := contact.Card.Addresses
		for i := range addrs {
			if (src.ID != "" && addrs[i].ID == src.ID) || (src.Index != nil && *src.Index == i) {
				sourceFound = true
				if matched != nil {
					break
				}
				addr := addrs[i]
				sens := models.RelationshipSensitivityNormal
				if addr.Sensitivity != "" {
					sens = addr.Sensitivity
				}
				addr.ID, addr.Sensitivity = "", "" // the entity column is the single source of sensitivity
				e := &models.Entity{UserID: userID, Kind: models.EntityKindPlace, Name: addressLabel(addrs[i]), Sensitivity: sens}
				if err := e.SetCore(&models.PlaceCore{Address: addr}); err != nil {
					apperrors.AbortWithError(c, apperrors.ErrInvalidInput("source", err.Error()).WithReason(apperrors.ReasonEntityCoreInvalid, map[string]any{"reason": err.Error()}))
					return
				}
				newEntity = e
				break
			}
		}
	}
	if !sourceFound {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact value").WithReason(apperrors.ReasonEntityLinkSourceNotFound, nil).
			WithDetails("kind", src.Kind))
		return
	}

	link := models.ContactEntityLink{
		UserID: userID, ContactVCardUID: contact.VCardUID, Kind: wantKind,
		Role: input.Role, RoleLabel: input.RoleLabel, Period: period,
		IsPrimary:   input.IsPrimary != nil && *input.IsPrimary,
		Sensitivity: orDefault(input.Sensitivity, models.RelationshipSensitivityNormal),
	}
	var entity *models.Entity
	if err := db.Transaction(func(tx *gorm.DB) error {
		entity = matched
		if newEntity != nil {
			if err := tx.Create(newEntity).Error; err != nil {
				return err
			}
			entity = newEntity
		}
		link.EntityID = entity.ID
		return services.CreateContactEntityLink(tx, &link)
	}); err != nil {
		abortLinkWrite(c, err)
		return
	}
	c.JSON(http.StatusCreated, models.ContactEntityLinkFromExistingResponse{
		Link:          newContactEntityLinkResponse(&link, entity.Name, todayPartial(c)),
		Entity:        models.NewEntityResponse(entity),
		EntityCreated: newEntity != nil,
	})
}
