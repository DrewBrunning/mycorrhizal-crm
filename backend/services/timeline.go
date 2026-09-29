package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Contact timeline composer (T66), moved here from controllers so the Atom
// feed (ADR 0030 decision 3) reuses the exact same six-table merge instead of
// maintaining a second query path.
//
// The REST endpoint (GET /contacts/:id/timeline) calls this with ContactID
// set; the feed calls it with ContactID nil for the aggregate over every
// non-archived contact the user owns. The merge strategy is described at
// length in the endpoint's original doc comment: fetch N+1 rows from each of
// the five SQL-orderable tables, merge, sort by (event_date, type, id), take
// the top N. Life events are the one exception (a JSON PartialDate cannot be
// ordered in SQL), so they are fetched in full and filtered in Go.
// ---------------------------------------------------------------------------

// ErrTimelineBadCursor is returned by ComposeTimeline when a cursor's id does
// not match the PK type of the table its type component names. The REST
// handler maps it to a 400.
var ErrTimelineBadCursor = errors.New("timeline cursor id is malformed")

// TimelineCursor is one position in the merged timeline's total order
// (event_date, type, id). Date is kept in its own location/format, never
// UTC-normalized, exactly like EncodeCursor: the per-table SQL predicate
// compares SQLite's stored DATETIME text lexicographically, so the encoded
// value must round-trip byte-for-byte against what the driver writes.
type TimelineCursor struct {
	Date time.Time
	Type string
	ID   string
}

// EncodeTimelineCursor encodes a position for the opaque ?cursor= parameter.
func EncodeTimelineCursor(date time.Time, typ, id string) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(date.Format(time.RFC3339Nano) + "|" + typ + "|" + id),
	)
}

// DecodeTimelineCursor parses an opaque timeline cursor back into its
// (event_date, type, id) position. The id is the trailing component (ids are
// uints or UUIDs — neither can contain "|"), so the first two separators are
// authoritative.
func DecodeTimelineCursor(raw string) (*TimelineCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("cursor is not valid base64url")
	}
	parts := strings.SplitN(string(decoded), "|", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("cursor is malformed")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("cursor timestamp is malformed")
	}
	if _, ok := models.TimelineTypeRank(parts[1]); !ok {
		return nil, errors.New("cursor type is unknown")
	}
	return &TimelineCursor{Date: t, Type: parts[1], ID: parts[2]}, nil
}

// TimelineQuery is the composer's input. ContactID nil selects aggregate mode:
// every contact the user owns whose archived = false (and which is not
// soft-deleted). Types empty means all six. Cutoff is the inclusive recency
// floor; NotAfter, when set, drops entries whose timeline date is after it
// (the feed's "future-dated entries are excluded" rule). Now carries the
// user's location for recency cutoffs and the yearless life-event date case.
type TimelineQuery struct {
	ContactID *uint
	Limit     int
	Desc      bool
	Types     []string
	Cutoff    *time.Time
	Cursor    *TimelineCursor
	NotAfter  *time.Time
	Now       time.Time
	// FilterSensitivity restricts each per-table query to sensitivity='normal'
	// for the tables listed in feedSensitivityFiltered. The REST timeline
	// leaves it false (the owner sees their own data); the Atom feed sets it
	// true because a feed is a copy that leaves the instance (ADR 0030
	// decision 5).
	FilterSensitivity bool
}

// feedSensitivityFiltered is the set of timeline tables the feed composer must
// restrict to sensitivity='normal'. It is empty today: none of the six
// timeline tables has a Sensitivity column (ADR 0030 decision 5's finding).
// TestFeedSensitivityCompleteness reflects over every models.TimelineTypes
// model and fails if one gains a Sensitivity field without being added here,
// so the omission cannot ship silently. The fix when it fails is to add the
// filter, never to allowlist the finding.
var feedSensitivityFiltered = map[string]bool{}

// liveContactIDSubquery is the aggregate-mode contact filter: ids of the
// user's live (non-archived, non-soft-deleted) contacts. Bound params:
// user_id, archived(false).
const liveContactIDSubquery = "SELECT id FROM contacts WHERE user_id = ? AND archived = ? AND deleted_at IS NULL"

