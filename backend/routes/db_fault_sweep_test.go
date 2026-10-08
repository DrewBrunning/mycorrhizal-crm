package routes

// TestDBFaultSweep is the fault-injection counterpart of the ownership matrix
// (issue #1476). The controllers talk to *gorm.DB directly, so the dozens of
// `if err := tx.X(...).Error; err != nil { return ErrDatabase(...) }` arms in
// a delete cascade, a merge commit or a membership write cannot be reached
// through the HTTP surface — and the real risk is not the 500 itself but what a
// mid-request failure leaves behind.
//
// For every scenario in faultScenarios (a curated list of mutating routes) the
// sweep:
//
//  1. runs the request once with the injector recording and requires success —
//     the number of statements it issued on the request goroutine is K;
//  2. re-runs it K times on a fresh, identically-seeded database, failing
//     statement i (1..K) each time, and requires for every i that
//     a. the response is a well-formed error envelope with a 4xx/5xx status
//     (never a 2xx — a swallowed failure — and never a panic),
//     b. the body leaks no SQL: no injected-error marker, no "sqlite"/"SQL"/
//     "constraint"/"no such table", and no table name from the schema,
//     c. the database is byte-for-byte what it was before the request (per
//     table: row count + a checksum over every row), i.e. everything the
//     request had already written was rolled back.
//
// A scenario/statement pair that legitimately cannot satisfy (a) or (c) — a
// best-effort side effect whose failure is logged and swallowed by design —
// must be declared in faultAllowlist with a written reason. An allowlist entry
// that no longer matches a real outcome fails the test, so it cannot rot.
//
// Statements are counted on the request goroutine only (see package dbfault):
// the fire-and-forget audit/webhook goroutines share the *gorm.DB but run on
// other goroutines and are neither counted nor failed.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbfault"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/fireandforget"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const faultOwnerPassword = "fault-sweep-password-123"

// faultEnv is one isolated database + live router + authenticated owner.
type faultEnv struct {
	db     *gorm.DB
	inj    *dbfault.Injector
	router *gin.Engine
	owner  models.User
	token  string
}

func newFaultEnv(t *testing.T) *faultEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	middleware.ConfigureAPIRateLimiter(time.Microsecond, 1_000_000)

	// Audit appends are best-effort side effects outside the request under
	// test, so the sweep runs with the production-shaped async recorder
	// (issue #1493): audit_events statements run on their own goroutine, which
	// the injector does not count, exactly as in production. The synchronous
	// test default would put them inside the request's transaction.
	db := dbtest.New(t, dbtest.WithFaults(), dbtest.WithAsyncAudit())
	db.Logger = logger.Default.LogMode(logger.Silent)

	cfg := &config.Config{
		JWTSecretKey:     "db-fault-sweep-secret-key-that-is-long-enough",
		JWTExpiryHours:   96,
		ProfilePhotoDir:  t.TempDir(),
		FrontendURL:      "http://localhost:5173",
		Port:             "7300",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
		CardDAVEnabled:   true,
		CalDAVEnabled:    true,
	}

	hashed, err := services.HashPassword(faultOwnerPassword)
	require.NoError(t, err)
	owner := models.User{Username: "fault-owner", Email: "fault-owner@example.com", Password: hashed}
	require.NoError(t, db.Create(&owner).Error)
	token, err := services.IssueSession(db, owner, cfg, "", "")
	require.NoError(t, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	RegisterRoutes(router, cfg, db, nil)

	return &faultEnv{db: db, inj: dbfault.For(db), router: router, owner: owner, token: token}
}

// faultReq is the single request a scenario issues.
type faultReq struct {
	method, path string
	body         any
}

func (e *faultEnv) do(t *testing.T, r faultReq) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if r.body != nil {
		var err error
		raw, err = json.Marshal(r.body)
		require.NoError(t, err)
	}
	req, err := http.NewRequest(r.method, r.path, bytes.NewReader(raw))
	require.NoError(t, err)
	req.RemoteAddr = uniqueTestClientIP() + ":1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

// ── database snapshot ─────────────────────────────────────────────────────

// faultVolatileTables are bookkeeping tables a request touches on every call
// regardless of outcome (the auth middleware's session activity stamp), so a
// before/after comparison would otherwise flag every statement. Nothing a user
// authored lives in them.
var faultVolatileTables = map[string]string{
	"sessions": "AuthMiddleware stamps last-used on the session row on every authenticated request, success or failure",
}

