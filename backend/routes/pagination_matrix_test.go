package routes

// TestPaginationMatrix is issue #1475: the exhaustive proof that every
// cursor-paginated list route in the API walks correctly.
//
// Cursor pagination is the contract every client (web, Android offline sync,
// third-party API users) depends on, and it is a recurring correctness trap:
// duplicates/gaps when `updated_at` ties, `limit+1` truncation, descending
// order. Before this matrix, 14 of the 18 controllers that build a next-page
// cursor never executed the next-page branch under any test.
//
// It mirrors TestAuthorizationMatrix / TestConditionalWriteMatrix's shape —
// route-table driven, with a bidirectional completeness guard — but derives the
// route set from backend/openapi.yaml: every GET whose 200 response schema has a
// `next_cursor` property must have a pgSpec row here, and every GET with a
// `limit` query parameter but no `next_cursor` must be in pgExcluded with a
// recorded reason. A new paginated route therefore lands red until it is
// classified.
//
// ── What each row asserts (real migrated schema via dbtest, trap #1) ───────
//
//   - 33 rows are seeded for the caller (+ a second user's rows that must never
//     appear) with updated_at deliberately tied in blocks of 6, so every small
//     page boundary lands inside a tie group. ~1 in 5 rows of a soft-deletable
//     entity is soft-deleted.
//   - ?cursor= (live list): pages of 2 / 7 / default (25) / exactly-len /
//     len-1, asc and desc, must reproduce the expected (key, id) order exactly
//     — which proves union == seeded set, no duplicates, no gaps — with a
//     non-empty next_cursor on every page but the last, only full pages before
//     the last, and soft-deleted rows excluded.
//   - ?since= (change feed, soft-deletable entities): forced ascending even when
//     order=desc is sent, includes every tombstone exactly once with
//     deleted:true and no live row flagged.
//   - The same walks again with every timestamp in a non-UTC offset (see
//     controllers.EncodeCursor's comment on why the offset matters).
//   - A concurrent insert between page 1 and page 2 (newer than everything,
//     older than everything, and tied with the cursor row) never duplicates or
//     skips a pre-existing row, and the new rows appear exactly when the
//     (key, id) order says they should.
//   - Tampered cursors are 400 with the standard error envelope; a foreign
//     user's or another endpoint's cursor is 200 with only the caller's rows
//     or a 400, never a 500 and never another user's data; a ?since= cursor
//     older than DELETED_RETENTION_DAYS is 410.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/controllers"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	pgSeedRows       = 33 // 33 rows: 7 tombstones + 26 live for soft-deletable entities
	pgTieGroup       = 6  // rows sharing one updated_at
	pgRetentionDays  = 30
	pgDefaultLimit   = 25
	pgMaxPagesMargin = 5
)

type pgKind int

const (
	pgTime     pgKind = iota // (updated_at, id)
	pgName                   // (sort_name, id) — GET /contacts?sort=name
	pgPosition               // (position, id) — GET /field-definitions
)

// pgFix is one isolated fixture: its own users + contacts, so each spec/variant
// sees only the rows it seeded.
type pgFix struct {
	t       *testing.T
	db      *gorm.DB
	router  http.Handler
	owner   uint
	victim  uint
	other   uint
	ownerC  models.Contact
	token   string
	base    time.Time
	counter int
}

// pgSeed parameterizes one row creation. Every key component is explicit so
// the harness controls ties.
type pgSeed struct {
	n     int
	ts    time.Time
	pos   int
	first string // last name; DeriveSortName prefers it, so it fixes sort_name
}

type pgRow struct {
	id       string
	ts       time.Time
	pos      int
	sortName string
	deleted  bool
}

type pgSpec struct {
	name       string
	oapi       string // OpenAPI path template, e.g. "/contacts/{id}/notes"
	path       func(f *pgFix) string
	query      string // fixed extra query (no leading &), e.g. "sort=name"
	key        string // JSON array key in the response
	idKey      string
	numeric    bool // uint PK (gorm.Model) vs UUID string PK
	kind       pgKind
	table      string
	since      bool // the route has the ?since= change feed
	softDelete bool
	create     func(f *pgFix, userID uint, s pgSeed) string
}

func (f *pgFix) newUID(prefix string) string {
	f.counter++
	return fmt.Sprintf("%s-%d-%s", prefix, f.counter, uuid.NewString()[:8])
}

func pgMustCreate(t *testing.T, db *gorm.DB, v any) {
	t.Helper()
	require.NoError(t, db.Create(v).Error)
}

