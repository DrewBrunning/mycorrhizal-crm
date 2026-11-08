package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2291, ADR 0042 decisions 10-11: lifecycle job, per-party audit trail,
// recipient notices and the pending count.

type lcWorld struct {
	db          *gorm.DB
	owner, rcpt models.User
	contact     models.Contact
	fake        *clock.Fake
	cfg         config.Config
}

func newLCWorld(t *testing.T) *lcWorld {
	t.Helper()
	db := dbtest.New(t)
	start := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	w := &lcWorld{db: db, fake: clock.NewFake(start), cfg: config.Config{ShareGrantRetentionDays: 30}}
	t.Cleanup(SetClock(w.fake))
	mk := func(name string) models.User {
		u := models.User{Username: name, Email: name + "@example.com", Password: "x"}
		require.NoError(t, db.Create(&u).Error)
		return u
	}
	w.owner, w.rcpt = mk("lc-owner"), mk("lc-rcpt")
	w.contact = models.Contact{UserID: w.owner.ID, Firstname: "Zebulon", Lastname: "Quillfeather"}
	require.NoError(t, db.Create(&w.contact).Error)
	return w
}

func (w *lcWorld) grant(t *testing.T, status string, mut func(g *models.ShareGrant)) models.ShareGrant {
	t.Helper()
	g := models.ShareGrant{FromUserID: w.owner.ID, ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote,
		EntityID: "note-" + t.Name() + "-" + status + time.Now().Format("150405.000000000"), Sections: []string{}, Status: status,
		CreatedAt: w.fake.Now(), UpdatedAt: w.fake.Now()}
	if mut != nil {
		mut(&g)
	}
	require.NoError(t, w.db.Create(&g).Error)
	return g
}

func (w *lcWorld) status(t *testing.T, id string) string {
	t.Helper()
	var g models.ShareGrant
	require.NoError(t, w.db.First(&g, "id = ?", id).Error)
	return g.Status
}

func (w *lcWorld) exists(id string) bool {
	var n int64
	w.db.Model(&models.ShareGrant{}).Where("id = ?", id).Count(&n)
	return n == 1
}

func (w *lcWorld) audits(t *testing.T, userID uint, op string) []models.AuditEvent {
	t.Helper()
	var ev []models.AuditEvent
	require.NoError(t, w.db.Where("entity_type = ? AND user_id = ? AND operation = ?", models.AuditEntityShareGrant, userID, op).Order("id").Find(&ev).Error)
	return ev
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestShareGrantLifecycleJob_ExpiresPastExpiresAtAndPendingTTL(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	past := now.Add(-time.Hour)

	activeExpired := w.grant(t, models.ShareGrantStatusActive, func(g *models.ShareGrant) { g.ExpiresAt = &past })
	pendingExpired := w.grant(t, models.ShareGrantStatusPending, func(g *models.ShareGrant) { g.ExpiresAt = &past })
	activeFuture := w.grant(t, models.ShareGrantStatusActive, func(g *models.ShareGrant) { g.ExpiresAt = ptrTime(now.Add(time.Hour)) })
	activeNever := w.grant(t, models.ShareGrantStatusActive, nil)
	pendingOld := w.grant(t, models.ShareGrantStatusPending, func(g *models.ShareGrant) { g.CreatedAt = now.Add(-ShareGrantPendingTTL - time.Minute) })
	pendingFresh := w.grant(t, models.ShareGrantStatusPending, func(g *models.ShareGrant) { g.CreatedAt = now.Add(-ShareGrantPendingTTL + time.Hour) })
	blockedOld := w.grant(t, models.ShareGrantStatusBlocked, func(g *models.ShareGrant) { g.CreatedAt = now.Add(-ShareGrantPendingTTL - time.Minute) })
	declined := w.grant(t, models.ShareGrantStatusDeclined, func(g *models.ShareGrant) { g.ExpiresAt = &past })

	res, err := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)
	assert.Equal(t, int64(4), res.Expired, "active+pending past expires_at, pending and blocked past the TTL")

	for _, g := range []models.ShareGrant{activeExpired, pendingExpired, pendingOld, blockedOld} {
		assert.Equal(t, models.ShareGrantStatusExpired, w.status(t, g.ID), g.ID)
	}
	for _, g := range []models.ShareGrant{activeFuture, activeNever} {
		assert.Equal(t, models.ShareGrantStatusActive, w.status(t, g.ID))
	}
	assert.Equal(t, models.ShareGrantStatusPending, w.status(t, pendingFresh.ID))
	assert.Equal(t, models.ShareGrantStatusDeclined, w.status(t, declined.ID), "terminal rows are not re-expired")

	var got models.ShareGrant
	require.NoError(t, w.db.First(&got, "id = ?", activeExpired.ID).Error)
	assert.Equal(t, int64(2), got.Revision, "revision advances (ADR 0006)")
	assert.True(t, got.UpdatedAt.Equal(now))

	// Idempotent: a second run changes nothing and writes no further audit.
	before := len(w.audits(t, w.owner.ID, models.AuditOpExpire))
	res, err = RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)
	assert.Zero(t, res.Expired)
	assert.Zero(t, res.Purged)
	assert.Equal(t, before, len(w.audits(t, w.owner.ID, models.AuditOpExpire)))
}

