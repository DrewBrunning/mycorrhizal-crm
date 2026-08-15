package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mycorrhizal/config"
	"mycorrhizal/httputil"
	"mycorrhizal/i18n"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"gorm.io/gorm"
)

// Sentinel errors for calendar sync failures, mapped to API errors in the controller.
var (
	ErrCalendarInvalidURL     = errors.New("calendar URL is invalid")
	ErrCalendarUnauthorized   = errors.New("calendar authentication failed")
	ErrCalendarNotFound       = errors.New("calendar not found")
	ErrCalendarUnreachable    = errors.New("calendar could not be reached")
	ErrCalendarInvalidData    = errors.New("calendar returned data that could not be parsed")
	ErrCalendarTooLarge       = errors.New("calendar response exceeds the size limit")
	ErrCalendarPrivateAddress = errors.New("calendar URL resolves to a private or loopback address")

	// errCalendarQueryUnsupported triggers the plain .ics GET fallback.
	errCalendarQueryUnsupported = errors.New("calendar server does not support CalDAV queries")
)

const (
	maxCalendarResponseBytes = 20 * 1024 * 1024
	maxEventsPerSync         = 500
	maxOccurrencesPerEvent   = 100
	calendarRequestTimeout   = 60 * time.Second
)

// CalendarSyncStats summarizes one sync run of a subscription.
type CalendarSyncStats struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type calendarEvent struct {
	UID            string
	Title          string
	Description    string
	Location       string
	Date           time.Time
	AttendeeEmails []string
	// ETag and Path are the remote calendar object's sync identity, captured
	// from the CalDAV query response (T13). Empty for events pulled via the
	// plain-ICS GET fallback, which has no collection semantics and therefore
	// no write path — such events can never be pushed back.
	ETag string
	Path string
}

func (e calendarEvent) contentHash() string {
	parts := []string{e.Title, e.Description, e.Location, e.Date.UTC().Format(time.RFC3339)}
	// Attendees are part of the hash so newly invited people get linked as
	// contacts on the next sync; sorted, since feeds don't order them stably.
	emails := append([]string(nil), e.AttendeeEmails...)
	sort.Strings(emails)
	sum := sha256.Sum256([]byte(strings.Join(append(parts, emails...), "\x1f")))
	return hex.EncodeToString(sum[:])
}

// CalendarSyncService imports events from CalDAV or plain iCalendar URLs as activities.
type CalendarSyncService struct {
	client *http.Client
}

// NewCalendarSyncService builds a sync service. When blockPrivateURLs is set,
// connections to private/loopback/link-local addresses are refused (SSRF
// protection for cloud deployments; self-hosted setups usually sync from LAN
// servers like Nextcloud, so this is opt-in via CALDAV_BLOCK_PRIVATE_URLS).
func NewCalendarSyncService(blockPrivateURLs bool) *CalendarSyncService {
	transport := &http.Transport{}
	if blockPrivateURLs {
		transport.DialContext = privateBlockingDialContext
	}
	return &CalendarSyncService{
		client: &http.Client{
			Timeout:   calendarRequestTimeout,
			Transport: &calendarRoundTripper{base: transport},
		},
	}
}

// privateBlockingDialContext refuses to connect to non-public addresses,
// pinning the resolved IP so DNS rebinding cannot redirect the dial inward.
// The address classification is shared with the photo-fetch and contact-sync
// paths (httputil.IsPublicIP) so all three enforce the same reserved ranges;
// only the sentinel errors stay calendar-specific.
func privateBlockingDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dial := httputil.SafeDialContext(
		fmt.Errorf("%w: could not resolve host", ErrCalendarUnreachable),
		ErrCalendarPrivateAddress,
	)
	return dial(ctx, network, addr)
}

// calendarRoundTripper maps HTTP auth/not-found responses to sentinel errors and
// caps the response body size, for both the CalDAV client and the GET fallback.
type calendarRoundTripper struct {
	base http.RoundTripper
}

func (t *calendarRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrCalendarUnauthorized
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return nil, ErrCalendarNotFound
	case http.StatusMethodNotAllowed, http.StatusNotImplemented:
		resp.Body.Close()
		return nil, errCalendarQueryUnsupported
	}
	resp.Body = &limitedBody{inner: resp.Body, remaining: maxCalendarResponseBytes}
	return resp, nil
}

