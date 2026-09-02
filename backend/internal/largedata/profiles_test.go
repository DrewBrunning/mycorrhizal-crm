package largedata

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- catalogue -------------------------------------------------------------

func TestProfileCatalog(t *testing.T) {
	names := map[string]bool{}
	for _, p := range Profiles() {
		assert.NotEmpty(t, p.Name, "every profile is named")
		assert.False(t, names[p.Name], "profile names are unique: %s", p.Name)
		names[p.Name] = true

		assert.GreaterOrEqual(t, p.Users, 1, "%s: at least one user", p.Name)
		assert.GreaterOrEqual(t, p.Contacts, BlocksOfManifest, "%s: at least one whole manifest block", p.Name)
		assert.Greater(t, p.ChainDepth, 0, "%s: a non-trivial traversal chain", p.Name)
		assert.Greater(t, p.HubFanout, 0, "%s: hubs actually fan out", p.Name)

		got, ok := ProfileByName(p.Name)
		require.True(t, ok, "ProfileByName round-trips %s", p.Name)
		assert.Equal(t, p, got)
	}
	for _, want := range []string{"smoke", "typical", "large", "stress"} {
		assert.True(t, names[want], "catalogue includes the documented %q profile", want)
	}
	_, ok := ProfileByName("does-not-exist")
	assert.False(t, ok, "an unknown name is not a profile")
}

func TestProfileRejectsBadInputs(t *testing.T) {
	base := readManifest(t)
	empty := &canonicalfixture.Manifest{}

	cases := []struct {
		name string
		p    Profile
		base *canonicalfixture.Manifest
	}{
		{"nil base", Smoke, nil},
		{"base with no contacts", Smoke, empty},
		{"zero users", Profile{Name: "x", Contacts: 150, Users: 0, ChainDepth: 1, HubFanout: 1}, base},
		{"contacts below one block", Profile{Name: "x", Contacts: 0, Users: 1, ChainDepth: 1, HubFanout: 1}, base},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.p.UserManifests(tc.base)
			require.Error(t, err)
			_, err = Populate(dbtest.New(t), tc.base, tc.p)
			require.Error(t, err, "Populate rejects the same bad inputs")
		})
	}

	_, err := Populate(nil, base, Smoke)
	require.Error(t, err, "Populate rejects a nil db")
}

// --- determinism ---------------------------------------------------------

func TestProfileUserManifestsDeterministic(t *testing.T) {
	base := readManifest(t)

	a, err := Smoke.UserManifests(base)
	require.NoError(t, err)
	b, err := Smoke.UserManifests(base)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(a, b),
		"the same (profile, seed) must produce byte-identical manifests so two benchmark runs are comparable")

	// A different seed keeps the shape but moves every contact UID, so a
	// regression is reproducible against a specific seed rather than a lucky
	// dataset.
	reseeded := Smoke
	reseeded.Seed = Smoke.Seed + 1
	c, err := reseeded.UserManifests(base)
	require.NoError(t, err)
	assert.Equal(t, len(a[0].Contacts), len(c[0].Contacts), "seed does not change the shape")
	assert.NotEqual(t, a[0].Contacts[0].Card.UID, c[0].Contacts[0].Card.UID,
		"a different seed moves the contact UIDs")
}

// --- manifest validity + multi-user keying ------------------------------

