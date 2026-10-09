package services

// Issue #1351: the purge's VCardUID-keyed cleanups must be scoped to the
// purged contact's OWNER and must not touch a UID that still has a live
// contact.
//
// vcard_uid is unique only per user and only among live rows (partial index
// idx_contacts_vcard_uid_user). The old cleanups matched by vcard_uid alone, so
// a purged contact also hard-deleted (a) another user's live same-UID
// contact's data (two accounts importing the same .vcf / syncing the same
// address book) and (b) the same user's re-created same-UID contact's data
// (CardDAV remote delete then re-add, re-import after delete).
//
// The tests are driven by purgeContactUIDCleanups itself — the slice the
// production code executes — and a schema scan fails if a table carrying a
// VCardUID-shaped column is neither in it nor excluded with a reason, so a new
// cleanup cannot skip the scoping tests.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// purgeUIDColumnExcluded lists tables with a VCardUID-shaped column that are
// deliberately not in purgeContactUIDCleanups. Every entry needs a reason.
var purgeUIDColumnExcluded = map[string]string{
	"attachments":                       "soft-deleted and purged by deleted_at (purgedSoftDeleteModels), row-keyed rather than UID-swept",
	"audit_events":                      "append-only audit log; never hard-deleted by the purge",
	"contacts":                          "the parent; purged last, row by row, by its own deleted_at",
	"feeds":                             "change-feed bookkeeping keyed by entity id, not a contact-owned row",
	"life_event_suggestion_resolutions": "hard-deleted by DeleteContact (user_id + uid scoped); not swept by the purge",
	"life_events":                       "soft-deleted and purged by deleted_at (purgedSoftDeleteModels), row-keyed",
	"reach_out_suggestions":             "hard-deleted by DeleteContact (user_id + uid scoped); not swept by the purge",
	"users":                             "self_contact_vcard_uid is a pointer on the user, not a contact-owned row",
}

func TestPurgeContactUIDCleanups_CoverEveryUIDColumnedTable(t *testing.T) {
	db := dbtest.New(t)

	var found []struct{ T, C string }
	require.NoError(t, db.Raw(`SELECT m.name AS t, p.name AS c
		FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%'
		  AND (p.name LIKE '%vcard_uid' OR p.name IN ('entity_id','source_id','target_id'))
		ORDER BY 1, 2`).Scan(&found).Error)

	covered := map[string]map[string]bool{}
	for _, spec := range purgeContactUIDCleanups {
		cols := map[string]bool{}
		for _, c := range spec.Cols {
			cols[c] = true
		}
		require.NotContains(t, covered, spec.Table, "%s listed twice in purgeContactUIDCleanups", spec.Table)
		covered[spec.Table] = cols
	}

	seenTables := map[string]bool{}
	for _, f := range found {
		seenTables[f.T] = true
		if reason, ok := purgeUIDColumnExcluded[f.T]; ok {
			assert.NotContains(t, covered, f.T, "%s is cleaned up AND excluded (%s) — drop the stale exclusion", f.T, reason)
			continue
		}
		if !assert.Contains(t, covered, f.T,
			"table %s has VCardUID-shaped column %s but is neither in purgeContactUIDCleanups (owner-scoped, "+
				"issue #1351) nor purgeUIDColumnExcluded with a reason", f.T, f.C) {
			continue
		}
		assert.True(t, covered[f.T][f.C], "%s.%s is a VCardUID-shaped column missing from its purgeContactUIDCleanups entry", f.T, f.C)
	}
	for table := range covered {
		assert.True(t, seenTables[table], "purgeContactUIDCleanups names %s but the schema has no VCardUID-shaped column there", table)
	}
	for table := range purgeUIDColumnExcluded {
		assert.True(t, seenTables[table], "stale purgeUIDColumnExcluded entry %s", table)
	}
}

