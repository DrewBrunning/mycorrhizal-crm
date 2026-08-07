package services

import (
	"context"
	"fmt"
	"io"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// T13 two-way calendar sync (docs/fork-plan/tickets/36-T13-two-way-calendar.md)
//
// Conflict policy under test (documented in pushLocalEdits' doc comment):
// local-wins on both-changed, no automatic deletions in either direction.
// The mock server below serves events with ETags via REPORT and records PUTs
// (body + If-Match), so every case is exercised against a real HTTP client.
// ---------------------------------------------------------------------------

// mockCalendarServer is a minimal CalDAV collection under test: serves one
// event with an ETag on REPORT, and records PUTs. putStatus can force a 412
// on the first PUT to exercise the both-changed retry path.
type mockCalendarServer struct {
	server    *httptest.Server
	mu        sync.Mutex
	event     string // the current remote VEVENT (inside a VCALENDAR)
	etag      string
	puts      []mockPut
	putStatus func(ifMatch string) int // nil => 200
}

type mockPut struct {
	body    string
	ifMatch string
}

func newMockCalendarServer(event, etag string) *mockCalendarServer {
	m := &mockCalendarServer{event: event, etag: etag}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			ifMatch := r.Header.Get("If-Match")
			status := http.StatusOK
			if m.putStatus != nil {
				status = m.putStatus(ifMatch)
			}
			m.puts = append(m.puts, mockPut{body: string(body), ifMatch: ifMatch})
			w.Header().Set("ETag", `"etag-after-put"`)
			w.WriteHeader(status)
			return
		}
		// REPORT (calendar-query). The getetag must be quoted — go-webdav's
		// client unquotes it, and an unquoted value fails to parse. An empty
		// event means an empty collection (the remote event vanished).
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		if m.event == "" {
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>`+"\n"+
				`<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+"\n"+
				`</d:multistatus>`)
			return
		}
		escaped := strings.ReplaceAll(strings.ReplaceAll(m.event, "&", "&amp;"), "<", "&lt;")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>`+"\n"+
			`<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+"\n"+
			`<d:response><d:href>/calendars/test/event1.ics</d:href>`+"\n"+
			`<d:propstat><d:prop><c:calendar-data>%s</c:calendar-data><d:getetag>"%s"</d:getetag></d:prop>`+"\n"+
			`<d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`+"\n"+
			`</d:multistatus>`, escaped, m.etag)
	}))
	return m
}

func (m *mockCalendarServer) Close() { m.server.Close() }

func (m *mockCalendarServer) putCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.puts)
}

func (m *mockCalendarServer) lastPut() *mockPut {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.puts) == 0 {
		return nil
	}
	return &m.puts[len(m.puts)-1]
}

func twoWayConfig() config.Config {
	cfg := calendarTestConfig()
	cfg.CalDAVTwoWayEnabled = true
	return cfg
}

func vcalendarWith(vevent string) string {
	return "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\n" + vevent + "\nEND:VCALENDAR"
}

func importedVEvent(summary string) string {
	return "BEGIN:VEVENT\nUID:event-1\nSUMMARY:" + summary + "\nDTSTART:" + icalDate(2) + "\nEND:VEVENT"
}

// loadLinkAndActivity fetches the subscription's single link + its activity.
func loadLinkAndActivity(t *testing.T, db *gorm.DB, sub *models.CalendarSubscription) (models.CalendarEventLink, models.Activity) {
	t.Helper()
	var link models.CalendarEventLink
	require.NoError(t, db.Where("subscription_id = ?", sub.ID).First(&link).Error)
	var activity models.Activity
	require.NoError(t, db.First(&activity, link.ActivityID).Error)
	return link, activity
}

func TestTwoWay_LocalOnlyChangePushesWithIfMatch(t *testing.T) {
	db := setupCalendarSyncTestDB(t)
	cfg := twoWayConfig()
	user := createCalendarTestUser(t, db)

	mock := newMockCalendarServer(vcalendarWith(importedVEvent("Original")), "etag-remote-1")
	defer mock.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, mock.server.URL+"/calendars/test/", "", "")
	service := NewCalendarSyncService(false)

	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Created: 1}, stats)

	link, activity := loadLinkAndActivity(t, db, sub)
	assert.Equal(t, "etag-remote-1", link.RemoteETag, "the remote ETag must be captured on import")
	assert.Equal(t, "/calendars/test/event1.ics", link.RemotePath, "the remote path must be captured on import")

	// Local edit: user renames the activity. Remote stays unchanged.
	time.Sleep(1100 * time.Millisecond)
	activity.Title = "Local rename"
	require.NoError(t, db.Save(&activity).Error)

	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Skipped: 1, Updated: 1}, stats, "import skips the unchanged remote, push updates the local edit")

	put := mock.lastPut()
	require.NotNil(t, put, "a local edit must be pushed back out")
	assert.Equal(t, `"etag-remote-1"`, put.ifMatch, "the push must carry the stored ETag as If-Match")
	assert.Contains(t, put.body, "Local rename")
	assert.Contains(t, put.body, "UID:event-1", "the push must reuse the remote event's UID so it updates in place")

	// No sync loop: a subsequent sync sees the pushed content as in sync and
	// must not push again.
	pushesBefore := mock.putCount()
	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, pushesBefore, mock.putCount(), "an unchanged state must not be pushed again")
	_ = stats
}