func TestProfileUserManifestsValidate(t *testing.T) {
	base := readManifest(t)

	for _, p := range []Profile{Smoke, Typical} {
		p := p
		t.Run(p.Name, func(t *testing.T) {
			ms, err := p.UserManifests(base)
			require.NoError(t, err)
			require.Len(t, ms, p.Users)

			blocks := p.blocksFor(base)
			usernames, emails := map[string]bool{}, map[string]bool{}
			for u, m := range ms {
				require.NoError(t, m.Validate(), "user %d manifest validates", u)
				assert.Equal(t, blocks*len(base.Contacts), len(m.Contacts),
					"contact count rounds up to a whole number of blocks")
				assert.GreaterOrEqual(t, len(m.Contacts), p.Contacts)

				assert.False(t, usernames[m.User.Username], "usernames are unique across users")
				assert.False(t, emails[m.User.Email], "emails are unique across users")
				usernames[m.User.Username] = true
				emails[m.User.Email] = true

				// The graph shape was appended: base is 10 edges/block, so
				// anything beyond blocks*10 is the chain + hubs.
				assert.Greater(t, len(m.Relationships), blocks*len(base.Relationships),
					"user %d manifest carries the added hub + chain edges", u)
			}
		})
	}
}

// TestProfileGraphShapeEdgeCases covers a hand-built one-block profile (no
// cross-block edges to add) and a tiny profile whose chain/hub knobs far
// exceed the block count (every clamp fires) — the shaped manifest must still
// validate and stay bounded.
func TestProfileGraphShapeEdgeCases(t *testing.T) {
	base := readManifest(t)

	// One block: appendGraphShape is a no-op, so the manifest is exactly the
	// block-scaled canonical one.
	oneBlock := Profile{Name: "t-one", Seed: 1, Contacts: BlocksOfManifest, Users: 1, Hubs: 3, HubFanout: 5, ChainDepth: 5}
	ms, err := oneBlock.UserManifests(base)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	require.NoError(t, ms[0].Validate())
	assert.Equal(t, len(base.Relationships), len(ms[0].Relationships),
		"a one-block profile adds no cross-block edges")

	// Two blocks, oversized knobs: chain clamps to 1 hop, each hub fans out to
	// at most blocks-1 = 1 other block.
	tiny := Profile{Name: "t-clamp", Seed: 1, Contacts: 2 * BlocksOfManifest, Users: 1, Hubs: 99, HubFanout: 99, ChainDepth: 99}
	ms, err = tiny.UserManifests(base)
	require.NoError(t, err)
	require.NoError(t, ms[0].Validate())
	added := len(ms[0].Relationships) - 2*len(base.Relationships)
	assert.Equal(t, 1+2*1, added,
		"2-block dataset: 1 chain hop + at most one hub edge per (clamped) hub")
}

// --- graph shape --------------------------------------------------------

// TestProfileGraphShapeIsNonUniform pins #468's core requirement: Scale alone
// yields a uniform forest of identical islands, but traversal cost lives in
// the tails, so a profile adds a deep chain and dense hubs. The test proves
// both exist in the populated graph and that the resulting out-degree
// distribution is genuinely lopsided.
func TestProfileGraphShapeIsNonUniform(t *testing.T) {
	p := Smoke
	db := dbtest.New(t)
	base := readManifest(t)
	ds, err := Populate(db, base, p)
	require.NoError(t, err)

	u0 := ds.Users[0]
	blocks := p.blocksFor(base)
	lead := func(block int) string {
		return u0.Contacts[fmt.Sprintf("%s_%06d", base.Contacts[0].Name, block)].VCardUID
	}

	// The deep chain: lead(i) --parent_of--> lead(i+1) for the clamped depth.
	depth := p.ChainDepth
	if depth > blocks-1 {
		depth = blocks - 1
	}
	require.Greater(t, depth, 1, "smoke profile must give a multi-hop chain")
	for i := 0; i < depth; i++ {
		var n int64
		require.NoError(t, db.Table("relationship_edges").
			Where("user_id = ? AND source_id = ? AND target_id = ? AND type = ?",
				u0.User.ID, lead(i), lead(i+1), "parent_of").
			Count(&n).Error)
		assert.EqualValues(t, 1, n, "chain hop %d->%d is present", i, i+1)
	}

	// Out-degree distribution: a hub lead contact has far more outgoing edges
	// than a plain block's lead contact.
	outDegree := func(uid string) int64 {
		var n int64
		require.NoError(t, db.Table("relationship_edges").
			Where("user_id = ? AND source_id = ?", u0.User.ID, uid).Count(&n).Error)
		return n
	}
	var maxOut, minOut int64 = 0, 1 << 30
	for b := 0; b < blocks; b++ {
		d := outDegree(lead(b))
		if d > maxOut {
			maxOut = d
		}
		if d < minOut {
			minOut = d
		}
	}
	fanout := p.HubFanout
	if fanout > blocks-1 {
		fanout = blocks - 1
	}
	assert.GreaterOrEqual(t, maxOut-minOut, int64(fanout),
		"a hub's out-degree must exceed a plain block's by at least the fanout (max=%d min=%d fanout=%d)", maxOut, minOut, fanout)
}