func TestShareGrantLifecycleJob_AuditsExpiryPerParty(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	past := now.Add(-time.Hour)
	visible := w.grant(t, models.ShareGrantStatusActive, func(g *models.ShareGrant) { g.ExpiresAt = &past })
	blocked := w.grant(t, models.ShareGrantStatusBlocked, func(g *models.ShareGrant) { g.CreatedAt = now.Add(-ShareGrantPendingTTL - time.Minute) })

	_, err := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)

	own := w.audits(t, w.owner.ID, models.AuditOpExpire)
	rec := w.audits(t, w.rcpt.ID, models.AuditOpExpire)
	ids := func(ev []models.AuditEvent) []string {
		var out []string
		for _, e := range ev {
			out = append(out, e.EntityID)
		}
		return out
	}
	assert.ElementsMatch(t, []string{visible.ID, blocked.ID}, ids(own), "the owner is told about both")
	assert.Equal(t, []string{visible.ID}, ids(rec), "the recipient never learns of a blocked grant (decision 8)")
	assert.Contains(t, own[0].BeforeSnapshot, `"status":"expired"`)
}

func TestShareGrantLifecycleJob_PurgesTerminalRowsPastRetentionOnly(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	old := now.AddDate(0, 0, -31)
	recent := now.AddDate(0, 0, -29)
	at := func(ts time.Time) func(g *models.ShareGrant) {
		return func(g *models.ShareGrant) { g.UpdatedAt = ts; g.CreatedAt = ts }
	}

	oldDeclined := w.grant(t, models.ShareGrantStatusDeclined, at(old))
	oldRevoked := w.grant(t, models.ShareGrantStatusRevoked, at(old))
	oldExpired := w.grant(t, models.ShareGrantStatusExpired, at(old))
	recentDeclined := w.grant(t, models.ShareGrantStatusDeclined, at(recent))
	// Active and pending rows are never purged by retention, however old.
	// (The pending one carries a far-future expires_at so TTL expiry does not
	// move it first; the active one has no expiry.)
	farFuture := now.AddDate(1, 0, 0)
	oldActive := w.grant(t, models.ShareGrantStatusActive, at(old))
	oldPending := w.grant(t, models.ShareGrantStatusPending, func(g *models.ShareGrant) {
		g.UpdatedAt = old
		g.CreatedAt = now.Add(-time.Hour)
		g.ExpiresAt = &farFuture
	})

	res, err := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.Purged)
	for _, g := range []models.ShareGrant{oldDeclined, oldRevoked, oldExpired} {
		assert.False(t, w.exists(g.ID), "terminal row past retention is hard-deleted: %s", g.Status)
	}
	for _, g := range []models.ShareGrant{recentDeclined, oldActive, oldPending} {
		assert.True(t, w.exists(g.ID), "must survive: %s", g.Status)
	}

	res, err = RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)
	assert.Zero(t, res.Purged, "idempotent")
}

