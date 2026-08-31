package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/models"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// CardDAV round-trip integration suite (issue #496).
//
// The fake server (fake_carddav_server_test.go) implements the RFC 6578 sync
// lifecycle the real integration depends on, so the full round trip can be
// exercised deterministically on every PR:
//
//	TEST-02 fixture (or inline cards) -> vCard exporter -> fake CardDAV server
//	    -> SyncSubscription (sync-collection + multiget + reconcile) -> DB
//
// These tests sit above the direct reconcileContactSync unit tests in
// contact_sync_service_test.go / sync_conflict_service_test.go: they drive the
// real HTTP plumbing — initial pull, incremental pull via sync-token/ETag,
// remote create/update/delete reconciliation, the deliberate local-edit-discard
// policy surfacing as conflicts, reconnection after an offline window, and the
// missing/malformed ETag/token degradation semantics — against the real
// migrated schema (dbtest.New, so the `etag` column name is the real one).
//
// Fidelity, not hostility: the hostile-input half of the live CardDAV/CalDAV
// path is issue #512's E2E job; this suite feeds the sync path only
// well-formed cards and concentrates on the lifecycle + round-trip semantics.
// ---------------------------------------------------------------------------

// setupCardDAVLifecycle builds a real-migrated-schema DB plus a user and a
// config whose photo dir is writable (so PHOTO-bearing fixture cards can
// round-trip through the photo bridge).
func setupCardDAVLifecycle(t *testing.T) (*gorm.DB, config.Config, models.User) {
	t.Helper()
	db := dbtest.New(t)
	cfg := config.Config{
		JWTSecretKey:    "test-jwt-secret-key-with-32-chars!!",
		ProfilePhotoDir: t.TempDir(),
	}
	user := createContactSyncTestUser(t, db)
	return db, cfg, user
}

// carddavTestVCard is a minimal vCard 4.0 fixture card for the lifecycle
// tests. N is Family;Given;Additional;Prefix;Suffix per RFC 6350 §6.2.2.
const carddavTestVCard = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:%s\r\nFN:%s %s\r\nN:%s;%s;;;\r\nEMAIL:%s\r\nEND:VCARD\r\n"

func lifecycleCard(t *testing.T, uid, first, last, email string) string {
	t.Helper()
	return fmt.Sprintf(carddavTestVCard, uid, first, last, last, first, email)
}

// newFakeSubscription wires a subscription at the fake's collection URL.
func newFakeSubscription(t *testing.T, db *gorm.DB, cfg config.Config, userID uint, url string) *models.ContactSubscription {
	t.Helper()
	return newContactTestSubscription(t, db, cfg, userID, url, "", "")
}

// syncFake runs one sync for an existing subscription (so the sync-token
// continuity the incremental tests depend on survives across calls) and
// reloads it.
func syncFake(t *testing.T, db *gorm.DB, cfg config.Config, sub *models.ContactSubscription) ContactSyncStats {
	t.Helper()
	service := NewContactSyncService(false)
	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err, "sync against the fake server must succeed")
	require.NoError(t, db.First(sub, sub.ID).Error, "subscription reload")
	return stats
}

func countContactRows(t *testing.T, db *gorm.DB, userID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

// --- initial pull + ETag/CTag incremental lifecycle -------------------------

func TestCardDAVLifecycle_InitialPullAndETagColumn(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	const bobHref = "/addressbooks/test/contacts/bob.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	fake.seedCard(bobHref, lifecycleCard(t, "bob-uid", "Bob", "Builder", "bob@example.com"))

	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{Created: 2}, stats)

	// Both cards landed as real Contact rows with the right flat fields.
	require.Equal(t, int64(2), countContactRows(t, db, user.ID))
	var alice models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&alice).Error)
	assert.Equal(t, "Alice", alice.Firstname)
	assert.Equal(t, "Wonderland", alice.Lastname)
	assert.Equal(t, "alice@example.com", alice.Email)

	// The per-resource ETag landed in the REAL `etag` column (dbtest applies
	// the hand-written migration SQL — this is the "assert against the real
	// column name" requirement of issue #496 item 2; an AutoMigrate-based DB
	// would silently agree with a wrong `e_tag` column name).
	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, aliceHref).First(&link).Error)
	fake.mu.Lock()
	wantETag := fake.cards[aliceHref].etag
	fake.mu.Unlock()
	assert.Equal(t, wantETag, link.ETag, "link.ETag must hold the server's etag in the real etag column")

	// The subscription recorded the server's sync-token for the next run.
	assert.Equal(t, fake.currentToken(), sub.SyncToken)
	assert.Equal(t, models.ContactSyncStatusSuccess, sub.LastSyncStatus)
}

