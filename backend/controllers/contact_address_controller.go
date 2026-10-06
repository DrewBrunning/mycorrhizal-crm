package controllers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
)

// rejectInvalidAddressMapFields aborts with a 400 and returns true when a
// submitted Card carries an address coordinate, sensitivity or ID that cannot
// be stored (ADR 0031). Same shape as rejectInvalidPeriods: a reference the
// server cannot honour is a caller error, never a silent drop.
func rejectInvalidAddressMapFields(c *gin.Context, card contactmodel.Card) bool {
	problems := models.ValidateAddressMapFields(card.Addresses)
	if len(problems) == 0 {
		return false
	}
	appErr := apperrors.ErrValidation("Request validation failed")
	for field, reason := range problems {
		appErr = appErr.WithDetails(field, reason)
	}
	apperrors.AbortWithError(c, appErr)
	return true
}

// MapConfigHandler serves GET /api/v1/config/map: the one unauthenticated
// bootstrap value the map clients need before they hold a session-scoped call,
// the MapLibre style URL. It mirrors OIDCConfigHandler and is deliberately
// narrow — no other instance setting may be added to this response (the value
// is public because the client fetches tiles from it directly).
func MapConfigHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tile_style_url": cfg.EffectiveMapTileStyleURL()})
	}
}

// addressGeocoder is the slice of *services.Geocoder the handler uses, so the
// controller tests can substitute a fake provider without opening a socket.
type addressGeocoder interface {
	Enabled() bool
	GeocodeAddress(ctx context.Context, userID uint, addr models.ContactAddress) (coordinates string, cached bool, err error)
}

// GeocodeContactAddress handles POST /contacts/:id/addresses/:addressId/geocode
// (ADR 0031). It is the ONLY path that ever sends address text to the
// geocoder: one explicit lookup for one address, never automatic, never bulk.
//
// An address above normal sensitivity is refused with 400 unless the request
// carries ?include_sensitive=true (the opt-in shape exports use). On success the
// resolved geo: URI is written to the address (flat copy and Card entry) and
// returned; an address that already had coordinates is overwritten, since the
// caller explicitly asked for a fresh lookup.
func GeocodeContactAddress(geocoder addressGeocoder) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := requirePathUintID(c, "id")
		if !ok {
			return
		}
		addressID := c.Param("addressId")
		db := c.MustGet("db").(*gorm.DB)
		userID, ok := currentUserID(c)
		if !ok {
			return
		}

		var contact models.Contact
		if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
			} else {
				apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
			}
			return
		}

		addr, _, found := contact.FindAddressByID(addressID)
		if !found {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Address").WithDetails("address_id", addressID))
			return
		}

		// Sensitivity gate first: a refused address must never reach the
		// geocoder, and the answer must not depend on whether geocoding is on.
		if addr.Sensitivity != "" && addr.Sensitivity != models.RelationshipSensitivityNormal && !includeSensitiveRequested(c) {
			apperrors.AbortWithError(c, apperrors.ErrValidation("This address is marked "+addr.Sensitivity+
				"; send include_sensitive=true to geocode it").
				WithDetails("sensitivity", addr.Sensitivity))
			return
		}

		if !geocoder.Enabled() {
			apperrors.AbortWithError(c, apperrors.ErrBusinessLogic(services.ErrGeocoderDisabled.Error()))
			return
		}

		coordinates, cached, err := geocoder.GeocodeAddress(c.Request.Context(), userID, addr)
		if err != nil {
			abortGeocodeError(c, err)
			return
		}

		if !contact.SetAddressCoordinates(addressID, coordinates) { // # pragma: no cover — the address was found above on this same contact value
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Address").WithDetails("address_id", addressID))
			return
		}
		if err := db.Save(&contact).Error; err != nil {
			if handleRevisionConflict(c, "Contact", err) {
				return
			}
			logger.FromContext(c).Error().Err(err).Msg("Error saving geocoded address coordinates")
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save coordinates").WithError(err))
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"address_id":  addressID,
			"coordinates": coordinates,
			"cached":      cached,
		})
	}
}

