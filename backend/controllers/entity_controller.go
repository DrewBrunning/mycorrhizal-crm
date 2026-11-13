package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Reusable entity CRUD (issue #2296, ADR 0043 decisions 1, 4, 5, 6, 9, 10).
//
// Every query is scoped by user_id; a foreign, unknown or soft-deleted id is a
// 404 and writes nothing. Sensitivity is shown to the owner (ADR 0043
// decision 4: /entities is an owner surface, nothing here leaves the instance),
// so there is no include_sensitive flag; ?sensitivity= is a plain filter.

func abortEntityNotFound(c *gin.Context, id string) {
	apperrors.AbortWithError(c, apperrors.ErrNotFound("Entity").WithDetails("id", id))
}

// loadOwnedEntity loads the live entity :id owned by userID or aborts.
func loadOwnedEntity(c *gin.Context, db *gorm.DB, userID uint, id string) (*models.Entity, bool) {
	var e models.Entity
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			abortEntityNotFound(c, id)
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve entity").WithError(err))
		}
		return nil, false
	}
	return &e, true
}

// normalizeEntityCore validates raw as the core of kind and returns its
// compact form. An absent or null core is the empty object.
func normalizeEntityCore(kind string, raw json.RawMessage) (string, *apperrors.AppError) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		s = "{}"
	}
	if err := models.ValidateEntityCore(kind, s); err != nil {
		return "", apperrors.ErrInvalidInput("core", err.Error()).WithReason(apperrors.ReasonEntityCoreInvalid, map[string]any{"reason": err.Error()})
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil { // # pragma: no cover — ValidateEntityCore already decoded s as JSON, so Compact cannot fail
		return "", apperrors.ErrInvalidInput("core", err.Error()).WithReason(apperrors.ReasonEntityCoreInvalid, map[string]any{"reason": err.Error()})
	}
	return buf.String(), nil
}

func validEntityKind(kind string) bool {
	for _, k := range models.EntityKinds() {
		if k == kind {
			return true
		}
	}
	return false
}

// entityCounts returns the live link count and the number of contacts that
// currently receive a projection from the entity (live primary link to a live
// contact whose period includes today), for the delete confirmation.
func entityCounts(db *gorm.DB, userID uint, entityID string, now clock.Clock) (links, projections int, err error) {
	var rows []models.ContactEntityLink
	if err = db.Where("entity_id = ? AND user_id = ?", entityID, userID).Find(&rows).Error; err != nil {
		return 0, 0, err
	}
	links = len(rows)
	t := now.Now()
	y, m, d := t.Year(), int(t.Month()), t.Day()
	today := &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}
	for _, l := range rows {
		if !l.IsPrimary {
			continue
		}
		var n int64
		if err = db.Model(&models.Contact{}).Where("user_id = ? AND vcard_uid = ?", userID, l.ContactVCardUID).Count(&n).Error; err != nil {
			return 0, 0, err
		}
		if n == 0 || !periodIncludes(l.Period, today) {
			continue
		}
		projections++
	}
	return links, projections, nil
}

// periodIncludes reports whether the (possibly nil/open) period contains day.
// An endpoint that cannot be compared (no year) does not exclude it.
func periodIncludes(p *contactmodel.TemporalRange, day *contactmodel.PartialDate) bool {
	if p == nil {
		return true
	}
	if cmp, ok := contactmodel.ComparePartialDates(p.Start, day); ok && cmp > 0 {
		return false
	}
	if cmp, ok := contactmodel.ComparePartialDates(day, p.End); ok && cmp > 0 {
		return false
	}
	return true
}

// CreateEntity handles POST /entities.
func CreateEntity(c *gin.Context) {
	input, verr := middleware.GetValidated[models.EntityInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if !validEntityKind(input.Kind) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("kind", "kind is required and must be organisation, place or pet").WithReason(apperrors.ReasonEntityKindRequired, nil))
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("name", "name is required").WithReason(apperrors.ReasonValidationRequired, map[string]any{"field": "name"}))
		return
	}
	core, cerr := normalizeEntityCore(input.Kind, input.Core)
	if cerr != nil {
		apperrors.AbortWithError(c, cerr)
		return
	}
	e := models.Entity{
		UserID: userID, Kind: input.Kind, Name: name, Core: core,
		Sensitivity: orDefault(input.Sensitivity, models.RelationshipSensitivityNormal),
	}
	if err := db.Create(&e).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save entity").WithError(err))
		return
	}
	c.JSON(http.StatusCreated, models.NewEntityResponse(&e))
}

// GetEntity handles GET /entities/:id.
func GetEntity(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	e, ok := loadOwnedEntity(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	links, projections, err := entityCounts(db, userID, e.ID, clock.FromContext(c))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count entity links").WithError(err))
		return
	}
	resp := models.NewEntityResponse(e)
	resp.LinkCount, resp.ProjectionCount = &links, &projections
	c.JSON(http.StatusOK, resp)
}