// --- multi-user isolation --------------------------------------------------

func TestProfileMultiUserIsolation(t *testing.T) {
	p := Smoke
	require.GreaterOrEqual(t, p.Users, 2, "smoke exercises the multi-user split")

	db := dbtest.New(t)
	base := readManifest(t)
	ds, err := Populate(db, base, p)
	require.NoError(t, err)
	require.Len(t, ds.Users, p.Users)

	var userRows, contactRows int64
	require.NoError(t, db.Table("users").Count(&userRows).Error)
	require.NoError(t, db.Table("contacts").Count(&contactRows).Error)
	assert.EqualValues(t, p.Users, userRows, "one users row per profile user")
	assert.EqualValues(t, contactRows, ds.ContactCount(), "ProfileDataset.ContactCount sums the per-user datasets")
	assert.Equal(t, p.Contacts*p.Users, ds.ContactCount(), "total contacts = per-user target x user count")

	seenID := map[uint]bool{}
	seenUID := map[string]int{} // vcard_uid -> owning user index
	for ui, u := range ds.Users {
		assert.False(t, seenID[u.User.ID], "distinct user IDs")
		seenID[u.User.ID] = true

		// Every user's contact UIDs are disjoint from every other user's — the
		// per-(seed, user) UID salt is load-bearing, so a graph benchmark that
		// forgets to scope by user still cannot accidentally join two users'
		// data. (Within one user, the pathological julie contact deliberately
		// re-uses her block's gina UID — a same-user repeat is expected.)
		for name, c := range u.Contacts {
			if prev, seen := seenUID[c.VCardUID]; seen && prev != ui {
				t.Fatalf("vcard_uid %s (contact %q) generated for both user %d and user %d", c.VCardUID, name, prev, ui)
			}
			seenUID[c.VCardUID] = ui
		}

		// Every relationship edge for this user stays within this user's own
		// contact set — no profile user can reference another's rows (CLAUDE.md
		// item 5: ownership scoping).
		own := map[string]bool{}
		for _, c := range u.Contacts {
			own[c.VCardUID] = true
		}
		type edge struct{ SourceID, TargetID string }
		var edges []edge
		require.NoError(t, db.Table("relationship_edges").
			Select("source_id", "target_id").
			Where("user_id = ?", u.User.ID).Scan(&edges).Error)
		require.NotEmpty(t, edges)
		for _, e := range edges {
			assert.True(t, own[e.SourceID], "edge source belongs to its user")
			assert.True(t, own[e.TargetID], "edge target belongs to its user")
		}

		// Contacts/notes counts scoped to this user match the dataset.
		var contacts int64
		require.NoError(t, db.Table("contacts").Where("user_id = ?", u.User.ID).Count(&contacts).Error)
		assert.EqualValues(t, len(u.Contacts), contacts)
	}
}

// --- pathological records at scale, per user -----------------------------

