package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/clock"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCSVSessionAt(m *ImportSessionManager, f *clock.Fake, user uint) string {
	f.Advance(time.Second)
	return m.CreateCSVSession(user, []string{"a"}, [][]string{{"x"}})
}

func TestImportSessionManager_Cancel(t *testing.T) {
	f := clock.NewFake(time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	defer SetClock(f)()
	m := NewImportSessionManager()
	mine := newCSVSessionAt(m, f, 1)
	theirs := newCSVSessionAt(m, f, 2)

	assert.False(t, m.Cancel(2, mine), "another user's cancel removes nothing")
	assert.Equal(t, 1, m.CountActive(1))

	assert.True(t, m.Cancel(1, mine))
	assert.Equal(t, 0, m.CountActive(1))
	assert.False(t, m.Cancel(1, mine), "idempotent: the second cancel is a no-op")
	assert.False(t, m.Cancel(1, "unknown"))
	assert.False(t, m.Cancel(1, ""))
	assert.Equal(t, 1, m.CountActive(2), "the other user's session is untouched")
	_ = theirs
}

func TestImportSessionManager_CancelLeavesConfirmedTombstone(t *testing.T) {
	f := clock.NewFake(time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	defer SetClock(f)()
	m := NewImportSessionManager()
	sid := newCSVSessionAt(m, f, 1)
	m.mu.Lock()
	delete(m.sessions, sid)
	m.mu.Unlock()
	m.rememberConfirmed(sid, 1, importResultForTest())

	assert.False(t, m.Cancel(1, sid), "a consumed session has nothing live to cancel")
	_, ok := m.replayConfirmed(sid, 1)
	assert.True(t, ok, "cancelling must not break the idempotent confirm replay")
}

func TestImportSessionManager_MakeRoomForSession(t *testing.T) {
	f := clock.NewFake(time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	defer SetClock(f)()
	m := NewImportSessionManager()

	var mine []string
	for i := 0; i < MaxImportSessionsPerUser; i++ {
		mine = append(mine, newCSVSessionAt(m, f, 1))
	}
	other := newCSVSessionAt(m, f, 2)

	assert.Equal(t, 1, m.MakeRoomForSession(1), "at the cap, exactly one (the oldest) is evicted")
	assert.Equal(t, MaxImportSessionsPerUser-1, m.CountActive(1))
	_, err := m.get(mine[0], 1)
	require.NotNil(t, err, "the oldest session is gone")
	_, err = m.get(mine[1], 1)
	assert.Nil(t, err, "the second oldest survives")
	_, err = m.get(other, 2)
	assert.Nil(t, err, "another user's session is never evicted")

	assert.Equal(t, 0, m.MakeRoomForSession(1), "below the cap nothing is evicted")
	assert.Equal(t, 0, m.MakeRoomForSession(3), "a user with no sessions needs no room")
}

func TestImportSessionManager_MakeRoomDropsExpiredFirstAndCountsOnlyLive(t *testing.T) {
	f := clock.NewFake(time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	defer SetClock(f)()
	m := NewImportSessionManager()
	var all []string
	for i := 0; i < MaxImportSessionsPerUser; i++ {
		all = append(all, newCSVSessionAt(m, f, 1))
	}
	f.Advance(sessionExpiry + time.Minute) // every session above is now expired
	fresh := newCSVSessionAt(m, f, 1)

	assert.Equal(t, 0, m.MakeRoomForSession(1), "expired sessions are swept, not counted as evictions")
	m.mu.RLock()
	_, expiredStillThere := m.sessions[all[0]]
	m.mu.RUnlock()
	assert.False(t, expiredStillThere, "expired sessions are released immediately")
	_, err := m.get(fresh, 1)
	assert.Nil(t, err, "the live session is kept")
}

func TestImportSessionManager_MakeRoomSkipsASessionBeingConfirmed(t *testing.T) {
	f := clock.NewFake(time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC))
	defer SetClock(f)()
	m := NewImportSessionManager()
	var ids []string
	for i := 0; i < MaxImportSessionsPerUser; i++ {
		ids = append(ids, newCSVSessionAt(m, f, 1))
	}
	oldest := m.sessions[ids[0]]
	oldest.confirmMu.Lock() // a confirm is running against the oldest session
	defer oldest.confirmMu.Unlock()

	assert.Equal(t, 1, m.MakeRoomForSession(1))
	_, err := m.get(ids[0], 1)
	assert.Nil(t, err, "a session mid-confirm is never evicted")
	_, err = m.get(ids[1], 1)
	assert.NotNil(t, err, "the next oldest is evicted instead")
}

func importResultForTest() models.ImportResult {
	return models.ImportResult{Created: 1, Errors: []string{}, Issues: []models.ImportRowIssue{}}
}