type limitedBody struct {
	inner     io.ReadCloser
	remaining int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		// The limit was consumed exactly; only fail if more data follows.
		var probe [1]byte
		n, err := b.inner.Read(probe[:])
		if n > 0 {
			return 0, ErrCalendarTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.inner.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.inner.Close() }

// NormalizeCalendarURL validates the subscription URL and converts webcal:// to https://.
func NormalizeCalendarURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, ErrCalendarInvalidURL
	}
	if strings.EqualFold(parsed.Scheme, "webcal") {
		parsed.Scheme = "https"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrCalendarInvalidURL
	}
	if parsed.Hostname() == "" {
		return nil, ErrCalendarInvalidURL
	}
	return parsed, nil
}

var calendarSubscriptionLocks sync.Map // subscription ID → *sync.Mutex

// SyncSubscription fetches the calendar and imports its events as activities.
// It updates the subscription's last-sync bookkeeping fields in the database.
func (s *CalendarSyncService) SyncSubscription(ctx context.Context, db *gorm.DB, cfg config.Config, sub *models.CalendarSubscription) (CalendarSyncStats, error) {
	lock, _ := calendarSubscriptionLocks.LoadOrStore(sub.ID, &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	stats, err := s.syncSubscription(ctx, db, cfg, sub)
	err = redactURLPassword(err, sub.URL)

	now := time.Now().UTC()
	sub.LastSyncedAt = &now
	if err != nil {
		sub.LastSyncStatus = models.CalendarSyncStatusError
		sub.LastSyncError = clampRunes(err.Error(), 500)
	} else {
		sub.LastSyncStatus = models.CalendarSyncStatusSuccess
		sub.LastSyncError = ""
	}
	if saveErr := db.Model(&models.CalendarSubscription{}).Where("id = ?", sub.ID).Updates(map[string]interface{}{
		"last_synced_at":   sub.LastSyncedAt,
		"last_sync_status": sub.LastSyncStatus,
		"last_sync_error":  sub.LastSyncError,
	}).Error; saveErr != nil {
		logger.Error().Err(saveErr).Uint("subscription_id", sub.ID).Msg("Failed to update calendar sync status")
	}

	return stats, err
}

// removes a password embedded in the subscription URL from error text
func redactURLPassword(err error, rawURL string) error {
	if err == nil {
		return nil
	}
	parsed, parseErr := url.Parse(strings.TrimSpace(rawURL))
	if parseErr != nil {
		return err
	}
	password, hasPassword := parsed.User.Password()
	if !hasPassword || password == "" {
		return err
	}
	message := strings.ReplaceAll(err.Error(), password, "xxxxx")
	if message == err.Error() {
		return err
	}
	return &redactedError{message: message, cause: err}
}

// redactedError preserves errors.Is/As matching while overriding the message.
type redactedError struct {
	message string
	cause   error
}

func (e *redactedError) Error() string { return e.message }
func (e *redactedError) Unwrap() error { return e.cause }

func (s *CalendarSyncService) syncSubscription(ctx context.Context, db *gorm.DB, cfg config.Config, sub *models.CalendarSubscription) (CalendarSyncStats, error) {
	parsedURL, err := NormalizeCalendarURL(sub.URL)
	if err != nil {
		return CalendarSyncStats{}, err
	}

	password, err := DecryptCredential(cfg.JWTSecretKey, sub.PasswordEncrypted)
	if err != nil {
		return CalendarSyncStats{}, fmt.Errorf("%w: %v", ErrCalendarUnauthorized, err)
	}

	windowStart := time.Now().AddDate(0, 0, -sub.PastDays).UTC()
	windowEnd := time.Now().AddDate(0, 0, sub.FutureDays).UTC()

	events, err := s.fetchEvents(ctx, parsedURL, sub.Username, password, windowStart, windowEnd)
	if err != nil {
		return CalendarSyncStats{}, err
	}

	if len(events) > maxEventsPerSync {
		logger.Warn().Uint("subscription_id", sub.ID).Int("events", len(events)).Int("limit", maxEventsPerSync).
			Msg("Calendar returned more events than the per-sync limit, truncating")
		events = events[:maxEventsPerSync]
	}

	localizeUntitledEvents(db, sub.UserID, events)

	stats, err := importEvents(db, sub, events, cfg.CalDAVTwoWayEnabled)
	if err != nil {
		return stats, err
	}

	// Two-way (T13): after pulling, push any local CRM edits back out to the
	// subscribed remote calendar — but only when the deployment opted in.
	// Default off, so enabling this never surprises an operator by writing
	// to a real calendar it was only asked to read.
	if cfg.CalDAVTwoWayEnabled {
		var httpClient webdav.HTTPClient = s.client
		if sub.Username != "" || password != "" {
			httpClient = webdav.HTTPClientWithBasicAuth(s.client, sub.Username, password)
		}
		pushStats, err := s.pushLocalEdits(ctx, db, sub, events, httpClient)
		if err != nil {
			return stats, err
		}
		stats.Created += pushStats.Created
		stats.Updated += pushStats.Updated
		stats.Skipped += pushStats.Skipped
	}

	return stats, nil
}

// localizeUntitledEvents fills empty event titles with a fallback in the
// subscription owner's language. Applied before hashing in importEvents, so a
// language change updates existing untitled activities on the next sync.
func localizeUntitledEvents(db *gorm.DB, userID uint, events []calendarEvent) {
	var user models.User
	if err := db.Select("language").First(&user, userID).Error; err != nil {
		logger.Warn().Err(err).Uint("user_id", userID).Msg("Could not load user language for calendar sync, using default")
	}
	title := i18n.T(user.Language, "calendar.untitledEvent")
	for i := range events {
		if events[i].Title == "" {
			events[i].Title = title
		}
	}
}

// fetchEvents retrieves events within the window, preferring a CalDAV
// calendar-query (server-side time-range filtering) and falling back to a
// plain GET for simple .ics exports. Events are window-filtered client-side
// in both cases since not all servers honor the filter.
func (s *CalendarSyncService) fetchEvents(ctx context.Context, calURL *url.URL, username, password string, windowStart, windowEnd time.Time) ([]calendarEvent, error) {
	var httpClient webdav.HTTPClient = s.client
	if username != "" || password != "" {
		httpClient = webdav.HTTPClientWithBasicAuth(s.client, username, password)
	}

	// URLs pointing directly at an .ics file are never CalDAV collections.
	if !strings.HasSuffix(strings.ToLower(calURL.Path), ".ics") {
		events, err := s.queryCalDAV(ctx, httpClient, calURL, windowStart, windowEnd)
		if err == nil {
			return filterEventsByWindow(events, windowStart, windowEnd), nil
		}
		if errors.Is(err, ErrCalendarUnauthorized) || errors.Is(err, ErrCalendarNotFound) ||
			errors.Is(err, ErrCalendarTooLarge) || errors.Is(err, ErrCalendarPrivateAddress) {
			return nil, err
		}
		logger.Debug().Err(err).Str("url", calURL.Redacted()).Msg("CalDAV query failed, falling back to plain GET")
	}

	events, err := s.fetchICS(ctx, httpClient, calURL, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	return filterEventsByWindow(events, windowStart, windowEnd), nil
}

func (s *CalendarSyncService) queryCalDAV(ctx context.Context, httpClient webdav.HTTPClient, calURL *url.URL, windowStart, windowEnd time.Time) ([]calendarEvent, error) {
	client, err := caldav.NewClient(httpClient, calURL.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarInvalidURL, err)
	}

	query := &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{
			Name:     ical.CompCalendar,
			AllProps: true,
			Comps:    []caldav.CalendarCompRequest{{Name: ical.CompEvent, AllProps: true}},
		},
		CompFilter: caldav.CompFilter{
			Name: ical.CompCalendar,
			Comps: []caldav.CompFilter{{
				Name:  ical.CompEvent,
				Start: windowStart,
				End:   windowEnd,
			}},
		},
	}

	objects, err := client.QueryCalendar(ctx, calURL.Path, query)
	if err != nil {
		return nil, err
	}

	var events []calendarEvent
	for _, object := range objects {
		if object.Data == nil {
			continue
		}
		for _, extracted := range extractEvents(object.Data, windowStart, windowEnd) {
			extracted.ETag = object.ETag
			extracted.Path = object.Path
			events = append(events, extracted)
		}
	}
	return events, nil
}

