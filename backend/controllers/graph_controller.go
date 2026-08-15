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

// GetGraph returns the network graph data for visualization
func GetGraph(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// 1. Fetch all contacts (minimal fields for performance), excluding archived
	var contacts []models.Contact
	if err := db.Select("id", "vcard_uid", "firstname", "lastname", "photo_thumbnail", "circles").
		Where("user_id = ? AND archived = ?", userID, false).
		Find(&contacts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch contacts"})
		return
	}

	// 2. Fetch confirmed RelationshipEdges. Only "confirmed" is graphed --
	// never "suggested" (household-inferred edges awaiting user review):
	// RelationshipEdge.Status's own doc comment (models/relationship_edge.go)
	// says suggested edges must never be "read as a hard edge by anything
	// outside a review surface", and the graph is exactly that kind of
	// consumer.
	var relationshipEdges []models.RelationshipEdge
	if err := db.Where("user_id = ? AND status = ?", userID, models.RelationshipStatusConfirmed).
		Find(&relationshipEdges).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch relationship edges"})
		return
	}

	// 3. Fetch all activities with their contacts
	var activities []models.Activity
	if err := db.Preload("Contacts", func(db *gorm.DB) *gorm.DB {
		return db.Select("id").Where("user_id = ?", userID)
	}).Where("user_id = ?", userID).Find(&activities).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch activities"})
		return
	}

	// Build nodes array
	nodes := make([]models.GraphNode, 0, len(contacts)+len(activities))

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
		nodes = append(nodes, models.GraphNode{
			ID:             nodeID,
			Type:           "contact",
			Label:          label,
			PhotoThumbnail: contact.PhotoThumbnail,
		})
	}

	// Add activity nodes (only for activities with 2+ contacts)
	activityNodeIDs := make(map[uint]bool)
	for _, activity := range activities {
		if len(activity.Contacts) >= 2 {
			nodes = append(nodes, models.GraphNode{
				ID:    fmt.Sprintf("a-%d", activity.ID),
				Type:  "activity",
				Label: activity.Title,
			})
			activityNodeIDs[activity.ID] = true
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
	for _, activity := range activities {
		if activityNodeIDs[activity.ID] {
			activityNodeID := fmt.Sprintf("a-%d", activity.ID)
			for _, contact := range activity.Contacts {
				edges = append(edges, models.GraphEdge{
					ID:     fmt.Sprintf("ae-%d-%d", activity.ID, contact.ID),
					Source: activityNodeID,
					Target: fmt.Sprintf("c-%d", contact.ID),
					Type:   "activity",
					Label:  activity.Title,
				})
			}
		}
	}

	c.JSON(http.StatusOK, models.GraphResponse{
		Nodes: nodes,
		Edges: edges,
	})
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
	db := c.MustGet("db").(*gorm.DB)
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
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	chains, err := services.TraverseGraph(db, userID, from, depth, c.Query("relation"))
	if err != nil {
		if errors.Is(err, services.ErrTraversalTooDeep) {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("depth", fmt.Sprintf("depth must be at most 5")))
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to traverse graph").WithError(err))
		return
	}

	fromName := strings.TrimSpace(fromContact.Firstname + " " + fromContact.Lastname)
	if fromName == "" {
		fromName = "Unknown"
	}

	c.JSON(http.StatusOK, models.GraphConnectionsResponse{
		FromVCardUID: from,
		FromName:     fromName,
		Depth:        depth,
		Chains:       chains,
	})
}
