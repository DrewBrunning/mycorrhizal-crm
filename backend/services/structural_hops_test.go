package services

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1791: structuralHopsByEntityID is a BFS over one edge query instead
// of a TraverseGraph path enumeration. These tests pin that it answers the
// same question the old implementation did (the reference below IS the old
// implementation) and the edge cases the BFS has to get right itself.

func scoringCfg(t *testing.T) scoring.Config {
	t.Helper()
	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)
	return cfg
}

// structuralHopsViaTraversal is the pre-#1791 implementation, kept as the
// differential oracle: enumerate every path with TraverseGraph, keep the
// shortest depth per target, drop chains with a non-structural step.
func structuralHopsViaTraversal(db *gorm.DB, userID uint, selfUID string, cfg scoring.Config) (map[string]int, error) {
	chains, err := TraverseGraph(db, userID, selfUID, maxTraversalDepth, "")
	if err != nil {
		return nil, err
	}
	result := make(map[string]int, len(chains))
	for _, chain := range chains {
		if !isStructuralChain(chain, cfg) {
			continue
		}
		if existing, ok := result[chain.TargetVCardUID]; !ok || chain.Depth < existing {
			result[chain.TargetVCardUID] = chain.Depth
		}
	}
	return result, nil
}

// isStructuralChain is the old implementation's chain filter: every step's
// relation must have a closeness tier.
func isStructuralChain(chain models.GraphChain, cfg scoring.Config) bool {
	for _, step := range chain.Steps {
		if _, ok := cfg.RelationCloseness[step.Relation]; !ok {
			return false
		}
	}
	return true
}

func TestStructuralHops_ChainDepthsAndCap(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "hops-chain")
	cfg := scoringCfg(t)

	people := make([]models.Contact, 7)
	for i := range people {
		people[i] = createScoreTestContact(t, db, user.ID, fmt.Sprintf("P%d", i))
	}
	for i := 0; i < len(people)-1; i++ {
		createScoreTestEdge(t, db, user.ID, people[i].VCardUID, people[i+1].VCardUID, "friend_of", models.RelationshipSensitivityNormal)
	}

	hops, err := structuralHopsByEntityID(db, user.ID, people[0].VCardUID, cfg)
	require.NoError(t, err)

	for i := 1; i <= maxTraversalDepth; i++ {
		assert.Equal(t, i, hops[people[i].VCardUID], "P%d is %d hops away", i, i)
	}
	assert.NotContains(t, hops, people[maxTraversalDepth+1].VCardUID, "beyond the traversal cap is unreachable")
	assert.NotContains(t, hops, people[0].VCardUID, "self is not its own hop")
}

func TestStructuralHops_AffinityOnlyPathsAreExcluded(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "hops-affinity")
	cfg := scoringCfg(t)

	self := createScoreTestContact(t, db, user.ID, "Self")
	nemesis := createScoreTestContact(t, db, user.ID, "Nemesis")
	friend := createScoreTestContact(t, db, user.ID, "Friend")
	friendOfNemesis := createScoreTestContact(t, db, user.ID, "FriendOfNemesis")
	viaFriend := createScoreTestContact(t, db, user.ID, "ViaFriend")
	createScoreTestEdge(t, db, user.ID, self.VCardUID, nemesis.VCardUID, "conflicts_with", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, nemesis.VCardUID, friendOfNemesis.VCardUID, "friend_of", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, friend.VCardUID, "friend_of", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, friend.VCardUID, viaFriend.VCardUID, "friend_of", models.RelationshipSensitivityNormal)

	hops, err := structuralHopsByEntityID(db, user.ID, self.VCardUID, cfg)
	require.NoError(t, err)

	assert.NotContains(t, hops, nemesis.VCardUID, "a direct affinity edge is not structural")
	assert.NotContains(t, hops, friendOfNemesis.VCardUID, "a path through an affinity edge is not structural")
	assert.Equal(t, 1, hops[friend.VCardUID])
	assert.Equal(t, 2, hops[viaFriend.VCardUID])
}

// A contact that is directly linked by an affinity edge AND reachable by a
// longer fully-structural path is hop-close by that longer path. (The old
// per-target de-dup could instead pick the shorter, non-structural chain and
// then drop the contact entirely.)
func TestStructuralHops_LongerStructuralPathBeatsShorterAffinityEdge(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "hops-longer")
	cfg := scoringCfg(t)

	self := createScoreTestContact(t, db, user.ID, "Self")
	mutual := createScoreTestContact(t, db, user.ID, "Mutual")
	target := createScoreTestContact(t, db, user.ID, "Target")
	createScoreTestEdge(t, db, user.ID, self.VCardUID, target.VCardUID, "conflicts_with", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, mutual.VCardUID, "friend_of", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, mutual.VCardUID, target.VCardUID, "friend_of", models.RelationshipSensitivityNormal)

	hops, err := structuralHopsByEntityID(db, user.ID, self.VCardUID, cfg)
	require.NoError(t, err)
	assert.Equal(t, 2, hops[target.VCardUID])
}