func TestShareGrantLifecycleJob_RetentionUsesInjectedClockAndZeroDisables(t *testing.T) {
	w := newLCWorld(t)
	g := w.grant(t, models.ShareGrantStatusRevoked, nil) // updated "now"

	_, err := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)
	assert.True(t, w.exists(g.ID), "not yet past retention")

	w.fake.Advance(31 * 24 * time.Hour)
	disabled := w.cfg
	disabled.ShareGrantRetentionDays = 0
	res, err := RunShareGrantLifecycle(context.Background(), w.db, disabled)
	require.NoError(t, err)
	assert.Zero(t, res.Purged)
	assert.True(t, w.exists(g.ID), "retention 0 disables the purge rather than deleting every row")

	res, err = RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.Purged)
	assert.False(t, w.exists(g.ID))
}

func TestShareGrantLifecycleJob_PurgesOldNotices(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	mk := func(ts time.Time) models.ShareGrantNotification {
		n := models.ShareGrantNotification{UserID: w.rcpt.ID, FromUserID: w.owner.ID, Kind: models.ShareGrantNoticePending,
			EntityType: models.ShareGrantEntityNote, CreatedAt: ts, UpdatedAt: ts}
		require.NoError(t, w.db.Create(&n).Error)
		return n
	}
	old, fresh := mk(now.AddDate(0, 0, -31)), mk(now.AddDate(0, 0, -1))
	_, err := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.NoError(t, err)
	var n int64
	w.db.Model(&models.ShareGrantNotification{}).Where("id = ?", old.ID).Count(&n)
	assert.Zero(t, n)
	w.db.Model(&models.ShareGrantNotification{}).Where("id = ?", fresh.ID).Count(&n)
	assert.Equal(t, int64(1), n)
}

func TestShareGrantLifecycleScheduled_LockSkipAndReport(t *testing.T) {
	w := newLCWorld(t)
	past := w.fake.Now().Add(-time.Hour)
	w.grant(t, models.ShareGrantStatusActive, func(g *models.ShareGrant) { g.ExpiresAt = &past })
	n, err := RunShareGrantLifecycleScheduled(w.db, w.cfg)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "reports the number of rows it moved or removed")
	_, err = RunShareGrantLifecycleScheduled(w.db, w.cfg)
	assert.ErrorIs(t, err, ErrJobSkipped, "ADR 0011 catch-up window de-dups a rapid second run")
}

func TestShareGrantLifecycle_DBFailureSurfaces(t *testing.T) {
	w := newLCWorld(t)
	dbtest.HideTable(t, w.db, "share_grants")
	_, err := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
	require.Error(t, err)
}

// --- audit trail ----------------------------------------------------------

func (w *lcWorld) note(t *testing.T) models.Note {
	t.Helper()
	n := models.Note{UserID: w.owner.ID, Content: "TOPSECRET note text", Date: w.fake.Now()}
	require.NoError(t, w.db.Create(&n).Error)
	return n
}

