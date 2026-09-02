package services

// DB-03 (issue #494) — the deeper, table-driven per-invariant matrix that ADR
// 0012's "Checked by" lines defer here. Where data_integrity_service_test.go
// pins each probe against a minimal hand-built fixture, this file drives every
// at-rest invariant from the TEST-02 canonical pathological dataset (#430), so
// the clean case is a realistic worst case (soft-deleted contact with a full
// cascade surface, a re-used vcard_uid, duplicate pairs, unicode, edge-case
// dates) rather than two rows.
//
// Every row is one invariant with:
//
//   - a HOLDS case — the untouched TEST-02 fixture must report OK with no
//     violation finding (asserted once, shared, in the matrix runner's
//     baseline step and standalone in TestDataIntegrity_TEST02_FixtureIsClean);
//   - a DETECTED case — one targeted mutation on top of the fixture that the
//     checker must find, name with the right INV-D* id and Check slug, and
//     that must flip report.OK. The mutation is deliberately surgical: it adds
//     exactly one new violation class, which the runner asserts, so a probe
//     that over-reports on pathological input fails here too.
//
// The "break the check" hand-verification the issue calls the deliverable is
// recorded in the PR: neuter each probe in data_integrity_service.go and
// confirm exactly the matching row below fails.
//
// INV-D2's registry-involution half and every INV-A*/operation invariant are
// not expressible as a single at-rest mutation and live elsewhere:
// data_integrity_operations_test.go (merge / cascade / import / migration /
// restore) and internal/propertytest (INV-A1..A6, INV-D8 reprojection).

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// loadCanonicalFixture populates a fresh migrated database with the TEST-02
// dataset. See populateCanonicalFixture for what the cfg / file backing do.
func loadCanonicalFixture(t *testing.T) (*gorm.DB, config.Config, *canonicalfixture.Dataset) {
	t.Helper()
	db := dbtest.New(t)
	cfg, ds := populateCanonicalFixture(t, db)
	return db, cfg, ds
}

// populateCanonicalFixture loads the TEST-02 dataset into an already-migrated
// db and writes a backing file for every live attachment / bare-filename
// profile photo, so a clean run has no INV-D4 missing-file false positive. It
// returns a cfg whose file directories point at the populated temp dirs and
// the created Dataset. Split from loadCanonicalFixture so the operation tests
// can drive it against a file-backed dbtest.NewAt database (backup/restore).
func populateCanonicalFixture(t *testing.T, db *gorm.DB) (config.Config, *canonicalfixture.Dataset) {
	t.Helper()

	m, err := canonicalfixture.Read()
	require.NoError(t, err, "read TEST-02 manifest")
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err, "populate TEST-02 fixture")

	cfg := config.Config{
		DBIntegrityCheckEnabled:       true,
		DBIntegrityCheckIntervalHours: 24,
		AttachmentsDir:                t.TempDir(),
		ProfilePhotoDir:               t.TempDir(),
	}

	var storedNames []string
	require.NoError(t, db.Raw(`SELECT stored_name FROM attachments WHERE deleted_at IS NULL`).Scan(&storedNames).Error)
	for _, n := range storedNames {
		require.NoError(t, os.WriteFile(filepath.Join(cfg.AttachmentsDir, n), []byte("x"), 0o600))
	}

	var photos []string
	require.NoError(t, db.Raw(`SELECT photo FROM contacts WHERE deleted_at IS NULL AND photo <> ''`).Scan(&photos).Error)
	for _, p := range photos {
		if strings.HasPrefix(p, "data:") || strings.Contains(p, "://") || p != filepath.Base(p) {
			continue
		}
		require.NoError(t, os.WriteFile(filepath.Join(cfg.ProfilePhotoDir, p), []byte("x"), 0o600))
	}

	return cfg, ds
}

// violationSlugs is the set of distinct Check slugs among a report's violation
// (not info) findings.
func violationSlugs(r DataIntegrityReport) map[string]bool {
	out := map[string]bool{}
	for _, f := range r.Findings {
		if f.Severity == IntegritySeverityViolation {
			out[f.Check] = true
		}
	}
	return out
}

