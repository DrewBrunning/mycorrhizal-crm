package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// helpers — all against the real migrated schema (dbtest), CLAUDE.md trap #1
// ---------------------------------------------------------------------------

func integrityTestDB(t *testing.T) (*gorm.DB, config.Config) {
	t.Helper()
	db := dbtest.New(t)
	cfg := config.Config{
		DBIntegrityCheckEnabled:       true,
		DBIntegrityCheckIntervalHours: 24,
		ProfilePhotoDir:               t.TempDir(),
		AttachmentsDir:                t.TempDir(),
	}
	return db, cfg
}

func mkUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{Username: name, Email: name + "@example.com", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func mkContact(t *testing.T, db *gorm.DB, userID uint, first string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: first}
	require.NoError(t, db.Create(&c).Error)
	require.NotEmpty(t, c.VCardUID)
	return c
}

// softDeleteContact marks a contact deleted the way DeleteContact does, but
// deliberately leaves its graph rows behind — the invalid-but-valid state the
// checker exists to catch (foreign keys cannot see it).
func softDeleteContact(t *testing.T, db *gorm.DB, c models.Contact) {
	t.Helper()
	require.NoError(t, db.Delete(&models.Contact{}, "id = ?", c.ID).Error)
	var check models.Contact
	require.NoError(t, db.Unscoped().First(&check, "id = ?", c.ID).Error)
	require.NotNil(t, check.DeletedAt) // trap #6: assert with Unscoped, not Count
}

func mkEdge(t *testing.T, db *gorm.DB, userID uint, src, tgt, typ, status string) models.RelationshipEdge {
	t.Helper()
	e := models.RelationshipEdge{
		UserID: userID, SourceID: src, TargetID: tgt, Type: typ,
		Directional: !models.IsSymmetricRelationType(typ),
		Source:      models.RelationshipSourceUserConfirmed, Confidence: 1,
		Status: status, Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&e).Error)
	return e
}

// findingFor returns the finding for a check slug (and user), or false.
func findingFor(r DataIntegrityReport, check string, userID uint) (IntegrityFinding, bool) {
	for _, f := range r.Findings {
		if f.Check == check && f.UserID == userID {
			return f, true
		}
	}
	return IntegrityFinding{}, false
}

func runDataChecks(t *testing.T, db *gorm.DB, cfg config.Config) DataIntegrityReport {
	t.Helper()
	r, err := RunDataIntegrityChecks(context.Background(), db, cfg)
	require.NoError(t, err, "no probe should fail to run on a well-formed schema")
	return r
}

// ---------------------------------------------------------------------------
// healthy baseline
// ---------------------------------------------------------------------------

func TestDataIntegrity_HealthyDBIsClean(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	b := mkContact(t, db, u.ID, "B")
	mkEdge(t, db, u.ID, a.VCardUID, b.VCardUID, "friend_of", models.RelationshipStatusConfirmed)
	require.NoError(t, db.Create(&models.CircleMember{
		CircleID: mkCircle(t, db, u.ID), UserID: u.ID, MemberVCardUID: a.VCardUID,
	}).Error)

	r := runDataChecks(t, db, cfg)
	assert.True(t, r.OK, "healthy DB must report clean, got: %+v", r.Findings)
	assert.Empty(t, r.Findings)
}

func mkCircle(t *testing.T, db *gorm.DB, userID uint) string {
	t.Helper()
	c := models.Circle{UserID: userID, Name: "friends"}
	require.NoError(t, db.Create(&c).Error)
	return c.ID
}

// ---------------------------------------------------------------------------
// INV-D1 — relationship endpoint references a contact that no longer exists
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_D1_EndpointMissing(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	mkEdge(t, db, u.ID, a.VCardUID, "00000000-0000-4000-8000-000000000000", "friend_of", models.RelationshipStatusConfirmed)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "relationship_edge.endpoint_missing", u.ID)
	require.True(t, ok, "expected INV-D1 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D1", f.Invariant)
	assert.Equal(t, IntegritySeverityViolation, f.Severity)
	assert.Equal(t, 1, f.Count)
	assert.True(t, f.Repairable)
	assert.False(t, r.OK)
}