// snapshotDB returns table -> "count:sha256(all rows)" for every real table,
// so "the database is unchanged" is one map comparison.
func snapshotDB(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	var tables []string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`).Scan(&tables).Error)
	out := make(map[string]string, len(tables))
	for _, tbl := range tables {
		if _, skip := faultVolatileTables[tbl]; skip {
			continue
		}
		rows, err := db.Raw(fmt.Sprintf(`SELECT * FROM %q ORDER BY rowid`, tbl)).Rows()
		if err != nil {
			// WITHOUT ROWID tables (FTS5 config shadow tables): primary-key order.
			rows, err = db.Raw(fmt.Sprintf(`SELECT * FROM %q ORDER BY 1`, tbl)).Rows()
		}
		require.NoError(t, err, "snapshot %s", tbl)
		cols, err := rows.Columns()
		require.NoError(t, err)
		h := sha256.New()
		n := 0
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			require.NoError(t, rows.Scan(ptrs...))
			fmt.Fprintf(h, "%v\n", vals)
			n++
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		out[tbl] = fmt.Sprintf("%d:%s", n, hex.EncodeToString(h.Sum(nil))[:16])
	}
	return out
}

func diffSnapshots(before, after map[string]string) []string {
	var d []string
	for tbl, b := range before {
		if a := after[tbl]; a != b {
			d = append(d, fmt.Sprintf("%s: %s -> %s", tbl, b, a))
		}
	}
	sort.Strings(d)
	return d
}

// ── leak check ────────────────────────────────────────────────────────────

var faultLeakWords = regexp.MustCompile(`(?i)sqlite|\bsql\b|no such table|constraint failed|gorm|disk i/o|` + dbfault.Marker)

// faultLeak returns a description of any internal database detail in body, or
// "". tableNames are the schema's tables that contain an underscore (so
// ordinary words like "contacts" cannot false-positive).
func faultLeak(body string, tableNames []string) string {
	if m := faultLeakWords.FindString(body); m != "" {
		return fmt.Sprintf("leaked %q", m)
	}
	for _, tbl := range tableNames {
		if strings.Contains(body, tbl) {
			return fmt.Sprintf("leaked table name %q", tbl)
		}
	}
	return ""
}

func underscoreTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var tables []string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '%\_%' ESCAPE '\' AND name NOT LIKE 'sqlite_%'`).Scan(&tables).Error)
	return tables
}

// ── scenarios ─────────────────────────────────────────────────────────────

type faultScenario struct {
	name  string
	setup func(t *testing.T, e *faultEnv) faultReq
}

// seedRichContact creates a contact with a row in (nearly) every table a
// contact owns, via the shared authorization-matrix fixture, plus circle / tag
// / household membership. Returns the heavily-associated contact.
func seedRichContact(t *testing.T, e *faultEnv) (contact models.Contact, other models.Contact) {
	t.Helper()
	res := seedResources(t, e.db, e.owner.ID)
	require.NoError(t, e.db.Where("user_id = ? AND firstname = ?", e.owner.ID, "Matrix Entity").First(&contact).Error)
	require.NoError(t, e.db.Where("user_id = ? AND vcard_uid = ?", e.owner.ID, res.contactUID).First(&other).Error)

	require.NoError(t, e.db.Create(&models.CircleMember{CircleID: res.circle, UserID: e.owner.ID, MemberVCardUID: contact.VCardUID}).Error)
	require.NoError(t, e.db.Create(&models.ContactTag{TagID: res.tag, UserID: e.owner.ID, ContactVCardUID: contact.VCardUID}).Error)
	require.NoError(t, e.db.Create(&models.HouseholdMember{HouseholdID: res.household, UserID: e.owner.ID, MemberVCardUID: contact.VCardUID}).Error)
	return contact, other
}

func faultNewContactBody(given string) models.ContactRecordInput {
	return models.ContactRecordInput{Card: contactmodel.Card{
		Name:      &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: given}}},
		Emails:    []contactmodel.Email{{Address: given + "@example.com"}},
		Addresses: []contactmodel.Address{{Full: "1 Main St"}},
	}}
}