func (s *CalendarSyncService) fetchICS(ctx context.Context, httpClient webdav.HTTPClient, calURL *url.URL, windowStart, windowEnd time.Time) ([]calendarEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, calURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCalendarInvalidURL, err)
	}
	req.Header.Set("User-Agent", "MycorrhizalCRM/1.0 CalendarSync")
	req.Header.Set("Accept", "text/calendar, application/octet-stream, */*")

	resp, err := httpClient.Do(req)
	if err != nil {
		if isCalendarSentinel(err) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrCalendarUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: server returned status %d", ErrCalendarUnreachable, resp.StatusCode)
	}

	var events []calendarEvent
	decoder := ical.NewDecoder(resp.Body)
	for {
		cal, err := decoder.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, ErrCalendarTooLarge) {
				return nil, ErrCalendarTooLarge
			}
			// Tolerate trailing garbage after at least one valid calendar.
			if len(events) > 0 {
				break
			}
			return nil, fmt.Errorf("%w: %v", ErrCalendarInvalidData, err)
		}
		events = append(events, extractEvents(cal, windowStart, windowEnd)...)
	}
	return events, nil
}

func isCalendarSentinel(err error) bool {
	return errors.Is(err, ErrCalendarUnauthorized) || errors.Is(err, ErrCalendarNotFound) ||
		errors.Is(err, ErrCalendarTooLarge) || errors.Is(err, ErrCalendarPrivateAddress) ||
		errors.Is(err, errCalendarQueryUnsupported)
}