// escapeLike escapes the LIKE metacharacters of a user-supplied fragment.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ListEntities handles GET /entities?kind=&q=&sensitivity=&cursor=.
func ListEntities(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	params, perr := GetCursorParams(c)
	if perr != nil {
		apperrors.AbortWithError(c, perr)
		return
	}
	q := db.Model(&models.Entity{}).Where("entities.user_id = ?", userID)
	if kind := c.Query("kind"); kind != "" {
		if !validEntityKind(kind) {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("kind", "must be one of: organisation, place, pet").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "kind"}))
			return
		}
		q = q.Where("entities.kind = ?", kind)
	}
	if s := c.Query("sensitivity"); s != "" {
		if s != "normal" && s != "private" && s != "secret" {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("sensitivity", "must be one of: normal, private, secret").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "sensitivity"}))
			return
		}
		q = q.Where("entities.sensitivity = ?", s)
	}
	norm := services.NormalizeSearchTerm(c.Query("q"))
	term := models.EntityNameKey(norm)
	if term == "" { // punctuation-only query: match it literally
		term = strings.ToLower(strings.TrimSpace(norm))
	}
	if term != "" {
		q = q.Where(`entities.name_key LIKE ? ESCAPE '\'`, "%"+escapeLike(term)+"%")
	}
	desc := params.Order == "desc"
	if params.Cursor != nil {
		pred, t, idv := cursorPredicate("entities", params.Cursor, params.Cursor.ID, desc)
		q = q.Where(pred, t, idv)
	}
	var rows []models.Entity
	if err := cursorOrderBy(q, "entities", desc).Limit(params.Limit + 1).Find(&rows).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve entities").WithError(err))
		return
	}
	next := ""
	if len(rows) > params.Limit {
		rows = rows[:params.Limit]
		next = EncodeCursor(rows[len(rows)-1].UpdatedAt, rows[len(rows)-1].ID)
	}
	out := make([]models.EntityResponse, 0, len(rows))
	for i := range rows {
		out = append(out, models.NewEntityResponse(&rows[i]))
	}
	c.JSON(http.StatusOK, models.EntityListResponse{Entities: out, NextCursor: next, Limit: params.Limit})
}

// UpdateEntity handles PUT /entities/:id. Only name, core and sensitivity are
// writable; kind must match when supplied. An omitted core or sensitivity is
// kept.
func UpdateEntity(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	e, ok := loadOwnedEntity(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	if !checkIfMatch(c, e.Revision) {
		return
	}
	input, verr := middleware.GetValidated[models.EntityInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	if input.Kind != "" && input.Kind != e.Kind {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("kind", "kind cannot be changed").WithReason(apperrors.ReasonEntityKindImmutable, nil))
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("name", "name is required").WithReason(apperrors.ReasonValidationRequired, map[string]any{"field": "name"}))
		return
	}
	if len(input.Core) > 0 {
		core, cerr := normalizeEntityCore(e.Kind, input.Core)
		if cerr != nil {
			apperrors.AbortWithError(c, cerr)
			return
		}
		e.Core = core
	}
	e.Name = name
	if input.Sensitivity != "" {
		e.Sensitivity = input.Sensitivity
	}
	if err := services.SaveEntity(db, e); err != nil {
		if handleRevisionConflict(c, "Entity", err) {
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save entity").WithError(err))
		return
	}
	c.JSON(http.StatusOK, models.NewEntityResponse(e))
}

// DeleteEntity handles DELETE /entities/:id: the ADR 0043 decision 6
// transaction (services.DeleteEntity). The response reports how many contacts
// were receiving a projection from the entity.
func DeleteEntity(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	e, ok := loadOwnedEntity(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	if !checkIfMatch(c, e.Revision) {
		return
	}
	clk := clock.FromContext(c)
	links, projections, err := entityCounts(db, userID, e.ID, clk)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count entity links").WithError(err))
		return
	}
	if err := services.DeleteEntity(db, userID, e.ID, clk.Now()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			abortEntityNotFound(c, e.ID)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete entity").WithError(err))
		return
	}
	c.JSON(http.StatusOK, models.EntityDeleteResponse{Message: "Entity deleted", LinkCount: links, ProjectionCount: projections})
}

// RestoreEntity handles POST /entities/:id/restore.
func RestoreEntity(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id := c.Param("id")
	res, err := services.RestoreEntity(db, userID, id, clock.FromContext(c).Now())
	switch {
	case errors.Is(err, services.ErrEntityNotInTrash):
		abortEntityNotFound(c, id)
		return
	case errors.Is(err, services.ErrEntitySnapshotUnsupported):
		apperrors.AbortWithError(c, apperrors.ErrConflict("This entity's deletion snapshot is not supported by this version").WithReason(apperrors.ReasonEntitySnapshotUnsupported, nil))
		return
	case err != nil:
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to restore entity").WithError(err))
		return
	}
	skipped := make([]models.EntityRestoreSkip, 0, len(res.Skipped))
	for _, s := range res.Skipped {
		skipped = append(skipped, models.EntityRestoreSkip{Table: s.Table, Reason: s.Reason, Count: s.Count})
	}
	c.JSON(http.StatusOK, models.EntityRestoreResponse{Entity: models.NewEntityResponse(&res.Entity), Skipped: skipped, SameNameLive: res.SameNameLive, LinksRevived: res.LinksRevived, MergedInto: res.MergedInto})
}

// ListEntityContacts handles GET /entities/:id/contacts: the entity's live
// links with their live contacts.
func ListEntityContacts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	e, ok := loadOwnedEntity(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	var links []models.ContactEntityLink
	if err := db.Where("entity_id = ? AND user_id = ?", e.ID, userID).Order("created_at, id").Find(&links).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve entity links").WithError(err))
		return
	}
	out := make([]models.EntityContactLink, 0, len(links))
	for _, l := range links {
		var contact models.Contact
		err := db.Where("user_id = ? AND vcard_uid = ?", userID, l.ContactVCardUID).First(&contact).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue // contact deleted: its link is cascade-tombstoned, skip a stray
		}
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
			return
		}
		out = append(out, models.EntityContactLink{
			LinkID: l.ID, ContactID: contact.ID, ContactVCardUID: l.ContactVCardUID,
			Firstname: contact.Firstname, Lastname: contact.Lastname,
			Role: l.Role, RoleLabel: l.RoleLabel, IsPrimary: l.IsPrimary, Period: l.Period, Sensitivity: l.Sensitivity,
		})
	}
	c.JSON(http.StatusOK, models.EntityContactsResponse{Contacts: out})
}