func faultScenarios() []faultScenario {
	return []faultScenario{
		{"DELETE /contacts/:id", func(t *testing.T, e *faultEnv) faultReq {
			c, _ := seedRichContact(t, e)
			return faultReq{method: http.MethodDelete, path: fmt.Sprintf("/api/v1/contacts/%d", c.ID)}
		}},
		{"POST /contacts", func(t *testing.T, e *faultEnv) faultReq {
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts", body: faultNewContactBody("Created")}
		}},
		{"PUT /contacts/:id", func(t *testing.T, e *faultEnv) faultReq {
			c, _ := seedRichContact(t, e)
			return faultReq{method: http.MethodPut, path: fmt.Sprintf("/api/v1/contacts/%d", c.ID), body: faultNewContactBody("Renamed")}
		}},
		{"POST /contacts/merge", func(t *testing.T, e *faultEnv) faultReq {
			keep, loser := seedRichContact(t, e)
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts/merge", body: models.ContactMergeRequest{KeepID: keep.ID, MergeID: loser.ID, Resolutions: map[string]string{"firstname": keep.Firstname}}}
		}},
		{"DELETE /account", func(t *testing.T, e *faultEnv) faultReq {
			seedRichContact(t, e)
			return faultReq{method: http.MethodDelete, path: "/api/v1/account", body: map[string]string{"current_password": faultOwnerPassword}}
		}},
		{"POST /circles/:id/members", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodPost, path: "/api/v1/circles/" + res.circle + "/members", body: models.CircleMemberInput{MemberVCardUID: res.contactUID}}
		}},
		{"POST /tags/:id/contacts", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodPost, path: "/api/v1/tags/" + res.tag + "/contacts", body: models.ContactTagInput{ContactVCardUID: res.contactUID}}
		}},
		{"POST /households/:id/members", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodPost, path: "/api/v1/households/" + res.household + "/members", body: models.HouseholdMemberInput{MemberVCardUID: res.contactUID}}
		}},
		{"POST /contacts/:id/reminders", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts/" + res.contact + "/reminders",
				body: map[string]any{"message": "call", "remind_at": time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339), "recurrence": "weekly", "contact_id": 1}}
		}},
		{"POST /contacts/import/vcf/confirm", func(t *testing.T, e *faultEnv) faultReq {
			up := e.do(t, faultReq{method: http.MethodPost, path: "/api/v1/contacts/import/records", body: models.ImportRecordsRequest{
				Records: []models.ContactRecordInput{faultNewContactBody("ImportedOne"), faultNewContactBody("ImportedTwo")},
			}})
			require.Equal(t, http.StatusOK, up.Code, up.Body.String())
			var prev models.ImportPreviewResponse
			require.NoError(t, json.Unmarshal(up.Body.Bytes(), &prev))
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts/import/vcf/confirm", body: models.ImportConfirmRequest{
				SessionID: prev.SessionID,
				Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "add"}},
			}}
		}},
		{"DELETE /circles/:id", func(t *testing.T, e *faultEnv) faultReq {
			seedRichContact(t, e)
			var circle models.Circle
			require.NoError(t, e.db.Where("user_id = ?", e.owner.ID).First(&circle).Error)
			return faultReq{method: http.MethodDelete, path: "/api/v1/circles/" + circle.ID}
		}},
		{"DELETE /tags/:id", func(t *testing.T, e *faultEnv) faultReq {
			seedRichContact(t, e)
			var tag models.Tag
			require.NoError(t, e.db.Where("user_id = ?", e.owner.ID).First(&tag).Error)
			return faultReq{method: http.MethodDelete, path: "/api/v1/tags/" + tag.ID}
		}},
		{"DELETE /households/:id", func(t *testing.T, e *faultEnv) faultReq {
			seedRichContact(t, e)
			var h models.Household
			require.NoError(t, e.db.Where("user_id = ?", e.owner.ID).First(&h).Error)
			return faultReq{method: http.MethodDelete, path: "/api/v1/households/" + h.ID}
		}},
		{"DELETE /circles/:id/members/:vcard_uid", func(t *testing.T, e *faultEnv) faultReq {
			c, _ := seedRichContact(t, e)
			var m models.CircleMember
			require.NoError(t, e.db.Where("user_id = ?", e.owner.ID).First(&m).Error)
			return faultReq{method: http.MethodDelete, path: "/api/v1/circles/" + m.CircleID + "/members/" + c.VCardUID}
		}},
		{"DELETE /notes/:id", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodDelete, path: "/api/v1/notes/" + res.note}
		}},
		{"POST /contacts/:id/archive", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts/" + res.contact + "/archive"}
		}},
		{"POST /contacts/:id/unarchive", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", res.contact).Update("archived", true).Error)
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts/" + res.contact + "/unarchive"}
		}},
		{"POST /contacts/:id/favorite", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts/" + res.contact + "/favorite"}
		}},
		{"POST /contacts/:id/unfavorite", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", res.contact).Update("is_favorite", true).Error)
			return faultReq{method: http.MethodPost, path: "/api/v1/contacts/" + res.contact + "/unfavorite"}
		}},
		{"PATCH /users/language", func(t *testing.T, e *faultEnv) faultReq {
			return faultReq{method: http.MethodPatch, path: "/api/v1/users/language", body: map[string]string{"language": "de"}}
		}},
		{"PATCH /users/date-format", func(t *testing.T, e *faultEnv) faultReq {
			return faultReq{method: http.MethodPatch, path: "/api/v1/users/date-format", body: map[string]string{"date_format": "us"}}
		}},
		{"POST /users/change-password", func(t *testing.T, e *faultEnv) faultReq {
			return faultReq{method: http.MethodPost, path: "/api/v1/users/change-password",
				body: map[string]string{"current_password": faultOwnerPassword, "new_password": "N3w-Str0ng-Passw0rd!x"}}
		}},
		{"POST /reminders/:id/complete", func(t *testing.T, e *faultEnv) faultReq {
			res := seedResources(t, e.db, e.owner.ID)
			return faultReq{method: http.MethodPost, path: "/api/v1/reminders/" + res.reminder + "/complete"}
		}},
	}
}

