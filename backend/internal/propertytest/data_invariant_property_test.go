// DB-03 (issue #494): the application invariants from ADR 0012 that cannot be
// decided against a database at rest — they are properties of an *operation*
// (durability, atomicity, wholeness, projection stability) and so are stated
// here as universally-quantified generative properties rather than example
// tests (#494 recommended action 3: "example tests sample it badly").
//
// Coverage map (ADR 0012 §"Application invariants"):
//
//	INV-A1  durability      TestDataInvariant_A1_CommittedStateSurvivesReopen
//	INV-A2  atomicity       TestDataInvariant_A2_CancelledImportCommitsNothing
//	INV-A3  idempotency     TestIdempotentMutation_RetryIsFixpoint (idempotency_property_test.go)
//	INV-A4  import wholeness TestDataInvariant_A4_ImportAccountsForEveryRecord
//	INV-A5  regenerable     services.TestSearchIndex_RebuildMatchesIncremental (search_index_property_test.go)
//	INV-A6  convergence     services.TestDataInvariant_A6_ContactSyncConvergesToAFixpoint
//	INV-D8  reprojection    TestDataInvariant_D8_FlatProjectionIsAFixpoint (the flat/nested
//	                        comparison ADR 0012 deferred from #460 for false-positive risk)
package propertytest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/internal/contactgen"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// ---------------------------------------------------------------------------
// INV-A1 — every successful mutation is durable
// ---------------------------------------------------------------------------

// TestDataInvariant_A1_CommittedStateSurvivesReopen: after a batch of creates
// and updates returns without error, closing the connection and reopening the
// file the way a process restart would must find exactly the same content.
func TestDataInvariant_A1_CommittedStateSurvivesReopen(t *testing.T) {
	t.Run("durable", rapid.MakeCheck(func(t *rapid.T) {
		db, path, err := contactgen.MigratedDB(t)
		require.NoError(t, err)

		user, err := contactgen.NewUser(db, "a1-durable")
		require.NoError(t, err)

		contacts, err := contactgen.Populate(db, user.ID, contactgen.Records(t, drawInt(t, "n", 1, 8)))
		require.NoError(t, err)
		for i := range contacts {
			if drawBool(t, fmt.Sprintf("update-%d", i)) {
				require.NoError(t, db.Model(&contacts[i]).Update("firstname", "Renamed").Error)
			}
		}

		before, err := contentFingerprint(db)
		require.NoError(t, err)
		wantUIDs := contactVCardUIDs(t, db, user.ID)

		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())

		reopened, err := database.OpenMigratedFile(path)
		require.NoError(t, err)
		defer func() {
			if s, err := reopened.DB(); err == nil {
				_ = s.Close()
			}
		}()

		after, err := contentFingerprint(reopened)
		require.NoError(t, err)
		require.Equal(t, before, after, "committed state must survive a process restart unchanged")
		require.ElementsMatch(t, wantUIDs, contactVCardUIDs(t, reopened, user.ID))
	}))
}

// ---------------------------------------------------------------------------
// INV-A2 — a failed mutation leaves the canonical model unchanged
// ---------------------------------------------------------------------------

// TestDataInvariant_A2_CancelledImportCommitsNothing: a source import runs in
// one ctx-bound transaction, so cancelling it part-way (here from the
// per-contact progress callback, at a random point) must roll the whole run
// back — no partial multi-table write is visible. The example-seam version is
// services.TestConfirmInjectedDBErrorFailsClosed.
func TestDataInvariant_A2_CancelledImportCommitsNothing(t *testing.T) {
	t.Run("atomic", rapid.MakeCheck(func(t *rapid.T) {
		db, _, err := contactgen.MigratedDB(t)
		require.NoError(t, err)
		user, err := contactgen.NewUser(db, "a2-atomic")
		require.NoError(t, err)

		// Pre-existing content so "unchanged" is a non-trivial assertion.
		_, err = contactgen.Populate(db, user.ID, contactgen.Records(t, drawInt(t, "pre", 0, 4)))
		require.NoError(t, err)
		before, err := contentFingerprint(db)
		require.NoError(t, err)

		total := drawInt(t, "batch", 2, 8)
		plan := &services.ImportSourcePlan{System: "test"}
		members := make([]services.SourceRef, 0, total)
		for i := 0; i < total; i++ {
			r := services.SourceRef{System: "test", ExternalID: fmt.Sprintf("c/%d", i)}
			plan.Contacts = append(plan.Contacts, services.MappedContact{Ref: r, Record: contactgen.Record(t)})
			members = append(members, r)
		}
		// A circle spanning every contact guarantees a pass-2 statement after
		// the last contact, so a cancel at total still lands before commit.
		plan.Circles = []services.MappedCircle{{Ref: services.SourceRef{System: "test", ExternalID: "cir/1"}, Name: "All", Members: members}}

		cancelAfter := drawInt(t, "cancelAfter", 1, total)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		progress := func(done, _ int) {
			if done >= cancelAfter {
				cancel()
			}
		}

		_, _, err = services.ExecuteSourceImportWithActions(ctx, db, user.ID, plan, nil, progress)
		require.Error(t, err, "a cancelled import must fail, not partially succeed")

		after, err := contentFingerprint(db)
		require.NoError(t, err)
		require.Equal(t, before, after, "a cancelled import must commit nothing")
	}))
}

// ---------------------------------------------------------------------------
// INV-A4 — an import is wholly accepted or explicitly reported partial
// ---------------------------------------------------------------------------

