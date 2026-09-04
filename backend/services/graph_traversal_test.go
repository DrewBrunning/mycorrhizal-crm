package services

import (
	"mycorrhizal/models"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newGraphTestDB migrates the models the traversal tests touch.
func newGraphTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.RelationshipEdge{}))
	return db
}

// seedGraphContacts creates n contacts and returns them by index.
func seedGraphContacts(t *testing.T, db *gorm.DB, userID uint, names ...string) []models.Contact {
	t.Helper()
	contacts := make([]models.Contact, len(names))
	for i, name := range names {
		c := models.Contact{UserID: userID, Firstname: name}
		require.NoError(t, db.Create(&c).Error)
		contacts[i] = c
	}
	return contacts
}

// addEdge is a helper to add a confirmed edge between two contacts.
func addEdge(t *testing.T, db *gorm.DB, userID uint, source, target models.Contact, relType string, status, sensitivity string) {
	t.Helper()
	e := models.RelationshipEdge{
		UserID: userID, SourceID: source.VCardUID, TargetID: target.VCardUID,
		Type: relType, Directional: !models.IsSymmetricRelationType(relType),
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: status, Sensitivity: sensitivity,
	}
	require.NoError(t, db.Create(&e).Error)
}

func TestTraverseGraph_TwoHopChain(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse", Password: "password123!A", Email: "traverse@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// John --(sibling_of)--> Sister --(spouse_of)--> Husband
	john, sister, husband := seedGraphContacts(t, db, user.ID, "John", "Sister", "Husband")[0], seedGraphContacts(t, db, user.ID, "John", "Sister", "Husband")[1], seedGraphContacts(t, db, user.ID, "John", "Sister", "Husband")[2]
	// Edges are stored "source is <type> of target": source=Sister, target=John
	// means Sister is John's sister. So traversal from John hops backward.
	addEdge(t, db, user.ID, sister, john, "sibling_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, husband, sister, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	chains, err := TraverseGraph(db, user.ID, john.VCardUID, 3, "")
	require.NoError(t, err)
	require.Len(t, chains, 2, "sister (depth 1) and husband (depth 2) are both reachable")

	// Find the depth-2 chain: John's sister's husband.
	var husbandChain *models.GraphChain
	for i := range chains {
		if chains[i].TargetVCardUID == husband.VCardUID {
			husbandChain = &chains[i]
		}
	}
	require.NotNil(t, husbandChain, "husband must be reachable")
	assert.Equal(t, 2, husbandChain.Depth)
	require.Len(t, husbandChain.Steps, 2)
	assert.Equal(t, "sibling_of", husbandChain.Steps[0].Relation, "first hop: Sister is John's sister")
	assert.Equal(t, "spouse_of", husbandChain.Steps[1].Relation, "second hop: Husband is Sister's husband")
	assert.Equal(t, "Husband", husbandChain.TargetName)
}

// TestTraverseGraph_InferredGrandparent pins the "inferred relations computed,
// not stored" requirement: a grandparent derived from two parent_of edges is
// reachable as a 2-hop chain, never persisted as its own edge.
func TestTraverseGraph_InferredGrandparent(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-gp", Password: "password123!A", Email: "traverse-gp@example.com"}
	require.NoError(t, db.Create(&user).Error)

	john, parent, grandparent := seedGraphContacts(t, db, user.ID, "John", "Parent", "Grandparent")[0], seedGraphContacts(t, db, user.ID, "John", "Parent", "Grandparent")[1], seedGraphContacts(t, db, user.ID, "John", "Parent", "Grandparent")[2]
	// Stored "Parent is parent_of John" and "Grandparent is parent_of Parent".
	addEdge(t, db, user.ID, parent, john, "parent_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, grandparent, parent, "parent_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	chains, err := TraverseGraph(db, user.ID, john.VCardUID, 3, "")
	require.NoError(t, err)
	var gpChain *models.GraphChain
	for i := range chains {
		if chains[i].TargetVCardUID == grandparent.VCardUID {
			gpChain = &chains[i]
		}
	}
	require.NotNil(t, gpChain, "grandparent must be reachable via two hops")
	assert.Equal(t, 2, gpChain.Depth)
	require.Len(t, gpChain.Steps, 2)
	assert.Equal(t, "parent_of", gpChain.Steps[0].Relation, "John's parent")
	assert.Equal(t, "parent_of", gpChain.Steps[1].Relation, "John's parent's parent")
}

// TestTraverseGraph_DirectionAsymmetricToken pins the highest-risk logic: the
// displayed relation must be the INVERSE when hopping along the stored edge's
// forward direction. A parent_of edge stored source=Parent,target=Child must
// display child_of when traversed Parent→Child and parent_of when traversed
// Child→Parent.
func TestTraverseGraph_DirectionAsymmetricToken(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-dir", Password: "password123!A", Email: "traverse-dir@example.com"}
	require.NoError(t, db.Create(&user).Error)

	parent, child := seedGraphContacts(t, db, user.ID, "Parent", "Child")[0], seedGraphContacts(t, db, user.ID, "Parent", "Child")[1]
	// Edge stored forward: source=Parent, target=Child, type=parent_of.
	addEdge(t, db, user.ID, parent, child, "parent_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	// Traverse from Child (hops against the stored direction) → displays parent_of.
	fromChild, err := TraverseGraph(db, user.ID, child.VCardUID, 2, "")
	require.NoError(t, err)
	require.Len(t, fromChild, 1)
	require.Len(t, fromChild[0].Steps, 1)
	assert.Equal(t, "parent_of", fromChild[0].Steps[0].Relation, "walking backward displays the stored type")

	// Traverse from Parent (hops along the stored direction) → displays child_of.
	fromParent, err := TraverseGraph(db, user.ID, parent.VCardUID, 2, "")
	require.NoError(t, err)
	require.Len(t, fromParent, 1)
	require.Len(t, fromParent[0].Steps, 1)
	assert.Equal(t, "child_of", fromParent[0].Steps[0].Relation, "walking forward displays the inverse")
}

// TestTraverseGraph_CycleTerminates pins that a cyclic graph (mutual edges /
// a household loop) terminates via the per-branch visited set.
func TestTraverseGraph_CycleTerminates(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-cycle", Password: "password123!A", Email: "traverse-cycle@example.com"}
	require.NoError(t, db.Create(&user).Error)

	a, b, c := seedGraphContacts(t, db, user.ID, "A", "B", "C")[0], seedGraphContacts(t, db, user.ID, "A", "B", "C")[1], seedGraphContacts(t, db, user.ID, "A", "B", "C")[2]
	// Triangle: A<->B, B<->C, C<->A (symmetric edges stored one direction each).
	addEdge(t, db, user.ID, a, b, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, b, c, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, c, a, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	// Deep traversal must terminate and reach both other nodes, never hang.
	chains, err := TraverseGraph(db, user.ID, a.VCardUID, 5, "")
	require.NoError(t, err)
	require.Len(t, chains, 2, "B and C reachable from A; the cycle must not loop")
}

// TestTraverseGraph_DepthCap pins that the CTE stops at the requested depth.
func TestTraverseGraph_DepthCap(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-depth", Password: "password123!A", Email: "traverse-depth@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Linear chain n0 -> n1 -> n2 -> n3 -> n4 -> n5 (6 contacts).
	contacts := seedGraphContacts(t, db, user.ID, "N0", "N1", "N2", "N3", "N4", "N5")
	for i := 0; i < len(contacts)-1; i++ {
		// Store "later is sibling_of earlier"? Simpler: symmetric sibling edges.
		addEdge(t, db, user.ID, contacts[i], contacts[i+1], "sibling_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	}

	depth2, err := TraverseGraph(db, user.ID, contacts[0].VCardUID, 2, "")
	require.NoError(t, err)
	// From N0: depth1 reaches N1, depth2 reaches N2. N3+ unreachable.
	reachable := map[string]bool{}
	for _, ch := range depth2 {
		reachable[ch.TargetVCardUID] = true
	}
	assert.True(t, reachable[contacts[2].VCardUID], "N2 reachable at depth 2")
	assert.False(t, reachable[contacts[3].VCardUID], "N3 must NOT be reachable at depth 2")

	// A depth beyond the max is rejected up front.
	_, err = TraverseGraph(db, user.ID, contacts[0].VCardUID, 6, "")
	assert.ErrorIs(t, err, ErrTraversalTooDeep)
}

// TestTraverseGraph_ExcludesSuggestedAndSecret pins the participation rules:
// a suggested edge is never traversed, and a secret edge never leaks into a
// chain. A private edge remains visible (matching the graph display).
func TestTraverseGraph_ExcludesSuggestedAndSecret(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-rule", Password: "password123!A", Email: "traverse-rule@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice, bob, carol, dave := seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Dave")[0], seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Dave")[1], seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Dave")[2], seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Dave")[3]

	// Suggested edge Alice->Bob: must NOT appear.
	addEdge(t, db, user.ID, alice, bob, "friend_of", models.RelationshipStatusSuggested, models.RelationshipSensitivityNormal)
	// Secret edge Alice->Carol: must NOT appear.
	addEdge(t, db, user.ID, alice, carol, "partner_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret)
	// Private edge Alice->Dave: appears (graph display shows private too).
	addEdge(t, db, user.ID, alice, dave, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityPrivate)

	chains, err := TraverseGraph(db, user.ID, alice.VCardUID, 3, "")
	require.NoError(t, err)
	reachable := map[string]bool{}
	for _, ch := range chains {
		reachable[ch.TargetVCardUID] = true
	}
	assert.Len(t, chains, 1, "only the private edge is traversed")
	assert.False(t, reachable[bob.VCardUID], "suggested edge must be excluded")
	assert.False(t, reachable[carol.VCardUID], "secret edge must be excluded from traversal")
	assert.True(t, reachable[dave.VCardUID], "private edge remains visible")
}

// TestTraverseGraph_SynonymFilter pins T11's synonym consumer: a relation
// filter of "brother" resolves through the registry to sibling_of and filters
// chains by their display relation.
func TestTraverseGraph_SynonymFilter(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-syn", Password: "password123!A", Email: "traverse-syn@example.com"}
	require.NoError(t, db.Create(&user).Error)

	john, brother, sister := seedGraphContacts(t, db, user.ID, "John", "Brother", "Sister")[0], seedGraphContacts(t, db, user.ID, "John", "Brother", "Sister")[1], seedGraphContacts(t, db, user.ID, "John", "Brother", "Sister")[2]
	addEdge(t, db, user.ID, brother, john, "sibling_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, sister, john, "sibling_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	// "brother" is a registry synonym for sibling_of.
	chains, err := TraverseGraph(db, user.ID, john.VCardUID, 3, "brother")
	require.NoError(t, err)
	require.Len(t, chains, 2, "both siblings match the sibling_of display relation")

	// An unresolvable term yields no chains, not an error.
	none, err := TraverseGraph(db, user.ID, john.VCardUID, 3, "not-a-relation")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// TestTraverseGraph_ScopesToUser pins ownership: a user's traversal only walks
// their own edges.
func TestTraverseGraph_ScopesToUser(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-scope", Password: "password123!A", Email: "traverse-scope@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "traverse-other", Password: "password123!A", Email: "traverse-other@example.com"}
	require.NoError(t, db.Create(&other).Error)

	alice, bob := seedGraphContacts(t, db, user.ID, "Alice", "Bob")[0], seedGraphContacts(t, db, user.ID, "Alice", "Bob")[1]
	otherContact := models.Contact{UserID: other.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&otherContact).Error)

	addEdge(t, db, user.ID, alice, bob, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, other.ID, otherContact, bob, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	// From Bob, only Alice (user's own edge) is reachable; the other user's
	// edge to their contact is not.
	chains, err := TraverseGraph(db, user.ID, bob.VCardUID, 3, "")
	require.NoError(t, err)
	require.Len(t, chains, 1)
	assert.Equal(t, alice.VCardUID, chains[0].TargetVCardUID)
}

// TestTraverseGraph_FilterBeforeDedup pins a subtle interaction: when the same
// target is reachable at the same depth via two different chains and only one
// matches the relation filter, the matching chain must be returned even if the
// non-matching chain was visited first. (Filtering before dedup makes the
// "shortest matching chain per target" deterministic rather than depending on
// SQLite's CTE expansion order.)
func TestTraverseGraph_FilterBeforeDedup(t *testing.T) {
	db := newGraphTestDB(t)
	user := models.User{Username: "traverse-fbd", Password: "password123!A", Email: "traverse-fbd@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice, bob, carol, target := seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Target")[0], seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Target")[1], seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Target")[2], seedGraphContacts(t, db, user.ID, "Alice", "Bob", "Carol", "Target")[3]
	// Two depth-2 paths to the same target: Bob→Target is spouse_of (matches
	// the filter), Carol→Target is friend_of (does not).
	addEdge(t, db, user.ID, bob, alice, "sibling_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, bob, target, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, carol, alice, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	addEdge(t, db, user.ID, carol, target, "friend_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	chains, err := TraverseGraph(db, user.ID, alice.VCardUID, 3, "spouse_of")
	require.NoError(t, err)

	// The shortest chain to Target must be the spouse_of one (depth 2), not the
	// friend_of one — regardless of which chain SQLite happened to expand first.
	var targetChain *models.GraphChain
	for i := range chains {
		if chains[i].TargetVCardUID == target.VCardUID {
			if targetChain == nil || chains[i].Depth < targetChain.Depth {
				targetChain = &chains[i]
			}
		}
	}
	require.NotNil(t, targetChain, "a spouse_of chain to Target must survive dedup")
	assert.Equal(t, 2, targetChain.Depth, "the shortest chain to Target is via Bob (spouse)")
	require.Len(t, targetChain.Steps, 2)
	assert.Equal(t, "spouse_of", targetChain.Steps[1].Relation, "the surviving chain must be the spouse_of one")
}

// TestTraverseGraph_UsesEndpointIndexes is the issue #792 regression guard: the
// recursive CTE must seek relationship_edges by source_id / target_id, never
// fall back to a scan of the whole user's edge set. The `(source_id = ? OR
// target_id = ?)` form it replaced could not use either endpoint index, so
// SQLite used idx_relationship_edges_user_id and rescanned every edge the user
// has for every partial walk — quadratic, and it did not finish at the PERF-01
// `large` scale. Two index-anchored UNION ALL terms + INDEXED BY keep the plan
// pinned to the endpoint indexes.
func TestTraverseGraph_UsesEndpointIndexes(t *testing.T) {
	db := newGraphTestDB(t)

	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+traversalCTE,
		"x", "x", 1, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret, maxTraversalDepth,
		1, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret, maxTraversalDepth,
	).Scan(&plan).Error)

	joined := ""
	for _, p := range plan {
		joined += p.Detail + "\n"
	}
	assert.Contains(t, joined, "idx_relationship_edges_source_id",
		"the source_id-anchored recursive term must seek by idx_relationship_edges_source_id\nplan:\n%s", joined)
	assert.Contains(t, joined, "idx_relationship_edges_target_id",
		"the target_id-anchored recursive term must seek by idx_relationship_edges_target_id\nplan:\n%s", joined)
	assert.NotContains(t, joined, "idx_relationship_edges_user_id",
		"the recursive step must not fall back to scanning the whole user's edge set (issue #792)\nplan:\n%s", joined)
}
