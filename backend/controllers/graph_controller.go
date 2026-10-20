package controllers

import (
	"errors"
	"fmt"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// defaultGraphDepth is the hop radius of GET /graph's default ego-network.
const defaultGraphDepth = 2

// graphScope is the resolved shape of one GET /graph request.
type graphScope struct {
	// full: return every non-archived contact (the explicit "entire network"
	// view). Otherwise the graph is the ego-network of fromUID.
	full    bool
	fromUID string
	depth   int
}

// graphActivityRow is one (activity, participant) pair of the bulk activity
// read.
type graphActivityRow struct {
	ActivityID uint
	Title      string
	ContactID  uint
}

// GetGraph returns the network graph data for visualization.
//
// Issue #1791: by default this is the ego-network (depth 2) of the caller's
// self contact -- or of ?from=<Contact.VCardUID>, with ?depth=1..5 -- not the
// whole account. The whole-account graph is an explicit opt-in
// (?full=true) and is what the web client's "show entire network" action
// asks for; an account with no self contact and no ?from= has no anchor, so
// it also gets the full graph. Every read is bound to the request context, so
// a client that disconnects (or the server's request deadline) stops the work.
func GetGraph(c *gin.Context) {
	db := requestDB(c)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	scope, ok := resolveGraphScope(c, db, userID)
	if !ok {
		return
	}

	// 1. Fetch contacts (minimal fields for performance), excluding archived.
	// "card" is included despite the encrypted-JSON decrypt cost on every row
	// (issue #1193): the deceased state has no other flat/queryable home
	// (Card is `serializer:encryptedjson`, so SQL cannot filter on it), and
	// this is the only place a deceased contact's graph node gets colored.
	// "updated_at" feeds the health score (computed for exactly these rows).
	contactQuery := db.Select("id", "vcard_uid", "firstname", "lastname", "photo_thumbnail", "circles", "card", "updated_at").
		Where("user_id = ? AND archived = ?", userID, false)
	if !scope.full {
		contactQuery = contactQuery.Where("vcard_uid IN (?)", services.GraphNeighbourhood(db, userID, scope.fromUID, scope.depth))
	}
	var contacts []models.Contact
	if err := contactQuery.Find(&contacts).Error; err != nil {
		abortQueryFailure(c, "Failed to fetch contacts", err)
		return
	}

	// 2. Fetch confirmed RelationshipEdges. Only "confirmed" is graphed --
	// never "suggested" (household-inferred edges awaiting user review):
	// RelationshipEdge.Status's own doc comment (models/relationship_edge.go)
	// says suggested edges must never be "read as a hard edge by anything
	// outside a review surface", and the graph is exactly that kind of
	// consumer. In ego mode only edges between two in-scope contacts are
	// read (an edge to a contact outside the radius would dangle).
	edgeQuery := db.Where("user_id = ? AND status = ?", userID, models.RelationshipStatusConfirmed)
	if !scope.full {
		hood := services.GraphNeighbourhood(db, userID, scope.fromUID, scope.depth)
		edgeQuery = edgeQuery.Where("source_id IN (?) AND target_id IN (?)", hood, hood)
	}
	var relationshipEdges []models.RelationshipEdge
	if err := edgeQuery.Find(&relationshipEdges).Error; err != nil {
		abortQueryFailure(c, "Failed to fetch relationship edges", err)
		return
	}

	// 3. Fetch the (activity, participant) pairs for the in-scope contacts in
	// one join -- never a Preload, whose IN (every activity id) list grows
	// with the account.
	activityQuery := db.Table("activities").
		Select("activities.id AS activity_id, activities.title AS title, ac.contact_id AS contact_id").
		Joins("JOIN activity_contacts ac ON ac.activity_id = activities.id").
		Joins("JOIN contacts c ON c.id = ac.contact_id AND c.user_id = ? AND c.archived = ?", userID, false).
		Where("activities.user_id = ? AND activities.deleted_at IS NULL", userID).
		Order("activities.id ASC, ac.contact_id ASC")
	if !scope.full {
		activityQuery = activityQuery.Where("c.vcard_uid IN (?)", services.GraphNeighbourhood(db, userID, scope.fromUID, scope.depth))
	}
	var activityRows []graphActivityRow
	if err := activityQuery.Scan(&activityRows).Error; err != nil {
		abortQueryFailure(c, "Failed to fetch activities", err)
		return
	}

	// Group participants by activity, preserving id order.
	type activityGroup struct {
		title      string
		contactIDs []uint
	}
	var activityOrder []uint
	groups := make(map[uint]*activityGroup)
	for _, row := range activityRows {
		g, seen := groups[row.ActivityID]
		if !seen {
			g = &activityGroup{title: row.Title}
			groups[row.ActivityID] = g
			activityOrder = append(activityOrder, row.ActivityID)
		}
		g.contactIDs = append(g.contactIDs, row.ContactID)
	}

	// Build nodes array
	nodes := make([]models.GraphNode, 0, len(contacts)+len(groups))

	// Relationship health score (issue #383), computed once in bulk -- never
	// per-node -- and only for the contacts being returned. A scoring failure
	// degrades the graph to its pre-#383 uncolored shape rather than 500ing
	// the whole graph, since coloring is an enhancement, not core graph data.
	scores, scoreErr := services.ComputeContactScores(db, userID, reminderNow(c), contacts)
	if scoreErr != nil {
		scores = nil
	}

	// Add contact nodes, tracking VCardUID -> node ID so RelationshipEdge
	// rows (which reference contacts by VCardUID, not the numeric ID this
	// graph's node-ID scheme uses) can be resolved below.
	nodeIDByVCardUID := make(map[string]string, len(contacts))
	for _, contact := range contacts {
		label := strings.TrimSpace(contact.Firstname + " " + contact.Lastname)
		if label == "" {
			label = "Unknown"
		}
		nodeID := fmt.Sprintf("c-%d", contact.ID)
		nodeIDByVCardUID[contact.VCardUID] = nodeID
		node := models.GraphNode{
			ID:             nodeID,
			Type:           "contact",
			Label:          label,
			PhotoThumbnail: contact.PhotoThumbnail,
			Deceased:       contact.Card.IsDeceased(),
		}
		if result, ok := scores[contact.ID]; ok {
			score := result.Score
			node.HealthScore = &score
			node.HealthBand = result.Band
		}
		nodes = append(nodes, node)
	}

	// Add activity nodes (only for activities with 2+ visible participants)
	for _, id := range activityOrder {
		if g := groups[id]; len(g.contactIDs) >= 2 {
			nodes = append(nodes, models.GraphNode{
				ID:    fmt.Sprintf("a-%d", id),
				Type:  "activity",
				Label: g.title,
			})
		}
	}

	// Build edges array
	edges := make([]models.GraphEdge, 0)

	// Add relationship edges (contact -> contact). Skip an edge if either
	// endpoint isn't in nodeIDByVCardUID (e.g. it points at an archived
	// contact, already excluded from the contacts query above) rather than
	// emitting a dangling edge referencing a node ID that doesn't exist.
	for _, edge := range relationshipEdges {
		sourceNodeID, sourceOK := nodeIDByVCardUID[edge.SourceID]
		targetNodeID, targetOK := nodeIDByVCardUID[edge.TargetID]
		if !sourceOK || !targetOK {
			continue
		}
		edges = append(edges, models.GraphEdge{
			ID:     edge.ID,
			Source: sourceNodeID,
			Target: targetNodeID,
			Type:   "relationship",
			Label:  edge.Type,
		})
	}

	// Add activity edges (star pattern: activity node -> each participating contact)
	for _, id := range activityOrder {
		g := groups[id]
		if len(g.contactIDs) < 2 {
			continue
		}
		activityNodeID := fmt.Sprintf("a-%d", id)
		for _, contactID := range g.contactIDs {
			edges = append(edges, models.GraphEdge{
				ID:     fmt.Sprintf("ae-%d-%d", id, contactID),
				Source: activityNodeID,
				Target: fmt.Sprintf("c-%d", contactID),
				Type:   "activity",
				Label:  g.title,
			})
		}
	}

	resp := models.GraphResponse{Nodes: nodes, Edges: edges, Scoped: !scope.full}
	if !scope.full {
		resp.FromVCardUID = scope.fromUID
		resp.Depth = scope.depth
	}
	c.JSON(http.StatusOK, resp)
}

// resolveGraphScope parses GET /graph's ?full= / ?from= / ?depth= and picks
// the anchor of an ego-network. It aborts the request (and returns false) on
// a malformed parameter or an unknown ?from=.
func resolveGraphScope(c *gin.Context, db *gorm.DB, userID uint) (graphScope, bool) {
	if raw := c.Query("full"); raw != "" {
		full, err := strconv.ParseBool(raw)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("full", "full must be true or false"))
			return graphScope{}, false
		}
		if full {
			return graphScope{full: true}, true
		}
	}

	depth := defaultGraphDepth
	if raw := c.Query("depth"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > services.MaxGraphDepth {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("depth", fmt.Sprintf("depth must be an integer from 1 to %d", services.MaxGraphDepth)))
			return graphScope{}, false
		}
		depth = parsed
	}

	from := strings.TrimSpace(c.Query("from"))
	if from != "" {
		var count int64
		if err := db.Model(&models.Contact{}).Where("user_id = ? AND vcard_uid = ?", userID, from).Count(&count).Error; err != nil {
			abortQueryFailure(c, "Failed to retrieve contact", err)
			return graphScope{}, false
		}
		if count == 0 {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("vcard_uid", from))
			return graphScope{}, false
		}
		return graphScope{fromUID: from, depth: depth}, true
	}

	// No explicit anchor: the caller's self contact. An account without one
	// (never logged in since T90, or admin-created) has nothing to centre on,
	// so it falls back to the full graph rather than an empty one.
	var user models.User
	if err := db.Select("self_contact_vcard_uid").Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return graphScope{full: true}, true
		}
		abortQueryFailure(c, "Failed to retrieve user", err)
		return graphScope{}, false
	}
	if user.SelfContactVCardUID == nil || *user.SelfContactVCardUID == "" {
		return graphScope{full: true}, true
	}
	return graphScope{fromUID: *user.SelfContactVCardUID, depth: depth}, true
}