// liveContactUIDSubquery is the same set keyed by VCardUID, for the
// entity_id-addressed tables (external activities, gifts, life events).
const liveContactUIDSubquery = "SELECT vcard_uid FROM contacts WHERE user_id = ? AND archived = ? AND deleted_at IS NULL"

// ComposeTimeline merges the six timeline tables for a user (optionally one
// contact) into a deterministic (event_date, type, id) ordered page. It
// returns the page items and the opaque next cursor ("" when there is no
// further page). Items is never nil.
func ComposeTimeline(db *gorm.DB, userID uint, q TimelineQuery) ([]models.TimelineItem, string, error) {
	if q.Limit <= 0 {
		q.Limit = 25
	}
	types := q.Types
	if len(types) == 0 {
		types = models.TimelineTypes
	}
	if q.Now.IsZero() {
		q.Now = time.Now()
	}

	tc := &timelineComposer{
		db:                db,
		userID:            userID,
		limit:             q.Limit,
		desc:              q.Desc,
		cur:               q.Cursor,
		cutoff:            q.Cutoff,
		notAfter:          q.NotAfter,
		filterSensitivity: q.FilterSensitivity,
		now:               q.Now,
	}

	if q.ContactID != nil {
		var contact models.Contact
		if err := db.Where("id = ? AND user_id = ?", *q.ContactID, userID).First(&contact).Error; err != nil {
			return nil, "", err
		}
		tc.contact = &contact
	}

	if tc.cur != nil {
		ids, err := resolveTimelineCursorIDs(types, tc.cur)
		if err != nil {
			return nil, "", err
		}
		tc.cursorIDs = ids
	} else {
		tc.cursorIDs = map[string]any{}
	}

	include := make(map[string]bool, len(types))
	for _, t := range types {
		include[t] = true
	}

	entries := make([]timelineEntry, 0, 6*(q.Limit+1))
	if include[models.TimelineTypeLifeEvent] {
		life, err := tc.fetchLifeEvents()
		if err != nil {
			return nil, "", err
		}
		entries = append(entries, life...)
	}
	for _, t := range types {
		if t == models.TimelineTypeLifeEvent {
			continue
		}
		var fetched []timelineEntry
		var err error
		switch t {
		case models.TimelineTypeNote:
			fetched, err = tc.fetchNotes()
		case models.TimelineTypeActivity:
			fetched, err = tc.fetchActivities()
		case models.TimelineTypeCompletion:
			fetched, err = tc.fetchCompletions()
		case models.TimelineTypeExternalActivity:
			fetched, err = tc.fetchExternalActivities()
		case models.TimelineTypeGift:
			fetched, err = tc.fetchGifts()
		}
		if err != nil {
			return nil, "", err
		}
		entries = append(entries, fetched...)
	}

	sort.Slice(entries, func(i, j int) bool {
		return timelineEntryBefore(entries[i], entries[j], tc.desc)
	})

	nextCursor := ""
	if len(entries) > q.Limit {
		last := entries[q.Limit-1]
		nextCursor = EncodeTimelineCursor(last.date, last.typ, last.id)
		entries = entries[:q.Limit]
	}

	items := make([]models.TimelineItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, models.TimelineItem{Type: e.typ, ID: e.id, Date: e.date, Data: e.data})
	}
	return items, nextCursor, nil
}

// timelineComposer holds the per-request state shared by the six per-type
// fetches and the merge.
type timelineComposer struct {
	db        *gorm.DB
	userID    uint
	contact   *models.Contact // nil in aggregate mode
	limit     int
	desc      bool
	cur       *TimelineCursor
	cursorIDs map[string]any
	cutoff    *time.Time
	notAfter  *time.Time
	// filterSensitivity restricts the per-table query to sensitivity='normal'
	// for tables in feedSensitivityFiltered (see TimelineQuery).
	filterSensitivity bool
	// now is the "current time" in the user's configured location, used for
	// recency cutoffs and for the yearless month/day life-event date case.
	now time.Time
}

// aggregate reports whether this composer is scanning every live contact
// rather than a single one.
func (tc *timelineComposer) aggregate() bool { return tc.contact == nil }