func pgSpecs() []pgSpec {
	uidPath := func(p string) func(*pgFix) string { return func(*pgFix) string { return p } }
	contactScoped := func(suffix string) func(*pgFix) string {
		return func(f *pgFix) string {
			return fmt.Sprintf("/api/v1/contacts/%d/%s", f.ownerC.ID, suffix)
		}
	}
	idStr := func(id uint) string { return strconv.FormatUint(uint64(id), 10) }

	newActivity := func(f *pgFix, userID uint, s pgSeed, join bool) string {
		a := models.Activity{UserID: userID, Title: fmt.Sprintf("a%d", s.n), Date: s.ts}
		pgMustCreate(f.t, f.db, &a)
		if join {
			require.NoError(f.t, f.db.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", a.ID, f.ownerC.ID).Error)
		}
		return idStr(a.ID)
	}
	entity := func(f *pgFix) string { return f.ownerC.VCardUID }

	return []pgSpec{
		{
			name: "contacts", oapi: "/contacts", path: uidPath("/api/v1/contacts"), key: "contacts", idKey: "id",
			numeric: true, kind: pgTime, table: "contacts", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				c := models.Contact{UserID: userID, Firstname: "Fn", Lastname: s.first}
				pgMustCreate(f.t, f.db, &c)
				return idStr(c.ID)
			},
		},
		{
			name: "contacts-sort-name", oapi: "/contacts", path: uidPath("/api/v1/contacts"), query: "sort=name", key: "contacts", idKey: "id",
			numeric: true, kind: pgName, table: "contacts", softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				c := models.Contact{UserID: userID, Firstname: "Fn", Lastname: s.first}
				pgMustCreate(f.t, f.db, &c)
				return idStr(c.ID)
			},
		},
		{
			name: "contact-notes", oapi: "/contacts/{id}/notes", path: contactScoped("notes"), key: "notes", idKey: "ID",
			numeric: true, kind: pgTime, table: "notes", softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				n := models.Note{UserID: userID, Content: fmt.Sprintf("n%d", s.n), Date: s.ts, ContactID: &f.ownerC.ID}
				pgMustCreate(f.t, f.db, &n)
				return idStr(n.ID)
			},
		},
		{
			name: "unassigned-notes", oapi: "/notes", path: uidPath("/api/v1/notes"), key: "notes", idKey: "ID",
			numeric: true, kind: pgTime, table: "notes", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				n := models.Note{UserID: userID, Content: fmt.Sprintf("n%d", s.n), Date: s.ts}
				pgMustCreate(f.t, f.db, &n)
				return idStr(n.ID)
			},
		},
		{
			name: "contact-activities", oapi: "/contacts/{id}/activities", path: contactScoped("activities"), key: "activities", idKey: "ID",
			numeric: true, kind: pgTime, table: "activities", softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string { return newActivity(f, userID, s, true) },
		},
		{
			name: "activities", oapi: "/activities", path: uidPath("/api/v1/activities"), key: "activities", idKey: "ID",
			numeric: true, kind: pgTime, table: "activities", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string { return newActivity(f, userID, s, false) },
		},
		{
			name: "circles", oapi: "/circles", path: uidPath("/api/v1/circles"), key: "circles", idKey: "id", kind: pgTime, table: "circles",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				c := models.Circle{UserID: userID, Name: f.newUID("circle")}
				pgMustCreate(f.t, f.db, &c)
				return c.ID
			},
		},
		{
			name: "households", oapi: "/households", path: uidPath("/api/v1/households"), key: "households", idKey: "id", kind: pgTime, table: "households",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				h := models.Household{UserID: userID, Name: f.newUID("hh"), Type: models.HouseholdTypeOther}
				pgMustCreate(f.t, f.db, &h)
				return h.ID
			},
		},
		{
			name: "tags", oapi: "/tags", path: uidPath("/api/v1/tags"), key: "tags", idKey: "id", kind: pgTime, table: "tags",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				tg := models.Tag{UserID: userID, Name: f.newUID("tag")}
				pgMustCreate(f.t, f.db, &tg)
				return tg.ID
			},
		},
		{
			name: "field-definitions", oapi: "/field-definitions", path: uidPath("/api/v1/field-definitions"), key: "field_definitions", idKey: "id",
			kind: pgPosition, table: "field_definitions",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				fd := models.FieldDefinition{
					UserID: userID, Label: "f", Key: f.newUID("k"), Target: models.FieldDefinitionTargetContact,
					Type: models.FieldTypeString, Projection: "internal-only", Sensitivity: models.RelationshipSensitivityNormal,
					Position: s.pos,
				}
				pgMustCreate(f.t, f.db, &fd)
				return fd.ID
			},
		},
		{
			name: "relationship-edges", oapi: "/relationship-edges", path: uidPath("/api/v1/relationship-edges"), key: "relationship_edges", idKey: "id",
			kind: pgTime, table: "relationship_edges",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				e := models.RelationshipEdge{
					UserID: userID, SourceID: uuid.NewString(), TargetID: uuid.NewString(), Type: "friend_of",
					Source: models.RelationshipSourceUserConfirmed, Confidence: 1, Status: models.RelationshipStatusConfirmed,
				}
				pgMustCreate(f.t, f.db, &e)
				return e.ID
			},
		},
		{
			name: "contact-shares-incoming", oapi: "/contact-shares/incoming", path: uidPath("/api/v1/contact-shares/incoming"), key: "contact_shares", idKey: "id",
			kind: pgTime, table: "contact_shares",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				sh := models.ContactShare{FromUserID: f.other, ToUserID: userID, ContactDisplayName: "x", Payload: "{}", Status: "pending"}
				pgMustCreate(f.t, f.db, &sh)
				return sh.ID
			},
		},
		{
			name: "contact-shares-outgoing", oapi: "/contact-shares/outgoing", path: uidPath("/api/v1/contact-shares/outgoing"), key: "contact_shares", idKey: "id",
			kind: pgTime, table: "contact_shares",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				sh := models.ContactShare{FromUserID: userID, ToUserID: f.other, ContactDisplayName: "x", Payload: "{}", Status: "pending"}
				pgMustCreate(f.t, f.db, &sh)
				return sh.ID
			},
		},
		{
			name: "life-events", oapi: "/life-events", path: uidPath("/api/v1/life-events"), key: "life_events", idKey: "id",
			kind: pgTime, table: "life_events", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.LifeEvent{UserID: userID, EntityID: entity(f), Type: models.LifeEventTypeMoved}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "conversation-agenda", oapi: "/conversation-agenda", path: uidPath("/api/v1/conversation-agenda"), key: "conversation_agenda", idKey: "id",
			kind: pgTime, table: "conversation_agenda", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.ConversationAgenda{UserID: userID, EntityID: entity(f), Content: f.newUID("item")}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "gifts", oapi: "/gifts", path: uidPath("/api/v1/gifts"), key: "gifts", idKey: "id",
			kind: pgTime, table: "gifts", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.Gift{UserID: userID, EntityID: entity(f), Description: f.newUID("gift")}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "preferences", oapi: "/preferences", path: uidPath("/api/v1/preferences"), key: "preferences", idKey: "id",
			kind: pgTime, table: "preferences", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.Preference{UserID: userID, EntityID: entity(f), Category: "food", Value: f.newUID("pref")}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "occasion-obligations", oapi: "/occasion-obligations", path: uidPath("/api/v1/occasion-obligations"), key: "occasion_obligations", idKey: "id",
			kind: pgTime, table: "occasion_obligations", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.OccasionObligation{UserID: userID, EntityID: entity(f), Kind: "card", Label: f.newUID("ob")}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "occasion-events", oapi: "/occasion-events", path: uidPath("/api/v1/occasion-events"), key: "occasion_events", idKey: "id",
			kind: pgTime, table: "occasion_events", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.OccasionEvent{UserID: userID, Title: f.newUID("ev"), StartsAt: s.ts}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "cadence-policies", oapi: "/cadence-policies", path: uidPath("/api/v1/cadence-policies"), key: "cadence_policies", idKey: "id",
			kind: pgTime, table: "cadence_policies", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.CadencePolicy{UserID: userID, EntityID: uuid.NewString(), TargetIntervalDays: 30}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "data-decay-policies", oapi: "/data-decay-policies", path: uidPath("/api/v1/data-decay-policies"), key: "data_decay_policies", idKey: "id",
			kind: pgTime, table: "data_decay_policies", since: true, softDelete: true,
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.DataDecayPolicy{UserID: userID, EntityID: uuid.NewString(), IntervalDays: 365}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "external-identities", oapi: "/external-identities", path: uidPath("/api/v1/external-identities"), key: "external_identities", idKey: "id",
			kind: pgTime, table: "external_identities",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.ExternalIdentity{UserID: userID, EntityID: entity(f), System: "immich", ExternalID: f.newUID("p")}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
		{
			name: "external-activities", oapi: "/external-activities", path: uidPath("/api/v1/external-activities"), key: "external_activities", idKey: "id",
			kind: pgTime, table: "external_activities",
			create: func(f *pgFix, userID uint, s pgSeed) string {
				v := models.ExternalActivity{UserID: userID, EntityID: entity(f), SourceSystem: "immich", ExternalID: f.newUID("a"), Type: "photo-appearance", OccurredAt: s.ts}
				pgMustCreate(f.t, f.db, &v)
				return v.ID
			},
		},
	}
}