// TestDataInvariant_A4_ImportAccountsForEveryRecord: every good contact is
// created and every unmappable graph entity (here relationships whose
// endpoints are not in the plan) is named in report.Issues — none is silently
// dropped, and the result leaves no integrity hole.
func TestDataInvariant_A4_ImportAccountsForEveryRecord(t *testing.T) {
	t.Run("wholeness", rapid.MakeCheck(func(t *rapid.T) {
		db, _, err := contactgen.MigratedDB(t)
		require.NoError(t, err)
		user, err := contactgen.NewUser(db, "a4-whole")
		require.NoError(t, err)

		good := drawInt(t, "good", 1, 6)
		bad := drawInt(t, "bad", 0, 4)

		plan := &services.ImportSourcePlan{System: "test"}
		for i := 0; i < good; i++ {
			plan.Contacts = append(plan.Contacts, services.MappedContact{
				Ref: services.SourceRef{System: "test", ExternalID: fmt.Sprintf("c/%d", i)}, Record: namedRecord(i),
			})
		}
		for i := 0; i < bad; i++ {
			// A distinct unresolved source ref per bad row so appendIssue's
			// exact-repeat dedup does not collapse them.
			plan.Relationships = append(plan.Relationships, services.MappedRelationship{
				Ref:    services.SourceRef{System: "test", ExternalID: fmt.Sprintf("r/%d", i)},
				Source: services.SourceRef{System: "test", ExternalID: fmt.Sprintf("ghost-%d", i)},
				Target: services.SourceRef{System: "test", ExternalID: fmt.Sprintf("c/%d", i%good)},
				Type:   "friend_of",
			})
		}

		report, err := services.ExecuteSourceImport(db, user.ID, plan)
		require.NoError(t, err, "unmappable graph entities are reported, not fatal")

		require.Equal(t, good, report.ContactsCreated, "every good contact is created")
		require.Equal(t, 0, report.RelationshipsCreated, "no unmappable relationship is created")

		named := 0
		for _, iss := range report.Issues {
			if strings.Contains(iss.Message, "references a contact that was not imported") {
				named++
			}
		}
		require.Equal(t, bad, named, "every unmappable relationship is named in the report")

		r, err := services.RunDataIntegrityChecks(context.Background(), db, config.Config{})
		require.NoError(t, err)
		require.True(t, r.OK, "a partially-reported import leaves no integrity hole: %+v", r.Findings)
	}))
}

// ---------------------------------------------------------------------------
// INV-D8 — canonical records are internally consistent: the flat contacts.*
// columns are a faithful, deterministic projection of Card
// ---------------------------------------------------------------------------

// TestDataInvariant_D8_FlatProjectionIsAFixpoint: re-saving a loaded contact
// re-derives the flat columns from Card via BeforeSave's T75 merge; a faithful,
// deterministic projection is a fixpoint of that. This is the flat-vs-nested
// consistency ADR 0012 deferred here from #460 (false-positive risk against
// the TEST-02 fixture); over generated records it is safe.
//
// The comparison is the flat columns only, not the whole Card blob: a
// Card.Media {kind:"photo"} entry whose photo has not been downloaded to
// contacts.photo has no flat home, so it is outside this fixpoint by design
// (ADR 0012 INV-D8 "transient-photo carve-out"; mergeMedia preserves such an
// entry across a plain db.Save, and canonical_record.unresolved_remote_photo
// surfaces it). INV-D8's structural half (valid JSON, unique element ids) is
// covered by the RunDataIntegrityChecks sweep below.
func TestDataInvariant_D8_FlatProjectionIsAFixpoint(t *testing.T) {
	t.Run("reprojection", rapid.MakeCheck(func(t *rapid.T) {
		db, _, err := contactgen.MigratedDB(t)
		require.NoError(t, err)
		user, err := contactgen.NewUser(db, "d8-reproject")
		require.NoError(t, err)

		contacts, err := contactgen.Populate(db, user.ID, contactgen.Records(t, drawInt(t, "n", 1, 6)))
		require.NoError(t, err)

		for _, c := range contacts {
			var loaded models.Contact
			require.NoError(t, db.First(&loaded, "id = ?", c.ID).Error)
			before := flatColumns(t, db, c.ID)

			require.NoError(t, db.Save(&loaded).Error) // BeforeSave re-derives flat from Card
			after := flatColumns(t, db, c.ID)

			require.Equal(t, before, after,
				"the flat projection must be a fixpoint of a plain re-save (contact id %d)", c.ID)
		}

		r, err := services.RunDataIntegrityChecks(context.Background(), db, config.Config{})
		require.NoError(t, err)
		for _, f := range r.Findings {
			require.NotEqual(t, "INV-D8", f.Invariant, "generated contacts must not trip an INV-D8 probe: %+v", f)
		}
	}))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func contactVCardUIDs(t *rapid.T, db *gorm.DB, userID uint) []string {
	var uids []string
	require.NoError(t, db.Raw(`SELECT vcard_uid FROM contacts WHERE user_id = ? AND deleted_at IS NULL`, userID).Scan(&uids).Error)
	return uids
}

// flatColumns reads the denormalized contact columns that are a projection of
// Card, so any drift across a re-save is visible.
func flatColumns(t *rapid.T, db *gorm.DB, id uint) map[string]any {
	var rows []map[string]any
	require.NoError(t, db.Raw(`
		SELECT firstname, lastname, nickname, email, phone, organization, department,
		       job_title, role, birthday, anniversary, gender, sort_name
		FROM contacts WHERE id = ?`, id).Scan(&rows).Error)
	require.Len(t, rows, 1)
	return rows[0]
}

// namedRecord is a minimal but valid neutral record — enough that the import's
// validateMappedContact accepts it, so "created" counts are exact.
func namedRecord(i int) *contactmodel.Record {
	return &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
			{Kind: "given", Value: fmt.Sprintf("Given%d", i)},
			{Kind: "surname", Value: fmt.Sprintf("Sur%d", i)},
		}},
	}}
}