func TestShareGrantAudit_EveryTransitionWritesTheRightTrails(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	n := w.note(t)
	create := func(entity string) *models.ShareGrant {
		g, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: entity}, now)
		require.Nil(t, e)
		return g
	}
	count := func(user uint, op string) int { return len(w.audits(t, user, op)) }

	g := create(n.UUID)
	assert.Equal(t, 1, count(w.owner.ID, models.AuditOpCreate))
	assert.Zero(t, count(w.rcpt.ID, models.AuditOpCreate), "create is the owner's alone")

	_, e := AcceptShareGrant(w.db, w.rcpt.ID, g.ID, now)
	require.Nil(t, e)
	assert.Equal(t, 1, count(w.owner.ID, models.AuditOpAccept))
	assert.Equal(t, 1, count(w.rcpt.ID, models.AuditOpAccept))
	_, e = AcceptShareGrant(w.db, w.rcpt.ID, g.ID, now) // idempotent re-accept
	require.Nil(t, e)
	assert.Equal(t, 1, count(w.rcpt.ID, models.AuditOpAccept), "an idempotent no-op writes nothing")

	_, e = UpdateShareGrant(w.db, w.owner.ID, g.ID, models.ShareGrantPatchInput{ExpiresAt: models.NullableTime{Set: true, Value: ptrTime(now.Add(time.Hour))}}, nil, now)
	require.Nil(t, e)
	assert.Equal(t, 1, count(w.owner.ID, models.AuditOpUpdate))
	assert.Zero(t, count(w.rcpt.ID, models.AuditOpUpdate))

	_, e = ReconfirmShareGrant(w.db, w.owner.ID, g.ID, now)
	require.Nil(t, e)
	assert.Equal(t, 1, count(w.owner.ID, models.AuditOpReconfirm))
	assert.Zero(t, count(w.rcpt.ID, models.AuditOpReconfirm))

	_, e = RevokeShareGrant(w.db, w.owner.ID, g.ID, now)
	require.Nil(t, e)
	assert.Equal(t, 1, count(w.owner.ID, models.AuditOpRevoke))
	assert.Equal(t, 1, count(w.rcpt.ID, models.AuditOpRevoke))
	_, e = RevokeShareGrant(w.db, w.owner.ID, g.ID, now) // idempotent
	require.Nil(t, e)
	assert.Equal(t, 1, count(w.owner.ID, models.AuditOpRevoke))

	n2 := w.note(t)
	g2 := create(n2.UUID)
	_, e = DeclineShareGrant(w.db, w.rcpt.ID, g2.ID, false, now)
	require.Nil(t, e)
	assert.Equal(t, 1, count(w.owner.ID, models.AuditOpDecline))
	assert.Equal(t, 1, count(w.rcpt.ID, models.AuditOpDecline))

	n3 := w.note(t)
	g3 := create(n3.UUID)
	_, e = AcceptShareGrant(w.db, w.rcpt.ID, g3.ID, now)
	require.Nil(t, e)
	_, e = LeaveShareGrant(w.db, w.rcpt.ID, g3.ID, now)
	require.Nil(t, e)
	assert.Equal(t, 2, count(w.owner.ID, models.AuditOpDecline), "leave is recorded as decline")
	assert.Equal(t, 2, count(w.rcpt.ID, models.AuditOpDecline))
}

func TestShareGrantAudit_BlockEndsOtherGrantsAndAuditsThem(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	a, b := w.note(t), w.note(t)
	ga, _ := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: a.UUID}, now)
	gb, _ := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: b.UUID}, now)
	_, e := AcceptShareGrant(w.db, w.rcpt.ID, gb.ID, now)
	require.Nil(t, e)

	_, e = DeclineShareGrant(w.db, w.rcpt.ID, ga.ID, true, now)
	require.Nil(t, e)
	assert.Len(t, w.audits(t, w.owner.ID, models.AuditOpDecline), 2, "the declined grant and the one the block ended")
	assert.Len(t, w.audits(t, w.rcpt.ID, models.AuditOpDecline), 2)
}

func TestShareGrantAudit_BlockedGrantNeverAppearsOnTheRecipientTrailAndOwnerSeesPending(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	require.NoError(t, w.db.Create(&models.ShareBlock{BlockerUserID: w.rcpt.ID, BlockedUserID: w.owner.ID}).Error)
	n := w.note(t)
	g, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: n.UUID}, now)
	require.Nil(t, e)
	own := w.audits(t, w.owner.ID, models.AuditOpCreate)
	require.Len(t, own, 1)
	assert.Contains(t, own[0].BeforeSnapshot, `"status":"pending"`, "decision 8: the sender's trail reads blocked as pending")
	assert.NotContains(t, own[0].BeforeSnapshot, "blocked")

	_, e = RevokeShareGrant(w.db, w.owner.ID, g.ID, now)
	require.Nil(t, e)
	assert.Len(t, w.audits(t, w.owner.ID, models.AuditOpRevoke), 1)
	assert.Empty(t, w.audits(t, w.rcpt.ID, models.AuditOpRevoke), "no recipient trail row for a grant they never saw")
	var all int64
	w.db.Model(&models.AuditEvent{}).Where("entity_type = ? AND user_id = ?", models.AuditEntityShareGrant, w.rcpt.ID).Count(&all)
	assert.Zero(t, all)
}