// addedSlugs is the sorted list of violation slugs in after that were not in
// before.
func addedSlugs(before, after map[string]bool) []string {
	var out []string
	for s := range after {
		if !before[s] {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func findingByCheck(r DataIntegrityReport, check string) (IntegrityFinding, bool) {
	for _, f := range r.Findings {
		if f.Check == check {
			return f, true
		}
	}
	return IntegrityFinding{}, false
}

// randUID is a syntactically valid v4 UUID that never collides with a fixture
// contact — the manifest's contacts are all 10000000-0000-4000-8000-0000000000NN.
func randUID(tag string) string {
	return "ffffffff-0000-4000-8000-0000000000" + tag
}

// ---------------------------------------------------------------------------
// HOLDS — the untouched TEST-02 fixture is clean
// ---------------------------------------------------------------------------

func TestDataIntegrity_TEST02_FixtureIsClean(t *testing.T) {
	db, cfg, _ := loadCanonicalFixture(t)

	r, err := RunDataIntegrityChecks(context.Background(), db, cfg)
	require.NoError(t, err, "no probe should error on the TEST-02 schema")
	assert.True(t, r.OK, "the TEST-02 pathological fixture must be clean — no false positives; got: %+v", r.Findings)

	for _, f := range r.Findings {
		assert.NotEqual(t, IntegritySeverityViolation, f.Severity,
			"unexpected violation on the clean fixture: %+v", f)
	}
}

// ---------------------------------------------------------------------------
// DETECTED — one surgical violation per invariant, on top of TEST-02
// ---------------------------------------------------------------------------

type matrixCase struct {
	name       string
	invariant  string
	wantCheck  string
	repairable bool
	info       bool // finding is info severity and must NOT flip report.OK
	mutate     func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset)
}

func TestDataIntegrity_TEST02_InvariantMatrix(t *testing.T) {
	cases := []matrixCase{
		{
			name:      "INV-D1/relationship endpoint references a contact that never existed",
			invariant: "INV-D1", wantCheck: "relationship_edge.endpoint_missing", repairable: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				mkEdge(t, db, ds.User.ID, ds.Contacts["ada"].VCardUID, randUID("d1"), "friend_of", models.RelationshipStatusConfirmed)
			},
		},
		{
			name:      "INV-D2/stored edge type is not in the registry",
			invariant: "INV-D2", wantCheck: "relationship_edge.unknown_type", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Exec(
					`UPDATE relationship_edges SET type = 'frenemy_of' WHERE user_id = ? AND type = 'coworker_of'`,
					ds.User.ID).Error)
			},
		},
		{
			name:      "INV-D3/circle membership references a contact that never existed",
			invariant: "INV-D3", wantCheck: "circle_member.orphaned_contact", repairable: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.CircleMember{
					CircleID: ds.Circles[0].ID, UserID: ds.User.ID, MemberVCardUID: randUID("31"),
				}).Error)
			},
		},
		{
			name:      "INV-D3/household membership references a contact that never existed",
			invariant: "INV-D3", wantCheck: "household_member.orphaned_contact", repairable: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.HouseholdMember{
					HouseholdID: ds.Households[0].ID, UserID: ds.User.ID, MemberVCardUID: randUID("32"), Role: "roommate",
				}).Error)
			},
		},
		{
			name:      "INV-D3/tagging references a contact that never existed",
			invariant: "INV-D3", wantCheck: "contact_tag.orphaned_contact", repairable: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.ContactTag{
					TagID: ds.Tags[0].ID, UserID: ds.User.ID, ContactVCardUID: randUID("33"),
				}).Error)
			},
		},
		{
			name:      "INV-D3/field value references a contact that never existed",
			invariant: "INV-D3", wantCheck: "field_value.orphaned_contact", repairable: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.FieldValue{
					FieldDefinitionID: ds.FieldDefinitions[0].ID, UserID: ds.User.ID,
					EntityID: randUID("34"), Value: json.RawMessage(`"x"`),
				}).Error)
			},
		},
		{
			name:      "INV-D4/external identity references a contact that no longer exists",
			invariant: "INV-D4", wantCheck: "external_identity.dangling_contact", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.ExternalIdentity{
					UserID: ds.User.ID, EntityID: randUID("41"), System: "github", ExternalID: "octocat",
				}).Error)
			},
		},
		{
			name:      "INV-D4/external activity references a contact that no longer exists",
			invariant: "INV-D4", wantCheck: "external_activity.dangling_contact", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.ExternalActivity{
					UserID: ds.User.ID, EntityID: randUID("42"), SourceSystem: "immich",
					ExternalID: "asset-1", Type: "photo-appearance", OccurredAt: time.Now(),
				}).Error)
			},
		},
		{
			name:      "INV-D4/import source link references a contact that no longer exists",
			invariant: "INV-D4", wantCheck: "import_source_link.dangling_contact", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.ImportSourceLink{
					UserID: ds.User.ID, System: "monica", ExternalID: "contact/999",
					EntityKind: models.ImportSourceLinkKindContact, EntityUID: randUID("43"),
				}).Error)
			},
		},
		{
			name:      "INV-D4/field value references a definition that was removed",
			invariant: "INV-D4", wantCheck: "field_value.dangling_definition", repairable: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				// ds.FieldDefinitions[0] (dietary_restrictions) has two values.
				// Drop it with FK enforcement off so the ON DELETE CASCADE does
				// not also take the values — the dangling state a migration that
				// toggled foreign_keys would leave.
				require.NoError(t, db.Exec("PRAGMA foreign_keys=OFF").Error)
				require.NoError(t, db.Exec("DELETE FROM field_definitions WHERE id = ?", ds.FieldDefinitions[0].ID).Error)
				require.NoError(t, db.Exec("PRAGMA foreign_keys=ON").Error)
			},
		},
		{
			name:      "INV-D4/attachment row names a file that is not on disk",
			invariant: "INV-D4", wantCheck: "attachment.missing_file", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Create(&models.Attachment{
					UserID: ds.User.ID, ContactVCardUID: ds.Contacts["ada"].VCardUID,
					StoredName: "ghost-file-not-on-disk", OriginalName: "x.pdf",
					ContentType: "application/pdf", SizeBytes: 1,
				}).Error)
			},
		},
		{
			name:      "INV-D4/contact names a profile photo file that is not on disk",
			invariant: "INV-D4", wantCheck: "contact.missing_photo_file", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Exec("UPDATE contacts SET photo = 'ghost-photo.jpg' WHERE id = ?",
					ds.Contacts["dmitri"].ID).Error)
			},
		},
		{
			name:      "INV-D4(info)/audit row describes a contact with no row at all",
			invariant: "INV-D4", wantCheck: "audit_event.vanished_contact", info: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Exec(
					`INSERT INTO audit_events (created_at, updated_at, entity_type, entity_id, operation, user_id, hash, prev_hash)
					 VALUES (CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, 'delete', ?, '', '')`,
					models.AuditEntityContact, randUID("4a"), ds.User.ID).Error)
			},
		},
		{
			name:      "INV-D5/contact has a blank vcard_uid",
			invariant: "INV-D5", wantCheck: "contact.missing_vcard_uid", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				// A fresh, unconnected contact — blanking its uid must not also
				// orphan an edge endpoint.
				c := mkContact(t, db, ds.User.ID, "NoUID")
				require.NoError(t, db.Exec("UPDATE contacts SET vcard_uid = '' WHERE id = ?", c.ID).Error)
			},
		},
		{
			name:      "INV-D6/relationship edge has an out-of-range status",
			invariant: "INV-D6", wantCheck: "relationship_edge.invalid_enum", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Exec(
					`UPDATE relationship_edges SET status = 'bogus' WHERE user_id = ? AND type = 'owns'`,
					ds.User.ID).Error)
			},
		},
		{
			name:      "INV-D7/soft-deleted contact is still a confirmed edge endpoint",
			invariant: "INV-D7", wantCheck: "relationship_edge.endpoint_soft_deleted", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				x := mkContact(t, db, ds.User.ID, "X7e")
				z := mkContact(t, db, ds.User.ID, "Z7e")
				mkEdge(t, db, ds.User.ID, x.VCardUID, z.VCardUID, "friend_of", models.RelationshipStatusConfirmed)
				softDeleteContact(t, db, z)
			},
		},
		{
			name:      "INV-D7/soft-deleted contact is still a live circle member",
			invariant: "INV-D7", wantCheck: "circle_member.soft_deleted_contact", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				x := mkContact(t, db, ds.User.ID, "X7c")
				require.NoError(t, db.Create(&models.CircleMember{
					CircleID: ds.Circles[0].ID, UserID: ds.User.ID, MemberVCardUID: x.VCardUID,
				}).Error)
				softDeleteContact(t, db, x)
			},
		},
		{
			name:      "INV-D7/soft-deleted contact is still a live household member",
			invariant: "INV-D7", wantCheck: "household_member.soft_deleted_contact", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				x := mkContact(t, db, ds.User.ID, "X7h")
				require.NoError(t, db.Create(&models.HouseholdMember{
					HouseholdID: ds.Households[0].ID, UserID: ds.User.ID, MemberVCardUID: x.VCardUID, Role: "roommate",
				}).Error)
				softDeleteContact(t, db, x)
			},
		},
		{
			name:      "INV-D7/soft-deleted contact is still a live tagging",
			invariant: "INV-D7", wantCheck: "contact_tag.soft_deleted_contact", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				x := mkContact(t, db, ds.User.ID, "X7t")
				require.NoError(t, db.Create(&models.ContactTag{
					TagID: ds.Tags[0].ID, UserID: ds.User.ID, ContactVCardUID: x.VCardUID,
				}).Error)
				softDeleteContact(t, db, x)
			},
		},
		{
			name:      "INV-D8/canonical record Card column is not valid JSON",
			invariant: "INV-D8", wantCheck: "canonical_record.invalid_json", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", "{not json", ds.Contacts["dmitri"].ID).Error)
			},
		},
		{
			name:      "INV-D8/canonical record Card collection has a duplicate element id",
			invariant: "INV-D8", wantCheck: "canonical_record.duplicate_element_id", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				// Splice a duplicate-id collection into dmitri's existing card
				// rather than replacing it wholesale: personalInfo has no flat
				// column, so this breaks only the round-trip invariant and
				// leaves the flat projection (and every other probe) untouched.
				id := ds.Contacts["dmitri"].ID
				var raw string
				require.NoError(t, db.Raw("SELECT card FROM contacts WHERE id = ?", id).Scan(&raw).Error)
				var card map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(raw), &card))
				card["personalInfo"] = json.RawMessage(
					`[{"id":"p1","kind":"hobby","value":"a"},{"id":"p1","kind":"hobby","value":"b"}]`)
				patched, err := json.Marshal(card)
				require.NoError(t, err)
				require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", string(patched), id).Error)
			},
		},
		{
			name:      "INV-A5/derived FTS index diverges from the base table row count",
			invariant: "INV-A5", wantCheck: "derived_index.fts_row_count_divergent", repairable: true,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				require.NoError(t, db.Exec("DELETE FROM contacts_fts WHERE rowid = (SELECT MIN(id) FROM contacts)").Error)
			},
		},
		{
			name:      "INV-A5/denormalized contact column diverges from Card (issue #497)",
			invariant: "INV-A5", wantCheck: "derived_contact_column.divergent", repairable: false,
			mutate: func(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) {
				// A raw write to sort_name only — surgical: it does not touch
				// Card and trips no other probe.
				require.NoError(t, db.Exec(
					"UPDATE contacts SET sort_name = ? WHERE id = ?", "zzz-drift", ds.Contacts["ada"].ID).Error)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			db, cfg, ds := loadCanonicalFixture(t)

			base, err := RunDataIntegrityChecks(context.Background(), db, cfg)
			require.NoError(t, err)
			require.True(t, base.OK, "TEST-02 baseline must be clean before the mutation; got: %+v", base.Findings)
			baseSlugs := violationSlugs(base)

			tc.mutate(t, db, ds)

			r, err := RunDataIntegrityChecks(context.Background(), db, cfg)
			require.NoError(t, err, "the mutation must produce a finding, not a probe error")

			f, ok := findingByCheck(r, tc.wantCheck)
			require.True(t, ok, "expected the checker to report %q; got: %+v", tc.wantCheck, r.Findings)
			assert.Equal(t, tc.invariant, f.Invariant, "finding cites the wrong ADR invariant id")
			assert.GreaterOrEqual(t, f.Count, 1)
			assert.Equal(t, tc.repairable, f.Repairable, "Repairable flag")
			assert.NotEmpty(t, f.Detail)

			added := addedSlugs(baseSlugs, violationSlugs(r))
			if tc.info {
				assert.Equal(t, IntegritySeverityInfo, f.Severity)
				assert.True(t, r.OK, "an info finding must not flip report.OK")
				assert.Empty(t, added, "an info mutation must add no violation class")
				return
			}
			assert.Equal(t, IntegritySeverityViolation, f.Severity)
			assert.False(t, r.OK, "a violation must flip report.OK")
			assert.Equal(t, []string{tc.wantCheck}, added,
				"the mutation must add exactly one new violation class (a probe that over-reports fails here)")
		})
	}
}