// ---------------------------------------------------------------------------
// INV-D7 — a soft-deleted contact is still a live relationship target.
// This is the case foreign keys CANNOT catch (issue #460 "How to verify").
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_D7_SoftDeletedEndpoint(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	b := mkContact(t, db, u.ID, "B")
	mkEdge(t, db, u.ID, a.VCardUID, b.VCardUID, "friend_of", models.RelationshipStatusConfirmed)

	softDeleteContact(t, db, b)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "relationship_edge.endpoint_soft_deleted", u.ID)
	require.True(t, ok, "expected INV-D7 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D7", f.Invariant)
	assert.Equal(t, IntegritySeverityViolation, f.Severity)
	assert.False(t, f.Repairable, "the contact may yet be undeleted — never auto-repair this")
	assert.Contains(t, f.Detail, "confirmed edge")
	assert.False(t, r.OK)

	// And it must NOT be miscounted as INV-D1: the row still exists.
	_, isMissing := findingFor(r, "relationship_edge.endpoint_missing", u.ID)
	assert.False(t, isMissing)
}

// ---------------------------------------------------------------------------
// INV-D2 — reciprocal relationships are consistent / derivable
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_D2_UnknownStoredType(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	b := mkContact(t, db, u.ID, "B")
	// A registered type first, then force an unregistered one past the model
	// validator with a raw update.
	e := mkEdge(t, db, u.ID, a.VCardUID, b.VCardUID, "friend_of", models.RelationshipStatusConfirmed)
	require.NoError(t, db.Exec("UPDATE relationship_edges SET type = ? WHERE id = ?", "frenemy_of", e.ID).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "relationship_edge.unknown_type", u.ID)
	require.True(t, ok, "expected INV-D2 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D2", f.Invariant)
	assert.Contains(t, f.Detail, "frenemy_of")
	assert.False(t, r.OK)
}

func TestDataIntegrity_INV_D2_RegistryIsAConsistentInvolution(t *testing.T) {
	// A pure guard on the shipped registry: every known token's inverse
	// round-trips. If this ever fails it is a code defect in
	// relationship_type_registry.go, caught here before it reaches data.
	db, cfg := integrityTestDB(t)
	r := runDataChecks(t, db, cfg)
	_, broken := findingFor(r, "relationship_type.registry_inconsistent", 0)
	assert.False(t, broken, "shipped relation-type registry must be a consistent involution")
}

// ---------------------------------------------------------------------------
// INV-D3 — orphaned join rows (contact side, which has no foreign key)
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_D3_OrphanedJoinRows(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	circleID := mkCircle(t, db, u.ID)

	// circle_members.member_vcard_uid -> a contact that never existed.
	require.NoError(t, db.Create(&models.CircleMember{
		CircleID: circleID, UserID: u.ID, MemberVCardUID: "11111111-1111-4111-8111-111111111111",
	}).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "circle_member.orphaned_contact", u.ID)
	require.True(t, ok, "expected INV-D3 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D3", f.Invariant)
	assert.True(t, f.Repairable)
	assert.False(t, r.OK)
}

func TestDataIntegrity_INV_D3_FieldValueOrphanedContact(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	defID := mkFieldDefinition(t, db, u.ID)

	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: defID, UserID: u.ID,
		EntityID: "22222222-2222-4222-8222-222222222222", Value: json.RawMessage(`"x"`),
	}).Error)

	r := runDataChecks(t, db, cfg)
	_, ok := findingFor(r, "field_value.orphaned_contact", u.ID)
	require.True(t, ok, "expected field_value INV-D3 finding, got: %+v", r.Findings)
}

func mkFieldDefinition(t *testing.T, db *gorm.DB, userID uint) string {
	t.Helper()
	d := models.FieldDefinition{
		UserID: userID, Label: "Shoe size", Key: "shoe_size", Target: "contact", Type: "text",
	}
	require.NoError(t, db.Create(&d).Error)
	return d.ID
}

// ---------------------------------------------------------------------------
// INV-D4 — dangling external references and missing files
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_D4_DanglingExternalIdentity(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	require.NoError(t, db.Create(&models.ExternalIdentity{
		UserID: u.ID, EntityID: "33333333-3333-4333-8333-333333333333",
		System: "github", ExternalID: "octocat",
	}).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "external_identity.dangling_contact", u.ID)
	require.True(t, ok, "expected INV-D4 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D4", f.Invariant)
	assert.False(t, f.Repairable)
}

