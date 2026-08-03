package controllers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultPage  = 1
	defaultLimit = 25
	maxLimit     = 100
)

// PaginationParams represents sanitized pagination query values.
type PaginationParams struct {
	Page   int
	Limit  int
	Offset int
}

func currentUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("userID")
	if !exists {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Authentication required"))
		return 0, false
	}

	userID, ok := value.(uint)
	if !ok {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Authentication required"))
		return 0, false
	}

	return userID, true
}

func currentConfig(c *gin.Context) config.Config {
	if val, exists := c.Get("cfg"); exists {
		if cfg, ok := val.(config.Config); ok {
			return cfg
		}
	}
	return config.Config{}
}

// GetPaginationParams extracts pagination query params using shared defaults and bounds.
func GetPaginationParams(c *gin.Context) PaginationParams {
	page := parsePositiveOrDefault(c.DefaultQuery("page", "1"), defaultPage)
	limit := parsePositiveOrDefault(c.DefaultQuery("limit", "25"), defaultLimit)
	if limit > maxLimit {
		limit = maxLimit
	}

	return PaginationParams{
		Page:   page,
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
}

func parsePositiveOrDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

// ---------------------------------------------------------------------------
// Cursor pagination + change feeds (T17 — docs/fork-plan/tickets/
// 17-T17-change-feeds.md).
//
// Every list endpoint paginates with an opaque cursor instead of
// page/offset. The cursor is base64url("<RFC3339Nano UTC updated_at>|<id>")
// — a position in the total order (updated_at, id), which is stable under
// concurrent writes because `id` breaks ties that a bare `updated_at` would
// drop or duplicate at page boundaries. The shape is opaque to clients so it
// can change without another API-contract break.
// ---------------------------------------------------------------------------

// SyncMode describes how a collection is synced (T17's decided split).
const (
	// SyncModeIncremental collections are soft-deleted, support ?since= change
	// feeds, and normally drop `total` from their list response (exact counts are
	// expensive on the large, unboundedly-growing tables — contacts, notes,
	// activities, life_events). Preferences is a deliberate exception: it
	// soft-deletes and supports ?since= but is small enough that `total` stays
	// in browse mode.
	SyncModeIncremental = "incremental"
	// SyncModeFullResync collections are hard-deleted and bounded-small; a
	// client re-pulls them wholesale. `total` is cheap here and kept.
	SyncModeFullResync = "full_resync"
)

// IncrementalSyncCollections and FullResyncSyncCollections are the static
// sync-mode contract every list response carries, so a client can plan a
// sync without hardcoding which endpoints feed and which resync. Keep this
// in step with each entity's soft/hard-delete decision (see T17 and T26).
var (
	IncrementalSyncCollections = []string{
		"contacts", "notes", "activities", "life_events", "preferences",
		"cadence_policies", "conversation_agenda", "gifts",
	}
	FullResyncSyncCollections = []string{
		"circles", "households", "tags", "relationship_edges", "field_definitions",
	}
)

// buildSyncMeta returns the `sync` object every list response embeds: which
// sync mode THIS collection uses, plus the full static map of incremental vs
// full-resync collections so a client can plan a complete sync.
func buildSyncMeta(mode string) gin.H {
	return gin.H{
		"mode":        mode,
		"incremental": IncrementalSyncCollections,
		"full_resync": FullResyncSyncCollections,
	}
}

// Cursor is one position in the (updated_at, id) total order. ID is kept as
// its wire string and re-typed at each call site, since PK types differ
// (uint for gorm.Model entities, UUID strings for the others).
type Cursor struct {
	UpdatedAt time.Time
	ID        string
}

// EncodeCursor renders a cursor as an opaque base64url string. id may be any
// of the PK types (uint, uint64, string); it is rendered with fmt.Sprint.
//
// The timestamp is encoded in its OWN location, not normalized to UTC:
// SQLite stores DATETIME values as TEXT in the driver's format WITH the
// timezone offset (e.g. "2006-01-02 15:04:05.999999999-07:00"), and the
// cursor predicate compares that TEXT lexicographically. A cursor converted
// to UTC would bind as "+00:00" and sort on the wrong side of every stored
// local-time string — the exact bug this comment exists to prevent. Keeping
// the wall-clock + offset lets the round-tripped bound value match the
// stored text byte-for-byte.
func EncodeCursor(updatedAt time.Time, id any) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(updatedAt.Format(time.RFC3339Nano) + "|" + fmt.Sprint(id)),
	)
}

