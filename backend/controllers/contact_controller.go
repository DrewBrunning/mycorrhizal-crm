package controllers

import (
	"errors"
	"fmt"
	"mycorrhizal/attachments"
	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// contactListColumns adds updated_at to the slim projection — the cursor
// (updated_at, id) needs it to build the response's next_cursor. Fresh slice
// per var so appends can never leak into models.ContactSummaryColumns'
// backing array.
var contactListColumns = append(append([]string{}, models.ContactSummaryColumns...), "updated_at")

// contactNameSortedColumns additionally selects sort_name — the T73 name-sort
// cursor (sort_name, id) needs it to build the response's next_cursor.
var contactNameSortedColumns = append(append([]string{}, contactListColumns...), "sort_name")

// contactFeedColumns additionally selects deleted_at so the ?since= change
// feed can mark tombstones (Unscoped rows whose deleted_at is set).
var contactFeedColumns = append(append([]string{}, contactListColumns...), "deleted_at")

// contactInfoClause is the T103 "has contact info" predicate: at least one of
// a non-empty flat email, a non-empty flat phone, or a non-empty entry in the
// emails, phones or urls JSON arrays. Reading the flat scalars AND the arrays
// is deliberate — the flat columns are derived from [0] of each array, so
// they cover the common case cheaply, but a contact whose only email arrived
// via a path that didn't populate the scalar must still count. The flat legs
// COALESCE the NULL-able scalars to ” (raw-SQL/legacy rows can store NULL
// where GORM writes ”): without it, `length(trim(NULL)) > 0` is NULL, which
// the hidden-count query's `NOT (clause)` turns into NULL too — such a row is
// excluded from the visible list but silently NOT counted as hidden, so the
// "N contacts hidden" disclosure under-reports. No args.
var contactInfoClause = `(
	length(trim(COALESCE(email, ''))) > 0
	OR length(trim(COALESCE(phone, ''))) > 0
	OR (json_valid(emails) AND EXISTS (SELECT 1 FROM json_each(contacts.emails) WHERE length(trim(json_extract(json_each.value, '$.value'))) > 0))
	OR (json_valid(phones) AND EXISTS (SELECT 1 FROM json_each(contacts.phones) WHERE length(trim(json_extract(json_each.value, '$.value'))) > 0))
	OR (json_valid(urls) AND EXISTS (SELECT 1 FROM json_each(contacts.urls) WHERE length(trim(json_extract(json_each.value, '$.value'))) > 0))
 )`

// rejectInvalidPeriods aborts with a 400 and returns true when the submitted
// CRMEnvelope carries a period that cannot be stored: both endpoints absent, a
// reversed range, or an EntryID that does not resolve to an entry of its Kind
// on the submitted Card (docs/adrs/0025-temporal-periods.md). A reference that
// cannot resolve is a caller error, never a silent drop — the degradation
// policy's drop-with-warning applies to export formats, not to user input.
func rejectInvalidPeriods(c *gin.Context, card contactmodel.Card, crm contactmodel.CRMEnvelope) bool {
	bad := crm.InvalidPeriods(card)
	if len(bad) == 0 {
		return false
	}
	apperrors.AbortWithError(c, apperrors.ErrValidation("Request validation failed").
		WithDetails("crm.periods", fmt.Sprintf(
			"%d period(s) reference a missing entry, are empty, or end before they start", len(bad))))
	return true
}

func CreateContact(c *gin.Context) {
	// Save to the database
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// Opt-in per-user quota (issue #950): refuse before any write once the
	// user's live-contact count has reached the operator's limit.
	if err := services.UserQuotaFromConfig(currentConfig(c)).CheckContactCreate(db, userID); err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	// Get validated input from validation middleware., this
	// is the new nested Card/CRM shape (models.ContactRecordInput), not the
	// old flat models.ContactInput.
	input, err := middleware.GetValidated[models.ContactRecordInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if rejectInvalidPeriods(c, input.Card, input.CRM) {
		return
	}
	if rejectInvalidAddressMapFields(c, input.Card) {
		return
	}

	contact := models.Contact{UserID: userID, Gender: input.Gender}
	models.ApplyRecordToContact(&contact, input.ToRecord(), currentConfig(c).ProfilePhotoDir)

	// Firstname is checked here (not via a struct tag on the nested input —
	// see ContactRecordInput's doc comment for why) because it's an
	// app-level invariant on Contact itself (existing `validate:"required"`
	// tag on models.Contact.Firstname, previously enforced indirectly by the
	// old flat ContactInput requiring it directly) that the new nested shape
	// has no simple field-level equivalent for: "at least one given-name
	// component, or a Name.Full, is present" is a derived condition, not a
	// single field.
	if contact.Firstname == "" {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Request validation failed").WithDetails("card.name", "at least one name component (kind=given) or name.full is required"))
		return
	}

	if err := db.Create(&contact).Error; err != nil {
		logger.FromContext(c).Error().Err(err).Msg("Error saving contact to database")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save contact").WithError(err))
		return
	}

	// A wedding anniversary on the new card mirrors into a married LifeEvent
	// (services/wedding_sync.go) so the timeline and the card never disagree.
	if err := services.SyncWeddingFromCard(db, userID, contact.VCardUID, services.WeddingDateFromCard(&contact.Card)); err != nil {
		logger.FromContext(c).Error().Err(err).Msg("Error syncing wedding anniversary to life event")
	}

	services.TriggerWebhooksAsync(c.Request.Context(), db, currentConfig(c), userID, "contact.created", contact)
	c.JSON(http.StatusCreated, gin.H{"message": "Contact created successfully", "contact": models.NewContactRecordResponse(&contact, currentConfig(c).ProfilePhotoDir, db)})
}

// applyContactSearch filters a contacts query by a free-text term. The base
// clause is LIKE-based substring matching over flat columns; T85
// (T85) ORs in an FTS5
// prefix-token match on contacts_fts, narrowing the same row set the LIKE
// clause narrows rather than replacing it — see the ticket's "FTS as a
// filter, not a ranker" decision and its "LIKE and FTS do not match the same
// rows" trap. LIKE alone finds "Joanne" for "ann" (substring); FTS alone does
// not (token-prefix). ORing keeps every match that worked before this ticket
// and adds the FTS matches on top.
func applyContactSearch(query *gorm.DB, userID uint, searchTerm string) *gorm.DB {
	like := "%" + searchTerm + "%"

	clause := "firstname LIKE ? OR lastname LIKE ? OR nickname LIKE ? " +
		"OR (firstname || ' ' || lastname) LIKE ? OR (nickname || ' ' || lastname) LIKE ? " +
		"OR email LIKE ? OR phone LIKE ? " +
		// T38: address text is searchable through the denormalized
		// addresses_flat column (populated by Contact.BeforeSave and
		// backfilled by migration 000010), the same flat-column surface
		// contacts_fts indexes.
		"OR addresses_flat LIKE ? " +
		"OR (json_valid(emails) AND EXISTS (SELECT 1 FROM json_each(contacts.emails) WHERE json_extract(json_each.value, '$.value') LIKE ?))"
	args := []interface{}{like, like, like, like, like, like, like, like, like}

	// T69: phone matching goes through the denormalized phones_normalized
	// column (populated by Contact.BeforeSave from every Phones[] entry,
	// backfilled by migration 000020). A raw substring match on the normalized
	// column already covers the flat primary scalar's digits; when the search
	// term is itself phone-shaped, it is additionally normalized to its digit
	// string + PhoneKey so punctuation/grouping/country-code differences
	// between the query and the stored value no longer hide a match.
	// The flat `phone LIKE ?` clause above is kept for rows that only ever had
	// the legacy scalar set (their phones JSON, and hence their normalized
	// column, may be empty).
	clause += " OR phones_normalized LIKE ?"
	args = append(args, like)
	if digits, key, ok := services.PhoneQueryTokens(searchTerm); ok {
		clause += " OR phones_normalized LIKE ?"
		args = append(args, "%"+digits+"%")
		if key != "" && key != digits {
			clause += " OR phones_normalized LIKE ?"
			args = append(args, "%"+key+"%")
		}
	}

	// T85: gate the FTS clause at two runes, matching services.Search's own
	// gate (search_service.go) — below that, this is LIKE-only, which is
	// exactly today's (pre-T85) behavior, so a one-character search does not
	// change at all. contacts_fts indexes archived and soft-deleted rows, so
	// this subquery is not trusted to filter them — the outer query's
	// existing `archived` Where and GORM's default soft-delete scope do that,
	// and they still apply here because this whole clause is only ever
	// ANDed onto the outer query, never a replacement for it.
	if len([]rune(strings.TrimSpace(searchTerm))) >= 2 {
		if match, ok := services.ContactFTSMatch(searchTerm); ok {
			// contacts_fts.user_id is redundant with the outer query's own
			// `user_id = ?` scope, but included anyway — defense-in-depth
			// matching the double-scoping services.Search already does, and
			// the ticket's own highest-risk-part callout.
			clause += " OR contacts.id IN (SELECT rowid FROM contacts_fts WHERE contacts_fts MATCH ? AND contacts_fts.user_id = ?)"
			args = append(args, match, userID)
		}
	}

	return query.Where(clause, args...)
}

func GetContacts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// ?vcard_uid= (repeatable) is a batch by-VCardUID lookup, short-circuiting
	// the rest of this handler's search/sort/pagination/includes logic
	// entirely -- it exists for callers (e.g. the RelationshipEdge frontend)
	// that already know exactly which contacts they need to resolve
	// a Contact.VCardUID reference back into a displayable name, and have no
	// use for the list-view machinery below. Bounded by how many UIDs were
	// requested, not paginated. Respects include_archived like the rest of
	// this handler (an edge can point at an archived contact). Deliberately
	// NOT migrated to cursor pagination (T17) — it is not a list view.
	if vcardUIDs := c.QueryArray("vcard_uid"); len(vcardUIDs) > 0 {
		query := db.Model(&models.Contact{}).Where("user_id = ? AND vcard_uid IN ?", userID, vcardUIDs).
			Select(models.ContactSummaryColumns)
		if c.Query("include_archived") != "true" {
			query = query.Where("archived = ?", false)
		}
		var contacts []models.Contact
		if err := query.Find(&contacts).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contacts").WithError(err))
			return
		}
		items := make([]models.ContactSummary, len(contacts))
		for i := range contacts {
			items[i] = models.NewContactSummary(&contacts[i])
		}
		c.JSON(http.StatusOK, gin.H{
			"contacts": items,
			"total":    int64(len(items)),
			"page":     1,
			"limit":    len(items),
		})
		return
	}

	// NOTE: the fields= partial-projection param is gone (docs/adrs/0001-neutral-hub-and-spoke-contact-model.md) — deliberately, not
	// an oversight. It is simply no longer read; a request that still sends
	// it is not rejected, it just has no effect. The fixed ContactSummary
	// shape (below) now serves the reason fields= existed (avoiding
	// over-fetch on a list view) — see models.ContactSummaryColumns.

	// T17: cursor pagination. The list endpoint pages by (updated_at, id) and
	// the ?since= query turns it into a change feed: everything the user's
	// data changed after the cursor, including soft-deleted rows (tombstones),
	// ordered forward so a replica can replay history. An exact `total` count
	// is deliberately gone from this endpoint — it is expensive on the
	// unboundedly-growing contacts table, and that is the usual thing cursor
	// pagination gives up.
	//
	// T73: an optional ?sort= adds a second cursor shape — "name" orders by
	// the denormalized sort_name key (a second cursor shape, not a generalized
	// one). Default stays "updated_at"; changing the web default is T77's
	// call, and only for new sessions.
	sort, err := parseContactSort(c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}
	nameSorted := sort == "name"

	// T103: ?has_contact_info=true|false narrows the list to contacts with at
	// least one non-empty email, phone or URL (the contactInfoClause
	// predicate). Defaults to OFF for existing API consumers; the web client
	// opts in by default (the Contacts page is an address book, not a graph
	// dump). Any value other than true/false is a 400, matching how sort is
	// treated — not a silent fallback. The ?vcard_uid= batch path returns
	// before this parse, so it never sees the param; the ?since= change feed
	// returns after the parse but ignores the flag — a feed is sync state and
	// must carry every row regardless of filters (a malformed value is still a
	// 400 there, since the parse is shared).
	hasContactInfo := false
	if raw := c.Query("has_contact_info"); raw != "" {
		switch raw {
		case "true":
			hasContactInfo = true
		case "false":
		default:
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("has_contact_info", "must be one of: true, false"))
			return
		}
	}

	params, err := GetCursorParamsForSort(c, sort)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	// T73: the ?since= change feed is sync state, not browsing — it has no
	// name-ordered cursor to resume from and no UpdatedAt for the purge
	// retention window to age-check against (CheckFeedCursorAge below).
	// Decided: sort=name combined with ?since= is a 400, NOT a silent
	// fallback to (updated_at, id) — a sync client would believe it held a
	// name-ordered feed and quietly diverge.
	if params.Since && nameSorted {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("sort", "sort=name cannot be combined with since="))
		return
	}
	if err := CheckFeedCursorAge(c, params); err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	// Change feed (?since=): return every row changed after the cursor —
	// created, updated, AND soft-deleted (read via Unscoped, marked
	// deleted:true) — regardless of archive state or search/circle filters.
	// A replica needs the complete picture, not a filtered view.
	if params.Since {
		query := db.Unscoped().Model(&models.Contact{}).
			Where("user_id = ?", userID).
			Select(contactFeedColumns)
		if params.Cursor != nil {
			id, ok := parseCursorID(params.Cursor)
			if !ok {
				apperrors.AbortWithError(c, apperrors.ErrInvalidInput("since", "cursor id is malformed"))
				return
			}
			pred, t, idv := cursorPredicate("contacts", params.Cursor, id, false)
			query = query.Where(pred, t, idv)
		}
		query = cursorOrderBy(query, "contacts", false).Limit(params.Limit + 1)

		var contacts []models.Contact
		if err := query.Find(&contacts).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contacts").WithError(err))
			return
		}
		nextCursor := ""
		if len(contacts) > params.Limit {
			contacts = contacts[:params.Limit]
			nextCursor = EncodeCursor(contacts[len(contacts)-1].UpdatedAt, contacts[len(contacts)-1].ID)
		}
		items := make([]models.ContactSummary, len(contacts))
		for i := range contacts {
			items[i] = models.NewContactSummary(&contacts[i])
			items[i].Deleted = contacts[i].DeletedAt.Valid
		}
		c.JSON(http.StatusOK, gin.H{
			"contacts":    items,
			"next_cursor": nextCursor,
			"limit":       params.Limit,
			"sync":        buildSyncMeta(SyncModeIncremental),
		})
		return
	}

	// Parse relationships to include with validation. This
	// mechanic is preserved exactly as-is; only the per-item shape changes
	// (ContactSummary, extended to ContactSummaryWithRelations when any
	// includes= relation is requested).
	//
	// "relationships" was removed from this map: the legacy
	// models.Relationship include had zero remaining frontend callers (the
	// RelationshipEdge UI fetches via its own /relationship-edges endpoint,
	// never by requesting this field). Matches this file's own fields=
	// precedent above -- a client still sending includes=relationships just
	// gets no match for that token, not an error.
	var relationshipMap = map[string]bool{
		"notes":      false,
		"activities": false,
		"reminders":  false,
	}
	includes := c.Query("includes")
	includesRequested := false
	for _, rel := range strings.Split(includes, ",") {
		if _, exists := relationshipMap[rel]; exists {
			relationshipMap[rel] = true
			includesRequested = true
		}
	}

	// Parse archive filtering parameters
	includeArchived := c.Query("include_archived") == "true"
	archivedOnly := c.Query("archived") == "true"

	// T73: name sort needs sort_name in the projection (to build the
	// next_cursor) and in the ORDER BY.
	columns := contactListColumns
	if nameSorted {
		columns = contactNameSortedColumns
	}
	var contacts []models.Contact
	query := db.Model(&models.Contact{}).Where("user_id = ?", userID).
		Select(columns)

	// Apply archive filtering
	if !includeArchived {
		if archivedOnly {
			query = query.Where("archived = ?", true)
		} else {
			query = query.Where("archived = ?", false)
		}
	}

	// Issue #173: ?favorites=true narrows the list to the caller's favorite
	// contacts. Composes with search/circle/archived/sort/cursor exactly like
	// the other predicates (all ANDed Wheres). Not applied to the ?since=
	// change feed — that path returns before this point (sync state must
	// carry every row regardless of filters).
	if c.Query("favorites") == "true" {
		query = query.Where("is_favorite = ?", true)
	}

	// Apply search filter using parameterization. The term is length-bounded
	// (issue #415): applyContactSearch wraps it in %...% LIKE clauses and an
	// FTS5 MATCH, so an unbounded term is a per-request cost an attacker can
	// drive, not a search.
	if searchTerm := c.Query("search"); searchTerm != "" {
		// I18N-02: fold to NFC (the storage form) before the rune-length gate
		// and applyContactSearch, so the LIKE arms — byte comparisons — see a
		// term comparable with every stored name regardless of the encoding
		// the client sent. Folding cannot lengthen a term (NFC composes
		// combining sequences), so the length check stays conservative.
		searchTerm = services.NormalizeSearchTerm(searchTerm)
		if len([]rune(searchTerm)) > services.MaxSearchTermLen {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("search", fmt.Sprintf("search must be at most %d characters", services.MaxSearchTermLen)))
			return
		}
		query = applyContactSearch(query, userID, searchTerm)
	}

	if circle := c.Query("circle"); circle != "" {
		query = query.Where(`EXISTS (
			SELECT 1 FROM circle_members cm
			JOIN circles c ON cm.circle_id = c.id AND c.user_id = ?
			WHERE cm.member_vcard_uid = contacts.vcard_uid AND c.name = ?
		)`, userID, circle)
	}

	// Temporary legacy filter for T2 triage migration — reads from the
	// old flat contacts.circles JSON column. Remove once migration is complete.
	if circle := c.Query("circle_legacy"); circle != "" {
		query = query.Where("EXISTS (SELECT 1 FROM json_each(contacts.circles) WHERE json_each.value = ?)", circle)
	}

	// T103: ?has_contact_info=true narrows the list to contacts you can
	// actually contact (contactInfoClause). The hidden count is computed over
	// the same filter scope (archive/search/circle) WITHOUT the predicate and
	// WITHOUT the resume cursor, so it reflects the whole set the user is
	// looking at, not the page they are on — the web client renders it as
	// "N contacts without contact info are hidden" so a default-on filter
	// never reads as silently lost data. Session() clones the builder so the
	// count does not pollute the Find below; hidden_count is only present in
	// the response while the filter is active.
	hiddenCount := int64(0)
	if hasContactInfo {
		countQuery := query.Session(&gorm.Session{})
		countQuery = countQuery.Where("NOT " + contactInfoClause)
		if err := countQuery.Count(&hiddenCount).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to count hidden contacts").WithError(err))
			return
		}
		query = query.Where(contactInfoClause)
	}

	// T17: resume-after cursor. Default order is (updated_at, id), so the
	// cursor position maps directly onto the ordering. T73: sort=name uses
	// the (sort_name, id) key and its own cursor shape — never mixed. Applied
	// after the filters (and the T103 count) so the hidden count can be
	// computed over the whole filtered set; WHERE clauses are ANDed, so the
	// ordering between filters and cursor is semantically irrelevant.
	desc := params.Order == "desc"
	if params.NameCursor != nil {
		id, ok := parseUintID(params.NameCursor.ID)
		if !ok {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("cursor", "cursor id is malformed"))
			return
		}
		pred, t, idv := nameCursorPredicate("contacts", params.NameCursor, id, desc)
		query = query.Where(pred, t, idv)
	} else if params.Cursor != nil {
		id, ok := parseCursorID(params.Cursor)
		if !ok {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("cursor", "cursor id is malformed"))
			return
		}
		pred, t, idv := cursorPredicate("contacts", params.Cursor, id, desc)
		query = query.Where(pred, t, idv)
	}

	// Preload requested relationships
	for rel, include := range relationshipMap {
		if include {
			switch rel {
			case "notes":
				query = query.Preload("Notes", "notes.user_id = ?", userID)
			case "activities":
				query = query.Preload("Activities", "activities.user_id = ?", userID)
			case "reminders":
				query = query.Preload("Reminders", "reminders.user_id = ?", userID)
			}
		}
	}

	// Execute query. Fetch one extra row so next_cursor presence is exact:
	// a full page may still be the last page, and a partial page never has
	// more.
	if nameSorted {
		query = nameCursorOrderBy(query, "contacts", desc)
	} else {
		query = cursorOrderBy(query, "contacts", desc)
	}
	query = query.Limit(params.Limit + 1)
	if err := query.Find(&contacts).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contacts").WithError(err))
		return
	}
	nextCursor := ""
	if len(contacts) > params.Limit {
		contacts = contacts[:params.Limit]
		last := contacts[len(contacts)-1]
		if nameSorted {
			nextCursor = EncodeNameCursor(last.SortName, last.ID)
		} else {
			nextCursor = EncodeCursor(last.UpdatedAt, last.ID)
		}
	}

	// Map contacts to the slim ContactSummary shape: plain
	// ContactSummary normally, or ContactSummaryWithRelations when includes=
	// requested at least one relation — never the full Card,
	// binding "list returns []ContactSummary, not the full Card" rule.
	if includesRequested {
		items := make([]models.ContactSummaryWithRelations, len(contacts))
		for i := range contacts {
			items[i] = models.NewContactSummaryWithRelations(&contacts[i])
		}
		resp := gin.H{
			"contacts":    items,
			"next_cursor": nextCursor,
			"limit":       params.Limit,
			"sync":        buildSyncMeta(SyncModeIncremental),
		}
		if hasContactInfo {
			resp["hidden_count"] = hiddenCount
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	items := make([]models.ContactSummary, len(contacts))
	for i := range contacts {
		items[i] = models.NewContactSummary(&contacts[i])
	}
	resp := gin.H{
		"contacts":    items,
		"next_cursor": nextCursor,
		"limit":       params.Limit,
		"sync":        buildSyncMeta(SyncModeIncremental),
	}
	if hasContactInfo {
		resp["hidden_count"] = hiddenCount
	}
	c.JSON(http.StatusOK, resp)
}

func GetContactsRandom(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var selectedFields = []string{"ID", "firstname", "lastname", "nickname", "circles", "photo_thumbnail"}

	var contacts []models.Contact
	query := db.Model(&models.Contact{}).Where("user_id = ?", userID).Where("archived = ?", false)

	if len(selectedFields) > 0 {
		query = query.Select(selectedFields)
	}

	// Get 5 random contacts
	query = query.Order("RANDOM()").Limit(5)

	// Execute query
	if err := query.Find(&contacts).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contacts").WithError(err))
		return
	}

	// Map to response with photo thumbnail
	contactResponses := make([]models.ContactResponse, len(contacts))
	for i, contact := range contacts {
		contactResponses[i] = models.ContactResponse{
			Contact:        contact,
			PhotoThumbnail: contact.PhotoThumbnail,
		}
	}

	// Respond with random contacts
	c.JSON(http.StatusOK, gin.H{
		"contacts": contactResponses,
	})
}

