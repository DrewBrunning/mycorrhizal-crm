// TEST-07 (issue #435) / CON-04 (#459) property: retry(op) ≡ op for
// mutations declared idempotent. For each registered operation, running it
// twice must leave the database in exactly the same state as running it once.
//
// The registry lists operations whose own doc comments declare them
// idempotent; the property is the load-bearing assertion of that claim. A
// mutation that gains a non-idempotent side effect (an extra row, a guard
// that fires only on the second run) fails here with a shrunk counterexample.
//
// Per check it builds a fresh migrated database, seeds generated content in
// the shape each operation is documented to act on, runs the operation once,
// fingerprints the whole database, runs it again, and asserts the
// fingerprints match.
package propertytest

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/contactgen"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// idempotentOp is one documented-idempotent database mutation and the setup
// that seeds the state it acts on. setup returns the per-check state apply
// needs (nil for operations that act on the whole database); apply runs the
// operation over that state.
type idempotentOp struct {
	name  string
	setup func(*rapid.T, *gorm.DB) (any, error)
	apply func(*gorm.DB, any) error
}

// selfContactState is the per-check state for the ensure_self_contact op: the
// user whose self contact the mutation creates-if-absent.
type selfContactState struct {
	user *models.User
}

// groupingsState is the per-check state for the materialize_imported_groupings
// op: the user and the staged contact whose circles/tags the mutation
// materializes.
type groupingsState struct {
	userID  uint
	contact *models.Contact
}

// idempotentOps is the CON-04 registry: every mutation here is declared
// idempotent by its own doc comment, and the property asserts that claim.
var idempotentOps = []idempotentOp{
	{
		name: "purge_soft_deleted_rows",
		setup: func(t *rapid.T, db *gorm.DB) (any, error) {
			user, err := contactgen.NewUser(db, "purge-prop")
			if err != nil {
				return nil, err
			}
			recs := contactgen.Records(t, drawInt(t, "n", 0, 5))
			contacts, err := contactgen.Populate(db, user.ID, recs)
			if err != nil {
				return nil, err
			}
			// Soft-delete a random subset of the generated contacts, some well
			// past the retention window and some well within it (the two runs'
			// cutoff drift must never flip a row across the boundary).
			for i := range contacts {
				if !drawBool(t, "purge.softdelete") {
					continue
				}
				backdate := time.Now().AddDate(0, 0, -45) // past the 30-day window
				if drawBool(t, "purge.past") {
					backdate = time.Now().AddDate(0, 0, -5) // inside the window
				}
				if err := db.Delete(&contacts[i]).Error; err != nil {
					return nil, err
				}
				if err := db.Model(&models.Contact{}).Unscoped().Where("id = ?", contacts[i].ID).Update("deleted_at", backdate).Error; err != nil {
					return nil, err
				}
			}
			return nil, nil
		},
		apply: func(db *gorm.DB, _ any) error {
			services.PurgeSoftDeletedRows(db, config.Config{DeleteRetentionDays: 30})
			return nil
		},
	},
	{
		name: "rebuild_search_index",
		setup: func(t *rapid.T, db *gorm.DB) (any, error) {
			user, err := contactgen.NewUser(db, "search-prop")
			if err != nil {
				return nil, err
			}
			recs := contactgen.Records(t, drawInt(t, "n", 0, 5))
			_, err = contactgen.Populate(db, user.ID, recs)
			return nil, err
		},
		apply: func(db *gorm.DB, _ any) error {
			return services.RebuildSearchIndex(db)
		},
	},
	{
		// services.EnsureSelfContact (services/user_service.go) is documented
		// "Safe to call multiple times — the second call is a no-op". A second
		// call must not create a second self contact, so the fixpoint is the
		// whole "one self contact + the user's pointer to it" state.
		name: "ensure_self_contact",
		setup: func(t *rapid.T, db *gorm.DB) (any, error) {
			user, err := contactgen.NewUser(db, "self-contact")
			if err != nil {
				return nil, err
			}
			return &selfContactState{user: &user}, nil
		},
		apply: func(db *gorm.DB, st any) error {
			s := st.(*selfContactState)
			return services.EnsureSelfContact(db, s.user)
		},
	},
	{
		// services.MaterializeImportedGroupings (services/import_groupings_service.go)
		// is documented "Idempotent in both halves: an entity with that name is
		// reused rather than duplicated, and an existing membership is left
		// alone". Re-running it over the same staged contact must not duplicate
		// the circle/tag entities or their membership rows — the fixpoint is the
		// whole circle/tag/membership set, and the staging columns also pin the
		// name normalization (case-insensitive dedup, empty-drop).
		name: "materialize_imported_groupings",
		setup: func(t *rapid.T, db *gorm.DB) (any, error) {
			user, err := contactgen.NewUser(db, "groupings")
			if err != nil {
				return nil, err
			}
			contacts, err := contactgen.Populate(db, user.ID, contactgen.Records(t, 1))
			if err != nil {
				return nil, err
			}
			c := contacts[0]
			// Deliberately overlapping spellings and an empty token: the
			// fixpoint must survive the normalization path too, not just a
			// clean input.
			c.Circles = []string{"Family", "family", "Book Club", "book club", ""}
			c.ImportedTags = []string{"VIP", "vip", ""}
			return &groupingsState{userID: user.ID, contact: &c}, nil
		},
		apply: func(db *gorm.DB, st any) error {
			s := st.(*groupingsState)
			return services.MaterializeImportedGroupings(db, s.userID, s.contact)
		},
	},
}

// TestIdempotentMutation_RetryIsFixpoint is the load-bearing property:
// running each documented-idempotent mutation twice is a fixpoint.
func TestIdempotentMutation_RetryIsFixpoint(t *testing.T) {
	t.Parallel()
	for _, op := range idempotentOps {
		op := op
		t.Run(op.name, rapid.MakeCheck(func(t *rapid.T) {
			db, _, err := contactgen.MigratedDB(t)
			require.NoError(t, err)

			state, err := op.setup(t, db)
			require.NoError(t, err)

			require.NoError(t, op.apply(db, state), "first application of %s", op.name)
			first, err := contentFingerprint(db)
			require.NoError(t, err)

			require.NoError(t, op.apply(db, state), "second application of %s", op.name)
			second, err := contentFingerprint(db)
			require.NoError(t, err)

			require.Equal(t, first, second, "%s must be idempotent: retrying it changed the database state", op.name)
		}))
	}
}