// applyDateBounds narrows a per-type query to the recency bucket and the
// not-after ceiling, when set. The cutoff is bound as a time.Time so GORM
// writes it in the same TEXT format the driver stores DATETIME columns in —
// the same byte-for-byte assumption the T17 ?since= predicate relies on.
func (tc *timelineComposer) applyDateBounds(query *gorm.DB, table, dateCol string) *gorm.DB {
	if tc.cutoff != nil {
		query = query.Where(table+"."+dateCol+" >= ?", *tc.cutoff)
	}
	if tc.notAfter != nil {
		query = query.Where(table+"."+dateCol+" <= ?", *tc.notAfter)
	}
	return query
}

// applyCursor narrows a per-type query to the cursor's page boundary. The id
// argument is the pre-typed value from cursorIDs.
func (tc *timelineComposer) applyCursor(query *gorm.DB, table, dateCol, typ string, id any) *gorm.DB {
	if tc.cur == nil {
		return query
	}
	pred, args := timelineCursorPredicate(table, dateCol, typ, tc.cur, id, tc.desc)
	return query.Where(pred, args...)
}

// buildTableQuery composes the common timeline bounds — recency bucket,
// cursor predicate, then (event_date, id) ordering and the bounded limit —
// onto a per-type base query. Limit+1 is fetched so next_cursor presence is
// exact, matching every T17 list handler.
func (tc *timelineComposer) buildTableQuery(base *gorm.DB, table, dateCol, typ string) *gorm.DB {
	query := tc.applySensitivityFilter(base, table, typ)
	query = tc.applyDateBounds(query, table, dateCol)
	query = tc.applyCursor(query, table, dateCol, typ, tc.cursorIDs[typ])
	return timelineDateOrder(query, table, dateCol, tc.desc).Limit(tc.limit + 1)
}

// applySensitivityFilter adds the sensitivity='normal' predicate for a table
// the feed must filter. It is a no-op unless FilterSensitivity is set and the
// table is in feedSensitivityFiltered (empty today, but enforced by the
// completeness test).
func (tc *timelineComposer) applySensitivityFilter(query *gorm.DB, table, typ string) *gorm.DB {
	if !tc.filterSensitivity || !feedSensitivityFiltered[typ] {
		return query
	}
	return query.Where(table+".sensitivity = ?", "normal")
}

// timelineEntry is one merged row: the raw entity plus the normalized sort
// key. numID is the numeric PK for uint-PK tables (nil for string-PK tables)
// — the id tiebreak must compare the way the per-table SQL predicate does,
// or a page boundary can disagree about which of two same-date rows of the
// same type comes first.
type timelineEntry struct {
	typ   string
	id    string
	date  time.Time
	numID *uint
	data  interface{}
}

func (tc *timelineComposer) fetchNotes() ([]timelineEntry, error) {
	base := tc.db.Where("user_id = ?", tc.userID)
	if tc.aggregate() {
		base = base.Where("contact_id IN ("+liveContactIDSubquery+")", tc.userID, false)
	} else {
		base = base.Where("contact_id = ? AND user_id = ?", tc.contact.ID, tc.userID)
	}
	var notes []models.Note
	if err := tc.buildTableQuery(base, "notes", "date", models.TimelineTypeNote).Find(&notes).Error; err != nil {
		return nil, err
	}
	entries := make([]timelineEntry, 0, len(notes))
	for i := range notes {
		n := &notes[i]
		entries = append(entries, timelineEntry{
			typ: models.TimelineTypeNote, id: fmt.Sprint(n.ID), date: n.Date,
			numID: &n.ID, data: *n,
		})
	}
	return entries, nil
}

func (tc *timelineComposer) fetchActivities() ([]timelineEntry, error) {
	var base *gorm.DB
	if tc.aggregate() {
		// Semi-join so an activity linked to several live contacts is
		// returned once; the Preload below then attaches every live
		// participant. ADR 0030 decision 3.
		base = tc.db.Model(&models.Activity{}).
			Where("activities.user_id = ?", tc.userID).
			Where("activities.id IN (SELECT ac.activity_id FROM activity_contacts ac JOIN contacts c ON c.id = ac.contact_id WHERE c.user_id = ? AND c.archived = ? AND c.deleted_at IS NULL)", tc.userID, false)
	} else {
		base = tc.db.Model(&models.Activity{}).
			Joins("JOIN activity_contacts ON activities.id = activity_contacts.activity_id").
			Where("activities.user_id = ? AND activity_contacts.contact_id = ?", tc.userID, tc.contact.ID)
	}
	query := tc.buildTableQuery(base, "activities", "date", models.TimelineTypeActivity)
	query = query.Preload("Contacts", func(db *gorm.DB) *gorm.DB {
		q := db.Select("ID", "Firstname", "Lastname", "PhotoThumbnail", "Circles").Where("user_id = ?", tc.userID)
		if tc.aggregate() {
			q = q.Where("archived = ?", false)
		}
		return q
	})
	var activities []models.Activity
	if err := query.Find(&activities).Error; err != nil {
		return nil, err
	}
	entries := make([]timelineEntry, 0, len(activities))
	for i := range activities {
		a := &activities[i]
		entries = append(entries, timelineEntry{
			typ: models.TimelineTypeActivity, id: fmt.Sprint(a.ID), date: a.Date,
			numID: &a.ID, data: *a,
		})
	}
	return entries, nil
}