func GetUpcomingBirthdays(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	birthdays, err := services.GetUpcomingBirthdays(db, userID, reminderNow(c))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve upcoming birthdays").WithError(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"birthdays": birthdays,
	})
}

func GetContact(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	db := c.MustGet("db").(*gorm.DB)

	// NOTE: fields= is gone here too — the detail endpoint now always
	// returns the full neutral Record/Card (ContactRecordResponse), which is
	// what fields= partial-fetching existed to approximate a slice of.
	//
	// Preload behavior: kept exactly as the  "no fields=" branch
	// (always preload all associations) — dedicated endpoints like
	// GET /contacts/:id/notes already exist and may make this redundant for
	// some callers, but changing that is a separate, larger decision this WP
	// doesn't need to make; preserving existing behavior here is the safer
	// default for backward compat.
	//
	// Relationships is no longer preloaded here — the legacy
	// models.Relationship association had zero remaining readers once the
	// RelationshipEdge frontend UI shipped  (it fetches relationships
	// via its own /relationship-edges endpoint, not off this response).
	var contact models.Contact
	query := db.Where("user_id = ?", userID).
		Preload("Notes", "notes.user_id = ?", userID).
		Preload("Activities", "activities.user_id = ?", userID).
		Preload("Reminders", "reminders.user_id = ?", userID)

	if err := query.First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}
	c.JSON(http.StatusOK, models.NewContactRecordResponse(&contact, currentConfig(c).ProfilePhotoDir, db))
}

