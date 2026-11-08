package controllers

import (
	"errors"
	"net/http"
	"strconv"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Entity merge, suggestions, dismissal and the create-from-contacts helper
// (issue #2299, ADR 0043 decisions 8, 9 and 10). Every query is scoped by
// user_id; a foreign, unknown or trashed entity id (either side of a merge or
// a dismissal) is a 404 and writes nothing. Nothing here runs unless the user
// calls the route: no background job, no import hook.

// MergeEntity handles POST /entities/:id/merge: :id is the loser, body.into
// the survivor.
func MergeEntity(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	loser, ok := loadOwnedEntity(c, db, userID, c.Param("id"))
	if !ok {
		return
	}
	input, verr := middleware.GetValidated[models.EntityMergeInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	res, err := services.MergeEntities(db, userID, loser.ID, input.Into, input.Resolutions, clock.FromContext(c).Now())
	var resErr *services.EntityMergeResolutionError
	switch {
	case errors.Is(err, services.ErrEntityMergeSame):
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic("An entity cannot be merged into itself").WithReason(apperrors.ReasonEntityMergeSameEntity, nil))
	case errors.Is(err, services.ErrEntityMergeKindMismatch):
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic("Only entities of the same kind can be merged").WithReason(apperrors.ReasonEntityMergeKindMismatch, nil))
	case errors.As(err, &resErr):
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic(resErr.Reason).WithReason(apperrors.ReasonEntityMergeResolutionInvalid, map[string]any{"reason": resErr.Reason}))
	case errors.Is(err, gorm.ErrRecordNotFound):
		abortEntityNotFound(c, input.Into)
	case err != nil:
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to merge entities").WithError(err))
	default:
		c.JSON(http.StatusOK, models.EntityMergeResponse{
			Survivor: models.NewEntityResponse(&res.Survivor), LoserID: loser.ID,
			LinksRepointed: res.LinksRepointed, LinksCollapsed: res.LinksCollapsed,
			PrimariesDemoted: res.PrimariesDemoted, FieldValuesMoved: res.FieldValuesMoved,
			FieldValuesDropped: res.FieldValuesDropped, ResolvedFields: res.ResolvedFields,
			RestoreScope: models.EntityRestoreScopeEntityOnly,
		})
	}
}

const (
	defaultSuggestionLimit = 25
	maxSuggestionLimit     = 100
)

// ListEntitySuggestions handles GET /entities/suggestions?kind=&mode=&cursor=&limit=.
// mode=pairs (default) lists candidate duplicate pairs of live entities;
// mode=contact_values lists groups of the contacts' own ORG/ADR values for the
// create-from-contacts helper (kind organisation or place required). Read-only
// and idempotent.
func ListEntitySuggestions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	kind := c.Query("kind")
	if kind != "" && !validEntityKind(kind) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("kind", "must be one of: organisation, place, pet").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "kind"}))
		return
	}
	mode := c.DefaultQuery("mode", models.EntitySuggestionModePairs)
	if mode != models.EntitySuggestionModePairs && mode != models.EntitySuggestionModeContactValues {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("mode", "must be one of: pairs, contact_values").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "mode"}))
		return
	}
	limit := defaultSuggestionLimit
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxSuggestionLimit {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("limit", "must be an integer from 1 to 100").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "limit"}))
			return
		}
		limit = n
	}
	resp := models.EntitySuggestionsResponse{Mode: mode, Pairs: []models.EntitySuggestionPair{}, Groups: []models.EntityValueGroup{}, Limit: limit}
	var err error
	if mode == models.EntitySuggestionModeContactValues {
		if kind != models.EntityKindOrganisation && kind != models.EntityKindPlace {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("kind", "contact_values mode needs kind organisation or place").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "kind"}))
			return
		}
		resp.Groups, resp.NextCursor, err = services.SuggestEntityValueGroups(db, userID, kind, c.Query("cursor"), limit)
	} else {
		resp.Pairs, resp.NextCursor, err = services.SuggestEntityPairs(db, userID, kind, c.Query("cursor"), limit)
	}
	if errors.Is(err, services.ErrBadSuggestionCursor) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("cursor", "invalid cursor").WithReason(apperrors.ReasonValidationInvalid, map[string]any{"field": "cursor"}))
		return
	}
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to compute suggestions").WithError(err))
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DismissEntitySuggestion handles POST /entities/suggestions/dismiss.
func DismissEntitySuggestion(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	input, verr := middleware.GetValidated[models.EntityDismissInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	low, high, err := services.DismissEntityPair(db, userID, input.EntityA, input.EntityB)
	switch {
	case errors.Is(err, services.ErrDismissSameEntity):
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic("A pair needs two different entities").WithReason(apperrors.ReasonEntityMergeSameEntity, nil))
	case errors.Is(err, services.ErrEntityMergeKindMismatch):
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic("Only entities of the same kind form a pair").WithReason(apperrors.ReasonEntityMergeKindMismatch, nil))
	case errors.Is(err, gorm.ErrRecordNotFound):
		abortEntityNotFound(c, input.EntityA+","+input.EntityB)
	case err != nil:
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to dismiss suggestion").WithError(err))
	default:
		c.JSON(http.StatusOK, models.EntityDismissResponse{IDLow: low, IDHigh: high})
	}
}

// CreateEntitiesFromContacts handles POST /entities/from-contacts: the
// user-confirmed half of the "create entities from my contacts" helper.
func CreateEntitiesFromContacts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	input, verr := middleware.GetValidated[models.EntityFromContactsInput](c)
	if verr != nil {
		apperrors.AbortWithError(c, verr)
		return
	}
	for _, g := range input.Groups {
		if !models.ValidEntityLinkRole(input.Kind, g.Role) {
			abortEntityLinkRole(c, input.Kind, g.Role)
			return
		}
	}
	out, err := services.CreateEntitiesFromContacts(db, userID, input.Kind, input.Groups)
	if err != nil {
		abortLinkWrite(c, err)
		return
	}
	today := todayPartial(c)
	resp := models.EntityFromContactsResponse{Created: make([]models.EntityFromContactsCreated, 0, len(out.Created)), Skipped: out.Skipped}
	for i := range out.Created {
		cr := &out.Created[i]
		links := make([]models.ContactEntityLinkResponse, 0, len(cr.Links))
		for j := range cr.Links {
			links = append(links, newContactEntityLinkResponse(&cr.Links[j], cr.Entity.Name, today))
		}
		resp.Created = append(resp.Created, models.EntityFromContactsCreated{Key: cr.Key, Entity: models.NewEntityResponse(&cr.Entity), Links: links})
	}
	c.JSON(http.StatusCreated, resp)
}