func (tc *timelineComposer) fetchCompletions() ([]timelineEntry, error) {
	base := tc.db.Where("user_id = ?", tc.userID)
	if tc.aggregate() {
		base = base.Where("contact_id IN ("+liveContactIDSubquery+")", tc.userID, false)
	} else {
		base = base.Where("contact_id = ? AND user_id = ?", tc.contact.ID, tc.userID)
	}
	var completions []models.ReminderCompletion
	if err := tc.buildTableQuery(base, "reminder_completions", "completed_at", models.TimelineTypeCompletion).Find(&completions).Error; err != nil {
		return nil, err
	}
	entries := make([]timelineEntry, 0, len(completions))
	for i := range completions {
		comp := &completions[i]
		entries = append(entries, timelineEntry{
			typ: models.TimelineTypeCompletion, id: fmt.Sprint(comp.ID), date: comp.CompletedAt,
			numID: &comp.ID, data: *comp,
		})
	}
	return entries, nil
}

func (tc *timelineComposer) fetchExternalActivities() ([]timelineEntry, error) {
	base := tc.db.Where("user_id = ?", tc.userID)
	if tc.aggregate() {
		base = base.Where("entity_id IN ("+liveContactUIDSubquery+")", tc.userID, false)
	} else {
		base = base.Where("entity_id = ?", tc.contact.VCardUID)
	}
	var activities []models.ExternalActivity
	if err := tc.buildTableQuery(base, "external_activities", "occurred_at", models.TimelineTypeExternalActivity).Find(&activities).Error; err != nil {
		return nil, err
	}
	entries := make([]timelineEntry, 0, len(activities))
	for i := range activities {
		a := &activities[i]
		entries = append(entries, timelineEntry{
			typ: models.TimelineTypeExternalActivity, id: a.ID, date: a.OccurredAt, data: *a,
		})
	}
	return entries, nil
}

// fetchGifts fetches only timeline-eligible gifts: given/received records
// with a handover date. Undated ideas are deliberately not timeline events
// (the web timeline filters them the same way), though they remain visible in
// the gifts block of the M4 composite.
func (tc *timelineComposer) fetchGifts() ([]timelineEntry, error) {
	base := tc.db.Where("user_id = ?", tc.userID)
	if tc.aggregate() {
		base = base.Where("entity_id IN ("+liveContactUIDSubquery+")", tc.userID, false)
	} else {
		base = base.Where("entity_id = ?", tc.contact.VCardUID)
	}
	base = base.Where("status IN ?", []string{models.GiftStatusGiven, models.GiftStatusReceived}).
		Where("date IS NOT NULL")
	var gifts []models.Gift
	if err := tc.buildTableQuery(base, "gifts", "date", models.TimelineTypeGift).Find(&gifts).Error; err != nil {
		return nil, err
	}
	entries := make([]timelineEntry, 0, len(gifts))
	for i := range gifts {
		g := &gifts[i]
		entries = append(entries, timelineEntry{
			typ: models.TimelineTypeGift, id: g.ID, date: *g.Date, data: *g,
		})
	}
	return entries, nil
}