// GetGraphConnections implements graph traversal + multi-hop chains (T10 /
// ): from a starting contact, list every reachable contact within
// ?depth= hops, each with the chain of relations describing how it is related
// to the anchor ("John's sister's husband"). Inferred relations (a grandparent
// from two parent_of edges) are computed, never stored.
//
// Query params:
//   - from=<Contact.VCardUID> (required, must belong to the caller)
//   - depth=1..5 (default 3; capped at 5)
//   - relation=<canonical token or registry synonym> (optional, T11's synonym
//     consumer: "brother" → sibling_of). Only chains containing a step whose
//     *display* relation equals the resolved token are returned.
func GetGraphConnections(c *gin.Context) {
	// Bound to the request context (issue #1791) like GetGraph.
	db := requestDB(c)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	from := strings.TrimSpace(c.Query("from"))
	if from == "" {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("from", "from (a Contact.VCardUID) is required"))
		return
	}

	depth := 3
	if raw := c.Query("depth"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("depth", "depth must be a positive integer"))
			return
		}
		depth = parsed
	}

	var fromContact models.Contact
	if err := db.Where("user_id = ? AND vcard_uid = ?", userID, from).First(&fromContact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("vcard_uid", from))
		} else {
			abortQueryFailure(c, "Failed to retrieve contact", err)
		}
		return
	}

	chains, err := services.TraverseGraph(db, userID, from, depth, c.Query("relation"))
	if err != nil {
		if errors.Is(err, services.ErrTraversalTooDeep) {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("depth", "depth must be at most 5"))
			return
		}
		abortQueryFailure(c, "Failed to traverse graph", err)
		return
	}

	fromName := strings.TrimSpace(fromContact.Firstname + " " + fromContact.Lastname)
	if fromName == "" {
		fromName = "Unknown"
	}

	// Decorate each chain's target with the relationship health score
	// (issue #383). This is Android's ONLY health-score-bearing surface —
	// it has no canvas graph and never calls GET /graph — so this
	// decoration is mandatory, not a mirror of GetGraph's for web's
	// benefit. Closeness is always relative to the caller's own
	// self-contact (services.ComputeAllContactScores), independent of
	// whatever `from` this traversal was anchored at.
	if scores, err := services.ComputeAllContactScores(db, userID, reminderNow(c)); err == nil {
		for i := range chains {
			if result, ok := scores[chains[i].TargetID]; ok {
				score := result.Score
				chains[i].HealthScore = &score
				chains[i].HealthBand = result.Band
			}
		}
	}

	// Decorate each chain's target with the deceased state (issue #1193) --
	// same reasoning as the health-score decoration above: this is Android's
	// only surface for it. A second bulk query (Card is encrypted, so
	// TraverseGraph's own contact resolution -- id/vcard_uid/firstname/
	// lastname only -- can't carry it) rather than widening that query for
	// every caller of TraverseGraph, most of which never look at Deceased.
	if len(chains) > 0 {
		targetUIDs := make([]string, len(chains))
		for i := range chains {
			targetUIDs[i] = chains[i].TargetVCardUID
		}
		var targets []models.Contact
		if err := db.Select("vcard_uid", "card").
			Where("user_id = ? AND vcard_uid IN ?", userID, targetUIDs).
			Find(&targets).Error; err == nil {
			deceasedByUID := make(map[string]bool, len(targets))
			for _, t := range targets {
				if t.Card.IsDeceased() {
					deceasedByUID[t.VCardUID] = true
				}
			}
			for i := range chains {
				chains[i].Deceased = deceasedByUID[chains[i].TargetVCardUID]
			}
		}
	}

	c.JSON(http.StatusOK, models.GraphConnectionsResponse{
		FromVCardUID: from,
		FromName:     fromName,
		Depth:        depth,
		Chains:       chains,
	})
}
