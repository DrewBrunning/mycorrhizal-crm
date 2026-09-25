package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/scoring"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCanonicalFixtureHealthSpread pins the point of the v1.2.0 demo personas
// (issue #1220): the canonical fixture must produce a realistic spread of
// relationship-health bands, not an all-chanterelle graph. The scoring service
// is derived, so this is the fixture's end-to-end proof that the seeded raw
// inputs (self-edges, cadence intervals, relative activity timing, reach-outs,
// back-dated updated_at) actually move the band.
func TestCanonicalFixtureHealthSpread(t *testing.T) {
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	// A pinned reference instant so the relative timing is deterministic.
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	db := dbtest.New(t)
	ds, err := canonicalfixture.PopulateAt(db, m, now)
	require.NoError(t, err)

	scores, err := ComputeAllContactScores(db, ds.User.ID, now)
	require.NoError(t, err)
	require.NotEmpty(t, scores)

	band := func(name string) string {
		r, ok := scores[ds.Contacts[name].ID]
		require.True(t, ok, "contact %q must be scored", name)
		return r.Band
	}

	// The deliberately-moss personas: a close self-edge plus frequent recent
	// interactions.
	assert.Equal(t, scoring.BandMoss, band("nadia"))
	assert.Equal(t, scoring.BandMoss, band("theo"))
	assert.Equal(t, scoring.BandMoss, band("marcus"))

	// The deliberately-russula persona: a distant relation long overdue.
	assert.Equal(t, scoring.BandRussula, band("bea"))

	// A spread, not a single band: several moss and several russula nodes at
	// once, so the network-graph screenshot shows all three colors.
	var moss, russula int
	for _, r := range scores {
		switch r.Band {
		case scoring.BandMoss:
			moss++
		case scoring.BandRussula:
			russula++
		}
	}
	assert.GreaterOrEqual(t, moss, 3, "the fixture must seed several healthy nodes")
	assert.GreaterOrEqual(t, russula, 4, "the fixture must seed several neglected nodes")

	// Deceased contacts are present and derive their state from the card.
	assert.True(t, ds.Contacts["margaret"].Card.IsDeceased())
	assert.True(t, ds.Contacts["harold"].Card.IsDeceased())
}
