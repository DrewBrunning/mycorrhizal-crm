package services

import (
	"fmt"
	"sort"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// T104 — suggest relationships from relationships (graph inference).
//
// The household engine (GenerateHouseholdSuggestions) only proposes edges
// *within* an existing household. This engine is the graph-level analog: one
// run composes confirmed edges two hops at a time and proposes the
// relationship the composition implies. It is the first producer of
// Source: graph-inferred suggestions and consumes the same review surface
// (status: suggested edges + accept/reject) every other suggester does.
// ---------------------------------------------------------------------------

// graphSuggestionRule is one row of the T104 rule table: a (relAtoX, relXtoB)
// pair of *displayed* relations along a length-2 path A–X–B, and the edge
// the composition implies.
//
// relAtoX is "what A is to X", relXtoB is "what X is to B" — both resolved in
// the walk direction A→X→B (inverting a stored edge's type via
// models.InverseRelationType exactly as graph_traversal.go does for display).
// The inferred token is stored as a fresh edge A→B.
//
// Deliberately just these three rows: grandparent (parent·parent), aunt/uncle
// (sibling·parent) and niece/nephew (child·sibling) are out of scope for v1
// (the ticket's "errs liberal within three" call — suggestions are
// accepted-or-rejected, never fact).
type graphSuggestionRule struct {
	relAtoX    string
	relXtoB    string
	inferred   string
	confidence float64
}

// graphSuggestionRules is the locked rule table. Confidence is a coarse
// per-rule constant for ranking only ("definitely" above "probably" in the
// flat review list); it never gates the type and there is no thresholding.
var graphSuggestionRules = []graphSuggestionRule{
	// R1 — sibling · sibling: the classic "your sibling's sibling is your
	// sibling". "Definitely."
	{relAtoX: "sibling_of", relXtoB: "sibling_of", inferred: "sibling_of", confidence: 0.9},
	// R2 — parent · sibling: "your sibling's parent is your parent". The
	// shared-parent inference. "Probably."
	{relAtoX: "parent_of", relXtoB: "sibling_of", inferred: "parent_of", confidence: 0.7},
	// R3 — spouse · parent: "your spouse's child is your child". The
	// step-parent inference, hence the lower confidence. "Probably."
	{relAtoX: "spouse_of", relXtoB: "parent_of", inferred: "parent_of", confidence: 0.5},
}

// graphAdjEntry is one neighbor of a contact along a confirmed edge, with the
// relation of THAT NEIGHBOR relative to the contact (e.g. for the stored edge
// A parent_of B, B's entry under A is {uid: B, rel: child_of} — "B is A's
// child"). Storing the neighbor-relative relation makes both rule tokens
// derivable from the middle node's adjacency list without extra lookups.
type graphAdjEntry struct {
	uid string
	rel string
}

// GenerateGraphSuggestions is the T104 engine: one round of two-hop
// composition over status: confirmed edges for userID.
//
// Seeding reads only confirmed edges with sensitivity != secret (mirroring
// graph_traversal.go's participation rules — a secret edge must not leak into
// a derived suggestion). Emitted suggestions are status: suggested,
// Source: graph-inferred, sensitivity: normal, with the per-rule confidence.
//
// One run = exactly one round: multi-hop propagation is the user pressing the
// trigger again, never a fixpoint (small, dense graphs in practice). Idempotent
// like the household engine — suggestEdgeIfNew never duplicates an existing
// edge (in either storage direction, any status), so re-running is safe.
//
// Returns the edges newly created by this call, sorted by (source_id,
// target_id, type) so the response and tests are deterministic.
func GenerateGraphSuggestions(db *gorm.DB, userID uint) ([]models.RelationshipEdge, error) {
	var edges []models.RelationshipEdge
	if err := db.Where("user_id = ? AND status = ? AND sensitivity != ?", userID, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret).
		Find(&edges).Error; err != nil {
		return nil, fmt.Errorf("loading confirmed edges for graph suggestions: %w", err)
	}
	if len(edges) < 2 {
		return []models.RelationshipEdge{}, nil
	}

	adjacency := map[string][]graphAdjEntry{}
	for _, e := range edges {
		adjacency[e.SourceID] = append(adjacency[e.SourceID], graphAdjEntry{uid: e.TargetID, rel: models.InverseRelationType(e.Type)})
		adjacency[e.TargetID] = append(adjacency[e.TargetID], graphAdjEntry{uid: e.SourceID, rel: e.Type})
	}

	created := []models.RelationshipEdge{}
	for _, neighbors := range adjacency {
		for _, a := range neighbors {
			for _, b := range neighbors {
				// A and B are the path's endpoints; the middle is this node.
				// The same contact must not appear at both ends (self-loop),
				// and a duplicate edge pair must not fire twice.
				if a.uid == b.uid {
					continue
				}
				// relAtoX: "what A is to X" is stored directly on A's entry.
				// relXtoB: "what X is to B" is the inverse of "what B is to X"
				// (which is what B's entry stores).
				relXtoB := models.InverseRelationType(b.rel)
				for _, rule := range graphSuggestionRules {
					if a.rel != rule.relAtoX || relXtoB != rule.relXtoB {
						continue
					}
					edge, err := suggestEdgeIfNew(db, userID, a.uid, b.uid, rule.inferred, models.RelationshipSourceGraphInferred, rule.confidence)
					if err != nil {
						return created, err
					}
					if edge != nil {
						created = append(created, *edge)
					}
				}
			}
		}
	}

	sort.Slice(created, func(i, j int) bool {
		if created[i].SourceID != created[j].SourceID {
			return created[i].SourceID < created[j].SourceID
		}
		if created[i].TargetID != created[j].TargetID {
			return created[i].TargetID < created[j].TargetID
		}
		return created[i].Type < created[j].Type
	})
	return created, nil
}
