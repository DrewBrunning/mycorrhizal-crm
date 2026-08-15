package services

import (
	"errors"
	"fmt"
	"mycorrhizal/models"
	"strings"

	"gorm.io/gorm"
)

// Graph traversal + multi-hop chains (T10 / WP-85, T10): "Teddy's owner", "John's sister's husband".
//
// The traversal is a recursive CTE over relationship_edges. Inferred
// relations (a grandparent from two parent_of edges) are computed at query
// time, never stored — the same discipline the model applies to never storing
// a reciprocal edge.
//
// Direction semantics (the single highest-risk piece of this ticket): a
// stored edge means "Source is <type>-relative to Target". The displayed
// relation at each hop is "what the NEXT contact is to the CURRENT contact":
//
//   - hopping source→target (forward along the stored edge): the target is
//     the INVERSE of the stored type relative to the source (a parent_of edge
//     walked parent→child displays child_of);
//   - hopping target→source (against the stored direction): the source IS the
//     stored type relative to the target (displays parent_of).
//
// This matches the frontend's existing edge-display convention
// (api/relationshipEdges.ts getDisplayLabel) and is pinned in both directions
// by TestGraphTraversalDirection*.
const (
	// maxTraversalDepth bounds the recursive CTE so a cyclic graph can never
	// run away. The controller also caps the user-supplied depth to this.
	maxTraversalDepth = 5
)

// ErrTraversalTooDeep is returned when a requested depth exceeds
// maxTraversalDepth.
var ErrTraversalTooDeep = errors.New("traversal depth exceeds the maximum")

// traversalRow is one row of the recursive CTE: a reachable contact plus the
// accumulated path/edge-type/direction strings that reconstruct the chain.
type traversalRow struct {
	CurrUID string
	Path    string
	Types   string
	Dirs    string
	Depth   int
}

