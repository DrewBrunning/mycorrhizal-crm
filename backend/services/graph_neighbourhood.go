package services

import (
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// MaxGraphDepth is the deepest ego-network GET /graph?from=&depth= will walk.
// It is the traversal cap (maxTraversalDepth) so both graph reads share one
// limit.
const MaxGraphDepth = maxTraversalDepth

// neighbourhoodCTE is the set of contact vCard UIDs within `depth` hops of a
// start UID over confirmed relationship edges (issue #1791). Unlike
// traversalCTE it tracks only (uid, depth) -- never the path -- and uses
// UNION (not UNION ALL), so a node reached by many routes is expanded once
// per depth level rather than once per distinct path. The work is bounded by
// nodes x depth instead of the number of simple paths, which on a dense
// graph is the difference between milliseconds and the 8-9 minute
// whole-graph walk the issue measured.
//
// The edge filter deliberately matches GetGraph's own edge query (every
// confirmed edge, any sensitivity -- it is the owner's own view), NOT
// TraverseGraph's (which drops `secret` edges because its output feeds derived
// results). The two recursive terms are anchored on source_id / target_id
// separately with INDEXED BY, for the same reason traversalCTE's are (an OR
// falls back to the far less selective user_id index, issue #792).
//
// Bind order: from_uid, then per term (user_id, status, max_depth) twice.
const neighbourhoodCTE = `
	WITH RECURSIVE nb(uid, depth) AS (
		SELECT CAST(? AS TEXT), 0
		UNION
		SELECT e.target_id, nb.depth + 1
		FROM nb
		JOIN relationship_edges e INDEXED BY idx_relationship_edges_source_id
			ON e.source_id = nb.uid AND e.user_id = ? AND e.status = ?
		WHERE nb.depth < ?
		UNION
		SELECT e.source_id, nb.depth + 1
		FROM nb
		JOIN relationship_edges e INDEXED BY idx_relationship_edges_target_id
			ON e.target_id = nb.uid AND e.user_id = ? AND e.status = ?
		WHERE nb.depth < ?
	)
	SELECT DISTINCT uid FROM nb`

// GraphNeighbourhood returns a sub-select yielding the vCard UID of fromUID
// and every contact within depth hops of it over confirmed relationship
// edges, for use as `vcard_uid IN (?)` / `source_id IN (?)` in another query
// on the same connection. It is lazy -- nothing runs until the outer query
// does -- so the context bound to db covers it.
func GraphNeighbourhood(db *gorm.DB, userID uint, fromUID string, depth int) *gorm.DB {
	return db.Raw(neighbourhoodCTE,
		fromUID,
		userID, models.RelationshipStatusConfirmed, depth,
		userID, models.RelationshipStatusConfirmed, depth,
	)
}