func TestCardDAVLifecycle_UnchangedCTagNoRefetch(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	fake.seedCard("/addressbooks/test/contacts/alice.vcf", lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))

	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	// The second sync sees an unchanged sync-token: the server returns no
	// delta, so the client must fetch NO card bodies and reconcile NOTHING.
	fake.mu.Lock()
	baseline := fake.counts
	fake.mu.Unlock()
	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{}, stats, "an unchanged address book must reconcile nothing")

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Equal(t, baseline.SyncCollection+1, fake.counts.SyncCollection, "the client still asks for a delta")
	assert.Equal(t, baseline.MultiGet, fake.counts.MultiGet, "an unchanged CTag must not re-fetch any card body (no multiget)")
	assert.Equal(t, baseline.Query, fake.counts.Query, "an unchanged CTag must not fall back to a full refetch")
	assert.Equal(t, sub.SyncToken, fake.currentToken())
}

func TestCardDAVLifecycle_RemoteUpdateRefetchesAndUpdates(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	// A remote edit bumps the resource ETag; the next sync must refetch that
	// one body and apply it.
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice.new@example.com"))

	fake.mu.Lock()
	baselineMultiGet := fake.counts.MultiGet
	fake.mu.Unlock()

	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats)
	require.Equal(t, int64(1), countContactRows(t, db, user.ID), "an update must not duplicate the contact")

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&contact).Error)
	assert.Equal(t, "alice.new@example.com", contact.Email)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Equal(t, baselineMultiGet+1, fake.counts.MultiGet, "the changed resource's body must be refetched exactly once")
	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, aliceHref).First(&link).Error)
	assert.Equal(t, fake.cards[aliceHref].etag, link.ETag, "the etag column must track the new server etag")
}

func TestCardDAVLifecycle_RemoteDeleteArchives(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	const bobHref = "/addressbooks/test/contacts/bob.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	fake.seedCard(bobHref, lifecycleCard(t, "bob-uid", "Bob", "Builder", "bob@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	// Remote delete of bob while alice stays.
	fake.deleteCard(bobHref)

	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{Archived: 1}, stats)

	var bob models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "bob-uid").First(&bob).Error)
	assert.True(t, bob.Archived, "a remote delete archives the contact (soft delete, undo-able), it does not hard-delete")

	var linkCount int64
	require.NoError(t, db.Model(&models.ContactSyncLink{}).
		Where("subscription_id = ? AND href = ?", sub.ID, bobHref).Count(&linkCount).Error)
	assert.Zero(t, linkCount, "the sync link for the archived contact must be dropped")
}

func TestCardDAVLifecycle_RemoteCreatePulledIncrementally(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	fake.seedCard("/addressbooks/test/contacts/alice.vcf", lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	// A card appears on the server after the first sync.
	fake.seedCard("/addressbooks/test/contacts/carol.vcf", lifecycleCard(t, "carol-uid", "Carol", "Danvers", "carol@example.com"))

	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{Created: 1}, stats)

	var carol models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "carol-uid").First(&carol).Error)
	assert.Equal(t, "Carol", carol.Firstname)

	var alice models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&alice).Error)
	assert.False(t, alice.Archived, "unrelated cards must not be disturbed by the new create")
}

// --- ETag / sync-token degradation semantics (issue #496 item 2) -----------

// TestCardDAVLifecycle_MissingETagStillProcesses pins that a server which
// omits getetag entirely degrades to a defined behavior — the change is
// processed on content, not silently dropped or skipped. ETag is advisory
// metadata here: change detection runs on the content hash, so a missing ETag
// must never become a silent no-op.
func TestCardDAVLifecycle_MissingETagStillProcesses(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	fake.mu.Lock()
	fake.suppressETags = true
	fake.mu.Unlock()

	// Remote change, now with no ETag anywhere in the protocol responses.
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice.missing-etag@example.com"))

	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats, "a missing ETag must not silently drop or skip the change")

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&contact).Error)
	assert.Equal(t, "alice.missing-etag@example.com", contact.Email, "the change must actually land")

	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, aliceHref).First(&link).Error)
	assert.Empty(t, link.ETag, "no ETag on the wire means an empty stored etag — the defined degradation, not a resync")
}