// faultAuthPrefix is the statements AuthMiddleware issues before any handler
// runs (session lookup). They fail with the middleware's own bare {"error": ...}
// 401/500 envelope, not the handler envelope, and are covered by
// TestDBFaultSweep_AuthMiddlewareFailure; the per-route sweep starts after them.
var faultAuthPrefix = []string{"query users", "query sessions"}

// faultAllowlist declares scenario|statement outcomes that legitimately differ
// from "error + database unchanged". Every entry carries a reason and must
// match a real outcome (checked below).
var faultAllowlist = buildFaultAllowlist()

const (
	faultReasonEnrich    = "post-commit read-only response enrichment (NewContactRecordResponse / wedding sync): the write is already committed, the failed read is logged and the response is returned without that section — a 5xx would invite a retry of a write that succeeded"
	faultReasonAudit     = "AfterSave audit snapshot read (auditAfterSave): the audit trail is best-effort by design; its failure must not fail the user's write"
	faultReasonImportRow = "import is per-row by design: a row whose write fails is reported in errors[] (as a generic message — the SQL text goes to the log only, which this sweep's leak check pins) and skipped while the other rows still import"
	faultReasonImportAux = "import auxiliary read/bookkeeping (disk-space probe, custom-field lookup, import_runs history row): logged and skipped, the import itself is unaffected"
	faultReasonRemind    = "reminder completion bookkeeping is best-effort by documented policy (\"Don't fail the entire operation\"): the timeline record / delivery clean-up / reach-out dismissal are logged and skipped, the reminder itself still completes"
)

func buildFaultAllowlist() map[string]string {
	m := map[string]string{}
	add := func(reason, scenario string, keys ...string) {
		for _, k := range keys {
			m[scenario+"|"+k] = reason
		}
	}
	add(faultReasonEnrich, "POST /contacts",
		"query life_events#1", "query relationship_edges#1", "query contact_tags#1", "query preferences#1", "query field_values#1")
	add(faultReasonEnrich, "PUT /contacts/:id",
		"query life_events#1", "query relationship_edges#1", "query contact_tags#1", "query tags#1", "query preferences#1", "query field_values#1")
	// The merge's cascade reads the same tables first, so its enrichment reads
	// carry later occurrence numbers.
	add(faultReasonEnrich, "POST /contacts/merge",
		"query relationship_edges#3", "query contact_tags#2", "query tags#1", "query preferences#2", "query field_values#4")
	add(faultReasonImportRow, "POST /contacts/import/vcf/confirm",
		"create contacts#1", "update contacts#1", "create contacts#2", "update contacts#2")
	add(faultReasonImportAux, "POST /contacts/import/vcf/confirm",
		"row pragma_database_list#1", "query field_definitions#1", "create import_runs#1")
	add(faultReasonAudit, "PUT /contacts/:id", "query contacts#2")
	add(faultReasonAudit, "POST /contacts/merge", "query contacts#3")
	for _, sc := range []string{"POST /contacts/:id/archive", "POST /contacts/:id/unarchive", "POST /contacts/:id/favorite", "POST /contacts/:id/unfavorite"} {
		add(faultReasonAudit, sc, "query contacts#2")
	}
	add("the password change and every revocation already committed atomically; minting the caller's replacement session is best-effort and the handler documents that the client re-authenticates", "POST /users/change-password", "create sessions#1")
	add(faultReasonRemind, "POST /reminders/:id/complete",
		"create reminder_completions#1", "delete notification_deliveries#1", "update reach_out_suggestions#1")
	return m
}