func TestShareGrantAudit_SnapshotsCarryGrantFieldsOnly(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	n := w.note(t)
	cg, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityContact,
		EntityID: w.contact.VCardUID, Sections: []string{"emails"}}, now)
	require.Nil(t, e)
	ng, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: n.UUID}, now)
	require.Nil(t, e)
	for _, id := range []string{cg.ID, ng.ID} {
		_, e = AcceptShareGrant(w.db, w.rcpt.ID, id, now)
		require.Nil(t, e)
		_, e = RevokeShareGrant(w.db, w.owner.ID, id, now)
		require.Nil(t, e)
	}
	var events []models.AuditEvent
	require.NoError(t, w.db.Where("entity_type = ?", models.AuditEntityShareGrant).Find(&events).Error)
	require.NotEmpty(t, events)
	for _, ev := range events {
		for _, banned := range []string{"Zebulon", "Quillfeather", "TOPSECRET", w.contact.VCardUID, n.UUID, "label", "content"} {
			assert.False(t, strings.Contains(ev.BeforeSnapshot, banned), "%s snapshot leaks %q: %s", ev.Operation, banned, ev.BeforeSnapshot)
		}
	}
}

func TestShareGrantAudit_EntityDeletionRevokeIsAuditedPerParty(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	n := w.note(t)
	g, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: n.UUID}, now)
	require.Nil(t, e)
	_, e = AcceptShareGrant(w.db, w.rcpt.ID, g.ID, now)
	require.Nil(t, e)

	require.NoError(t, w.db.Transaction(func(tx *gorm.DB) error {
		c, err := RevokeShareGrantsForEntity(tx, w.owner.ID, models.ShareGrantEntityNote, n.UUID, now)
		assert.Equal(t, int64(1), c)
		return err
	}))
	for _, u := range []uint{w.owner.ID, w.rcpt.ID} {
		ev := w.audits(t, u, models.AuditOpRevoke)
		require.Len(t, ev, 1)
		assert.Contains(t, ev[0].BeforeSnapshot, `"revoked_reason":"entity_deleted"`)
	}
}

// --- notices --------------------------------------------------------------

func TestShareGrantNotices_PendingAndRevokeNotifyRecipientOnly(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	n := w.note(t)
	g, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: n.UUID}, now)
	require.Nil(t, e)

	count, err := PendingShareGrantCount(w.db, w.rcpt.ID, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	oc, err := PendingShareGrantCount(w.db, w.owner.ID, now)
	require.NoError(t, err)
	assert.Zero(t, oc, "the count is the recipient's incoming only")

	notices, err := ListShareGrantNotices(w.db, w.rcpt.ID)
	require.NoError(t, err)
	require.Len(t, notices, 1)
	assert.Equal(t, models.ShareGrantNoticePending, notices[0].Kind)
	assert.Equal(t, "lc-owner", notices[0].SenderUsername)
	assert.Equal(t, models.ShareGrantEntityNote, notices[0].EntityType)

	_, e = AcceptShareGrant(w.db, w.rcpt.ID, g.ID, now)
	require.Nil(t, e)
	own, err := ListShareGrantNotices(w.db, w.owner.ID)
	require.NoError(t, err)
	assert.Empty(t, own, "the owner is not pushed on accept")
	count, _ = PendingShareGrantCount(w.db, w.rcpt.ID, now)
	assert.Zero(t, count, "accepted grants leave the badge")

	_, e = RevokeShareGrant(w.db, w.owner.ID, g.ID, now)
	require.Nil(t, e)
	notices, err = ListShareGrantNotices(w.db, w.rcpt.ID)
	require.NoError(t, err)
	require.Len(t, notices, 2)
	kinds := []string{notices[0].Kind, notices[1].Kind}
	assert.ElementsMatch(t, []string{models.ShareGrantNoticePending, models.ShareGrantNoticeRevoked}, kinds)

	marked, err := MarkShareGrantNoticesRead(w.db, w.rcpt.ID, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), marked)
	notices, _ = ListShareGrantNotices(w.db, w.rcpt.ID)
	assert.Empty(t, notices, "read notices drop out of the unread list")
	marked, _ = MarkShareGrantNoticesRead(w.db, w.owner.ID, now)
	assert.Zero(t, marked, "marking read is scoped to the caller")
}