// pgExcluded lists the GET routes with a `limit` query parameter that are NOT
// next_cursor-paginated (or are paginated by a different, separately tested
// mechanism), each with the recorded reason. Keyed by OpenAPI path.
var pgExcluded = map[string]string{
	"/contacts/{id}/timeline": "composite timeline cursor over several entity kinds; walked exhaustively by controllers/timeline_controller_test.go (walkTimeline)",
	"/contacts/duplicates":    "offset-paginated (page+limit) read-model, no next_cursor",
	"/search":                 "top-N search results bounded by limit, no cursor",
	"/audit":                  "bounded most-recent-N audit read, no cursor",
	"/admin/system-events":    "bounded most-recent-N admin read, no cursor",
	"/admin/job-runs":         "bounded most-recent-N admin read, no cursor",
	"/admin/users":            "offset-paginated (page+limit) admin list, no next_cursor",
}

// ── OpenAPI-derived route discovery ───────────────────────────────────────

type pgOpenAPIRoute struct {
	hasNextCursor bool
	hasLimit      bool
	hasSince      bool
}

func pgLoadOpenAPI(t *testing.T) map[string]pgOpenAPIRoute {
	t.Helper()
	raw, err := os.ReadFile("../openapi.yaml")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))

	var resolve func(v any) map[string]any
	resolve = func(v any) map[string]any {
		m, _ := v.(map[string]any)
		for m != nil {
			ref, ok := m["$ref"].(string)
			if !ok {
				return m
			}
			var cur any = doc
			for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
				next, _ := cur.(map[string]any)
				cur = next[part]
			}
			m, _ = cur.(map[string]any)
		}
		return m
	}
	var hasProp func(schema map[string]any, name string) bool
	hasProp = func(schema map[string]any, name string) bool {
		schema = resolve(schema)
		if schema == nil {
			return false
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			if _, ok := props[name]; ok {
				return true
			}
		}
		if all, ok := schema["allOf"].([]any); ok {
			for _, s := range all {
				if hasProp(resolve(s), name) {
					return true
				}
			}
		}
		return false
	}

	out := map[string]pgOpenAPIRoute{}
	paths, _ := doc["paths"].(map[string]any)
	for p, item := range paths {
		itemMap, _ := item.(map[string]any)
		get := resolve(itemMap["get"])
		if get == nil {
			continue
		}
		var r pgOpenAPIRoute
		params := []any{}
		if ps, ok := itemMap["parameters"].([]any); ok {
			params = append(params, ps...)
		}
		if ps, ok := get["parameters"].([]any); ok {
			params = append(params, ps...)
		}
		for _, raw := range params {
			switch resolve(raw)["name"] {
			case "limit":
				r.hasLimit = true
			case "since":
				r.hasSince = true
			}
		}
		resp, _ := get["responses"].(map[string]any)
		ok200 := resolve(resp["200"])
		content, _ := ok200["content"].(map[string]any)
		js, _ := content["application/json"].(map[string]any)
		if js != nil {
			schema, _ := js["schema"].(map[string]any)
			r.hasNextCursor = hasProp(schema, "next_cursor")
		}
		out[p] = r
	}
	return out
}

