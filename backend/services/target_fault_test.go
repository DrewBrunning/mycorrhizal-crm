package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// failNthQuery makes the nth SELECT against table fail (1-based), leaving every
// other statement alone. It pins error branches that sit behind earlier
// statements on the same table, which dbtest.HideTable (all-or-nothing) cannot.
func failNthQuery(t *testing.T, db *gorm.DB, table string, nth int32) {
	t.Helper()
	var n atomic.Int32
	name := "test:fail-nth-query:" + table
	fail := func(tx *gorm.DB) {
		if tx.Statement.Table == table && n.Add(1) == nth {
			_ = tx.AddError(errors.New("injected query failure"))
		}
	}
	// Scan over db.Table(...) runs the Row callbacks, Find/Pluck the Query ones.
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(name, fail))
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register(name, fail))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(name)
		_ = db.Callback().Row().Remove(name)
	})
}

func failUpdates(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	name := "test:fail-update:" + table
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(errors.New("injected update failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
}

func TestTargetBehind_LaterStatementFailuresSurface(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, e *targetEnv)
	}{
		// pluck(contacts)=#1, then the per-user Find(contacts)=#2.
		{"targeted contacts load", func(t *testing.T, e *targetEnv) { failNthQuery(t, e.db, "contacts", 2) }},
		// pending pluck=#1, then the per-user Find of existing rows=#2.
		{"existing suggestions load", func(t *testing.T, e *targetEnv) { failNthQuery(t, e.db, "reach_out_suggestions", 2) }},
		{"auto-dismiss update", func(t *testing.T, e *targetEnv) {
			// A stale pending row exists once the contact stops being behind.
			c := e.contact(t, "Alice", "close")
			e.activity(t, c, 40)
			require.Equal(t, 1, e.run(t))
			e.activity(t, c, 0)
			failUpdates(t, e.db, "reach_out_suggestions")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTargetEnv(t)
			logtest.AllowWarnings(t, "the pass logs the failure it returns")
			if tc.name != "auto-dismiss update" {
				c := e.contact(t, "Alice", "close")
				e.activity(t, c, 40)
			}
			tc.setup(t, e)
			_, err := detectTargetBehindSuggestions(context.Background(), e.db, e.cfg, Now())
			assert.Error(t, err)
		})
	}

	t.Run("score input gathering failure", func(t *testing.T) {
		e := newTargetEnv(t)
		logtest.AllowWarnings(t, "the pass logs the failure it returns")
		c := e.contact(t, "Alice", "close")
		e.activity(t, c, 40)
		failNthQuery(t, e.db, "cadence_policies", 1)
		_, err := detectTargetBehindSuggestions(context.Background(), e.db, e.cfg, Now())
		assert.Error(t, err)
	})
}

// The graph-reachability read is the second relationship_edges query; its
// failure reaches the caller like every other gather failure.
func TestGatherScoreInputs_HopQueryFailureSurfaces(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-hop-fail")
	self := createScoreTestContact(t, db, user.ID, "Self")
	friend := createScoreTestContact(t, db, user.ID, "Friend")
	setSelfContact(t, db, user.ID, self.VCardUID)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, friend.VCardUID, "friend_of", models.RelationshipSensitivityNormal)

	failNthQuery(t, db, "relationship_edges", 2)
	_, err := ComputeContactScore(db, user.ID, &friend, Now())
	assert.Error(t, err)
}