func UpdateContact(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	// CON-01 (issue #456, ADR 0008): reject a stale conditional write before
	// touching the row. No-op when the client sent no If-Match header.
	if !checkIfMatch(c, contact.Revision) {
		return
	}

	// Get validated input from validation middleware (new nested shape, see
	// CreateContact's comment).
	input, err := middleware.GetValidated[models.ContactRecordInput](c)
	if err != nil {
		apperrors.AbortWithError(c, err)
		return
	}

	if rejectInvalidPeriods(c, input.Card, input.CRM) {
		return
	}
	if rejectInvalidAddressMapFields(c, input.Card) {
		return
	}

	contact.Gender = input.Gender
	models.ApplyRecordToContact(&contact, input.ToRecord(), currentConfig(c).ProfilePhotoDir)

	if contact.Firstname == "" {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Request validation failed").WithDetails("card.name", "at least one name component (kind=given) or name.full is required"))
		return
	}

	if err := db.Save(&contact).Error; err != nil {
		if handleRevisionConflict(c, "Contact", err) {
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to update contact").WithError(err))
		return
	}

	// Mirror the (possibly changed) wedding anniversary into the contact's
	// married LifeEvent, and vice versa is handled by the life-event
	// controller. See services/wedding_sync.go.
	if err := services.SyncWeddingFromCard(db, userID, contact.VCardUID, services.WeddingDateFromCard(&contact.Card)); err != nil {
		logger.FromContext(c).Error().Err(err).Msg("Error syncing wedding anniversary to life event")
	}

	services.TriggerWebhooksAsync(c.Request.Context(), db, currentConfig(c), userID, "contact.updated", contact)
	c.JSON(http.StatusOK, models.NewContactRecordResponse(&contact, currentConfig(c).ProfilePhotoDir, db))
}

func DeleteContact(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// Check if contact exists first
	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	// CON-01 (issue #456, ADR 0008): a conditional DELETE with a stale
	// If-Match revision is rejected before the cascade runs.
	if !checkIfMatch(c, contact.Revision) {
		return
	}

	// Start a transaction to ensure all deletes succeed together
	var attachmentNames []string
	if err := db.Model(&models.Attachment{}).
		Where("contact_vcard_uid = ? AND user_id = ?", contact.VCardUID, userID).
		Pluck("stored_name", &attachmentNames).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to load contact attachments").WithError(err))
		return
	}
	// The cascade checklist is services.ContactCascadeRegistry (ADR 0035).
	if err := services.DeleteContact(db, contact, userID, clock.FromContext(c).Now()); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete contact and associated data").WithError(err))
		return
	}

	// Cleanup profile photos and attachment files after successful database
	// transaction. This is done outside the transaction since file deletion
	// cannot be rolled back.
	deleteContactPhotos(c, contact)
	deleteContactAttachmentFiles(c, attachmentNames)

	services.TriggerWebhooksAsync(c.Request.Context(), db, currentConfig(c), userID, "contact.deleted", gin.H{"id": contact.ID})
	c.JSON(http.StatusOK, gin.H{"message": "Contact deleted"})
}