func TestDataIntegrity_INV_D4_DanglingFieldDefinition(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")
	defID := mkFieldDefinition(t, db, u.ID)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: defID, UserID: u.ID, EntityID: c.VCardUID, Value: json.RawMessage(`"42"`),
	}).Error)
	// Drop the definition out from under the value with FK enforcement off, so
	// the CASCADE does not also take the value.
	require.NoError(t, db.Exec("PRAGMA foreign_keys=OFF").Error)
	require.NoError(t, db.Exec("DELETE FROM field_definitions WHERE id = ?", defID).Error)
	require.NoError(t, db.Exec("PRAGMA foreign_keys=ON").Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "field_value.dangling_definition", u.ID)
	require.True(t, ok, "expected INV-D4 finding, got: %+v", r.Findings)
	assert.True(t, f.Repairable)
}

func TestDataIntegrity_INV_D4_MissingAttachmentFile(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")

	// A row that names a file that is not on disk.
	require.NoError(t, db.Create(&models.Attachment{
		UserID: u.ID, ContactVCardUID: c.VCardUID,
		StoredName: "deadbeef-dead-4bee-8dead-beefdeadbeef", OriginalName: "cv.pdf",
		ContentType: "application/pdf", SizeBytes: 10,
	}).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "attachment.missing_file", u.ID)
	require.True(t, ok, "expected missing-file finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D4", f.Invariant)

	// Now put the file there — finding clears.
	p := filepath.Join(cfg.AttachmentsDir, "deadbeef-dead-4bee-8dead-beefdeadbeef")
	require.NoError(t, os.WriteFile(p, []byte("hi"), 0o600))
	r2 := runDataChecks(t, db, cfg)
	_, still := findingFor(r2, "attachment.missing_file", u.ID)
	assert.False(t, still, "finding must clear once the file exists")
}

func TestDataIntegrity_MissingFilesSkippedWhenDirsUnset(t *testing.T) {
	db, cfg := integrityTestDB(t)
	cfg.AttachmentsDir = ""
	cfg.ProfilePhotoDir = ""
	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")
	require.NoError(t, db.Create(&models.Attachment{
		UserID: u.ID, ContactVCardUID: c.VCardUID, StoredName: "x", OriginalName: "x",
		ContentType: "text/plain", SizeBytes: 1,
	}).Error)

	r := runDataChecks(t, db, cfg)
	_, ok := findingFor(r, "attachment.missing_file", u.ID)
	assert.False(t, ok, "file checks must be skipped when the directory is not configured")
}

// ---------------------------------------------------------------------------
// INV-D4 (info) — vanished audit refs do NOT flip OK
// ---------------------------------------------------------------------------

func TestDataIntegrity_VanishedAuditRef_IsInfoOnly(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	require.NoError(t, db.Exec(
		`INSERT INTO audit_events (created_at, updated_at, entity_type, entity_id, operation, user_id, hash, prev_hash)
		 VALUES (CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, 'delete', ?, '', '')`,
		models.AuditEntityContact, "44444444-4444-4444-8444-444444444444", u.ID).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "audit_event.vanished_contact", u.ID)
	require.True(t, ok)
	assert.Equal(t, IntegritySeverityInfo, f.Severity)
	assert.True(t, r.OK, "an info finding must not flip Report.OK")
}

// ---------------------------------------------------------------------------
// INV-D6 — required / enum fields
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_D6_InvalidEnum(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	b := mkContact(t, db, u.ID, "B")
	e := mkEdge(t, db, u.ID, a.VCardUID, b.VCardUID, "friend_of", models.RelationshipStatusConfirmed)
	require.NoError(t, db.Exec("UPDATE relationship_edges SET status = 'bogus' WHERE id = ?", e.ID).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "relationship_edge.invalid_enum", u.ID)
	require.True(t, ok, "expected INV-D6 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D6", f.Invariant)
	assert.False(t, r.OK)
}