func TestTwoWay_RemoteOnlyChangePullsWithoutPush(t *testing.T) {
	db := setupCalendarSyncTestDB(t)
	cfg := twoWayConfig()
	user := createCalendarTestUser(t, db)

	mock := newMockCalendarServer(vcalendarWith(importedVEvent("Original")), "etag-remote-1")
	defer mock.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, mock.server.URL+"/calendars/test/", "", "")
	service := NewCalendarSyncService(false)

	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)

	// Remote changes; local stays untouched.
	mock.mu.Lock()
	mock.event = vcalendarWith(importedVEvent("Remote change"))
	mock.etag = "etag-remote-2"
	mock.mu.Unlock()

	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Updated: 1}, stats)

	_, activity := loadLinkAndActivity(t, db, sub)
	assert.Equal(t, "Remote change", activity.Title, "a remote-only change pulls into the activity")
	assert.Equal(t, 0, mock.putCount(), "a remote-only change must not push anything")
}

func TestTwoWay_BothChanged_LocalWins(t *testing.T) {
	db := setupCalendarSyncTestDB(t)
	cfg := twoWayConfig()
	user := createCalendarTestUser(t, db)

	mock := newMockCalendarServer(vcalendarWith(importedVEvent("Original")), "etag-remote-1")
	defer mock.Close()

	// The remote moved on after our last fetch: the If-Match PUT gets a 412,
	// and the local-wins policy must retry without the precondition.
	mock.putStatus = func(ifMatch string) int {
		if ifMatch != "" {
			return http.StatusPreconditionFailed
		}
		return http.StatusOK
	}

	sub := newTestSubscription(t, db, cfg, user.ID, mock.server.URL+"/calendars/test/", "", "")
	service := NewCalendarSyncService(false)

	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)

	// Local edit AND remote edit before the next sync.
	_, activity := loadLinkAndActivity(t, db, sub)
	time.Sleep(1100 * time.Millisecond)
	activity.Title = "Local edit wins"
	require.NoError(t, db.Save(&activity).Error)

	mock.mu.Lock()
	mock.event = vcalendarWith(importedVEvent("Remote edit"))
	mock.etag = "etag-remote-2"
	mock.mu.Unlock()

	_, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)

	assert.Equal(t, 2, mock.putCount(), "both-changed must push (first with stale If-Match -> 412, then retried)")
	first := mock.puts[0]
	assert.Equal(t, `"etag-remote-1"`, first.ifMatch, "the first PUT carries the stale ETag")
	assert.Equal(t, "", mock.puts[1].ifMatch, "the 412 retry drops the precondition (local-wins)")

	_, activity = loadLinkAndActivity(t, db, sub)
	assert.Equal(t, "Local edit wins", activity.Title, "the local edit must survive a both-changed sync (local-wins)")
}

func TestTwoWay_LocalDeleteIsRespectedAndNotPushed(t *testing.T) {
	db := setupCalendarSyncTestDB(t)
	cfg := twoWayConfig()
	user := createCalendarTestUser(t, db)

	mock := newMockCalendarServer(vcalendarWith(importedVEvent("Original")), "etag-remote-1")
	defer mock.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, mock.server.URL+"/calendars/test/", "", "")
	service := NewCalendarSyncService(false)

	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)

	// User deletes the imported activity.
	_, _ = loadLinkAndActivity(t, db, sub)
	require.NoError(t, db.Where("title = ?", "Original").Delete(&models.Activity{}).Error)

	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Skipped: 2}, stats, "the deleted activity is not re-imported and nothing is pushed")
	assert.Equal(t, 0, mock.putCount(), "a local deletion must never be pushed to the remote calendar")
}

func TestTwoWay_RemoteDeleteDoesNotDeleteLocalActivity(t *testing.T) {
	db := setupCalendarSyncTestDB(t)
	cfg := twoWayConfig()
	user := createCalendarTestUser(t, db)

	mock := newMockCalendarServer(vcalendarWith(importedVEvent("Original")), "etag-remote-1")
	defer mock.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, mock.server.URL+"/calendars/test/", "", "")
	service := NewCalendarSyncService(false)

	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)

	// The event disappears from the remote (could be deleted — or just moved
	// out of the rolling window). The local activity must survive untouched.
	mock.mu.Lock()
	mock.event = ""
	mock.mu.Unlock()

	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{}, stats)

	_, activity := loadLinkAndActivity(t, db, sub)
	assert.Equal(t, "Original", activity.Title, "an absent remote event must not delete the local activity")
	assert.False(t, activity.DeletedAt.Valid)
}

func TestTwoWay_DisabledConfigNeverPushes(t *testing.T) {
	db := setupCalendarSyncTestDB(t)
	cfg := calendarTestConfig() // CalDAVTwoWayEnabled = false
	user := createCalendarTestUser(t, db)

	mock := newMockCalendarServer(vcalendarWith(importedVEvent("Original")), "etag-remote-1")
	defer mock.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, mock.server.URL+"/calendars/test/", "", "")
	service := NewCalendarSyncService(false)

	_, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)

	_, activity := loadLinkAndActivity(t, db, sub)
	time.Sleep(1100 * time.Millisecond)
	activity.Title = "Edited"
	require.NoError(t, db.Save(&activity).Error)

	_, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, 0, mock.putCount(), "two-way is opt-in; default config must never write to the remote")
}