// TestCardDAVLifecycle_MalformedETagFailsLoudly pins that a server emitting an
// unparseable (unquoted) ETag produces a LOUD, actionable sync failure — never
// a silent full resync and never a silent no-op. The library cannot decode the
// value, so the run fails with the ETag named in the error; the subscription's
// last-sync status records the failure.
func TestCardDAVLifecycle_MalformedETagFailsLoudly(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	fake.mu.Lock()
	fake.malformedETags = true
	fake.mu.Unlock()
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice.malformed-etag@example.com"))

	service := NewContactSyncService(false)
	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrContactSyncUnreachable, "a malformed ETag must surface as a classified sync failure")
	assert.Contains(t, strings.ToLower(err.Error()), "etag", "the failure must name the offending property (actionable, not 'sync failed')")

	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, models.ContactSyncStatusError, sub.LastSyncStatus)
	assert.NotEmpty(t, sub.LastSyncError)

	// Nothing was half-applied: the pre-change content is intact.
	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&contact).Error)
	assert.Equal(t, "alice@example.com", contact.Email, "a failed sync must not partially apply the remote change")
}

// TestCardDAVLifecycle_MissingSyncTokenDegradesToFullRefetchKeepsToken pins
// the defined behavior for a server that answers sync-collection but omits the
// RFC 6578 sync-token: the run processes the book as a full refetch and keeps
// the previous token, so a token-receiving server never regresses into a
// silent full-resync-every-run loop.
func TestCardDAVLifecycle_MissingSyncTokenDegradesToFullRefetchKeepsToken(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)
	priorToken := sub.SyncToken
	require.NotEmpty(t, priorToken)

	fake.mu.Lock()
	fake.emptySyncToken = true
	fake.mu.Unlock()
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice.no-token@example.com"))

	service := NewContactSyncService(false)
	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err, "a missing sync-token must degrade gracefully, not fail the run")
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats, "the accumulated change must still land")

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&contact).Error)
	assert.Equal(t, "alice.no-token@example.com", contact.Email)

	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, priorToken, sub.SyncToken, "the previous token must be kept, not overwritten with an empty one (which would force a full resync on every run)")
}

// TestCardDAVLifecycle_MissingSyncTokenAndBrokenQueryFailsLoudly covers the
// other half of the missing-token degradation: a server that omits the token
// (so we fall back to a full refetch) AND then fails that refetch must surface
// a classified, actionable error — never a silent no-op.
func TestCardDAVLifecycle_MissingSyncTokenAndBrokenQueryFailsLoudly(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	fake.seedCard("/addressbooks/test/contacts/alice.vcf", lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	fake.mu.Lock()
	fake.emptySyncToken = true
	fake.failQuery = true
	fake.mu.Unlock()
	fake.seedCard("/addressbooks/test/contacts/alice.vcf", lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice.broken@example.com"))

	service := NewContactSyncService(false)
	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrContactSyncUnreachable, "a broken full refetch after a missing token must surface as a classified failure")

	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, models.ContactSyncStatusError, sub.LastSyncStatus)

	// The change must not have half-applied.
	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&contact).Error)
	assert.Equal(t, "alice@example.com", contact.Email, "a failed sync must not partially apply the remote change")
}

// --- conflict paths (issue #496 item 4) ------------------------------------

// TestCardDAVLifecycle_OfflineWindowLocalEditSurfacesConflict is the full-HTTP
// version of the reconcile-level conflict test: a local edit made while the
// client is not syncing, followed by a remote change to an unrelated field, is
// recorded as a ContactSyncConflict on reconnect instead of being lost
// silently.
func TestCardDAVLifecycle_OfflineWindowLocalEditSurfacesConflict(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	// Local edit through the array path (the same shape the nested REST API
	// writes), made while no sync runs.
	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, aliceHref).First(&link).Error)
	var editTarget models.Contact
	require.NoError(t, db.First(&editTarget, link.ContactID).Error)
	editTarget.Phones = []models.ContactPhone{{Value: "555-0100"}}
	editTarget.JobTitle = "Local Edit"
	require.NoError(t, db.Save(&editTarget).Error)

	// Remote change to an unrelated field (email) happens while offline.
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice.conflict@example.com"))

	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats)

	var conflicts []models.ContactSyncConflict
	require.NoError(t, db.Where("user_id = ? AND contact_id = ?", user.ID, link.ContactID).Order("field ASC").Find(&conflicts).Error)
	require.Len(t, conflicts, 2, "the phone + job_title local edits must both surface as conflicts")

	fields := map[string]models.ContactSyncConflict{}
	for _, conflict := range conflicts {
		fields[conflict.Field] = conflict
		assert.Equal(t, models.SyncConflictStatusPending, conflict.Status)
		assert.Equal(t, sub.ID, conflict.SubscriptionID)
	}
	require.Contains(t, fields, models.SyncConflictFieldPhone)
	require.Contains(t, fields, models.SyncConflictFieldJobTitle)
	assert.Equal(t, `[{"type":"","value":"555-0100"}]`, fields[models.SyncConflictFieldPhone].LocalValue)
	assert.Equal(t, "Local Edit", fields[models.SyncConflictFieldJobTitle].LocalValue)
}