func TestShareGrantNotices_BlockedAndForeignAreInvisible(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	require.NoError(t, w.db.Create(&models.ShareBlock{BlockerUserID: w.rcpt.ID, BlockedUserID: w.owner.ID}).Error)
	n := w.note(t)
	g, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: n.UUID}, now)
	require.Nil(t, e)
	count, _ := PendingShareGrantCount(w.db, w.rcpt.ID, now)
	assert.Zero(t, count, "a blocked sender's grant never raises the badge")
	notices, _ := ListShareGrantNotices(w.db, w.rcpt.ID)
	assert.Empty(t, notices)
	_, e = RevokeShareGrant(w.db, w.owner.ID, g.ID, now)
	require.Nil(t, e)
	notices, _ = ListShareGrantNotices(w.db, w.rcpt.ID)
	assert.Empty(t, notices, "revoking a never-seen grant notifies nobody")
}

func TestShareGrantNotices_PendingCountIgnoresExpired(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	w.grant(t, models.ShareGrantStatusPending, func(g *models.ShareGrant) { g.CreatedAt = now.Add(-ShareGrantPendingTTL - time.Hour) })
	w.grant(t, models.ShareGrantStatusPending, func(g *models.ShareGrant) { g.ExpiresAt = ptrTime(now.Add(-time.Minute)) })
	w.grant(t, models.ShareGrantStatusPending, nil)
	count, err := PendingShareGrantCount(w.db, w.rcpt.ID, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "read-path expiry applies before the job runs")
}

func TestShareGrantNotices_FailureNeverFailsTheTransition(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	dbtest.HideTable(t, w.db, "share_grant_notifications")
	n := w.note(t)
	g, e := CreateShareGrant(w.db, w.owner.ID, models.ShareGrantInput{ToUserID: w.rcpt.ID, EntityType: models.ShareGrantEntityNote, EntityID: n.UUID}, now)
	require.Nil(t, e, "create succeeds although the notice cannot be written")
	assert.Equal(t, models.ShareGrantStatusPending, w.status(t, g.ID))
	_, e = AcceptShareGrant(w.db, w.rcpt.ID, g.ID, now)
	require.Nil(t, e)
	_, e = RevokeShareGrant(w.db, w.owner.ID, g.ID, now)
	require.Nil(t, e, "revoke succeeds although the notice cannot be written")
	assert.Equal(t, models.ShareGrantStatusRevoked, w.status(t, g.ID))
}

// --- recipient deletion ---------------------------------------------------

func TestAuditRecipientDeletion_OwnerGetsRevokeForLiveIncomingGrants(t *testing.T) {
	w := newLCWorld(t)
	now := w.fake.Now()
	live1 := w.grant(t, models.ShareGrantStatusActive, nil)
	live2 := w.grant(t, models.ShareGrantStatusPending, nil)
	blocked := w.grant(t, models.ShareGrantStatusBlocked, nil)
	terminal := w.grant(t, models.ShareGrantStatusDeclined, nil)
	// A grant the deleted user SENT is not a recipient_deleted event.
	out := models.ShareGrant{FromUserID: w.rcpt.ID, ToUserID: w.owner.ID, EntityType: models.ShareGrantEntityNote, EntityID: "n-out", Sections: []string{}, Status: models.ShareGrantStatusActive}
	require.NoError(t, w.db.Create(&out).Error)

	require.NoError(t, w.db.Transaction(func(tx *gorm.DB) error {
		return AuditRecipientDeletion(tx, w.rcpt.ID, now)
	}))
	ev := w.audits(t, w.owner.ID, models.AuditOpRevoke)
	var ids []string
	for _, e := range ev {
		ids = append(ids, e.EntityID)
		assert.Contains(t, e.BeforeSnapshot, `"revoked_reason":"recipient_deleted"`)
		assert.Contains(t, e.BeforeSnapshot, `"status":"revoked"`)
	}
	assert.ElementsMatch(t, []string{live1.ID, live2.ID, blocked.ID}, ids)
	assert.NotContains(t, ids, terminal.ID)
	assert.NotContains(t, ids, out.ID)
	assert.Empty(t, w.audits(t, w.rcpt.ID, models.AuditOpRevoke), "nothing is written to the departing user's own trail")
	// The helper audits only; the cascade does the delete.
	assert.True(t, w.exists(live1.ID))
}