// ── fixture + HTTP helpers ────────────────────────────────────────────────

type pgEnv struct {
	t      *testing.T
	db     *gorm.DB
	router http.Handler
	cfg    *config.Config
	seq    int
}

// fixFor builds a fixture for sp. The anchor contact (needed as the entity /
// parent of other specs' rows) is removed for the contacts list specs so it
// does not pollute the expected set.
func (e *pgEnv) fixFor(sp pgSpec) *pgFix {
	f := e.newFix()
	if sp.table == "contacts" {
		require.NoError(e.t, e.db.Exec("DELETE FROM contacts WHERE id = ?", f.ownerC.ID).Error)
	}
	return f
}

func (e *pgEnv) newFix() *pgFix {
	e.t.Helper()
	e.seq++
	mk := func(name string) models.User {
		u := models.User{Username: fmt.Sprintf("pg-%s-%d", name, e.seq), Email: fmt.Sprintf("pg-%s-%d@example.com", name, e.seq), Password: "password123"}
		require.NoError(e.t, e.db.Create(&u).Error)
		return u
	}
	owner, victim, other := mk("owner"), mk("victim"), mk("other")
	f := &pgFix{t: e.t, db: e.db, router: e.router, owner: owner.ID, victim: victim.ID, other: other.ID}
	f.ownerC = models.Contact{UserID: owner.ID, Firstname: "Anchor"}
	require.NoError(e.t, e.db.Create(&f.ownerC).Error)
	tok, err := services.IssueSession(e.db, owner, e.cfg, "", "")
	require.NoError(e.t, err)
	f.token = tok
	f.base = time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	return f
}