// TraverseGraph returns every contact reachable from fromUID within maxDepth
// hops, each with its chain of relation steps, using a recursive CTE over
// relationship_edges.
//
// Participation rules (matching GetGraph and tightening where the ticket
// demands it):
//   - only Status: confirmed edges (a suggested edge is never treated as a
//     hard fact outside a review surface);
//   - sensitivity != secret is excluded (a secret edge must not leak
//     into a derived traversal result); private edges remain visible, the same
//     as the graph display.
//
// The optional filterRelation, when non-empty, is resolved through the
// relation-type registry (canonical token or synonym — "brother" →
// sibling_of, T11's synonym consumer) and chains are returned only when at
// least one step's *display* relation equals it. fromUID must belong to
// userID; the caller verifies ownership.
func TraverseGraph(db *gorm.DB, userID uint, fromUID string, maxDepth int, filterRelation string) ([]models.GraphChain, error) {
	if maxDepth < 1 || maxDepth > maxTraversalDepth {
		return nil, ErrTraversalTooDeep
	}

	// Resolve the synonym/token to a canonical relation, if one was given.
	var canonicalFilter string
	if filterRelation != "" {
		resolved, ok := models.MatchLegacyRelationType(filterRelation)
		if !ok {
			// An unresolvable relation filter is a query with no matches, not
			// an error.
			return []models.GraphChain{}, nil
		}
		canonicalFilter = resolved
	}

	// The recursive CTE. Each recursion step joins the current contact to its
	// edges (either endpoint), moves to the other endpoint, and appends the
	// raw edge type + a direction flag ('0' = forward along the stored edge,
	// '1' = against it). Visited-set: the path string holds every uid seen on
	// this branch, so cycles terminate per-branch; the depth cap bounds the
	// branch length regardless.
	//
	// instr(path, uid) = 0 is the visited check — each uid is a full UUID, so
	// a substring match is unambiguous.
	const cte = `
		WITH RECURSIVE traverse AS (
			SELECT
				CAST(? AS TEXT) AS curr_uid,
				CAST(? AS TEXT) AS path,
				CAST('' AS TEXT) AS types,
				CAST('' AS TEXT) AS dirs,
				0 AS depth
			UNION ALL
			SELECT
				CASE WHEN e.source_id = t.curr_uid THEN e.target_id ELSE e.source_id END,
				t.path || '>' || (CASE WHEN e.source_id = t.curr_uid THEN e.target_id ELSE e.source_id END),
				t.types || '|' || e.type,
				t.dirs || '|' || CASE WHEN e.source_id = t.curr_uid THEN '0' ELSE '1' END,
				t.depth + 1
			FROM traverse t
			JOIN relationship_edges e
				ON (e.source_id = t.curr_uid OR e.target_id = t.curr_uid)
				AND e.user_id = ?
				AND e.status = ?
				AND e.sensitivity != ?
			WHERE t.depth < ?
				AND instr(t.path, CASE WHEN e.source_id = t.curr_uid THEN e.target_id ELSE e.source_id END) = 0
		)
		SELECT curr_uid, path, types, dirs, depth
		FROM traverse
		WHERE depth > 0
		ORDER BY depth, curr_uid`

	var rows []traversalRow
	if err := db.Raw(cte,
		fromUID,
		fromUID,
		userID,
		models.RelationshipStatusConfirmed,
		models.RelationshipSensitivitySecret,
		maxDepth,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("graph traversal: %w", err)
	}

	// Resolve display names for every contact that appears in any chain in one
	// batched query (avoids N+1).
	seenUIDs := map[string]bool{}
	for _, r := range rows {
		for _, uid := range strings.Split(r.Path, ">") {
			seenUIDs[uid] = true
		}
	}
	names := map[string]string{}
	ids := map[string]uint{}
	if len(seenUIDs) > 0 {
		uids := make([]string, 0, len(seenUIDs))
		for uid := range seenUIDs {
			uids = append(uids, uid)
		}
		var contacts []models.Contact
		if err := db.Select("id", "vcard_uid", "firstname", "lastname").
			Where("user_id = ? AND vcard_uid IN ?", userID, uids).
			Find(&contacts).Error; err != nil {
			return nil, fmt.Errorf("graph traversal: resolve contacts: %w", err)
		}
		for _, c := range contacts {
			name := strings.TrimSpace(c.Firstname + " " + c.Lastname)
			if name == "" {
				name = "Unknown"
			}
			names[c.VCardUID] = name
			ids[c.VCardUID] = c.ID
		}
	}

	chains := make([]models.GraphChain, 0, len(rows))
	for _, r := range rows {
		uids := strings.Split(r.Path, ">")
		rawTypes := splitLeadingSeparator(r.Types)
		dirs := splitLeadingSeparator(r.Dirs)

		steps := make([]models.GraphChainStep, 0, len(uids)-1)
		for i := 0; i < len(uids)-1; i++ {
			raw := rawTypes[i]
			relation := raw
			// dir '0' = forward along the stored edge → the next contact is
			// the inverse of the stored type relative to the current one;
			// dir '1' = against the stored direction → the stored type stands.
			if dirs[i] == "0" {
				if inv := models.InverseRelationType(raw); inv != "" {
					relation = inv
				}
			}
			steps = append(steps, models.GraphChainStep{
				ContactID:       ids[uids[i+1]],
				ContactVCardUID: uids[i+1],
				ContactName:     names[uids[i+1]],
				Relation:        relation,
			})
		}

		targetUID := uids[len(uids)-1]
		chains = append(chains, models.GraphChain{
			TargetID:       ids[targetUID],
			TargetVCardUID: targetUID,
			TargetName:     names[targetUID],
			Depth:          r.Depth,
			Steps:          steps,
		})
	}

	// De-dup chains targeting the same contact, keeping the shortest one (the
	// CTE can reach the same target via multiple branches, e.g. a diamond).
	// The relation filter is applied BEFORE dedup: otherwise, when the same
	// target is reachable at the same depth via a matching chain and a
	// non-matching one, which chain survives depends on SQLite's (unpredictable)
	// CTE expansion order — the matching chain could be dropped even though it
	// is the answer the filter asks for. Filtering first makes "shortest
	// matching chain per target" deterministic.
	best := map[string]models.GraphChain{}
	for _, ch := range chains {
		if canonicalFilter != "" && !chainHasRelation(ch, canonicalFilter) {
			continue
		}
		cur, ok := best[ch.TargetVCardUID]
		if !ok || ch.Depth < cur.Depth {
			best[ch.TargetVCardUID] = ch
		}
	}
	result := make([]models.GraphChain, 0, len(best))
	for _, ch := range best {
		result = append(result, ch)
	}
	if len(result) > 1 {
		sortChainsByDepth(result)
	}
	return result, nil
}

// chainHasRelation reports whether any step's display relation equals want.
func chainHasRelation(ch models.GraphChain, want string) bool {
	for _, s := range ch.Steps {
		if s.Relation == want {
			return true
		}
	}
	return false
}

// splitLeadingSeparator splits a '|'-joined accumulator string, tolerating the
// empty leading element the CTE's `|| '|' || ...` accumulation produces.
func splitLeadingSeparator(s string) []string {
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "|") {
		s = s[1:]
	}
	return strings.Split(s, "|")
}

// sortChainsByDepth orders chains shallowest-first, breaking ties by target
// name so the result is deterministic (the chains are otherwise assembled from
// a map's arbitrary iteration order).
func sortChainsByDepth(chains []models.GraphChain) {
	for i := 1; i < len(chains); i++ {
		for j := i; j > 0; j-- {
			if chains[j-1].Depth < chains[j].Depth {
				break
			}
			if chains[j-1].Depth == chains[j].Depth && chains[j-1].TargetName <= chains[j].TargetName {
				break
			}
			chains[j-1], chains[j] = chains[j], chains[j-1]
		}
	}
}