// --- pending count and notices scoping -------------------------------------

func TestListShareGrantNotices_CapsAtFifty(t *testing.T) {
	w := newLCWorld(t)
	for i := 0; i < 60; i++ {
		require.NoError(t, w.db.Create(&models.ShareGrantNotification{UserID: w.rcpt.ID, FromUserID: w.owner.ID, Kind: models.ShareGrantNoticePending, EntityType: models.ShareGrantEntityNote}).Error)
	}
	got, err := ListShareGrantNotices(w.db, w.rcpt.ID)
	require.NoError(t, err)
	assert.Len(t, got, 50)
}

// --- single-statement failures ---------------------------------------------

// Each scenario is replayed with the n-th statement failing (n = 1, 2, ...)
// until it issues fewer statements than n. A failure must never panic; a
// failure before any audit/notice bookkeeping must surface as an error; and a
// clean re-run afterwards must converge on the same end state (the job is
// idempotent, so a half-run is simply finished by the next tick).
func TestShareGrantLifecycle_SingleStatementFailures(t *testing.T) {
	seed := func(t *testing.T, w *lcWorld) {
		past := w.fake.Now().Add(-time.Hour)
		w.grant(t, models.ShareGrantStatusActive, func(g *models.ShareGrant) { g.ExpiresAt = &past })
		w.grant(t, models.ShareGrantStatusRevoked, func(g *models.ShareGrant) {
			g.UpdatedAt = w.fake.Now().AddDate(0, 0, -40)
		})
		require.NoError(t, w.db.Create(&models.ShareGrantNotification{UserID: w.rcpt.ID, FromUserID: w.owner.ID,
			Kind: models.ShareGrantNoticePending, EntityType: "note",
			CreatedAt: w.fake.Now().AddDate(0, 0, -40), UpdatedAt: w.fake.Now().AddDate(0, 0, -40)}).Error)
	}
	scenarios := map[string]func(w *lcWorld) error{
		"job": func(w *lcWorld) error {
			_, err := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
			return err
		},
		"scheduled": func(w *lcWorld) error {
			_, err := RunShareGrantLifecycleScheduled(w.db, w.cfg)
			return err
		},
		"recipient-deletion-audit": func(w *lcWorld) error {
			return AuditRecipientDeletion(w.db, w.rcpt.ID, w.fake.Now())
		},
	}
	for name, run := range scenarios {
		t.Run(name, func(t *testing.T) {
			errored := 0
			for n := 1; n < 40; n++ {
				w := newLCWorld(t)
				seed(t, w)
				inj := &injector{target: n}
				inj.install(w.db)
				err := run(w)
				if !inj.hit {
					break
				}
				if err != nil {
					errored++
				}
				inj.target = 0 // disarm
				if name != "recipient-deletion-audit" {
					_, rerr := RunShareGrantLifecycle(context.Background(), w.db, w.cfg)
					require.NoError(t, rerr, "a clean re-run finishes the half-run (n=%d)", n)
					var live, terminal int64
					w.db.Model(&models.ShareGrant{}).Where("status = ?", models.ShareGrantStatusActive).Count(&live)
					w.db.Model(&models.ShareGrant{}).Where("status = ?", models.ShareGrantStatusRevoked).Count(&terminal)
					assert.Zero(t, live, "the expired grant ends up expired (n=%d)", n)
					assert.Zero(t, terminal, "the old revoked row ends up purged (n=%d)", n)
				}
			}
			assert.Positive(t, errored, "at least the data-path statements surface their failure")
		})
	}
}
