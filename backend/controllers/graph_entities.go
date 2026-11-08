package controllers

import (
	"fmt"

	"mycorrhizal/models"
	"mycorrhizal/services"

	"gorm.io/gorm"
)

// graphEntityRow is one (live link, entity) pair of the opt-in entity read.
type graphEntityRow struct {
	LinkID          string
	ContactVCardUID string `gorm:"column:contact_vcard_uid"` // explicit: GORM would derive contact_v_card_uid (trap 1)
	EntityID        string
	Kind            string
	Name            string
}

// buildGraphEntities implements ?include_entities=true (issue #2298, ADR 0043
// decision 2). It is read-only and self-contained: it consumes only the
// already-resolved contact node IDs and the scope, and nothing it returns is
// ever fed back to traversal, household inference or the suggestion engine.
//
//   - Only live links at normal sensitivity to live entities at normal
//     sensitivity, owned by the caller (the same-owner join is in SQL).
//   - Only contacts that are graph nodes (non-archived, in scope) get edges, and
//     an entity node appears only if at least one such contact links to it.
//   - Ego scope: a contact->entity hop counts as one hop, so entities attach to
//     contacts within depth-1 hops of the anchor (the anchor alone at depth 1).
func buildGraphEntities(db *gorm.DB, userID uint, scope graphScope, nodeIDByVCardUID map[string]string) ([]models.GraphEntityNode, []models.GraphEntityEdge, error) {
	nodes := make([]models.GraphEntityNode, 0)
	edges := make([]models.GraphEntityEdge, 0)

	q := db.Table("contact_entity_links AS l").
		Select("l.id AS link_id, l.contact_vcard_uid AS contact_vcard_uid, e.id AS entity_id, e.kind AS kind, e.name AS name").
		Joins("JOIN entities e ON e.id = l.entity_id AND e.user_id = ? AND e.deleted_at IS NULL AND e.sensitivity = ?", userID, models.RelationshipSensitivityNormal).
		Where("l.user_id = ? AND l.deleted_at IS NULL AND l.sensitivity = ?", userID, models.RelationshipSensitivityNormal).
		Order("l.id ASC")
	if !scope.full {
		q = q.Where("l.contact_vcard_uid IN (?)", services.GraphNeighbourhood(db, userID, scope.fromUID, scope.depth-1))
	}
	var rows []graphEntityRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, nil, err
	}

	seen := make(map[string]bool)
	for _, r := range rows {
		contactNode, ok := nodeIDByVCardUID[r.ContactVCardUID]
		if !ok {
			continue // archived / not a graph node: never a dangling edge
		}
		entityNode := "e-" + r.EntityID
		if !seen[r.EntityID] {
			seen[r.EntityID] = true
			nodes = append(nodes, models.GraphEntityNode{ID: entityNode, Kind: r.Kind, Label: r.Name})
		}
		edges = append(edges, models.GraphEntityEdge{ID: fmt.Sprintf("ee-%s", r.LinkID), Source: contactNode, Target: entityNode})
	}
	return nodes, edges, nil
}