// deleteContactPhotos removes the profile photo file for a contact
// Note: thumbnails are stored as base64 in the database, not as files
func deleteContactPhotos(c *gin.Context, contact models.Contact) {
	uploadDir := currentConfig(c).ProfilePhotoDir
	if uploadDir == "" {
		return
	}

	log := logger.FromContext(c)

	// Delete main photo if it exists
	if contact.Photo != "" {
		photoPath := filepath.Join(uploadDir, contact.Photo)
		if err := os.Remove(photoPath); err != nil && !os.IsNotExist(err) {
			log.Warn().Err(err).Str("path", photoPath).Msg("Failed to delete contact photo")
		} else if err == nil {
			log.Debug().Str("path", photoPath).Msg("Deleted contact photo")
		}
	}

	// Delete legacy file-based thumbnail if it exists (not base64 data URL)
	if contact.PhotoThumbnail != "" && !strings.HasPrefix(contact.PhotoThumbnail, "data:") {
		thumbnailPath := filepath.Join(uploadDir, contact.PhotoThumbnail)
		if err := os.Remove(thumbnailPath); err != nil && !os.IsNotExist(err) {
			log.Warn().Err(err).Str("path", thumbnailPath).Msg("Failed to delete contact thumbnail")
		} else if err == nil {
			log.Debug().Str("path", thumbnailPath).Msg("Deleted contact thumbnail")
		}
	}
}