// ---------------------------------------------------------------------------
// INV-D8 — canonical record consistency
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_D8_InvalidCardJSON(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")
	require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", "{not json", c.ID).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "canonical_record.invalid_json", u.ID)
	require.True(t, ok, "expected INV-D8 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D8", f.Invariant)
	assert.False(t, r.OK)
}

func TestDataIntegrity_INV_D8_DuplicateElementID(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")
	dup := `{"emails":[{"id":"e1","address":"a@x.com"},{"id":"e1","address":"b@x.com"}]}`
	require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", dup, c.ID).Error)

	r := runDataChecks(t, db, cfg)
	_, ok := findingFor(r, "canonical_record.duplicate_element_id", u.ID)
	require.True(t, ok, "expected duplicate-element-id finding, got: %+v", r.Findings)
}

// A Card.Media photo entry pointing at a remote URL that has not been
// downloaded to the photo store (contacts.photo empty) is the transient-photo
// state mergeMedia preserves — surfaced as an advisory (info) finding, not a
// violation.
func TestDataIntegrity_INV_D8_UnresolvedRemotePhoto(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")
	// Splice a remote-URL photo entry into the existing (name-consistent) Card
	// via json_set so the denormalized contacts.* columns still project from it
	// — this isolates the INV-D8 canonical_record probe from the INV-A5
	// derived-column probe (issue #497). contacts.photo stays empty: the URL
	// has not been downloaded to the photo store.
	media := `[{"kind":"photo","uri":"https://example.com/grace.jpg","mediaType":"image/jpeg"}]`
	require.NoError(t, db.Exec(
		"UPDATE contacts SET card = json_set(card, '$.media', json(?)), photo = '' WHERE id = ?",
		media, c.ID).Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "canonical_record.unresolved_remote_photo", u.ID)
	require.True(t, ok, "expected INV-D8 unresolved-remote-photo finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-D8", f.Invariant)
	assert.Equal(t, IntegritySeverityInfo, f.Severity)
	assert.False(t, f.Repairable)
	assert.True(t, r.OK, "an advisory info finding must not flip Report.OK")
}

// Once the remote photo has been downloaded (contacts.photo populated), the
// Card.Media photo entry is a normal flat-backed bridge — no finding.
func TestDataIntegrity_INV_D8_DownloadedRemotePhotoIsClean(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	c := mkContact(t, db, u.ID, "A")
	// Same splice, but the photo has been downloaded: contacts.photo names a
	// real file in the photo store, so the Card.Media entry is a normal
	// flat-backed bridge and the advisory must not fire.
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ProfilePhotoDir, "grace-1234.jpg"), []byte("x"), 0o600))
	media := `[{"kind":"photo","uri":"https://example.com/grace.jpg","mediaType":"image/jpeg"}]`
	require.NoError(t, db.Exec(
		"UPDATE contacts SET card = json_set(card, '$.media', json(?)), photo = 'grace-1234.jpg' WHERE id = ?",
		media, c.ID).Error)

	r := runDataChecks(t, db, cfg)
	_, ok := findingFor(r, "canonical_record.unresolved_remote_photo", u.ID)
	assert.False(t, ok, "a downloaded photo (contacts.photo set) must not produce the advisory, got: %+v", r.Findings)
	assert.True(t, r.OK, "the spliced-but-consistent fixture must stay clean, got: %+v", r.Findings)
}

// ---------------------------------------------------------------------------
// INV-A5 — derived index (FTS) divergence, cheap count version
// ---------------------------------------------------------------------------

func TestDataIntegrity_INV_A5_FTSDivergence(t *testing.T) {
	db, cfg := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	mkContact(t, db, u.ID, "A")
	mkContact(t, db, u.ID, "B")

	// Delete a contact's FTS row without touching the base table — exactly
	// what a raw-SQL migration that bypassed the triggers would do.
	require.NoError(t, db.Exec("DELETE FROM contacts_fts WHERE rowid = (SELECT MIN(id) FROM contacts)").Error)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "derived_index.fts_row_count_divergent", 0)
	require.True(t, ok, "expected INV-A5 finding, got: %+v", r.Findings)
	assert.Equal(t, "INV-A5", f.Invariant)
	assert.True(t, f.Repairable)
	assert.False(t, r.OK)
}

// ---------------------------------------------------------------------------
// scoping — a reference that matches ANOTHER user's contact is still a
// violation (no cross-user "resolution", CLAUDE.md item 5)
// ---------------------------------------------------------------------------

func TestDataIntegrity_CrossUserReferenceIsStillAViolation(t *testing.T) {
	db, cfg := integrityTestDB(t)
	alice := mkUser(t, db, "alice")
	bob := mkUser(t, db, "bob")
	aliceContact := mkContact(t, db, alice.ID, "A")
	bobContact := mkContact(t, db, bob.ID, "B")

	// An edge OWNED BY alice whose target is BOB's contact vcard_uid.
	mkEdge(t, db, alice.ID, aliceContact.VCardUID, bobContact.VCardUID, "friend_of", models.RelationshipStatusConfirmed)

	r := runDataChecks(t, db, cfg)
	f, ok := findingFor(r, "relationship_edge.endpoint_missing", alice.ID)
	require.True(t, ok, "a cross-user endpoint must be reported as missing for the owning user")
	assert.Equal(t, uint(alice.ID), f.UserID)
}

// ---------------------------------------------------------------------------
// context cancellation
// ---------------------------------------------------------------------------