// timelineLifeEventDate resolves a LifeEvent's timeline date from its
// PartialDate, mirroring the web's fullDateFromPartial exactly: the full date
// when all three components exist (UTC midnight — how the web's YYYY-MM-DD
// string parses); the current year when only month/day; Jan 1 of the year
// when only year; CreatedAt when nothing usable (the web falls back to
// created_at the same way). now is the "current time" for the yearless
// month/day case.
func timelineLifeEventDate(e *models.LifeEvent, now time.Time) time.Time {
	if d := e.Date; d != nil {
		if d.Year != nil && d.Month != nil && d.Day != nil {
			return time.Date(*d.Year, time.Month(*d.Month), *d.Day, 0, 0, 0, 0, time.UTC)
		}
		if d.Month != nil && d.Day != nil {
			return time.Date(now.Year(), time.Month(*d.Month), *d.Day, 0, 0, 0, 0, time.UTC)
		}
		if d.Year != nil {
			return time.Date(*d.Year, 1, 1, 0, 0, 0, 0, time.UTC)
		}
	}
	return e.CreatedAt
}

// fetchLifeEvents fetches the contact's (or, in aggregate mode, each live
// contact's) life events in full and resolves each one's timeline date in Go,
// applying the cursor, bucket and not-after predicates here rather than in SQL
// — their PartialDate is JSON text with no orderable timestamp. This is the
// documented exception to the per-table bounded fetch (see the endpoint's doc
// comment).
func (tc *timelineComposer) fetchLifeEvents() ([]timelineEntry, error) {
	base := tc.db.Where("user_id = ?", tc.userID)
	if tc.aggregate() {
		base = base.Where("entity_id IN ("+liveContactUIDSubquery+")", tc.userID, false)
	} else {
		base = base.Where("entity_id = ?", tc.contact.VCardUID)
	}
	base = tc.applySensitivityFilter(base, "life_events", models.TimelineTypeLifeEvent)
	var events []models.LifeEvent
	if err := base.Order("created_at DESC").Find(&events).Error; err != nil {
		return nil, err
	}
	entries := make([]timelineEntry, 0, len(events))
	for i := range events {
		e := &events[i]
		date := timelineLifeEventDate(e, tc.now)
		if tc.cutoff != nil && date.Before(*tc.cutoff) {
			continue
		}
		if tc.notAfter != nil && date.After(*tc.notAfter) {
			continue
		}
		if tc.cur != nil && !timelineEntrySideOfCursor(models.TimelineTypeLifeEvent, date, e.ID, tc.cur, tc.desc) {
			continue
		}
		entries = append(entries, timelineEntry{
			typ: models.TimelineTypeLifeEvent, id: e.ID, date: date, data: *e,
		})
	}
	return entries, nil
}

// timelineCursorPredicate builds the SQL predicate selecting rows of one
// table that belong strictly on the requested side of the cursor position in
// the normalized (event_date, type, id) order. desc=true pages forward
// newest-first ("strictly before the cursor"); desc=false oldest-first
// ("strictly after").
//
// The type component is what makes the per-table predicate more than the
// usual row-value comparison: a table whose type ranks below the cursor's
// type is entirely before the cursor at the shared date (its type tiebreak
// is smaller), while a table ranking above it must be strictly older. id is
// the tiebreak for the single table whose type equals the cursor's. id must
// be pre-typed to the table's PK column type so SQLite's type ordering
// cannot miscompare.
func timelineCursorPredicate(table, dateCol, typ string, cur *TimelineCursor, id any, desc bool) (string, []any) {
	cursorRank, _ := models.TimelineTypeRank(cur.Type)
	tableRank, _ := models.TimelineTypeRank(typ)

	if desc {
		switch {
		case tableRank < cursorRank:
			return fmt.Sprintf("(%s.%s <= ?)", table, dateCol), []any{cur.Date}
		case tableRank == cursorRank:
			return fmt.Sprintf("(%s.%s < ? OR (%s.%s = ? AND %s.id < ?))", table, dateCol, table, dateCol, table), []any{cur.Date, cur.Date, id}
		default:
			return fmt.Sprintf("(%s.%s < ?)", table, dateCol), []any{cur.Date}
		}
	}

	switch {
	case tableRank < cursorRank:
		return fmt.Sprintf("(%s.%s > ?)", table, dateCol), []any{cur.Date}
	case tableRank == cursorRank:
		return fmt.Sprintf("(%s.%s > ? OR (%s.%s = ? AND %s.id > ?))", table, dateCol, table, dateCol, table), []any{cur.Date, cur.Date, id}
	default:
		return fmt.Sprintf("(%s.%s >= ?)", table, dateCol), []any{cur.Date}
	}
}

