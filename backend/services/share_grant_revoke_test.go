package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type grantRevokeFixture struct {
	db     *gorm.DB
	owner  models.User
	other  models.User
	rcpt   [3]models.User
	nowRef time.Time
}

func newGrantRevokeFixture(t *testing.T) grantRevokeFixture {
	t.Helper()
	db := dbtest.New(t)
	mk := func(name string) models.User {
		u := models.User{Username: name, Email: name + "@example.com", Password: "x"}
		require.NoError(t, db.Create(&u).Error)
		return u
	}
	f := grantRevokeFixture{db: db, owner: mk("grant-owner"), other: mk("grant-other"), nowRef: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	for i := range f.rcpt {
		f.rcpt[i] = mk("grant-rcpt-" + string(rune('a'+i)))
	}
	return f
}

func (f grantRevokeFixture) grant(t *testing.T, from, to uint, etype, eid, status string) models.ShareGrant {
	t.Helper()
	g := models.ShareGrant{FromUserID: from, ToUserID: to, EntityType: etype, EntityID: eid, Status: status}
	require.NoError(t, f.db.Create(&g).Error)
	return g
}

func (f grantRevokeFixture) reload(t *testing.T, id string) models.ShareGrant {
	t.Helper()
	var g models.ShareGrant
	require.NoError(t, f.db.First(&g, "id = ?", id).Error)
	return g
}

func TestRevokeShareGrantsForEntity_RevokesOnlyLiveGrantsOfThatOwnersEntity(t *testing.T) {
	f := newGrantRevokeFixture(t)
	const note = models.ShareGrantEntityNote

	pending := f.grant(t, f.owner.ID, f.rcpt[0].ID, note, "n-1", models.ShareGrantStatusPending)
	active := f.grant(t, f.owner.ID, f.rcpt[1].ID, note, "n-1", models.ShareGrantStatusActive)
	blocked := f.grant(t, f.owner.ID, f.rcpt[2].ID, note, "n-1", models.ShareGrantStatusBlocked)
	// History rows for the same entity must stay exactly as they are.
	declined := f.grant(t, f.owner.ID, f.rcpt[0].ID, note, "n-1", models.ShareGrantStatusDeclined)
	expired := f.grant(t, f.owner.ID, f.rcpt[1].ID, note, "n-1", models.ShareGrantStatusExpired)
	// Same owner, different entity / type.
	otherEntity := f.grant(t, f.owner.ID, f.rcpt[0].ID, note, "n-2", models.ShareGrantStatusActive)
	otherType := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityActivity, "n-1", models.ShareGrantStatusActive)
	// A different owner holding the same entity_id string (trap #5).
	foreign := f.grant(t, f.other.ID, f.rcpt[0].ID, note, "n-1", models.ShareGrantStatusActive)

	n, err := RevokeShareGrantsForEntity(f.db, f.owner.ID, note, "n-1", f.nowRef)
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)

	for _, g := range []models.ShareGrant{pending, active, blocked} {
		got := f.reload(t, g.ID)
		assert.Equal(t, models.ShareGrantStatusRevoked, got.Status, g.Status)
		require.NotNil(t, got.RevokedReason)
		assert.Equal(t, models.ShareGrantReasonEntityDeleted, *got.RevokedReason)
		require.NotNil(t, got.RevokedAt)
		assert.True(t, got.RevokedAt.Equal(f.nowRef))
		assert.True(t, got.UpdatedAt.Equal(f.nowRef))
		assert.EqualValues(t, 2, got.Revision, "revision advances so the change is observable")
	}
	for _, g := range []models.ShareGrant{declined, expired, otherEntity, otherType, foreign} {
		got := f.reload(t, g.ID)
		assert.Equal(t, g.Status, got.Status, "untouched")
		assert.Nil(t, got.RevokedReason)
		assert.Nil(t, got.RevokedAt)
		assert.EqualValues(t, 1, got.Revision)
	}

	// Idempotent: nothing live is left, so a second call changes nothing.
	n, err = RevokeShareGrantsForEntity(f.db, f.owner.ID, note, "n-1", f.nowRef.Add(time.Hour))
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestRevokeShareGrantsForEntity_PropagatesDatabaseError(t *testing.T) {
	f := newGrantRevokeFixture(t)
	require.NoError(t, f.db.Exec("DROP TABLE share_grants").Error)
	_, err := RevokeShareGrantsForEntity(f.db, f.owner.ID, models.ShareGrantEntityNote, "n-1", f.nowRef)
	assert.Error(t, err)
}

// The contact cascade step covers single delete, bulk delete and the merge
// loser, because all three walk ContactCascadeRegistry.
func TestContactCascade_RevokesContactGrantsWithEntityDeleted(t *testing.T) {
	f := newGrantRevokeFixture(t)
	contact := models.Contact{UserID: f.owner.ID, Firstname: "Del", Lastname: "Me"}
	require.NoError(t, f.db.Create(&contact).Error)
	keep := models.Contact{UserID: f.owner.ID, Firstname: "Keep", Lastname: "Me"}
	require.NoError(t, f.db.Create(&keep).Error)

	gone := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityContact, contact.VCardUID, models.ShareGrantStatusActive)
	goneBlocked := f.grant(t, f.owner.ID, f.rcpt[1].ID, models.ShareGrantEntityContact, contact.VCardUID, models.ShareGrantStatusBlocked)
	kept := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityContact, keep.VCardUID, models.ShareGrantStatusActive)
	circleSameID := f.grant(t, f.owner.ID, f.rcpt[0].ID, models.ShareGrantEntityCircle, contact.VCardUID, models.ShareGrantStatusActive)

	require.NoError(t, DeleteContact(f.db, contact, f.owner.ID, f.nowRef))

	for _, g := range []models.ShareGrant{gone, goneBlocked} {
		got := f.reload(t, g.ID)
		assert.Equal(t, models.ShareGrantStatusRevoked, got.Status)
		require.NotNil(t, got.RevokedReason)
		assert.Equal(t, models.ShareGrantReasonEntityDeleted, *got.RevokedReason)
	}
	assert.Equal(t, models.ShareGrantStatusActive, f.reload(t, kept.ID).Status, "another contact's grant is untouched")
	assert.Equal(t, models.ShareGrantStatusActive, f.reload(t, circleSameID.ID).Status, "only contact-type grants are matched")
}

func TestContactCascadeRegistry_RegistersShareGrantsRevokeStep(t *testing.T) {
	var found *CascadeStep
	for _, s := range ContactCascadeRegistry() {
		if s.Table == "share_grants" {
			s := s
			found = &s
		}
	}
	require.NotNil(t, found, "contact delete must revoke standing-share grants (ADR 0042 decision 10)")
	assert.Equal(t, CascadeMutate, found.Mode, "revoked, not deleted: the row is history")
	assert.NotNil(t, found.Apply)
	assert.NotEmpty(t, found.Reason)
}