// TestCardDAVLifecycle_ReconnectAfterOfflineWindowAppliesAccumulatedDelta
// pins the reconnect path: while the server is genuinely unreachable the sync
// fails and records the failure; remote changes accumulate; once the server is
// back, one sync applies the whole accumulated delta through the sync-token —
// no full resync, no data loss.
func TestCardDAVLifecycle_ReconnectAfterOfflineWindowAppliesAccumulatedDelta(t *testing.T) {
	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	const aliceHref = "/addressbooks/test/contacts/alice.vcf"
	const bobHref = "/addressbooks/test/contacts/bob.vcf"
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice@example.com"))
	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	syncFake(t, db, cfg, sub)

	// Server goes down.
	fake.mu.Lock()
	fake.failRequests = true
	fake.mu.Unlock()

	service := NewContactSyncService(false)
	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.Error(t, err, "an unreachable server must fail the sync")
	assert.ErrorIs(t, err, ErrContactSyncUnreachable)
	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, models.ContactSyncStatusError, sub.LastSyncStatus)
	priorToken := sub.SyncToken

	// While the server is still down, the remote changes (update alice, create
	// bob) accumulate directly in the fake's store — invisible to the client,
	// which cannot reach it.
	fake.seedCard(aliceHref, lifecycleCard(t, "alice-uid", "Alice", "Wonderland", "alice.reconnected@example.com"))
	fake.seedCard(bobHref, lifecycleCard(t, "bob-uid", "Bob", "Builder", "bob@example.com"))

	// Server comes back.
	fake.mu.Lock()
	fake.failRequests = false
	fake.mu.Unlock()

	// One sync after the reconnect applies the whole accumulated delta.
	stats := syncFake(t, db, cfg, sub)
	assert.Equal(t, ContactSyncStats{Created: 1, Updated: 1}, stats)
	require.Equal(t, int64(2), countContactRows(t, db, user.ID))

	var alice models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "alice-uid").First(&alice).Error)
	assert.Equal(t, "alice.reconnected@example.com", alice.Email)

	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.NotEqual(t, priorToken, sub.SyncToken, "the token must have advanced past the offline window")
	assert.Equal(t, models.ContactSyncStatusSuccess, sub.LastSyncStatus)
}

// --- TEST-02 fixture round trip through the server --------------------------