// deleteContactAttachmentFiles removes a contact's attachment files from disk
// (N7). Called after the database transaction that soft-deleted the attachment
// records, since file deletion cannot be rolled back. storedNames are the
// server-generated UUID filenames captured before the delete.
func deleteContactAttachmentFiles(c *gin.Context, storedNames []string) {
	dir := currentConfig(c).AttachmentsDir
	if dir == "" || len(storedNames) == 0 {
		return
	}
	log := logger.FromContext(c)
	for _, name := range storedNames {
		path, err := attachments.StoredPath(dir, name)
		if err != nil {
			log.Warn().Err(err).Str("stored_name", name).Msg("Failed to resolve attachment path for cleanup")
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Warn().Err(err).Str("path", path).Msg("Failed to delete contact attachment file")
		}
	}
}

// GetCircles returns all unique circles associated with contacts.
// GetCircles returns the authenticated user's Circles. After T4, this
// reads from the real `circles` table. When ?legacy=true, it reads from
// the legacy flat Contact.Circles JSON column instead — used by the
// T2 triage page during migration.
func GetCircles(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	if c.Query("legacy") == "true" {
		var circleNames []string
		err := db.Raw(`SELECT DISTINCT json_each.value AS circle
			FROM contacts, json_each(contacts.circles)
			WHERE contacts.user_id = ?`, userID).Scan(&circleNames).Error
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve legacy circles").WithError(err))
			return
		}
		c.JSON(http.StatusOK, circleNames)
		return
	}

	var circles []models.Circle
	if err := db.Where("user_id = ?", userID).Order("name").Find(&circles).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve circles").WithError(err))
		return
	}

	names := make([]string, len(circles))
	for i, circle := range circles {
		names[i] = circle.Name
	}
	c.JSON(http.StatusOK, names)
}