// extractEvents converts parsed iCalendar data to internal events. Recurring
// events are expanded into one event per occurrence inside the sync window
// (masters usually carry a DTSTART far in the past, so they would otherwise
// never survive the window filter). Recurrence overrides (RECURRENCE-ID)
// replace their master's occurrence and are imported at their own start time.
// Events that cannot be interpreted (missing/broken DTSTART) are skipped
// instead of failing the whole sync; an empty calendar simply yields no events.
func extractEvents(cal *ical.Calendar, windowStart, windowEnd time.Time) []calendarEvent {
	// Collect overridden occurrences per UID first, so the master expansion
	// below skips them (also for cancelled overrides, which delete a single
	// occurrence).
	overridden := make(map[string]map[string]struct{})
	for _, icalEvent := range cal.Events() {
		ridProp := icalEvent.Props.Get(ical.PropRecurrenceID)
		if ridProp == nil {
			continue
		}
		rid, err := ridProp.DateTime(time.UTC)
		if err != nil {
			continue
		}
		uid := propText(icalEvent.Props, ical.PropUID)
		if overridden[uid] == nil {
			overridden[uid] = make(map[string]struct{})
		}
		overridden[uid][occurrenceKey(rid)] = struct{}{}
	}

	var events []calendarEvent
	add := func(event calendarEvent) {
		if event.UID == "" {
			// RFC 5545 requires a UID but some exports omit it; derive a
			// stable pseudo-UID so re-syncs still deduplicate.
			event.UID = "derived-" + event.contentHash()
		}
		events = append(events, event)
	}

	for _, icalEvent := range cal.Events() {
		if status, err := icalEvent.Status(); err == nil && status == ical.EventCancelled {
			continue
		}

		date, err := icalEvent.DateTimeStart(time.UTC)
		if err != nil || date.IsZero() {
			logger.Debug().Err(err).Msg("Skipping calendar event without usable DTSTART")
			continue
		}

		uid := propText(icalEvent.Props, ical.PropUID)
		event := calendarEvent{
			UID:            uid,
			Title:          clampRunes(propText(icalEvent.Props, ical.PropSummary), 200),
			Description:    clampRunes(propText(icalEvent.Props, ical.PropDescription), 2000),
			Location:       clampRunes(propText(icalEvent.Props, ical.PropLocation), 300),
			Date:           date.UTC(),
			AttendeeEmails: extractAttendeeEmails(icalEvent.Props),
		}

		if ridProp := icalEvent.Props.Get(ical.PropRecurrenceID); ridProp != nil {
			rid, err := ridProp.DateTime(time.UTC)
			if err != nil {
				logger.Debug().Err(err).Msg("Skipping recurrence override with unusable RECURRENCE-ID")
				continue
			}
			// Keyed by the occurrence it replaces, so a moved occurrence keeps
			// updating the same activity across syncs.
			event.UID = occurrenceUID(uid, rid)
			add(event)
			continue
		}

		recurrence, err := icalEvent.RecurrenceSet(time.UTC)
		if err != nil {
			logger.Debug().Err(err).Msg("Could not parse recurrence rule, importing the event's first occurrence only")
		}
		if recurrence == nil {
			add(event)
			continue
		}

		occurrences := recurrence.Between(windowStart, windowEnd, true)
		if len(occurrences) > maxOccurrencesPerEvent {
			occurrences = occurrences[:maxOccurrencesPerEvent]
		}
		for _, occurrence := range occurrences {
			if _, replaced := overridden[uid][occurrenceKey(occurrence)]; replaced {
				continue
			}
			instance := event
			instance.Date = occurrence.UTC()
			instance.UID = occurrenceUID(uid, occurrence)
			add(instance)
		}
	}
	return events
}