func TestProfilePathologicalRecordsSurvivePerUser(t *testing.T) {
	p := Smoke
	db := dbtest.New(t)
	base := readManifest(t)
	ds, err := Populate(db, base, p)
	require.NoError(t, err)

	blocks := p.blocksFor(base)
	for ui, u := range ds.Users {
		for b := 0; b < blocks; b++ {
			gina, ok := u.Contacts[fmt.Sprintf("gina_%06d", b)]
			require.True(t, ok, "user %d block %d has a gina", ui, b)
			julie, ok := u.Contacts[fmt.Sprintf("julie_%06d", b)]
			require.True(t, ok, "user %d block %d has a julie", ui, b)
			assert.True(t, gina.DeletedAt.Valid, "gina is soft-deleted")
			assert.Equal(t, gina.VCardUID, julie.VCardUID,
				"julie re-uses gina's vcard_uid (partial unique index holds at scale, per user)")
		}
	}
}

// --- DB-01 invariants ---------------------------------------------------

// TestProfileDatasetPassesDB01 is #468's "the generated dataset passes DB-01's
// invariant checks (#460)" — a fixture that violates the application
// invariants would make every downstream measurement meaningless. The empty
// config.Config skips the attachment/photo on-disk file checks, which is
// correct: the canonical manifest stores attachment metadata only (its
// README), and a test that needs a physical file writes one itself.
func TestProfileDatasetPassesDB01(t *testing.T) {
	db := dbtest.New(t)
	base := readManifest(t)
	_, err := Populate(db, base, Smoke)
	require.NoError(t, err)

	rep, err := services.RunDataIntegrityChecks(context.Background(), db, config.Config{})
	require.NoError(t, err)
	for _, f := range rep.Findings {
		t.Logf("DB-01 %s %s severity=%s count=%d: %s", f.Invariant, f.Check, f.Severity, f.Count, f.Detail)
	}
	assert.True(t, rep.OK, "profile dataset must pass the DB-01 application-invariant sweep")

	var integrity string
	require.NoError(t, db.Raw("PRAGMA integrity_check").Scan(&integrity).Error)
	assert.Equal(t, "ok", integrity)
}

// --- scale (gated) -----------------------------------------------------

// TestProfilePopulatesAtScale populates the documented `typical` profile end
// to end and records the numbers docs/development/scale-profiles.md is
// measured from. Gated behind MYCORRHIZAL_LARGE_TESTS=1 (the nightly/main
// large-dataset job): a `typical` populate is ~10s under -race, more than a
// per-PR run should pay.
func TestProfilePopulatesAtScale(t *testing.T) {
	if !largeTestsEnabled() {
		t.Skip("profile populate-at-scale runs in the nightly/main-push large-dataset CI job (set MYCORRHIZAL_LARGE_TESTS=1 to run it locally)")
	}
	p := Typical

	dbPath := t.TempDir() + "/profile.db"
	db := dbtest.NewAt(t, dbPath)
	base := readManifest(t)

	ds, err := Populate(db, base, p)
	require.NoError(t, err)
	require.Len(t, ds.Users, p.Users)

	tables := []string{"contacts", "relationship_edges", "notes", "life_events", "gifts", "activities", "attachments", "preferences", "external_identities"}
	for _, tbl := range tables {
		var n int64
		require.NoError(t, db.Table(tbl).Count(&n).Error)
		t.Logf("%-20s %d", tbl, n)
	}
	assert.Equal(t, p.Contacts*p.Users, ds.ContactCount(), "populated contact count matches the profile target")

	rep, err := services.RunDataIntegrityChecks(context.Background(), db, config.Config{})
	require.NoError(t, err)
	assert.True(t, rep.OK, "the %s profile dataset passes DB-01 at scale", p.Name)

	var integrity string
	require.NoError(t, db.Raw("PRAGMA integrity_check").Scan(&integrity).Error)
	assert.Equal(t, "ok", integrity)

	fi, err := os.Stat(dbPath)
	require.NoError(t, err)
	t.Logf("%s profile DB size: %d bytes (%.1f MB)", p.Name, fi.Size(), float64(fi.Size())/(1<<20))
}