// ListEntityFieldValues handles GET /entities/:id/field-values.
func ListEntityFieldValues(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	e, ok := loadOwnedEntity(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	values := []models.FieldValue{}
	if err := db.Where("entity_id = ? AND user_id = ?", e.ID, userID).Find(&values).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field values").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"field_values": values})
}

// ReplaceEntityFieldValues handles PUT /entities/:id/field-values: the
// full-replace twin of ReplaceContactFieldValues. Each value's definition must
// be the caller's own and target this entity's kind; values are validated by
// services.ValidateFieldValue.
func ReplaceEntityFieldValues(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	e, ok := loadOwnedEntity(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	input, verr := middleware.GetValidated[models.ContactFieldValuesInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	var defs []models.FieldDefinition
	if err := db.Where("user_id = ?", userID).Find(&defs).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field definitions").WithError(err))
		return
	}
	defByID := make(map[string]models.FieldDefinition, len(defs))
	for _, d := range defs {
		defByID[d.ID] = d
	}
	seen := map[string]bool{}
	for _, fv := range input.FieldValues {
		def, found := defByID[fv.FieldDefinitionID]
		if !found {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("field_definition_id", "unknown or not-owned field definition").WithReason(apperrors.ReasonContactFieldDefinitionUnknown, nil))
			return
		}
		if def.Target != e.Kind {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("field_definition_id", "field definition does not target this kind of entity").WithReason(apperrors.ReasonContactFieldDefinitionTargetMismatch, nil))
			return
		}
		if seen[fv.FieldDefinitionID] {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("field_definition_id", "duplicate field definition").WithReason(apperrors.ReasonContactFieldValueInvalid, map[string]any{"reason": "duplicate field definition"}))
			return
		}
		seen[fv.FieldDefinitionID] = true
		if err := services.ValidateFieldValue(def, fv.Value); err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("value", err.Error()).WithReason(apperrors.ReasonContactFieldValueInvalid, map[string]any{"reason": err.Error()}))
			return
		}
	}
	txErr := db.Transaction(func(tx *gorm.DB) error {
		del := tx.Where("entity_id = ? AND user_id = ?", e.ID, userID)
		if len(input.FieldValues) > 0 {
			del = del.Where("field_definition_id NOT IN ?", incomingDefIDs(input.FieldValues))
		}
		if err := del.Delete(&models.FieldValue{}).Error; err != nil {
			return err
		}
		for _, fv := range input.FieldValues {
			var existing models.FieldValue
			err := tx.Where("field_definition_id = ? AND entity_id = ? AND user_id = ?", fv.FieldDefinitionID, e.ID, userID).First(&existing).Error
			switch {
			case err == nil:
				existing.Value = fv.Value
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				if err := tx.Create(&models.FieldValue{FieldDefinitionID: fv.FieldDefinitionID, UserID: userID, EntityID: e.ID, Value: fv.Value}).Error; err != nil {
					return err
				}
			default:
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save field values").WithError(txErr))
		return
	}
	values := []models.FieldValue{}
	if err := db.Where("entity_id = ? AND user_id = ?", e.ID, userID).Find(&values).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve field values").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Field values saved successfully", "field_values": values})
}