// ArchiveContact archives a contact and deletes all its reminders
func ArchiveContact(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	// Archive contact and delete reminders in a transaction
	err := db.Transaction(func(tx *gorm.DB) error {
		// N9: clear notification delivery state for this contact's reminders
		// before the (soft) reminder delete leaves the rows dangling.
		if err := tx.Where("reminder_id IN (SELECT id FROM reminders WHERE contact_id = ? AND user_id = ?)", id, userID).Delete(&models.NotificationDelivery{}).Error; err != nil {
			return err
		}
		// Delete all reminders for this contact
		if err := tx.Where("contact_id = ? AND user_id = ?", id, userID).Delete(&models.Reminder{}).Error; err != nil {
			return err
		}

		// Set archived to true
		if err := tx.Model(&contact).Update("archived", true).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to archive contact").WithError(err))
		return
	}

	contact.Archived = true
	c.JSON(http.StatusOK, contact)
}

// UnarchiveContact restores an archived contact
func UnarchiveContact(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	if err := db.Model(&contact).Update("archived", false).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to unarchive contact").WithError(err))
		return
	}

	contact.Archived = false
	c.JSON(http.StatusOK, contact)
}

// FavoriteContact marks a contact as a favorite (issue #173). Uses `Update`,
// not `UpdateColumn`, deliberately: `Update` fires BeforeSave/AfterSave,
// which bumps the ETag and records an audit event, so a favorite flip
// propagates through the T17 ?since= change feed and CardDAV sync — exactly
// what ArchiveContact already relies on.
func FavoriteContact(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	if err := db.Model(&contact).Update("is_favorite", true).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to favorite contact").WithError(err))
		return
	}

	contact.IsFavorite = true
	c.JSON(http.StatusOK, contact)
}

// UnfavoriteContact clears the favorite flag (issue #173). Same `Update`
// (hooks firing) discipline as FavoriteContact.
func UnfavoriteContact(c *gin.Context) {
	id, ok := requirePathUintID(c, "id")
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)

	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var contact models.Contact
	if err := db.Where("user_id = ?", userID).First(&contact, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err))
		}
		return
	}

	if err := db.Model(&contact).Update("is_favorite", false).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to unfavorite contact").WithError(err))
		return
	}

	contact.IsFavorite = false
	c.JSON(http.StatusOK, contact)
}
