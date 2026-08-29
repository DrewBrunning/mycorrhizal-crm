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
// that seeds the state it acts on.
type idempotentOp struct {
	name  string
	setup func(*rapid.T, *gorm.DB) error
	apply func(*gorm.DB) error
}

// idempotentOps is the CON-04 registry: every mutation here is declared
// idempotent by its own doc comment, and the property asserts that claim.
var idempotentOps = []idempotentOp{
	{
		name: "purge_soft_deleted_rows",
		setup: func(t *rapid.T, db *gorm.DB) error {
			user, err := contactgen.NewUser(db, "purge-prop")
			if err != nil {
				return err
			}
			recs := contactgen.Records(t, drawInt(t, "n", 0, 5))
			contacts, err := contactgen.Populate(db, user.ID, recs)
			if err != nil {
				return err
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
					return err
				}
				if err := db.Model(&models.Contact{}).Unscoped().Where("id = ?", contacts[i].ID).Update("deleted_at", backdate).Error; err != nil {
					return err
				}
			}
			return nil
		},
		apply: func(db *gorm.DB) error {
			services.PurgeSoftDeletedRows(db, config.Config{DeleteRetentionDays: 30})
			return nil
		},
	},
	{
		name: "rebuild_search_index",
		setup: func(t *rapid.T, db *gorm.DB) error {
			user, err := contactgen.NewUser(db, "search-prop")
			if err != nil {
				return err
			}
			recs := contactgen.Records(t, drawInt(t, "n", 0, 5))
			_, err = contactgen.Populate(db, user.ID, recs)
			return err
		},
		apply: func(db *gorm.DB) error {
			return services.RebuildSearchIndex(db)
		},
	},
}

// TestIdempotentMutation_RetryIsFixpoint is the load-bearing property:
// running each documented-idempotent mutation twice is a fixpoint.
func TestIdempotentMutation_RetryIsFixpoint(t *testing.T) {
	for _, op := range idempotentOps {
		op := op
		t.Run(op.name, rapid.MakeCheck(func(t *rapid.T) {
			db, _, err := contactgen.MigratedDB(t)
			require.NoError(t, err)

			require.NoError(t, op.setup(t, db))

			require.NoError(t, op.apply(db), "first application of %s", op.name)
			first, err := contentFingerprint(db)
			require.NoError(t, err)

			require.NoError(t, op.apply(db), "second application of %s", op.name)
			second, err := contentFingerprint(db)
			require.NoError(t, err)

			require.Equal(t, first, second, "%s must be idempotent: retrying it changed the database state", op.name)
		}))
	}
}