// faultKey names statement i (1-based) of a scenario by its verb+table and
// which occurrence of that pair it is, e.g. "PUT /contacts/:id|query contacts#2".
func faultKey(scenario string, stmts []dbfault.Statement, i int) string {
	n := 0
	for _, st := range stmts[:i] {
		if st == stmts[i-1] {
			n++
		}
	}
	return fmt.Sprintf("%s|%s#%d", scenario, stmts[i-1], n)
}

func TestDBFaultSweep(t *testing.T) {
	for _, sc := range faultScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			// 1. happy run, recording.
			base := newFaultEnv(t)
			req := sc.setup(t, base)
			base.inj.Record()
			w := base.do(t, req)
			base.inj.Disarm()
			fireandforget.Wait()
			require.Truef(t, w.Code >= 200 && w.Code < 300, "happy path must succeed: %d %s", w.Code, w.Body.String())
			stmts := base.inj.Statements()
			require.GreaterOrEqual(t, len(stmts), len(faultAuthPrefix))
			for i, want := range faultAuthPrefix {
				require.Equal(t, want, stmts[i].String(), "AuthMiddleware's statements are no longer the leading ones; update faultAuthPrefix")
			}
			tables := underscoreTables(t, base.db)
			t.Logf("%s: K=%d statements: %v", sc.name, len(stmts), stmts)

			var failures []string
			usedAllow := map[string]bool{}
			// 2. fail each statement in turn on a fresh identical database.
			for i := len(faultAuthPrefix) + 1; i <= len(stmts); i++ {
				e := newFaultEnv(t)
				req := sc.setup(t, e)
				fireandforget.Wait()
				before := snapshotDB(t, e.db)

				e.inj.FailNth(i)
				w := e.do(t, req)
				fired, ok := e.inj.Fired()
				e.inj.Disarm()
				fireandforget.Wait()
				after := snapshotDB(t, e.db)

				label := fmt.Sprintf("statement %d/%d (%s)", i, len(stmts), stmts[i-1])
				require.Truef(t, ok, "%s: injection did not fire (statement count drifted between runs)", label)
				require.Equalf(t, stmts[i-1], fired, "%s: statement order drifted between runs", label)

				key := faultKey(sc.name, stmts, i)
				if _, allowed := faultAllowlist[key]; allowed {
					usedAllow[key] = true
					if w.Code < 200 || w.Code >= 300 {
						failures = append(failures, fmt.Sprintf("%s: allowlisted %q but it now fails with HTTP %d — remove the entry", label, key, w.Code))
					}
					if l := faultLeak(w.Body.String(), tables); l != "" {
						failures = append(failures, fmt.Sprintf("%s: %s in body", label, l))
					}
					continue
				}
				for _, f := range faultViolations(w, tables, before, after) {
					failures = append(failures, fmt.Sprintf("%s [key %q]: %s", label, key, f))
				}
			}
			for key := range faultAllowlist {
				if strings.HasPrefix(key, sc.name+"|") && !usedAllow[key] {
					failures = append(failures, fmt.Sprintf("stale allowlist entry %q matches no statement", key))
				}
			}
			require.Emptyf(t, failures, "%d fault-injection violations in %s:\n  %s", len(failures), sc.name, strings.Join(failures, "\n  "))
		})
	}
}

// faultViolations checks one failed-statement outcome against the sweep's
// three invariants and returns a description of each one broken.
func faultViolations(w *httptest.ResponseRecorder, tables []string, before, after map[string]string) []string {
	var v []string
	if w.Code < 400 || w.Code >= 600 {
		v = append(v, fmt.Sprintf("failure surfaced as HTTP %d (a swallowed DB error): %s", w.Code, w.Body.String()))
	} else {
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || env.Error.Code == "" || env.Error.Message == "" {
			v = append(v, fmt.Sprintf("HTTP %d body is not a well-formed error envelope: %s", w.Code, w.Body.String()))
		}
	}
	if l := faultLeak(w.Body.String(), tables); l != "" {
		v = append(v, fmt.Sprintf("%s in body: %s", l, w.Body.String()))
	}
	if d := diffSnapshots(before, after); len(d) > 0 {
		v = append(v, fmt.Sprintf("HTTP %d but the database changed (partial write): %v", w.Code, d))
	}
	return v
}
