package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
)

// MaxContactMapPoints is the response ceiling for GET /contacts/map. The
// endpoint is not paginated — a map needs every point at once — so it is
// bounded instead: past the ceiling the response carries the first
// MaxContactMapPoints points in contact-ID order and truncated=true, which a
// client surfaces as "showing N of many" rather than silently dropping pins.
const MaxContactMapPoints = 5000

// ContactMapPoint is one plottable address on the contact map.
type ContactMapPoint struct {
	ContactID   uint   `json:"contact_id"`
	ContactUID  string `json:"contact_uid"`
	ContactName string `json:"contact_name"`
	AddressID   string `json:"address_id"`
	Label       string `json:"label"`
	Coordinates string `json:"coordinates"`
}

// ContactMapResponse is the GET /contacts/map body. Points carries no
// omitempty: an empty map is "points": [] (frontend trap #8).
type ContactMapResponse struct {
	Points    []ContactMapPoint `json:"points"`
	Truncated bool              `json:"truncated"`
}

// GetContactMap handles GET /contacts/map (issue #1427, ADR 0031): one item per
// address with a valid geo: coordinate across the caller's own non-archived,
// non-deleted contacts, in one query.
//
// It is deliberately NOT sensitivity-gated: private/secret addresses are
// included with no include_sensitive parameter. This is the owner's own view of
// their own data, not an export, sync or share — the same rule as the CSV and
// account-bundle exports (see CLAUDE.md "Sensitivity"). The vCard/JSContact
// exports and contact shares still exclude them, via
// models.ApplyFieldSelection's address filter (models/field_selection.go) —
// not by gating this owner-facing read.
func GetContactMap(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// addresses is a JSON text column, so the SQL pre-filter only narrows to
	// contacts that mention a coordinate at all; the authoritative range check
	// is ParseGeoURI below.
	rows, err := db.Model(&models.Contact{}).
		Select("id", "vcard_uid", "firstname", "lastname", "nickname", "org", "addresses").
		Where("user_id = ?", userID).
		Where("archived = ?", false).
		Where("addresses LIKE ?", `%"coordinates"%`).
		Order("id").
		Rows()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact map").WithError(err))
		return
	}
	defer rows.Close()

	resp := ContactMapResponse{Points: []ContactMapPoint{}}
scan:
	for rows.Next() {
		var contact models.Contact
		if err := db.ScanRows(rows, &contact); err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to read contact map").WithError(err))
			return
		}
		// An org-only or nickname-only contact has no first/last name; fall back
		// so the popup and the accessible list never show a blank name.
		name := strings.TrimSpace(contact.Firstname + " " + contact.Lastname)
		if name == "" {
			name = strings.TrimSpace(contact.Nickname)
		}
		if name == "" {
			name = strings.TrimSpace(contact.Org)
		}
		for _, a := range contact.Addresses {
			if a.ID == "" {
				continue
			}
			if _, _, ok := contactmodel.ParseGeoURI(a.Coordinates); !ok {
				continue // malformed stored value: skipped, never an error
			}
			if len(resp.Points) >= MaxContactMapPoints {
				resp.Truncated = true
				break scan
			}
			resp.Points = append(resp.Points, ContactMapPoint{
				ContactID:   contact.ID,
				ContactUID:  contact.VCardUID,
				ContactName: name,
				AddressID:   a.ID,
				Label:       models.FormatAddress(a),
				Coordinates: a.Coordinates,
			})
		}
	}
	if err := rows.Err(); err != nil { // # pragma: no cover — driver-level mid-iteration failure
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to read contact map").WithError(err))
		return
	}

	c.JSON(http.StatusOK, resp)
}