// GeocodeContactAddressDraft handles POST /contacts/:id/addresses/geocode
// (ADR 0031 amendment, issue #1286 follow-up): the stateless counterpart of
// GeocodeContactAddress. It geocodes the address *as submitted in the body*
// and returns the coordinate WITHOUT writing anything, so the editor can
// resolve coordinates for a not-yet-saved address or one whose text has
// unsaved edits. The coordinate then rides the ordinary contact save — which
// means Discard correctly reverts it, unlike the persisted route.
//
// Same privacy boundary as the persisted route: one explicit lookup, only the
// postal fields (street, city, region, postcode, country) are sent, and an
// address above normal sensitivity is refused with 400 unless the request
// carries ?include_sensitive=true. The contact lookup is ownership-scoped so
// an unknown or other user's contact is a 404, never a cross-tenant anchor.
func GeocodeContactAddressDraft(geocoder addressGeocoder) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := requirePathUintID(c, "id")
		if !ok {
			return
		}
		input, verr := middleware.GetValidated[models.GeocodeDraftInput](c)
		if verr != nil {
			apperrors.AbortWithError(c, verr)
			return
		}
		db := c.MustGet("db").(*gorm.DB)
		userID, ok := currentUserID(c)
		if !ok {
			return
		}

		// Ownership anchor, matching the persisted route: the contact must be
		// this user's (and not soft-deleted). Its contents are not read — the
		// body is authoritative — but the row must exist so a draft lookup
		// cannot be driven through a stranger's contact.
		var contact models.Contact
		if err := db.Select("id").Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
			} else {
				apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
			}
			return
		}

		addr := input.ToContactAddress()
		// Sensitivity gate first, exactly as the persisted route: a refused
		// address must never reach the geocoder, and the answer must not
		// depend on whether geocoding is enabled.
		if addr.Sensitivity != "" && addr.Sensitivity != models.RelationshipSensitivityNormal && !includeSensitiveRequested(c) {
			apperrors.AbortWithError(c, apperrors.ErrValidation("This address is marked "+addr.Sensitivity+
				"; send include_sensitive=true to geocode it").
				WithDetails("sensitivity", addr.Sensitivity))
			return
		}

		if !geocoder.Enabled() {
			apperrors.AbortWithError(c, apperrors.ErrBusinessLogic(services.ErrGeocoderDisabled.Error()))
			return
		}

		coordinates, cached, err := geocoder.GeocodeAddress(c.Request.Context(), userID, addr)
		if err != nil {
			abortGeocodeError(c, err)
			return
		}

		// No persistence: the client holds the coordinate in the draft until
		// the contact is saved through the ordinary update path.
		c.JSON(http.StatusOK, gin.H{
			"coordinates": coordinates,
			"cached":      cached,
		})
	}
}

// includeSensitiveRequested reads the ?include_sensitive= opt-in the same way
// the export handlers do (true or 1).
func includeSensitiveRequested(c *gin.Context) bool {
	switch c.Query("include_sensitive") {
	case "true", "1":
		return true
	}
	return false
}

// abortGeocodeError maps a geocoder client error onto an API error. Messages
// are fixed strings: the client already stripped the request URL (and the
// MapTiler key) from anything it wrapped, and nothing provider-supplied is
// echoed.
func abortGeocodeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrGeocoderDisabled):
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic(services.ErrGeocoderDisabled.Error()))
	case errors.Is(err, services.ErrGeocoderNoResult):
		apperrors.AbortWithError(c, apperrors.ErrBusinessLogic(services.ErrGeocoderNoResult.Error()))
	case errors.Is(err, services.ErrGeocoderRateLimited):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Geocoder", "the provider is rate limiting requests; try again shortly"))
	case errors.Is(err, services.ErrGeocoderUnauthorized):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Geocoder", "the provider rejected this server's credentials (check GEOCODER_API_KEY)"))
	case errors.Is(err, services.ErrGeocoderPrivateAddr):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Geocoder", "the provider resolved to a private address and was blocked"))
	case errors.Is(err, services.ErrGeocoderInvalidData):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Geocoder", "the provider returned an unusable response"))
	case errors.Is(err, services.ErrGeocoderUnreachable), errors.Is(err, services.ErrGeocoderNotFound),
		errors.Is(err, services.ErrGeocoderRequestFailed):
		apperrors.AbortWithError(c, apperrors.ErrExternal("Geocoder", "the provider could not be reached"))
	default:
		logger.FromContext(c).Error().Err(err).Msg("Unexpected geocoder error")
		apperrors.AbortWithError(c, apperrors.ErrInternal("Geocoding failed").WithError(err))
	}
}