// Every cleanup statement carries an owner-scoped predicate: both the
// purge-eligible and the live/in-retention subquery correlate on the target
// table's own user_id, and the table really has that column.
func TestPurgeContactUIDCleanups_QueriesAreOwnerScoped(t *testing.T) {
	db := dbtest.New(t)
	require.NotEmpty(t, purgeContactUIDCleanups, "registry must declare at least one UID cleanup")
	for _, spec := range purgeContactUIDCleanups {
		var hasUserID int
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'user_id'", spec.Table).Scan(&hasUserID).Error)
		require.Equal(t, 1, hasUserID, "%s has no user_id column to scope the purge by", spec.Table)

		q := spec.query()
		assert.Equal(t, 2*len(spec.Cols), strings.Count(q, "?"), "%s: two placeholders (cutoff) per column", spec.Table)
		for _, col := range spec.Cols {
			assert.Contains(t, q, fmt.Sprintf("d.user_id = %s.user_id AND d.vcard_uid = %s.%s", spec.Table, spec.Table, col), "%s.%s: purged-contact subquery not owner-scoped", spec.Table, col)
			assert.Contains(t, q, fmt.Sprintf("l.user_id = %s.user_id AND l.vcard_uid = %s.%s", spec.Table, spec.Table, col), "%s.%s: live-contact exclusion not owner-scoped", spec.Table, col)
			assert.Contains(t, q, "l.deleted_at IS NULL", "%s.%s: no live-contact exclusion", spec.Table, col)
		}
		assert.Len(t, spec.args(time.Now()), 2*len(spec.Cols))
	}
}

// ---------------------------------------------------------------------------
// Schema-driven row seeding: inserts a minimal valid row into any table using
// the migrated schema (NOT NULL columns get a filler value, NOT NULL FKs get a
// generated parent), so the scoping test needs no per-table hand-written setup
// that could silently miss a newly added table.
// ---------------------------------------------------------------------------

type seedColumn struct {
	Name string
	Type string
	Nn   int
	Dflt *string
	Pk   int
}

type seedFK struct {
	From  string `gorm:"column:from"`
	Table string `gorm:"column:table"`
}

type rowSeeder struct {
	t  *testing.T
	db *gorm.DB
	n  int
}

func (s *rowSeeder) tableColumns(table string) []seedColumn {
	var cols []seedColumn
	require.NoError(s.t, s.db.Raw("SELECT name, type, [notnull] AS nn, dflt_value AS dflt, pk FROM pragma_table_info(?)", table).Scan(&cols).Error)
	return cols
}

// insert adds a row to table with the given overrides and returns its id.
func (s *rowSeeder) insert(table string, over map[string]any) any {
	s.t.Helper()
	cols := s.tableColumns(table)
	var fks []seedFK
	require.NoError(s.t, s.db.Raw(`SELECT "from" AS "from", "table" AS "table" FROM pragma_foreign_key_list(?)`, table).Scan(&fks).Error)
	fkParent := map[string]string{}
	for _, fk := range fks {
		fkParent[fk.From] = fk.Table
	}
	has := map[string]bool{}
	for _, c := range cols {
		has[c.Name] = true
	}

	var names []string
	var vals []any
	var idVal any
	for _, c := range cols {
		s.n++
		typ := strings.ToUpper(c.Type)
		if v, ok := over[c.Name]; ok {
			names = append(names, c.Name)
			vals = append(vals, v)
			if c.Name == "id" {
				idVal = v
			}
			continue
		}
		if c.Pk == 1 && strings.Contains(typ, "INT") {
			continue // rowid alias
		}
		if c.Nn == 0 || c.Dflt != nil {
			if c.Pk != 1 {
				continue
			}
		}
		var v any
		switch {
		case fkParent[c.Name] != "":
			parentOver := map[string]any{}
			if u, ok := over["user_id"]; ok {
				parentOver["user_id"] = u
			}
			v = s.insert(fkParent[c.Name], parentOver)
		case strings.Contains(typ, "INT"), strings.Contains(typ, "BOOL"), strings.Contains(typ, "REAL"), strings.Contains(typ, "NUM"):
			v = 1
		case strings.Contains(typ, "TIME"), strings.Contains(typ, "DATE"):
			v = time.Now()
		default:
			v = fmt.Sprintf("seed-%s-%d", c.Name, s.n)
		}
		names = append(names, c.Name)
		vals = append(vals, v)
		if c.Name == "id" {
			idVal = v
		}
	}

	ph := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(names, ","), ph)
	require.NoError(s.t, s.db.Exec(q, vals...).Error, "seeding %s", table)
	if idVal != nil {
		return idVal
	}
	var id int64
	require.NoError(s.t, s.db.Raw("SELECT last_insert_rowid()").Scan(&id).Error)
	return id
}

func (s *rowSeeder) contact(userID uint, uid string, deletedAt *time.Time) {
	s.t.Helper()
	c := models.Contact{UserID: userID, Firstname: "C-" + uid, VCardUID: uid}
	require.NoError(s.t, s.db.Create(&c).Error)
	if deletedAt != nil {
		require.NoError(s.t, s.db.Unscoped().Model(&models.Contact{}).Where("id = ?", c.ID).
			Update("deleted_at", *deletedAt).Error)
	}
}