func TestStructuralHops_SecretSuggestedAndForeignEdgesDoNotConnect(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "hops-filters")
	other := createScoreTestUser(t, db, "hops-filters-other")
	cfg := scoringCfg(t)

	self := createScoreTestContact(t, db, user.ID, "Self")
	viaSecret := createScoreTestContact(t, db, user.ID, "ViaSecret")
	viaSuggested := createScoreTestContact(t, db, user.ID, "ViaSuggested")
	viaForeign := createScoreTestContact(t, db, user.ID, "ViaForeign")
	createScoreTestEdge(t, db, user.ID, self.VCardUID, viaSecret.VCardUID, "friend_of", models.RelationshipSensitivitySecret)
	suggested := models.RelationshipEdge{
		UserID: user.ID, SourceID: self.VCardUID, TargetID: viaSuggested.VCardUID, Type: "friend_of",
		Directional: false, Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: models.RelationshipStatusSuggested, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&suggested).Error)
	// Another user's edge between the same UIDs must not connect them.
	createScoreTestEdge(t, db, other.ID, self.VCardUID, viaForeign.VCardUID, "friend_of", models.RelationshipSensitivityNormal)

	hops, err := structuralHopsByEntityID(db, user.ID, self.VCardUID, cfg)
	require.NoError(t, err)
	assert.Empty(t, hops)
}

func TestStructuralHops_DirectionalRelationsMatchTraversal(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "hops-direction")
	cfg := scoringCfg(t)

	self := createScoreTestContact(t, db, user.ID, "Self")
	child := createScoreTestContact(t, db, user.ID, "Child")
	parent := createScoreTestContact(t, db, user.ID, "Parent")
	// self is parent_of child (walked forward: child_of); parent is parent_of
	// self (walked backward: parent_of). Both display relations are tiered.
	createScoreTestEdge(t, db, user.ID, self.VCardUID, child.VCardUID, "parent_of", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, parent.VCardUID, self.VCardUID, "parent_of", models.RelationshipSensitivityNormal)

	got, err := structuralHopsByEntityID(db, user.ID, self.VCardUID, cfg)
	require.NoError(t, err)
	want, err := structuralHopsViaTraversal(db, user.ID, self.VCardUID, cfg)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, got[child.VCardUID])
	assert.Equal(t, 1, got[parent.VCardUID])
}

// The BFS must agree with the path-enumerating oracle on arbitrary graphs.
// Only structural edge types appear: that is the domain where the two are
// provably the same question (they differ only when an affinity edge ties a
// structural path, pinned separately above).
func TestStructuralHops_DifferentialAgainstTraversal(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "hops-differential")
	cfg := scoringCfg(t)

	rng := rand.New(rand.NewSource(1791))
	const n = 24
	people := make([]models.Contact, n)
	for i := range people {
		people[i] = createScoreTestContact(t, db, user.ID, fmt.Sprintf("P%02d", i))
	}
	// Only types whose display relation is tiered in BOTH directions: a type
	// tiered one way only makes some chains non-structural, which is the
	// (intentional) divergence pinned by the LongerStructuralPath test.
	var types []string
	for _, typ := range []string{"friend_of", "parent_of", "sibling_of", "spouse_of", "colleague_of", "partner_of", "co_parent_of"} {
		_, fwd := cfg.RelationCloseness[traversalStepRelation(typ, true)]
		_, bwd := cfg.RelationCloseness[traversalStepRelation(typ, false)]
		if fwd && bwd {
			types = append(types, typ)
		}
	}
	require.GreaterOrEqual(t, len(types), 3, "need several structural types for the differential to mean anything")
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		a, b := rng.Intn(n), rng.Intn(n)
		typ := types[rng.Intn(len(types))]
		key := fmt.Sprintf("%d-%d-%s", min(a, b), max(a, b), typ)
		if a == b || seen[key] {
			continue
		}
		seen[key] = true
		sens := models.RelationshipSensitivityNormal
		if rng.Intn(6) == 0 {
			sens = models.RelationshipSensitivitySecret
		}
		createScoreTestEdge(t, db, user.ID, people[a].VCardUID, people[b].VCardUID, typ, sens)
	}

	for _, start := range []int{0, 7, 15} {
		got, err := structuralHopsByEntityID(db, user.ID, people[start].VCardUID, cfg)
		require.NoError(t, err)
		want, err := structuralHopsViaTraversal(db, user.ID, people[start].VCardUID, cfg)
		require.NoError(t, err)
		assert.Equal(t, want, got, "start P%02d", start)
	}
}

func TestStructuralHops_StopsOnCancelledContext(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "hops-cancel")
	self := createScoreTestContact(t, db, user.ID, "Self")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := structuralHopsByEntityID(db.WithContext(ctx), user.ID, self.VCardUID, scoringCfg(t))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestStatementContextErr(t *testing.T) {
	db := setupContactScoreTestDB(t)
	assert.NoError(t, statementContextErr(db), "a plain session has a background context")
	assert.NoError(t, statementContextErr(nil))
	assert.NoError(t, statementContextErr(&gorm.DB{}))

	ctx, cancel := context.WithCancel(context.Background())
	assert.NoError(t, statementContextErr(db.WithContext(ctx)))
	cancel()
	assert.ErrorIs(t, statementContextErr(db.WithContext(ctx)), context.Canceled)
}