// timelineDateOrder orders a query by (event_date, id) in the paging
// direction — the per-table order that makes the merged cursor pagination
// total and stable.
func timelineDateOrder(q *gorm.DB, table, dateCol string, desc bool) *gorm.DB {
	dir := "DESC"
	if !desc {
		dir = "ASC"
	}
	return q.Order(table + "." + dateCol + " " + dir).Order(table + ".id " + dir)
}

// resolveTimelineCursorIDs re-types the cursor's string id once per table
// shape — uint for the gorm.Model tables, string for the UUID-PK tables — so
// the per-table SQL predicates compare the way the columns store them
// (CLAUDE.md trap 1's exact silent-mismatch class, on the comparison side).
//
// Only the table whose type equals the cursor's type ever binds the id (the
// row-value tiebreak); every other table is selected purely on its date
// component. So the id is only validated for that one type, and only when it
// is a uint-PK type: a malformed numeric id there is an error (resuming a
// numeric-id page from a non-numeric id is a client error and fails loudly
// rather than silently matching nothing). A UUID-string cursor from a
// string-PK page boundary must NOT fail a subsequent mixed-type page — the
// uint tables' predicates are date-only against it.
func resolveTimelineCursorIDs(types []string, cur *TimelineCursor) (map[string]any, error) {
	ids := make(map[string]any, len(types))
	for _, t := range types {
		if t == cur.Type {
			switch t {
			case models.TimelineTypeNote, models.TimelineTypeActivity, models.TimelineTypeCompletion:
				id, ok := parseUintID(cur.ID)
				if !ok {
					return nil, ErrTimelineBadCursor
				}
				ids[t] = id
				continue
			}
		}
		ids[t] = cur.ID
	}
	return ids, nil
}

// parseUintID parses a numeric primary key from a cursor id, mirroring the
// controller helper of the same name (kept local so services does not import
// controllers). It returns uint64 rather than converting down to uint: the
// value is only ever bound as a SQL comparison against an INTEGER column, so
// keeping the full parsed width avoids a narrowing conversion that CodeQL
// flags as unguarded (go/incorrect-integer-conversion).
func parseUintID(raw string) (uint64, bool) {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// timelineEntrySideOfCursor is the Go-side equivalent of
// timelineCursorPredicate, used only for life events: it reports whether an
// entry's normalized (event_date, type, id) position is strictly before the
// cursor for desc paging, or strictly after it for asc paging. Life-event ids
// are UUID strings, so the id tiebreak is textual — matching the string-PK
// SQL comparisons.
func timelineEntrySideOfCursor(typ string, date time.Time, id string, cur *TimelineCursor, desc bool) bool {
	rank, _ := models.TimelineTypeRank(typ)
	cursorRank, _ := models.TimelineTypeRank(cur.Type)

	if desc {
		if date.Before(cur.Date) {
			return true
		}
		if date.After(cur.Date) {
			return false
		}
		if rank < cursorRank {
			return true
		}
		if rank > cursorRank {
			return false
		}
		return id < cur.ID
	}
	if date.After(cur.Date) {
		return true
	}
	if date.Before(cur.Date) {
		return false
	}
	if rank > cursorRank {
		return true
	}
	if rank < cursorRank {
		return false
	}
	return id > cur.ID
}

// timelineEntryBefore reports whether entry a sorts before entry b in the
// requested direction of the (event_date, type, id) order. id compares
// numerically for uint-PK tables and textually for string-PK ones, matching
// the per-table SQL predicate so a page boundary never disagrees with the
// merge.
func timelineEntryBefore(a, b timelineEntry, desc bool) bool {
	if a.date.Equal(b.date) {
		if a.typ == b.typ {
			if a.numID != nil && b.numID != nil {
				if desc {
					return *a.numID > *b.numID
				}
				return *a.numID < *b.numID
			}
			if desc {
				return a.id > b.id
			}
			return a.id < b.id
		}
		aRank, _ := models.TimelineTypeRank(a.typ)
		bRank, _ := models.TimelineTypeRank(b.typ)
		if desc {
			return aRank > bRank
		}
		return aRank < bRank
	}
	if desc {
		return a.date.After(b.date)
	}
	return a.date.Before(b.date)
}