func occurrenceKey(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

// occurrenceUID gives each occurrence of a recurring event its own import
// identity, so every occurrence maps to its own activity.
func occurrenceUID(uid string, occurrence time.Time) string {
	if uid == "" {
		return ""
	}
	return uid + "#" + occurrenceKey(occurrence)
}

func propText(props ical.Props, name string) string {
	text, err := props.Text(name)
	if err != nil {
		if prop := props.Get(name); prop != nil {
			return strings.TrimSpace(prop.Value)
		}
		return ""
	}
	return strings.TrimSpace(text)
}

func extractAttendeeEmails(props ical.Props) []string {
	seen := make(map[string]struct{})
	var emails []string
	collect := func(prop ical.Prop) {
		value := strings.TrimSpace(prop.Value)
		if !strings.HasPrefix(strings.ToLower(value), "mailto:") {
			return
		}
		email := strings.ToLower(strings.TrimSpace(value[len("mailto:"):]))
		if email == "" {
			return
		}
		if _, ok := seen[email]; ok {
			return
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}

	for _, prop := range props.Values(ical.PropAttendee) {
		collect(prop)
	}
	if organizer := props.Get(ical.PropOrganizer); organizer != nil {
		collect(*organizer)
	}
	return emails
}

func filterEventsByWindow(events []calendarEvent, start, end time.Time) []calendarEvent {
	filtered := events[:0]
	for _, event := range events {
		if event.Date.Before(start) || event.Date.After(end) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

// importEvents upserts events into activities, keyed by UID through
// CalendarEventLink. Existing unchanged events are skipped, changed events
// update their activity, and activities the user deleted are not re-imported.
//
// twoWay gates the both-changed conflict handling: only when two-way sync is
// enabled (T13) does a pending local edit win over a remote change (the local
// edit is preserved and left for the push phase to send back). With two-way
// off, the pre-T13 remote-wins behavior is unchanged, so an existing
// deployment that never opted in sees no difference.
func importEvents(db *gorm.DB, sub *models.CalendarSubscription, events []calendarEvent, twoWay bool) (CalendarSyncStats, error) {
	contactsByEmail, err := loadContactsByEmail(db, sub.UserID, events)
	if err != nil {
		return CalendarSyncStats{}, err
	}

	feedUIDs := make(map[string]struct{}, len(events))
	for _, event := range events {
		feedUIDs[event.UID] = struct{}{}
	}

	var stats CalendarSyncStats
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, event := range events {
			hash := event.contentHash()

			var link models.CalendarEventLink
			err := tx.Where("subscription_id = ? AND uid = ?", sub.ID, event.UID).First(&link).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Some feeds regenerate every UID on each request (common for
				// dynamically generated .ics exports). Match unchanged events
				// by content and adopt the new UID instead of duplicating them.
				// Links whose UID still appears in the feed belong to another
				// event that merely has identical content; leave those alone.
				var candidates []models.CalendarEventLink
				if err := tx.Where("subscription_id = ? AND content_hash = ?", sub.ID, hash).Order("id").Find(&candidates).Error; err != nil {
					return err
				}
				adopted := false
				for _, candidate := range candidates {
					if _, inFeed := feedUIDs[candidate.UID]; inFeed {
						continue
					}
					candidate.UID = event.UID
					if err := tx.Save(&candidate).Error; err != nil {
						return err
					}
					stats.Skipped++
					adopted = true
					break
				}
				if adopted {
					continue
				}
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			if errors.Is(err, gorm.ErrRecordNotFound) {
				activity := models.Activity{
					UserID:      sub.UserID,
					Title:       event.Title,
					Description: event.Description,
					Location:    event.Location,
					Date:        event.Date,
				}
				if err := tx.Create(&activity).Error; err != nil {
					return err
				}
				if contacts := matchContacts(contactsByEmail, event.AttendeeEmails); len(contacts) > 0 {
					if err := tx.Model(&activity).Association("Contacts").Append(contacts); err != nil {
						return err
					}
				}
				if err := tx.Create(&models.CalendarEventLink{
					SubscriptionID: sub.ID,
					UserID:         sub.UserID,
					UID:            event.UID,
					ActivityID:     activity.ID,
					ContentHash:    hash,
					RemoteETag:     event.ETag,
					RemotePath:     event.Path,
				}).Error; err != nil {
					return err
				}
				stats.Created++
				continue
			}

			if link.ContentHash == hash {
				stats.Skipped++
				continue
			}

			var activity models.Activity
			err = tx.Unscoped().Where("id = ? AND user_id = ?", link.ActivityID, sub.UserID).First(&activity).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if errors.Is(err, gorm.ErrRecordNotFound) || activity.DeletedAt.Valid {
				// The user deleted the imported activity; respect that.
				stats.Skipped++
				continue
			}

			// T13 both-changed conflict: the user edited this activity since
			// the last sync AND the remote changed too. Policy: local-wins —
			// see pushLocalEdits' doc comment. Leave the local activity
			// untouched and let the push phase overwrite the remote; leaving
			// link.ContentHash stale is what the push phase keys on. Only
			// applies when two-way is enabled — otherwise the pre-T13
			// remote-wins behavior is preserved for this deployment.
			if twoWay && activity.UpdatedAt.After(link.UpdatedAt) {
				stats.Skipped++
				continue
			}

			activity.Title = event.Title
			activity.Description = event.Description
			activity.Location = event.Location
			activity.Date = event.Date
			if err := tx.Save(&activity).Error; err != nil {
				return err
			}
			if contacts := matchContacts(contactsByEmail, event.AttendeeEmails); len(contacts) > 0 {
				// Only add newly matched contacts; never remove user-managed links.
				if err := tx.Model(&activity).Association("Contacts").Append(contacts); err != nil {
					return err
				}
			}
			link.ContentHash = hash
			link.RemoteETag = event.ETag
			link.RemotePath = event.Path
			if err := tx.Save(&link).Error; err != nil {
				return err
			}
			stats.Updated++
		}
		return nil
	})
	if err != nil {
		return CalendarSyncStats{}, fmt.Errorf("failed to import calendar events: %w", err)
	}
	return stats, nil
}

// ---------------------------------------------------------------------------
// T13 — two-way calendar sync (T13)
//
// CONFLICT POLICY — read before changing anything here.
//
// Local CRM edits to an imported activity are pushed back out to the
// subscribed remote calendar; the conflict policy when both sides changed is
// **local-wins**: the CRM is the personal relationship OS, a deliberate edit
// about an interaction (a corrected note, a moved date) is higher-value than
// the raw import, and a both-changed conflict is DETECTED (via the
// activity/link UpdatedAt ordering + the remote ETag), not silently assumed.
//
// Deliberately NOT inherited from contact_sync_service.go's reconcileContactSync
// (which full-overwrites local edits on any remote change): that policy would
// silently discard whatever the user typed into the CRM about an interaction,
// which is exactly the failure mode this ticket calls out.
//
// Deletion semantics — conservative, no automatic deletion in either
// direction:
//   - A locally-deleted activity stops being re-imported (existing behavior,
//     importEvents) and is never DELETE'd on the remote: the subscribed
//     calendar is the user's own real calendar, and CRM -> remote deletion is
//     too destructive to do silently.
//   - A remote event absent from a fetch is not treated as deleted: the sync
//     uses a rolling past/future window, so "deleted" is indistinguishable
//     from "moved out of the window". Orphaned links/activities are simply
//     left alone.
//
// The push is gated on CALDAV_TWO_WAY_ENABLED (default off) so no real
// calendar is ever written to unless a deployment opts in.
// ---------------------------------------------------------------------------

// activityContentHash hashes an activity's imported content (the four fields
// the import writes) — the post-push value stored on link.ContentHash so the
// next fetch recognizes the pushed state as in sync. Attendees are excluded:
// they are not representable in the push and would otherwise never converge.
func activityContentHash(a *models.Activity) string {
	parts := []string{a.Title, a.Description, a.Location, a.Date.UTC().Format(time.RFC3339)}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// activityToICalendar serializes an activity as a single-event iCalendar
// calendar for pushing to a remote. The VEVENT carries the REMOTE event's UID
// (link.UID), so the PUT updates the same object in place rather than
// creating a duplicate.
func activityToICalendar(remoteUID string, a *models.Activity) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Mycorrhizal CRM//CalendarSync//EN")

	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, remoteUID)
	event.Props.SetDateTime(ical.PropDateTimeStart, a.Date.UTC())
	event.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	event.Props.SetText(ical.PropSummary, a.Title)
	if a.Description != "" {
		event.Props.SetText(ical.PropDescription, a.Description)
	}
	if a.Location != "" {
		event.Props.SetText(ical.PropLocation, a.Location)
	}
	cal.Children = append(cal.Children, event.Component)
	return cal
}

// resolveRemoteObjectURL joins the subscription's scheme+host with the
// remote object's path captured from the CalDAV query. Remote paths are
// server-relative (e.g. /remote.php/calendars/u/cal/evt.ics).
func resolveRemoteObjectURL(subscriptionURL, remotePath string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(subscriptionURL))
	if err != nil {
		return nil, ErrCalendarInvalidURL
	}
	base.Path = ""
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	joined, err := url.Parse(remotePath)
	if err != nil {
		return nil, ErrCalendarInvalidURL
	}
	return base.ResolveReference(joined), nil
}

// putCalendarObject PUTs a serialized iCalendar calendar to the remote URL,
// carrying the stored ETag as an If-Match precondition when known. Returns the
// response ETag (the remote's new sync identity), or the HTTP status on
// failure so the caller can distinguish a 412 conflict.
func putCalendarObject(httpClient webdav.HTTPClient, remoteURL *url.URL, cal *ical.Calendar, ifMatch string) (string, int, error) {
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", 0, fmt.Errorf("encoding pushed event: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, remoteURL.String(), &buf)
	if err != nil {
		return "", 0, fmt.Errorf("building push request: %w", err)
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	if ifMatch != "" {
		req.Header.Set("If-Match", `"`+ifMatch+`"`)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if isCalendarSentinel(err) {
			return "", 0, err
		}
		return "", 0, fmt.Errorf("%w: %v", ErrCalendarUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return "", resp.StatusCode, nil
	}
	return strings.Trim(resp.Header.Get("ETag"), `"`), resp.StatusCode, nil
}

// pushLocalEdits propagates local CRM edits on imported activities back out
// to the subscribed remote calendar, per the conflict policy documented
// above. Events fetched via the plain-ICS fallback carry no remote path and
// are skipped (no write path exists for them).
func (s *CalendarSyncService) pushLocalEdits(ctx context.Context, db *gorm.DB, sub *models.CalendarSubscription, events []calendarEvent, httpClient webdav.HTTPClient) (CalendarSyncStats, error) {
	var stats CalendarSyncStats

	var links []models.CalendarEventLink
	if err := db.Where("subscription_id = ? AND remote_path <> ''", sub.ID).Find(&links).Error; err != nil {
		return stats, fmt.Errorf("loading calendar links for push: %w", err)
	}
	if len(links) == 0 {
		return stats, nil
	}

	parsedURL, err := NormalizeCalendarURL(sub.URL)
	if err != nil {
		return stats, err
	}

	for i := range links {
		link := &links[i]

		var activity models.Activity
		err := db.Where("id = ? AND user_id = ?", link.ActivityID, sub.UserID).First(&activity).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || activity.DeletedAt.Valid {
			// Locally deleted: not pushed (see the deletion-semantics note).
			stats.Skipped++
			continue
		}
		if err != nil {
			return stats, fmt.Errorf("loading activity for calendar push: %w", err)
		}

		// No local edit since the last sync -> nothing to push.
		if !activity.UpdatedAt.After(link.UpdatedAt) {
			continue
		}

		remoteURL, err := resolveRemoteObjectURL(parsedURL.String(), link.RemotePath)
		if err != nil {
			return stats, err
		}

		// Push with If-Match. A 412 means the remote moved on after our last
		// fetch — the both-changed case — and the policy is local-wins, so
		// retry unconditionally (the remote edit is deliberately overwritten).
		newETag, status, err := putCalendarObject(httpClient, remoteURL, activityToICalendar(link.UID, &activity), link.RemoteETag)
		if status == http.StatusPreconditionFailed {
			logger.Info().Uint("subscription_id", sub.ID).Str("uid", link.UID).
				Msg("Calendar push conflict (412) — local-wins policy overwrites the remote edit")
			newETag, status, err = putCalendarObject(httpClient, remoteURL, activityToICalendar(link.UID, &activity), "")
		}
		if err != nil {
			return stats, err
		}
		if status != http.StatusOK && status != http.StatusCreated && status != http.StatusNoContent {
			return stats, fmt.Errorf("%w: push returned status %d", ErrCalendarUnreachable, status)
		}

		// Mark the pushed content as in sync so the next fetch recognizes it
		// (and so a subsequent remote change is correctly seen as a remote
		// edit rather than a still-pending local one).
		link.ContentHash = activityContentHash(&activity)
		link.RemoteETag = newETag
		link.UpdatedAt = activity.UpdatedAt
		if err := db.Save(link).Error; err != nil {
			return stats, fmt.Errorf("saving calendar link after push: %w", err)
		}
		stats.Updated++
	}

	return stats, nil
}

// loadContactsByEmail maps every attendee email appearing in events to the
// user's contacts, matching against both the primary and additional emails.
func loadContactsByEmail(db *gorm.DB, userID uint, events []calendarEvent) (map[string]models.Contact, error) {
	needed := make(map[string]struct{})
	for _, event := range events {
		for _, email := range event.AttendeeEmails {
			needed[email] = struct{}{}
		}
	}
	if len(needed) == 0 {
		return nil, nil
	}

	var contacts []models.Contact
	if err := db.Where("user_id = ?", userID).Find(&contacts).Error; err != nil {
		return nil, err
	}

	byEmail := make(map[string]models.Contact)
	for _, contact := range contacts {
		addresses := make([]string, 0, len(contact.Emails)+1)
		if contact.Email != "" {
			addresses = append(addresses, contact.Email)
		}
		for _, email := range contact.Emails {
			addresses = append(addresses, email.Value)
		}
		for _, address := range addresses {
			address = strings.ToLower(strings.TrimSpace(address))
			if _, wanted := needed[address]; !wanted {
				continue
			}
			if _, taken := byEmail[address]; !taken {
				byEmail[address] = contact
			}
		}
	}
	return byEmail, nil
}

func matchContacts(byEmail map[string]models.Contact, emails []string) []models.Contact {
	if len(byEmail) == 0 {
		return nil
	}
	seen := make(map[uint]struct{})
	var contacts []models.Contact
	for _, email := range emails {
		contact, ok := byEmail[email]
		if !ok {
			continue
		}
		if _, dup := seen[contact.ID]; dup {
			continue
		}
		seen[contact.ID] = struct{}{}
		contacts = append(contacts, contact)
	}
	return contacts
}

func clampRunes(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

// SyncAllCalendars syncs every enabled subscription. Failures on one
// subscription are recorded on it and do not stop the others.
func SyncAllCalendars(db *gorm.DB, cfg config.Config) {
	var subscriptions []models.CalendarSubscription
	if err := db.Where("sync_enabled = ?", true).Find(&subscriptions).Error; err != nil {
		logger.Error().Err(err).Msg("Failed to load calendar subscriptions for scheduled sync")
		return
	}
	if len(subscriptions) == 0 {
		return
	}

	service := NewCalendarSyncService(cfg.CalDAVBlockPrivateURLs)
	for i := range subscriptions {
		sub := &subscriptions[i]
		ctx, cancel := context.WithTimeout(context.Background(), calendarRequestTimeout)
		stats, err := service.SyncSubscription(ctx, db, cfg, sub)
		cancel()
		if err != nil {
			logger.Warn().Err(err).Uint("subscription_id", sub.ID).Uint("user_id", sub.UserID).
				Msg("Scheduled calendar sync failed")
			continue
		}
		logger.Info().Uint("subscription_id", sub.ID).
			Int("created", stats.Created).Int("updated", stats.Updated).Int("skipped", stats.Skipped).
			Msg("Scheduled calendar sync completed")
	}
}

// SyncCalendarsWithRateLimit wraps SyncAllCalendars with the shared job lock so
// rapid restarts or multiple instances don't sync repeatedly.
func SyncCalendarsWithRateLimit(db *gorm.DB, cfg config.Config) {
	minInterval := time.Duration(cfg.CalDAVSyncIntervalHours)*time.Hour - 30*time.Minute
	if minInterval < 30*time.Minute {
		minInterval = 30 * time.Minute
	}

	acquired, err := acquireJobLock(db, models.JobNameCalendarSync, minInterval)
	if err != nil {
		logger.Error().Err(err).Msg("Error checking calendar sync job lock")
		return
	}
	if !acquired {
		logger.Info().Msg("Skipping calendar sync job - rate limited")
		return
	}

	SyncAllCalendars(db, cfg)

	if err := releaseJobLock(db, models.JobNameCalendarSync, true); err != nil {
		logger.Error().Err(err).Msg("Error releasing calendar sync job lock")
	}
}