func TestDataIntegrity_RespectsContextCancellation(t *testing.T) {
	db, cfg := integrityTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunDataIntegrityChecks(ctx, db, cfg)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// query plan: the contact-ref joins must use an index, not scan `contacts`
// ---------------------------------------------------------------------------

// TestDataIntegrity_ContactRefJoinsUseAnIndex pins migration 000050: every
// DB-01 pass that LEFT JOINs `contacts` on vcard_uid without a
// `deleted_at IS NULL` filter (so it can see tombstones — INV-D3/D7) must
// resolve that join through an index. Before 000050 the only vcard_uid index
// was PARTIAL (`WHERE deleted_at IS NULL`), unusable here, and every one of
// these scans was a full table scan of `contacts` — ~16 s at 4,000 contacts
// (CAP-01). CheckDBIntegrityScheduled runs this pass on a timer in production.
func TestDataIntegrity_ContactRefJoinsUseAnIndex(t *testing.T) {
	db := dbtest.New(t)

	// The real checker queries (referenced, not re-typed, so they can't drift).
	queries := map[string]string{
		"relationship_endpoints": relationshipEndpointsQuery,
	}
	for _, tgt := range []contactRefTarget{
		{table: "circle_members", col: "member_vcard_uid"},
		{table: "household_members", col: "member_vcard_uid"},
		{table: "contact_tags", col: "contact_vcard_uid"},
		{table: "field_values", col: "entity_id"},
		{table: "external_identities", col: "entity_id"},
		{table: "import_source_links", col: "entity_uid", extraWhere: "j.entity_kind = 'contact'"},
	} {
		queries[tgt.table] = tgt.query()
	}

	// contactsProbeConstrainsVCardUID reports whether the plan's join into
	// `contacts` (alias c) narrows on vcard_uid. Without migration 000050 the
	// best index available is idx_contacts_user_id, so the probe reads
	// `SEARCH c USING INDEX idx_contacts_user_id (user_id=?)` — a per-user
	// linear filter for the vcard_uid, i.e. O(contacts-per-user) per ref. With
	// 000050 it reads `... (user_id=? AND vcard_uid=?)` — an O(log n) point
	// lookup. The vcard_uid token in the plan line for `c` is the signal.
	contactsProbeConstrainsVCardUID := func(t *testing.T, sql string) bool {
		t.Helper()
		var plan []struct {
			Detail string `gorm:"column:detail"`
		}
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+sql, models.RelationshipStatusConfirmed).Scan(&plan).Error)
		require.NotEmpty(t, plan)
		for _, row := range plan {
			d := row.Detail
			isContactsProbe := strings.Contains(d, " c ") || strings.HasSuffix(d, " c") ||
				strings.Contains(d, " c USING") || strings.Contains(d, " contacts")
			if isContactsProbe && (strings.Contains(d, "SEARCH") || strings.Contains(d, "SCAN")) {
				return strings.Contains(d, "vcard_uid")
			}
		}
		t.Fatalf("no plan line for the `contacts` join in:\n%s", strings.Join(planDetails(plan), "\n"))
		return false
	}

	for name, sql := range queries {
		t.Run(name, func(t *testing.T) {
			assert.Truef(t, contactsProbeConstrainsVCardUID(t, sql),
				"the `contacts` join in the %s integrity query does not narrow on vcard_uid — migration 000050's index is missing or unused (this is the ~16s-at-4k-contacts scan from CAP-01)", name)
		})
	}

	// Hand-verify inside the test: drop 000050's index and the same joins fall
	// back to the user_id-only index, proving the assertion above has teeth.
	t.Run("negative control: without 000050 the join no longer narrows on vcard_uid", func(t *testing.T) {
		require.NoError(t, db.Exec("DROP INDEX idx_contacts_user_vcard_uid_all").Error)
		t.Cleanup(func() {
			require.NoError(t, db.Exec("CREATE INDEX idx_contacts_user_vcard_uid_all ON contacts(user_id, vcard_uid)").Error)
		})
		allNarrow := true
		for _, sql := range queries {
			if !contactsProbeConstrainsVCardUID(t, sql) {
				allNarrow = false
			}
		}
		assert.False(t, allNarrow, "with 000050's index dropped the contact-ref joins must lose the vcard_uid narrowing")
	})
}

func planDetails(plan []struct {
	Detail string `gorm:"column:detail"`
}) []string {
	out := make([]string, len(plan))
	for i, r := range plan {
		out[i] = "  " + r.Detail
	}
	return out
}