func (f *pgFix) get(path string) (int, string) {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = uniqueTestClientIP() + ":1234"
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func pgEnc(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// ── seeding ───────────────────────────────────────────────────────────────

func (f *pgFix) insert(sp pgSpec, userID uint, s pgSeed) pgRow {
	f.t.Helper()
	id := sp.create(f, userID, s)
	var idArg any = id
	if sp.numeric {
		n, err := strconv.ParseUint(id, 10, 64)
		require.NoError(f.t, err)
		idArg = n
	}
	require.NoError(f.t, f.db.Exec("UPDATE "+sp.table+" SET updated_at = ? WHERE id = ?", s.ts, idArg).Error)
	row := pgRow{id: id, ts: s.ts, pos: s.pos}
	if sp.kind == pgName {
		require.NoError(f.t, f.db.Raw("SELECT sort_name FROM contacts WHERE id = ?", idArg).Scan(&row.sortName).Error)
	}
	return row
}

func (f *pgFix) tombstone(sp pgSpec, r *pgRow) {
	f.t.Helper()
	var idArg any = r.id
	if sp.numeric {
		n, _ := strconv.ParseUint(r.id, 10, 64)
		idArg = n
	}
	require.NoError(f.t, f.db.Exec("UPDATE "+sp.table+" SET deleted_at = ?, updated_at = ? WHERE id = ?", r.ts, r.ts, idArg).Error)
	r.deleted = true
}

func (f *pgFix) seedFor(sp pgSpec, userID uint, zone *time.Location) []pgRow {
	f.t.Helper()
	rows := make([]pgRow, 0, pgSeedRows)
	for i := 0; i < pgSeedRows; i++ {
		g := i / pgTieGroup
		r := f.insert(sp, userID, pgSeed{
			n:     i,
			ts:    f.base.Add(time.Duration(g) * time.Second).In(zone),
			pos:   g,
			first: fmt.Sprintf("Pat%02d", g),
		})
		if sp.softDelete && i%5 == 2 {
			f.tombstone(sp, &r)
		}
		rows = append(rows, r)
	}
	return rows
}

// seedVictim creates rows for the second user that must never be visible to
// the owner — one live, one tombstone where the entity soft-deletes.
func (f *pgFix) seedVictim(sp pgSpec, zone *time.Location) []pgRow {
	f.t.Helper()
	rows := []pgRow{f.insert(sp, f.victim, pgSeed{n: 900, ts: f.base.Add(2 * time.Second).In(zone), pos: 1, first: "Pat01"})}
	if sp.softDelete {
		r := f.insert(sp, f.victim, pgSeed{n: 901, ts: f.base.Add(3 * time.Second).In(zone), pos: 2, first: "Pat02"})
		f.tombstone(sp, &r)
		rows = append(rows, r)
	}
	return rows
}

// ── expectation + walking ─────────────────────────────────────────────────

func pgIDLess(a, b string, numeric bool) bool {
	if numeric {
		x, _ := strconv.ParseUint(a, 10, 64)
		y, _ := strconv.ParseUint(b, 10, 64)
		return x < y
	}
	return a < b
}

func pgLess(sp pgSpec, a, b pgRow) bool {
	switch sp.kind {
	case pgTime:
		if !a.ts.Equal(b.ts) {
			return a.ts.Before(b.ts)
		}
	case pgName:
		if a.sortName != b.sortName {
			return a.sortName < b.sortName
		}
	case pgPosition:
		if a.pos != b.pos {
			return a.pos < b.pos
		}
	}
	return pgIDLess(a.id, b.id, sp.numeric)
}

// pgExpected is the oracle: the rows a mode must return, in order.
func pgExpected(sp pgSpec, rows []pgRow, desc, feed bool) []pgRow {
	var out []pgRow
	for _, r := range rows {
		if feed || !r.deleted {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if desc && !feed {
			return pgLess(sp, out[j], out[i])
		}
		return pgLess(sp, out[i], out[j])
	})
	return out
}

type pgPage struct {
	status  int
	body    string
	ids     []string
	deleted map[string]bool
	hasDel  map[string]bool // item carried the "deleted" key at all
	next    string
	limit   int
}

func (f *pgFix) page(sp pgSpec, url string) pgPage {
	f.t.Helper()
	st, body := f.get(url)
	p := pgPage{status: st, body: body, deleted: map[string]bool{}, hasDel: map[string]bool{}}
	if st != http.StatusOK {
		return p
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var top map[string]any
	require.NoError(f.t, dec.Decode(&top), body)
	next, _ := top["next_cursor"].(string)
	p.next = next
	if l, ok := top["limit"].(json.Number); ok {
		n, _ := l.Int64()
		p.limit = int(n)
	}
	items, ok := top[sp.key].([]any)
	require.Truef(f.t, ok, "%s: response has no array %q: %s", url, sp.key, body)
	for _, it := range items {
		m := it.(map[string]any)
		id := fmt.Sprint(m[sp.idKey])
		if n, ok := m[sp.idKey].(json.Number); ok {
			id = n.String()
		}
		p.ids = append(p.ids, id)
		if d, ok := m["deleted"]; ok {
			p.hasDel[id] = true
			p.deleted[id] = d == true
		}
	}
	return p
}

func (f *pgFix) url(sp pgSpec, limit int, order, cursorParam, cursor string) string {
	q := []string{}
	if sp.query != "" {
		q = append(q, sp.query)
	}
	if limit > 0 {
		q = append(q, "limit="+strconv.Itoa(limit))
	}
	if order != "" {
		q = append(q, "order="+order)
	}
	if cursor != "" {
		q = append(q, cursorParam+"="+cursor)
	}
	u := sp.path(f)
	if len(q) > 0 {
		u += "?" + strings.Join(q, "&")
	}
	return u
}

// walk follows next_cursor to the end and returns every page's ids, flattened,
// plus the page count and the tombstone map. It asserts the structural page
// invariants (full pages before the last, non-empty cursor exactly between
// pages).
func (f *pgFix) walk(sp pgSpec, limit int, order string, feed bool, start string) (ids []string, tomb map[string]bool, live map[string]bool, pages int) {
	f.t.Helper()
	eff := limit
	if eff == 0 {
		eff = pgDefaultLimit
	}
	param := "cursor"
	if feed {
		param = "since"
	}
	tomb, live = map[string]bool{}, map[string]bool{}
	cursor := start
	for {
		pages++
		require.LessOrEqualf(f.t, pages, pgSeedRows+pgMaxPagesMargin, "walk did not terminate (cursor loop?) for %s", sp.name)
		p := f.page(sp, f.url(sp, limit, order, param, cursor))
		require.Equalf(f.t, http.StatusOK, p.status, "%s page %d: %s", sp.name, pages, p.body)
		require.LessOrEqual(f.t, len(p.ids), eff, "page larger than limit")
		if p.limit != 0 {
			require.Equal(f.t, eff, p.limit, "response limit echo")
		}
		// A next_cursor must never lead to an empty page: that is the
		// `>=` / `limit` (instead of `limit+1`) truncation bug, where a final
		// page of exactly `limit` rows advertises a page that does not exist.
		require.NotEmptyf(f.t, p.ids, "%s page %d is empty: the previous page advertised a next_cursor with no rows behind it", sp.name, pages)
		ids = append(ids, p.ids...)
		for id := range p.deleted {
			if p.deleted[id] {
				tomb[id] = true
			} else {
				live[id] = true
			}
		}
		for _, id := range p.ids {
			if !p.hasDel[id] {
				live[id] = true
			}
		}
		if p.next == "" {
			return
		}
		require.Lenf(f.t, p.ids, eff, "%s page %d has a next_cursor but is not a full page", sp.name, pages)
		cursor = p.next
	}
}

func pgIDs(rows []pgRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.id
	}
	return out
}

// ── the matrix ────────────────────────────────────────────────────────────

func TestPaginationMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := dbtest.New(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	cfg := &config.Config{
		JWTSecretKey:        "pagination-matrix-test-secret-key-that-is-long-enough",
		JWTExpiryHours:      96,
		ProfilePhotoDir:     t.TempDir(),
		FrontendURL:         "http://localhost:5173",
		Port:                "7300",
		ReminderTime:        "12:00",
		ReminderTimezone:    "UTC",
		DeleteRetentionDays: pgRetentionDays,
	}
	middleware.ConfigureAPIRateLimiter(time.Microsecond, 1_000_000)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	RegisterRoutes(router, cfg, db, nil)
	env := &pgEnv{t: t, db: db, router: router, cfg: cfg}

	specs := pgSpecs()

	t.Run("completeness", func(t *testing.T) {
		oa := pgLoadOpenAPI(t)
		covered := map[string]bool{}
		for _, sp := range specs {
			covered[sp.oapi] = true
			r, ok := oa[sp.oapi]
			require.Truef(t, ok && r.hasNextCursor, "spec %q: %s is not a next_cursor GET route in openapi.yaml (stale row?)", sp.name, sp.oapi)
			if sp.kind != pgName {
				require.Equalf(t, r.hasSince, sp.since, "spec %q: since=%v but openapi says the route's ?since= param is %v", sp.name, sp.since, r.hasSince)
			}
		}
		for p, r := range oa {
			if r.hasNextCursor && !covered[p] {
				reason, ok := pgExcluded[p]
				require.Truef(t, ok && reason != "", "GET %s returns next_cursor but has no pgSpec row — add one (or an explicit reasoned pgExcluded entry if another test walks it)", p)
			} else if !r.hasNextCursor && r.hasLimit && !covered[p] {
				reason, ok := pgExcluded[p]
				require.Truef(t, ok && reason != "", "GET %s has a `limit` query param but is neither in the pagination matrix nor in pgExcluded with a reason", p)
			}
		}
		for p, reason := range pgExcluded {
			if reason == "" {
				continue
			}
			_, ok := oa[p]
			require.Truef(t, ok, "pgExcluded entry %q matches no GET route in openapi.yaml (stale?)", p)
			require.Falsef(t, covered[p], "pgExcluded entry %q is also covered by a spec", p)
		}
	})

	for _, sp := range specs {
		t.Run(sp.name, func(t *testing.T) {
			e := &pgEnv{t: t, db: env.db, router: env.router, cfg: env.cfg, seq: env.seq * 1000}
			env.seq++
			runPaginationSpec(t, e, sp)
		})
	}
}

func runPaginationSpec(t *testing.T, e *pgEnv, sp pgSpec) {
	zones := map[string]*time.Location{
		"utc":    time.UTC,
		"offset": time.FixedZone("+0530", 5*3600+1800),
	}
	for _, zname := range []string{"utc", "offset"} {
		zone := zones[zname]
		t.Run(zname, func(t *testing.T) {
			e.t = t
			f := e.fixFor(sp)
			rows := f.seedFor(sp, f.owner, zone)
			victim := f.seedVictim(sp, zone)
			victimIDs := map[string]bool{}
			for _, v := range victim {
				victimIDs[v.id] = true
			}

			assertWalk := func(name string, limit int, order string, wantPages int) {
				t.Run(name, func(t *testing.T) {
					f.t = t
					want := pgExpected(sp, rows, order == "desc", false)
					got, tomb, _, pages := f.walk(sp, limit, order, false, "")
					require.Equal(t, pgIDs(want), got, "pages must reproduce the documented (key,id) order exactly: no duplicates, no gaps")
					require.Empty(t, tomb, "the live list must exclude soft-deleted rows")
					for _, id := range got {
						require.Falsef(t, victimIDs[id], "another user's row %s leaked", id)
					}
					if wantPages > 0 {
						require.Equal(t, wantPages, pages)
					}
					if limit == 0 {
						require.Equal(t, (len(want)+pgDefaultLimit-1)/pgDefaultLimit, pages, "default limit page count")
					} else {
						require.Equal(t, (len(want)+limit-1)/limit, pages)
					}
				})
			}
			live := len(pgExpected(sp, rows, false, false))
			for _, order := range []string{"asc", "desc"} {
				assertWalk("limit2-"+order, 2, order, 0)
				assertWalk("limit7-"+order, 7, order, 0)
				assertWalk("default-"+order, 0, order, 0)
				assertWalk("limit-eq-total-"+order, live, order, 1)
				assertWalk("limit-total-minus-1-"+order, live-1, order, 2)
			}

			t.Run("default-order-is-documented", func(t *testing.T) {
				f.t = t
				p := f.page(sp, f.url(sp, 3, "", "", ""))
				require.Equal(t, http.StatusOK, p.status, p.body)
				desc := sp.kind != pgPosition // newest-first for time/name lists; position is display order
				if sp.kind == pgName {
					desc = true
				}
				want := pgIDs(pgExpected(sp, rows, desc, false))
				require.Equal(t, want[:3], p.ids)
			})

			if sp.since {
				start := controllers.EncodeCursor(f.base.Add(-2*time.Hour).In(zone), "0")
				total := len(pgExpected(sp, rows, false, true))
				for _, limit := range []int{2, 0, total, total - 1} {
					for _, order := range []string{"", "desc"} {
						t.Run(fmt.Sprintf("feed-limit%d-order%q", limit, order), func(t *testing.T) {
							f.t = t
							want := pgExpected(sp, rows, false, true)
							got, tomb, liveSeen, pages := f.walk(sp, limit, order, true, start)
							effLimit := limit
							if effLimit == 0 {
								effLimit = pgDefaultLimit
							}
							require.Equal(t, (len(want)+effLimit-1)/effLimit, pages, "feed page count")
							require.Equal(t, pgIDs(want), got, "the feed must be ascending, complete and duplicate-free regardless of order=")
							wantTomb := 0
							for _, r := range want {
								if r.deleted {
									wantTomb++
									require.Truef(t, tomb[r.id], "tombstone %s missing deleted:true", r.id)
								} else {
									require.Truef(t, liveSeen[r.id], "live row %s wrongly flagged deleted", r.id)
								}
							}
							require.Len(t, tomb, wantTomb)
							require.Positive(t, wantTomb)
							for _, id := range got {
								require.False(t, victimIDs[id], "another user's row leaked into the feed")
							}
						})
					}
				}
			}

			if zname != "utc" {
				return
			}

			t.Run("tampered-cursors", func(t *testing.T) {
				f.t = t
				params := []string{"cursor"}
				if sp.since {
					params = append(params, "since")
				}
				bad := map[string]string{
					"not-base64":       "!!!not*base64",
					"no-separator":     pgEnc("nopipe"),
					"bad-timestamp":    pgEnc("not-a-time|1"),
					"empty-timestamp":  pgEnc("|1"),
					"empty-id":         pgEnc("2026-03-01T12:00:00Z|"),
					"truncated-base64": pgEnc("2026-03-01T12:00:00Z|1")[:5],
				}
				if sp.kind == pgName {
					// "not-a-time|1" is a perfectly good (sort_name, id) pair.
					delete(bad, "bad-timestamp")
				}
				if sp.numeric {
					bad["non-numeric-id"] = controllers.EncodeCursor(f.base, "abc")
				}
				for _, param := range params {
					for name, c := range bad {
						st, body := f.get(f.url(sp, 5, "", param, c))
						require.Equalf(t, http.StatusBadRequest, st, "%s=%s (%s): %s", param, name, c, body)
						code, msg := ownParseErr(body)
						require.NotEmptyf(t, code, "%s=%s: error envelope has no code: %s", param, name, body)
						require.NotEmptyf(t, msg, "%s=%s: error envelope has no message: %s", param, name, body)
					}
				}
			})

			t.Run("foreign-and-cross-endpoint-cursors", func(t *testing.T) {
				f.t = t
				owned := map[string]bool{}
				for _, r := range rows {
					owned[r.id] = true
				}
				cursors := map[string]string{}
				v := victim[0]
				switch sp.kind {
				case pgPosition:
					cursors["victim"] = controllers.EncodePositionCursor(v.pos, v.id)
					cursors["time-cursor-from-another-endpoint"] = controllers.EncodeCursor(f.base, uuid.NewString())
				case pgName:
					cursors["victim"] = controllers.EncodeNameCursor(v.sortName, v.id)
					cursors["numeric-id-from-another-endpoint"] = controllers.EncodeNameCursor("pat00", "99999999")
					cursors["uuid-id-from-another-endpoint"] = controllers.EncodeNameCursor("pat00", uuid.NewString())
				default:
					cursors["victim"] = controllers.EncodeCursor(v.ts, v.id)
					cursors["numeric-id-from-another-endpoint"] = controllers.EncodeCursor(f.base, "99999999")
					cursors["uuid-id-from-another-endpoint"] = controllers.EncodeCursor(f.base, uuid.NewString())
					cursors["position-cursor-from-another-endpoint"] = controllers.EncodePositionCursor(3, uuid.NewString())
					cursors["name-cursor-from-another-endpoint"] = controllers.EncodeNameCursor("pat01", "5")
				}
				for name, c := range cursors {
					for _, param := range []string{"cursor", "since"} {
						if param == "since" && !sp.since {
							continue
						}
						for _, order := range []string{"asc", "desc"} {
							st, body := f.get(f.url(sp, 100, order, param, c))
							require.Containsf(t, []int{http.StatusOK, http.StatusBadRequest}, st, "%s %s %s -> %d (never a 5xx): %s", name, param, order, st, body)
							if st == http.StatusBadRequest {
								code, _ := ownParseErr(body)
								require.NotEmpty(t, code, "400 must carry the standard error envelope: "+body)
								continue
							}
							p := f.page(sp, f.url(sp, 100, order, param, c))
							for _, id := range p.ids {
								require.Truef(t, owned[id], "%s %s: returned %s which is not the caller's row", name, param, id)
								require.Falsef(t, victimIDs[id], "%s %s: leaked the other user's row", name, param)
							}
						}
					}
				}
			})

			if sp.since {
				t.Run("feed-cursor-older-than-retention-is-410", func(t *testing.T) {
					f.t = t
					old := controllers.EncodeCursor(time.Now().AddDate(0, 0, -(pgRetentionDays+5)), "1")
					st, body := f.get(f.url(sp, 5, "", "since", old))
					require.Equal(t, http.StatusGone, st, body)
					code, msg := ownParseErr(body)
					require.NotEmpty(t, code)
					require.Contains(t, msg, "DELETED_RETENTION_DAYS")

					// Inside the window is fine.
					fresh := controllers.EncodeCursor(time.Now().AddDate(0, 0, -(pgRetentionDays-5)), "0")
					st, body = f.get(f.url(sp, 5, "", "since", fresh))
					require.Equal(t, http.StatusOK, st, body)

					// A plain ?cursor= is navigation, not sync state: never 410.
					st, body = f.get(f.url(sp, 5, "", "cursor", old))
					require.Equal(t, http.StatusOK, st, body)
				})
			}

			if sp.kind == pgName {
				t.Run("name-sort-cannot-combine-with-since", func(t *testing.T) {
					f.t = t
					st, body := f.get(sp.path(f) + "?sort=name&since=" + controllers.EncodeCursor(f.base, "0"))
					require.Equal(t, http.StatusBadRequest, st, body)
				})
				t.Run("time-cursor-rejected-for-name-sort", func(t *testing.T) {
					f.t = t
					st, body := f.get(f.url(sp, 5, "", "cursor", controllers.EncodeCursor(f.base, "1")))
					require.Equal(t, http.StatusBadRequest, st, body)
				})
			}

			// Concurrent inserts between page 1 and page 2.
			concurrent := []string{"asc", "desc"}
			if sp.since {
				concurrent = append(concurrent, "feed")
			}
			for _, mode := range concurrent {
				t.Run("concurrent-insert-"+mode, func(t *testing.T) {
					e.t = t
					cf := e.fixFor(sp)
					crows := cf.seedFor(sp, cf.owner, time.UTC)
					cf.concurrentInsert(sp, crows, mode)
				})
			}
		})
	}
}

// concurrentInsert pages once, inserts rows around the cursor, then finishes
// the walk. mode is "asc"/"desc" (live list) or "feed" (?since=).
func (f *pgFix) concurrentInsert(sp pgSpec, rows []pgRow, mode string) {
	f.t.Helper()
	feed := mode == "feed"
	desc := mode == "desc"
	order := "asc"
	if desc {
		order = "desc"
	}
	param := "cursor"
	start := ""
	if feed {
		param = "since"
		start = controllers.EncodeCursor(f.base.Add(-2*time.Hour), "0")
	}
	const limit = 5
	first := f.page(sp, f.url(sp, limit, order, param, start))
	require.Equal(f.t, http.StatusOK, first.status, first.body)
	require.Len(f.t, first.ids, limit)
	require.NotEmpty(f.t, first.next)

	byID := map[string]pgRow{}
	for _, r := range rows {
		byID[r.id] = r
	}
	last := byID[first.ids[len(first.ids)-1]]

	newest := f.insert(sp, f.owner, pgSeed{n: 1000, ts: f.base.Add(time.Hour), pos: 1000, first: "Zed"})
	oldest := f.insert(sp, f.owner, pgSeed{n: 1001, ts: f.base.Add(-time.Hour), pos: -1, first: "Abe"})
	// For the name sort the tie row reuses the cursor row's own last name, so
	// its derived sort_name ties the cursor row's sort key exactly.
	tiedLast := ""
	if sp.kind == pgName {
		tiedLast = f.lastNameFor(last.id)
	}
	tied := f.insert(sp, f.owner, pgSeed{n: 1002, ts: last.ts, pos: last.pos, first: tiedLast})

	ids := append([]string{}, first.ids...)
	cursor := first.next
	for pages := 0; cursor != ""; pages++ {
		require.Less(f.t, pages, 3*pgSeedRows, "walk did not terminate")
		p := f.page(sp, f.url(sp, limit, order, param, cursor))
		require.Equal(f.t, http.StatusOK, p.status, p.body)
		require.NotEmpty(f.t, p.ids, "a next_cursor must never lead to an empty page")
		ids = append(ids, p.ids...)
		cursor = p.next
	}

	seen := map[string]int{}
	for _, id := range ids {
		seen[id]++
		require.Equalf(f.t, 1, seen[id], "row %s returned twice across a concurrent insert", id)
	}
	for _, r := range rows {
		if feed || !r.deleted {
			require.Containsf(f.t, seen, r.id, "pre-existing row %s skipped across a concurrent insert", r.id)
		}
	}
	if !desc {
		require.Contains(f.t, seen, newest.id, "a row newer than everything must appear later in an ascending walk")
		require.NotContains(f.t, seen, oldest.id, "a row older than the cursor is already behind an ascending walk")
	} else {
		require.Contains(f.t, seen, oldest.id, "a row older than everything must appear later in a descending walk")
		require.NotContains(f.t, seen, newest.id, "a row newer than the cursor is already behind a descending walk")
	}
	// The tie row sits at the cursor row's exact key: whether it is still ahead
	// is decided purely by the id tie-break.
	after := pgLess(sp, last, tied)
	if desc {
		after = pgLess(sp, tied, last)
	}
	if after {
		require.Contains(f.t, seen, tied.id, "tie-key row ordered after the cursor must be returned (id tie-break)")
	} else {
		require.NotContains(f.t, seen, tied.id, "tie-key row ordered before the cursor must not be re-returned")
	}
}

// lastNameFor reads a contact's last name back so a new contact can tie its
// sort_name exactly (DeriveSortName prefers the last name).
func (f *pgFix) lastNameFor(id string) string {
	var first string
	n, _ := strconv.ParseUint(id, 10, 64)
	require.NoError(f.t, f.db.Raw("SELECT lastname FROM contacts WHERE id = ?", n).Scan(&first).Error)
	return first
}