// DecodeCursor parses an opaque cursor back into its (updated_at, id)
// position. Returns an error for anything that is not a well-formed cursor.
func DecodeCursor(raw string) (*Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("cursor is not valid base64url")
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, errors.New("cursor is malformed")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("cursor timestamp is malformed")
	}
	return &Cursor{UpdatedAt: t, ID: parts[1]}, nil
}

// CursorParams is the sanitized set of pagination controls shared by every
// list handler.
type CursorParams struct {
	// Limit bounds the page size (default 25, max 100).
	Limit int
	// Order is "asc" or "desc" (default "desc"). The change feed (?since=)
	// always resolves to "asc" so a replica replays history forward.
	Order string
	// Cursor is the resume-after position supplied via ?cursor= or ?since=.
	Cursor *Cursor
	// Since is true when the request was a ?since= change-feed request.
	Since bool
}

// GetCursorParams extracts cursor pagination controls from the request,
// sharing defaults/bounds with the legacy offset parser above. A malformed
// ?cursor= or ?since= is an error (caller aborts with 400); both are
// exclusive-of-the-position "resume after this row" predicates.
func GetCursorParams(c *gin.Context) (CursorParams, *apperrors.AppError) {
	limit := parsePositiveOrDefault(c.DefaultQuery("limit", "25"), defaultLimit)
	if limit > maxLimit {
		limit = maxLimit
	}

	params := CursorParams{
		Limit: limit,
		Order: c.DefaultQuery("order", "desc"),
	}
	if params.Order != "asc" {
		params.Order = "desc"
	}

	if raw := c.Query("cursor"); raw != "" {
		cur, err := DecodeCursor(raw)
		if err != nil {
			return params, apperrors.ErrInvalidInput("cursor", err.Error())
		}
		params.Cursor = cur
	}
	if raw := c.Query("since"); raw != "" {
		cur, err := DecodeCursor(raw)
		if err != nil {
			return params, apperrors.ErrInvalidInput("since", err.Error())
		}
		params.Cursor = cur
		params.Since = true
	}
	// The change feed must always move forward: newest-page-first browsing is
	// only meaningful for a live list, not for replaying history.
	if params.Since {
		params.Order = "asc"
	}
	return params, nil
}

// CheckFeedCursorAge returns a 410 Gone error when a ?since= cursor predates
// the purge retention window (T26) — the sync horizon. Soft-deleted rows
// older than the window have been hard-deleted by the purge job, so a client
// resuming from such a cursor has permanently missed tombstones and can only
// converge via a full resync. The retention number here is deliberately the
// SAME config the purge job reads (config.Config.DeleteRetentionDays) so the
// two can never drift apart. Only ?since= (feed) cursors are checked: a
// plain ?cursor= is transient UI navigation, not long-lived sync state.
func CheckFeedCursorAge(c *gin.Context, params CursorParams) *apperrors.AppError {
	if !params.Since || params.Cursor == nil {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -currentConfig(c).DeleteRetentionDays)
	if params.Cursor.UpdatedAt.Before(cutoff) {
		return apperrors.ErrGone(
			"cursor is older than the retention window (DELETED_RETENTION_DAYS); " +
				"deleted rows have been purged — full resync required",
		)
	}
	return nil
}

// cursorPredicate returns the SQL row-value predicate that selects rows
// strictly after (asc) or strictly before (desc) the cursor position, using
// the (updated_at, id) feed key. id is passed already re-typed to the
// table's PK column type so SQLite's type ordering cannot miscompare (a TEXT
// "5" compares less than INTEGER 5).
func cursorPredicate(table string, cursor *Cursor, id any, desc bool) (string, any, any) {
	op := ">"
	if desc {
		op = "<"
	}
	return fmt.Sprintf("(%s.updated_at, %s.id) %s (?, ?)", table, table, op), cursor.UpdatedAt, id
}

// cursorOrderBy orders a query by the (updated_at, id) feed key in the given
// direction — the order that makes cursor pagination total and stable.
func cursorOrderBy(q *gorm.DB, table string, desc bool) *gorm.DB {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	return q.Order(table + ".updated_at " + dir).Order(table + ".id " + dir)
}

// parseCursorID re-types a cursor's string ID to a uint PK column value.
func parseCursorID(cursor *Cursor) (uint, bool) {
	id, err := strconv.ParseUint(cursor.ID, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}
