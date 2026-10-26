package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	truncationProbeContacts = 2001
	truncationProbeEvents   = 501
)

func syncMultistatusResponse(entries map[string]string, token string) string {
	response := addressMultistatusResponse(entries)
	if token == "" {
		return response
	}
	return strings.Replace(response, "</d:multistatus>", "<d:sync-token>"+token+"</d:sync-token></d:multistatus>", 1)
}

func generatedContactEntries(t *testing.T, n int) (map[string]string, []carddav.AddressObject) {
	t.Helper()
	entries := make(map[string]string, n)
	objects := make([]carddav.AddressObject, 0, n)
	for i := 0; i < n; i++ {
		href := fmt.Sprintf("/addressbooks/test/contact-%04d.vcf", i)
		uid := fmt.Sprintf("truncation-%04d", i)
		card := testCard(t, uid, "First", fmt.Sprintf("Last%04d", i), fmt.Sprintf("person%04d@example.com", i))
		entries[href] = fmt.Sprintf(testVCard4Template, uid, "First", fmt.Sprintf("Last%04d", i), fmt.Sprintf("Last%04d", i), "First", fmt.Sprintf("person%04d@example.com", i))
		objects = append(objects, carddav.AddressObject{Path: href, ETag: "\"" + uid + "-etag\"", Card: card})
	}
	return entries, objects
}

func TestSyncSubscriptionImportsAllIncrementalChangesPastOldLimit(t *testing.T) {
	logtest.AllowWarnings(t, "the buggy path truncates and logs a warning; the fix imports every change")
	entries, _ := generatedContactEntries(t, truncationProbeContacts)

	var syncCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		switch {
		case strings.Contains(string(body), "sync-collection"):
			call := atomic.AddInt32(&syncCalls, 1)
			w.WriteHeader(http.StatusMultiStatus)
			if call == 1 {
				fmt.Fprint(w, syncMultistatusResponse(entries, "token-after-first-page"))
				return
			}
			fmt.Fprint(w, syncMultistatusResponse(nil, "token-after-first-page"))
		case strings.Contains(string(body), "addressbook-multiget"):
			w.WriteHeader(http.StatusMultiStatus)
			fmt.Fprint(w, addressMultistatusResponse(entries))
		default:
			w.WriteHeader(http.StatusMultiStatus)
			fmt.Fprint(w, addressMultistatusResponse(entries))
		}
	}))
	defer server.Close()

	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, server.URL+"/addressbooks/test/", "", "")
	service := NewContactSyncService(false)

	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, truncationProbeContacts, stats.Created,
		"every object in the first sync-collection page must be imported, not truncated")

	var contacts int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contacts).Error)
	assert.EqualValues(t, truncationProbeContacts, contacts)

	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, "token-after-first-page", sub.SyncToken)

	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{}, stats)

	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contacts).Error)
	assert.EqualValues(t, truncationProbeContacts, contacts,
		"the object past the old per-sync limit must not be skipped by the advanced token")
}

func TestSyncSubscriptionFullRefetchDoesNotArchivePastOldLimit(t *testing.T) {
	logtest.AllowWarnings(t, "the buggy path truncates and logs a warning; the fix reconciles every object")
	entries, objects := generatedContactEntries(t, truncationProbeContacts)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "sync-collection") {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, addressMultistatusResponse(entries))
	}))
	defer server.Close()

	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, server.URL+"/addressbooks/test/", "", "")

	_, err := reconcileContactSync(db, sub, objects, nil, false, "")
	require.NoError(t, err)

	service := NewContactSyncService(false)
	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Zero(t, stats.Archived,
		"a full refetch must never archive links merely because the response exceeded the old limit")

	var links int64
	require.NoError(t, db.Model(&models.ContactSyncLink{}).Where("subscription_id = ?", sub.ID).Count(&links).Error)
	assert.EqualValues(t, truncationProbeContacts, links)

	var archived int64
	require.NoError(t, db.Model(&models.Contact{}).
		Where("user_id = ? AND archived = ?", user.ID, true).Count(&archived).Error)
	assert.Zero(t, archived)
}

func TestCalendarSyncImportsAllEventsPastOldLimit(t *testing.T) {
	logtest.AllowWarnings(t, "the buggy path truncates and logs a warning; the fix imports every event")
	db := setupCalendarSyncTestDB(t)
	cfg := calendarTestConfig()
	user := createCalendarTestUser(t, db)

	calendars := make([]string, 0, truncationProbeEvents)
	for i := 0; i < truncationProbeEvents; i++ {
		calendars = append(calendars, fmt.Sprintf(
			"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:event-%04d\nSUMMARY:Event %04d\nDTSTART:%s\nEND:VEVENT\nEND:VCALENDAR",
			i, i, icalDate(1),
		))
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, multistatusResponse(calendars...))
	}))
	defer server.Close()

	sub := newTestSubscription(t, db, cfg, user.ID, server.URL+"/calendars/test/", "", "")
	stats, err := NewCalendarSyncService(false).SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, CalendarSyncStats{Created: truncationProbeEvents}, stats,
		"every fetched event must be imported, not silently dropped past the old limit")

	var activities int64
	require.NoError(t, db.Model(&models.Activity{}).Where("user_id = ?", user.ID).Count(&activities).Error)
	assert.EqualValues(t, truncationProbeEvents, activities)
}