func seededRowCount(t *testing.T, db *gorm.DB, table, col string, userID uint, uid string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_id = ? AND %s = ?", table, col), userID, uid).Scan(&n).Error)
	return n
}

// For EVERY table/column in purgeContactUIDCleanups, four UIDs cover the
// scoping matrix. All four have a contact of userA soft-deleted 60 days ago
// (purge-eligible at 30-day retention):
//
//	P  purged:   nothing else            -> A's row must be removed
//	S  shared:   userB has a LIVE contact-> A's row removed, B's row KEPT
//	R  reborn:   A re-created it (live)  -> A's row KEPT
//	W  window:   A also has a second copy soft-deleted 5 days ago (undo window
//	             still open)             -> A's row KEPT
func TestPurgeContactUIDCleanups_OwnerScopedBehavior(t *testing.T) {
	require.NotEmpty(t, purgeContactUIDCleanups, "registry must declare at least one UID cleanup")
	for _, spec := range purgeContactUIDCleanups {
		require.NotEmpty(t, spec.Cols, "%s must declare at least one UID column", spec.Table)
		for _, col := range spec.Cols {
			t.Run(spec.Table+"."+col, func(t *testing.T) {
				db := dbtest.New(t)
				userA := models.User{Username: "scope-a", Email: "scope-a@example.com", Password: "x"}
				userB := models.User{Username: "scope-b", Email: "scope-b@example.com", Password: "x"}
				require.NoError(t, db.Create(&userA).Error)
				require.NoError(t, db.Create(&userB).Error)

				old := time.Now().AddDate(0, 0, -60)
				recent := time.Now().AddDate(0, 0, -5)
				seed := &rowSeeder{t: t, db: db}

				const uidP, uidS, uidR, uidW = "uid-purged", "uid-shared", "uid-reborn", "uid-window"
				for _, uid := range []string{uidP, uidS, uidR, uidW} {
					seed.contact(userA.ID, uid, &old)
				}
				seed.contact(userB.ID, uidS, nil) // B's live contact, same UID
				seed.contact(userA.ID, uidR, nil) // A re-created it
				// A second soft-deleted copy still inside the undo window. The
				// partial unique index only covers live rows, so this is legal.
				seed.contact(userA.ID, uidW, &recent)

				rowFor := func(u models.User, uid string) {
					over := map[string]any{"user_id": u.ID, col: uid}
					for _, other := range spec.Cols {
						if other != col {
							over[other] = "filler-" + other // an unrelated UID
						}
					}
					seed.insert(spec.Table, over)
				}
				rowFor(userA, uidP)
				rowFor(userA, uidS)
				rowFor(userB, uidS)
				rowFor(userA, uidR)
				rowFor(userA, uidW)

				require.NoError(t, PurgeSoftDeletedRows(db, purgeConfig()))

				count := func(u models.User, uid string) int64 { return seededRowCount(t, db, spec.Table, col, u.ID, uid) }
				assert.Zero(t, count(userA, uidP), "purged contact's own row must be removed")
				assert.Zero(t, count(userA, uidS), "owner's row for the purged contact must be removed even when another user has a live same-UID contact")
				assert.Equal(t, int64(1), count(userB, uidS), "ANOTHER USER's live same-UID contact's row must survive")
				assert.Equal(t, int64(1), count(userA, uidR), "same user's re-created live same-UID contact's row must survive")
				assert.Equal(t, int64(1), count(userA, uidW), "row of a UID whose other soft-deleted copy is still inside the retention window must survive")

				// The purge-eligible contacts themselves are gone; the live and
				// in-window ones are not.
				var purged, kept int64
				require.NoError(t, db.Unscoped().Model(&models.Contact{}).
					Where("user_id = ? AND deleted_at < ?", userA.ID, time.Now().AddDate(0, 0, -30)).Count(&purged).Error)
				require.NoError(t, db.Unscoped().Model(&models.Contact{}).
					Where("(user_id = ? AND vcard_uid IN ?) OR user_id = ?", userA.ID, []string{uidR, uidW}, userB.ID).
					Where("deleted_at IS NULL OR deleted_at >= ?", time.Now().AddDate(0, 0, -30)).Count(&kept).Error)
				assert.Zero(t, purged)
				assert.Equal(t, int64(3), kept)
			})
		}
	}
}