// TestCardDAVRoundTrip_FixtureThroughServer_SemanticEquality is issue #496
// item 5 against the fake: every live contact of the canonical pathological
// fixture is exported to vCard, staged on a real-protocol CardDAV server,
// pulled back through SyncSubscription, and compared semantically per TEST-03
// (issue #431) via semanticequal.
//
// The assertion is "the server round trip is no worse than the format round
// trip": for a contact the vCard 4.0 format round-trips cleanly in-process
// (via the exact import->store->read-back pipeline the sync path uses), the
// server round trip must also be clean; and no fixture contact may acquire NEW
// differences by passing through a real protocol. A divergence here is a real
// re-serialize/re-parse cycle bug — exactly the class ADR-0002 warns about and
// the fake-only suites cannot see.
func TestCardDAVRoundTrip_FixtureThroughServer_SemanticEquality(t *testing.T) {
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	db, cfg, user := setupCardDAVLifecycle(t)
	fake := newFakeCardDAVServer(t, "/addressbooks/test/contacts/")
	defer fake.Close()

	// The manifest's second user hosts the no-server baseline contacts, so
	// their VCardUIDs cannot collide with the synced contacts' (the partial
	// unique index idx_contacts_vcard_uid_user is per-user).
	baselineUser := models.User{Username: "baseline-fixture-user", Password: "password123!A", Email: "baseline-fixture@example.com"}
	require.NoError(t, db.Create(&baselineUser).Error)

	orig := make(map[string]*contactmodel.Record, len(m.Contacts))
	uidByEntry := make(map[string]string, len(m.Contacts))
	for _, entry := range m.Contacts {
		if entry.SoftDeleted {
			continue // gina is tombstoned in the fixture; no live card to round-trip
		}
		rec := entry.Record()
		if entry.RecreatesVCardUIDOf != "" {
			// julie re-uses a soft-deleted contact's UID (fixture contract);
			// resolve it the same way the round-trip driver does.
			for _, other := range m.Contacts {
				if other.Name == entry.RecreatesVCardUIDOf {
					rec.Card.UID = other.Card.UID
				}
			}
		}
		require.NotEmpty(t, rec.Card.UID, "%s must carry a UID to be a valid vCard", entry.Name)

		raw, _, exportErr := vcard4.Adapter{}.Export(rec)
		require.NoError(t, exportErr, "export %s (same adapter the CardDAV server serves)", entry.Name)

		orig[entry.Name] = rec
		uidByEntry[entry.Name] = rec.Card.UID
		fake.seedCard("/addressbooks/test/contacts/"+rec.Card.UID+".vcf", string(raw))
	}

	sub := newFakeSubscription(t, db, cfg, user.ID, fake.bookURL())
	stats := syncFake(t, db, cfg, sub)
	require.Equal(t, len(orig), stats.Created, "every live fixture contact must be pulled in")

	for name, original := range orig {
		original := original
		t.Run(name, func(t *testing.T) {
			uid := uidByEntry[name]

			var link models.ContactSyncLink
			require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, "/addressbooks/test/contacts/"+uid+".vcf").First(&link).Error)
			var contact models.Contact
			require.NoError(t, db.First(&contact, link.ContactID).Error)

			pulled := models.RecordForContact(&contact, cfg.ProfilePhotoDir, db)
			serverReport := semanticequal.Compare(original, pulled)

			// Baseline: the same import->store->read-back pipeline without the
			// server in the middle (parse the fixture record's own export, put
			// it through ApplyRecordToContact + RecordForContact under a
			// different user so the VCardUID cannot collide).
			baseline := fixtureBaselineRecord(t, db, baselineUser.ID, original, cfg.ProfilePhotoDir)
			baselineReport := semanticequal.Compare(original, baseline)

			baselineConcepts := differenceConcepts(baselineReport)
			for _, d := range serverReport.Differences {
				_, ok := baselineConcepts[d.Concept]
				assert.True(t, ok,
					"concept %q diverged ONLY in the server round trip (not in the format round trip) — a real re-serialize/re-parse bug:\n%s",
					d.Concept, serverReport.DiffText())
			}
			if len(baselineReport.Differences) == 0 {
				assert.True(t, serverReport.Equal(),
					"a contact the format round-trips cleanly must also survive the server round trip:\n%s",
					serverReport.DiffText())
			}
		})
	}
}

// fixtureBaselineRecord runs the same record->vCard->import->ApplyRecordToContact
// ->RecordForContact pipeline the sync path uses, but without any server: it is
// the "what the format itself can and cannot preserve" reference the server
// round trip is compared against.
func fixtureBaselineRecord(t *testing.T, db *gorm.DB, userID uint, rec *contactmodel.Record, photoDir string) *contactmodel.Record {
	t.Helper()

	raw, _, err := vcard4.Adapter{}.Export(rec)
	require.NoError(t, err)

	var adapter contactmodel.Importer = vcard4.Adapter{}
	record, _, err := adapter.Import(raw)
	require.NoError(t, err)

	contact := models.Contact{UserID: userID}
	models.ApplyRecordToContact(&contact, record, photoDir)
	require.NoError(t, db.Create(&contact).Error)
	return models.RecordForContact(&contact, photoDir, db)
}

// differenceConcepts returns the set of concept_ids a report flags.
func differenceConcepts(report semanticequal.Report) map[string]bool {
	out := make(map[string]bool, len(report.Differences))
	for _, d := range report.Differences {
		out[d.Concept] = true
	}
	return out
}

// TestCardDAVRoundTrip_FixtureStructure guards the round-trip suite's own
// preconditions: the fixture must keep exactly the soft-deleted/recreate
// structure the loader depends on (issue #430 contract), or the "no worse than
// baseline" comparison above would be comparing the wrong shape.
func TestCardDAVRoundTrip_FixtureStructure(t *testing.T) {
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	var softDeleted, recreates int
	for _, entry := range m.Contacts {
		if entry.SoftDeleted {
			softDeleted++
		}
		if entry.RecreatesVCardUIDOf != "" {
			recreates++
		}
	}
	assert.Equal(t, 1, softDeleted, "exactly one fixture contact is tombstoned (issue #430)")
	assert.Equal(t, 1, recreates, "exactly one fixture contact re-uses a tombstoned UID (issue #430)")
	assert.GreaterOrEqual(t, len(m.Contacts), 15, "the fixture must keep exercising the pathological shapes")
}
