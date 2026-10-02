package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mcpFixture is a real-migrated-schema router with POST /mcp mounted behind a
// stand-in for AuthMiddleware that resolves the user from X-Test-User.
type mcpFixture struct {
	db     *gorm.DB
	router *gin.Engine
	alice  models.User
	bob    models.User
}

func newMCPFixture(t *testing.T) *mcpFixture {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)

	alice := models.User{Username: "alice", Password: "password123", Email: "alice@example.com"}
	bob := models.User{Username: "bob", Password: "password123", Email: "bob@example.com"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", config.Config{ReminderTimezone: "UTC"})
		if raw := c.GetHeader("X-Test-User"); raw != "" {
			id, _ := strconv.ParseUint(raw, 10, 64)
			c.Set("userID", uint(id))
		}
		c.Next()
	})
	router.POST("/mcp", MCPHandler())
	return &mcpFixture{db: db, router: router, alice: alice, bob: bob}
}

type mcpCallResult struct {
	status     int
	isError    bool
	text       string
	structured map[string]any
}

// rpc POSTs one JSON-RPC request to /mcp as the given user (0 = none).
func (f *mcpFixture) rpc(t *testing.T, userID uint, method string, params any) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if userID != 0 {
		req.Header.Set("X-Test-User", strconv.FormatUint(uint64(userID), 10))
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (f *mcpFixture) call(t *testing.T, userID uint, tool string, args map[string]any) mcpCallResult {
	t.Helper()
	status, out := f.rpc(t, userID, "tools/call", map[string]any{"name": tool, "arguments": args})
	res := mcpCallResult{status: status}
	result, ok := out["result"].(map[string]any)
	if !ok {
		return res
	}
	res.isError, _ = result["isError"].(bool)
	if content, ok := result["content"].([]any); ok && len(content) > 0 {
		res.text, _ = content[0].(map[string]any)["text"].(string)
	}
	res.structured, _ = result["structuredContent"].(map[string]any)
	return res
}

func seedMCPContacts(t *testing.T, db *gorm.DB, userID uint, n int, prefix string) []models.Contact {
	t.Helper()
	contacts := make([]models.Contact, n)
	for i := range contacts {
		contacts[i] = models.Contact{UserID: userID, Firstname: fmt.Sprintf("%s%03d", prefix, i), Lastname: "Mcp"}
		require.NoError(t, db.Create(&contacts[i]).Error)
	}
	return contacts
}

func TestMCP_ListsExactlyTheFourV1Tools(t *testing.T) {
	f := newMCPFixture(t)
	status, out := f.rpc(t, f.alice.ID, "tools/list", map[string]any{})
	require.Equal(t, http.StatusOK, status)

	tools := out["result"].(map[string]any)["tools"].([]any)
	names := map[string]map[string]any{}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		names[tool["name"].(string)] = tool
	}
	require.Len(t, names, 4)
	for _, name := range []string{"search_contacts", "get_contact", "list_timeline", "run_cadence_report"} {
		tool, ok := names[name]
		require.Truef(t, ok, "tool %s missing", name)
		props := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		assert.Containsf(t, props, "include_sensitive", "%s must expose include_sensitive", name)
	}
	required := func(name string) []any {
		r, _ := names[name]["inputSchema"].(map[string]any)["required"].([]any)
		return r
	}
	assert.Contains(t, required("search_contacts"), "query")
	assert.Contains(t, required("get_contact"), "id")
	assert.Contains(t, required("list_timeline"), "contact_id")
}

func TestMCP_UnauthenticatedIsRejected(t *testing.T) {
	f := newMCPFixture(t)
	status, _ := f.rpc(t, 0, "tools/list", map[string]any{})
	assert.Equal(t, http.StatusUnauthorized, status, "no user in context must never reach a tool")
}

func TestMCP_SearchContacts(t *testing.T) {
	f := newMCPFixture(t)
	seedMCPContacts(t, f.db, f.alice.ID, 55, "Zedmund")
	seedMCPContacts(t, f.db, f.bob.ID, 3, "Zedmund")

	contactCount := func(r mcpCallResult) int {
		require.False(t, r.isError, r.text)
		return len(r.structured["contacts"].([]any))
	}

	t.Run("default limit is 20", func(t *testing.T) {
		assert.Equal(t, 20, contactCount(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund"})))
	})
	t.Run("explicit limit honoured", func(t *testing.T) {
		assert.Equal(t, 7, contactCount(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund", "limit": 7})))
	})
	t.Run("over-max limit is clamped to 50, not rejected", func(t *testing.T) {
		assert.Equal(t, 50, contactCount(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund", "limit": 1000})))
	})
	t.Run("offset pages through results", func(t *testing.T) {
		assert.Equal(t, 5, contactCount(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund", "limit": 50, "offset": 50})))
		assert.Equal(t, 0, contactCount(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund", "offset": 500})))
	})
	t.Run("pages do not overlap", func(t *testing.T) {
		ids := func(r mcpCallResult) map[any]bool {
			out := map[any]bool{}
			for _, c := range r.structured["contacts"].([]any) {
				out[c.(map[string]any)["id"]] = true
			}
			return out
		}
		first := ids(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund", "limit": 10}))
		second := ids(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund", "limit": 10, "offset": 10}))
		require.Len(t, first, 10)
		require.Len(t, second, 10)
		for id := range second {
			assert.False(t, first[id])
		}
	})
	t.Run("ownership scoping: another user's contacts are never returned", func(t *testing.T) {
		assert.Equal(t, 3, contactCount(f.call(t, f.bob.ID, "search_contacts", map[string]any{"query": "Zedmund"})))
	})
	t.Run("blank query is a tool error", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "   "})
		assert.True(t, r.isError)
		assert.Contains(t, r.text, "required")
	})
	t.Run("overlong query is a tool error", func(t *testing.T) {
		long := make([]byte, 300)
		for i := range long {
			long[i] = 'a'
		}
		r := f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": string(long)})
		assert.True(t, r.isError)
		assert.Contains(t, r.text, "at most")
	})
	t.Run("include_sensitive is accepted", func(t *testing.T) {
		assert.Equal(t, 20, contactCount(f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund", "include_sensitive": true})))
	})
	t.Run("same shape as GET /search", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "search_contacts", map[string]any{"query": "Zedmund"})
		for _, key := range []string{"query", "contacts", "notes", "activities"} {
			assert.Contains(t, r.structured, key)
		}
	})
}

func TestMCP_GetContact(t *testing.T) {
	f := newMCPFixture(t)
	subject := seedMCPContacts(t, f.db, f.alice.ID, 1, "Subject")[0]
	friend := seedMCPContacts(t, f.db, f.alice.ID, 1, "Friend")[0]
	privateParty := seedMCPContacts(t, f.db, f.alice.ID, 1, "Priv")[0]
	secretParty := seedMCPContacts(t, f.db, f.alice.ID, 1, "Secr")[0]
	foreign := seedMCPContacts(t, f.db, f.bob.ID, 1, "Foreign")[0]

	for _, e := range []struct{ target, sens string }{
		{friend.VCardUID, models.RelationshipSensitivityNormal},
		{privateParty.VCardUID, models.RelationshipSensitivityPrivate},
		{secretParty.VCardUID, models.RelationshipSensitivitySecret},
	} {
		require.NoError(t, f.db.Create(&models.RelationshipEdge{
			UserID: f.alice.ID, SourceID: subject.VCardUID, TargetID: e.target,
			Type: "friend_of", Status: models.RelationshipStatusConfirmed, Sensitivity: e.sens,
		}).Error)
	}

	edgeTargets := func(r mcpCallResult) map[string]bool {
		require.False(t, r.isError, r.text)
		out := map[string]bool{}
		for _, raw := range r.structured["relationship_edges"].([]any) {
			out[raw.(map[string]any)["edge"].(map[string]any)["target_id"].(string)] = true
		}
		return out
	}

	t.Run("default excludes private and secret relationships", func(t *testing.T) {
		got := edgeTargets(f.call(t, f.alice.ID, "get_contact", map[string]any{"id": subject.ID}))
		assert.True(t, got[friend.VCardUID])
		assert.False(t, got[privateParty.VCardUID])
		assert.False(t, got[secretParty.VCardUID])
	})
	t.Run("include_sensitive=false is the default", func(t *testing.T) {
		got := edgeTargets(f.call(t, f.alice.ID, "get_contact", map[string]any{"id": subject.ID, "include_sensitive": false}))
		assert.Len(t, got, 1)
	})
	t.Run("include_sensitive=true includes every tier", func(t *testing.T) {
		got := edgeTargets(f.call(t, f.alice.ID, "get_contact", map[string]any{"id": subject.ID, "include_sensitive": true}))
		assert.True(t, got[friend.VCardUID])
		assert.True(t, got[privateParty.VCardUID])
		assert.True(t, got[secretParty.VCardUID])
	})
	t.Run("same shape as GET /contacts/:id/detail", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": subject.ID})
		for _, key := range []string{"contact", "user", "notes", "activities", "relationship_edges", "life_events", "gifts"} {
			assert.Contains(t, r.structured, key)
		}
		assert.EqualValues(t, subject.ID, r.structured["contact"].(map[string]any)["id"])
	})
	t.Run("ownership scoping: another user's contact is not found", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": foreign.ID})
		assert.True(t, r.isError)
		assert.Contains(t, r.text, "not found")
		assert.NotContains(t, r.text, "Foreign")
	})
	t.Run("unknown and non-positive ids are not found", func(t *testing.T) {
		for _, id := range []int{99999, 0, -3} {
			r := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": id})
			assert.Truef(t, r.isError, "id %d", id)
			assert.Contains(t, r.text, "not found")
		}
	})
	t.Run("soft-deleted contact is not found", func(t *testing.T) {
		gone := seedMCPContacts(t, f.db, f.alice.ID, 1, "Gone")[0]
		require.NoError(t, f.db.Delete(&gone).Error)
		assert.True(t, f.call(t, f.alice.ID, "get_contact", map[string]any{"id": gone.ID}).isError)
	})
}

func TestMCP_GetContact_SensitiveRecordProjection(t *testing.T) {
	f := newMCPFixture(t)
	subject := seedMCPContacts(t, f.db, f.alice.ID, 1, "Subject")[0]
	other := seedMCPContacts(t, f.db, f.alice.ID, 1, "Other")[0]
	require.NoError(t, f.db.Create(&models.RelationshipEdge{
		UserID: f.alice.ID, SourceID: subject.VCardUID, TargetID: other.VCardUID,
		Type: "parent_of", Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityPrivate,
	}).Error)

	relatedCount := func(include bool) int {
		r := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": subject.ID, "include_sensitive": include})
		require.False(t, r.isError, r.text)
		card := r.structured["contact"].(map[string]any)["card"].(map[string]any)
		related, _ := card["relatedTo"].([]any)
		return len(related)
	}
	assert.Zero(t, relatedCount(false), "private edge must not project into card.related_to by default")
	assert.Positive(t, relatedCount(true), "include_sensitive must thread into the record projection")
}

func TestMCP_ListTimeline(t *testing.T) {
	f := newMCPFixture(t)
	subject := seedMCPContacts(t, f.db, f.alice.ID, 1, "Subject")[0]
	foreign := seedMCPContacts(t, f.db, f.bob.ID, 1, "Foreign")[0]

	now := time.Now().UTC() // the app compares UTC-bound params against stored values
	for i := 0; i < 105; i++ {
		require.NoError(t, f.db.Create(&models.Note{
			UserID: f.alice.ID, ContactID: &subject.ID,
			Content: fmt.Sprintf("note %d", i), Date: now.Add(-time.Duration(i) * time.Hour),
		}).Error)
	}
	require.NoError(t, f.db.Create(&models.Note{
		UserID: f.alice.ID, ContactID: &subject.ID, Content: "ancient", Date: now.AddDate(-3, 0, 0),
	}).Error)

	items := func(r mcpCallResult) []any {
		require.False(t, r.isError, r.text)
		return r.structured["items"].([]any)
	}
	contents := func(its []any) []string {
		out := make([]string, len(its))
		for i, it := range its {
			data := it.(map[string]any)["data"].(map[string]any)
			out[i], _ = data["content"].(string)
		}
		return out
	}

	t.Run("default limit is 25 and reports it", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID})
		assert.Len(t, items(r), 25)
		assert.EqualValues(t, 25, r.structured["limit"])
	})
	t.Run("over-max limit is clamped to 100, not rejected", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "limit": 5000})
		assert.Len(t, items(r), 100)
		assert.EqualValues(t, 100, r.structured["limit"])
	})
	t.Run("offset skips newest items", func(t *testing.T) {
		first := contents(items(f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "limit": 5})))
		second := contents(items(f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "limit": 5, "offset": 5})))
		require.Len(t, second, 5)
		assert.NotEqual(t, first, second)
		assert.Equal(t, "note 5", second[0])
		assert.Empty(t, items(f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "offset": 1000})))
	})
	t.Run("default window excludes items older than 366 days", func(t *testing.T) {
		all := contents(items(f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "limit": 100, "offset": 100})))
		assert.NotContains(t, all, "ancient")
		assert.Len(t, all, 5) // 105 recent notes, first 100 skipped
	})
	t.Run("since/until bound the window (inclusive dates)", func(t *testing.T) {
		day := now.AddDate(-1, 0, 0)
		require.NoError(t, f.db.Create(&models.Note{
			UserID: f.alice.ID, ContactID: &subject.ID, Content: "last-year", Date: day,
		}).Error)
		d := day.Format("2006-01-02")
		got := contents(items(f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "since": d, "until": d})))
		assert.Equal(t, []string{"last-year"}, got)
	})
	t.Run("span wider than 366 days is clamped to the 366 days ending at until", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "list_timeline", map[string]any{
			"contact_id": subject.ID, "limit": 100, "since": "2000-01-01", "until": now.Format("2006-01-02"),
		})
		assert.NotContains(t, contents(items(r)), "ancient")
	})
	t.Run("since alone is capped to since+366d", func(t *testing.T) {
		since := now.AddDate(-3, 0, -5).Format("2006-01-02")
		got := contents(items(f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "since": since})))
		assert.Equal(t, []string{"ancient"}, got)
	})
	t.Run("RFC 3339 timestamps are accepted", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "list_timeline", map[string]any{
			"contact_id": subject.ID, "since": now.Add(-90 * time.Minute).UTC().Format(time.RFC3339), "until": now.UTC().Format(time.RFC3339),
		})
		// "now" itself is truncated to whole seconds in the bound, so only the
		// note an hour ago is inside.
		assert.Equal(t, []string{"note 1"}, contents(items(r)))
	})
	t.Run("bad dates are tool errors", func(t *testing.T) {
		for _, args := range []map[string]any{
			{"contact_id": subject.ID, "since": "yesterday"},
			{"contact_id": subject.ID, "until": "31/12/2026"},
			{"contact_id": subject.ID, "since": "2026-02-01", "until": "2026-01-01"},
		} {
			r := f.call(t, f.alice.ID, "list_timeline", args)
			assert.Truef(t, r.isError, "%v", args)
		}
	})
	t.Run("ownership scoping: another user's contact is not found", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": foreign.ID})
		assert.True(t, r.isError)
		assert.Contains(t, r.text, "not found")
	})
	t.Run("unknown contact is not found", func(t *testing.T) {
		assert.True(t, f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": 99999}).isError)
	})
	t.Run("include_sensitive is accepted", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "list_timeline", map[string]any{"contact_id": subject.ID, "include_sensitive": true})
		assert.Len(t, items(r), 25)
	})
}

func TestMCP_RunCadenceReport(t *testing.T) {
	f := newMCPFixture(t)
	mine := seedMCPContacts(t, f.db, f.alice.ID, 3, "Overdue")
	theirs := seedMCPContacts(t, f.db, f.bob.ID, 2, "BobOverdue")

	seedOverdue := func(userID uint, c models.Contact, daysAgo int) {
		require.NoError(t, f.db.Create(&models.Activity{
			UserID: userID, Title: "Call", Type: models.InteractionTypeCall,
			Date: time.Now().AddDate(0, 0, -daysAgo), Contacts: []models.Contact{c},
		}).Error)
		require.NoError(t, f.db.Create(&models.CadencePolicy{
			UserID: userID, EntityID: c.VCardUID, TargetIntervalDays: 7,
			QualifyingTypes: []string{models.InteractionTypeCall},
		}).Error)
	}
	seedOverdue(f.alice.ID, mine[0], 20)
	seedOverdue(f.alice.ID, mine[1], 60)
	seedOverdue(f.alice.ID, mine[2], 40)
	for _, c := range theirs {
		seedOverdue(f.bob.ID, c, 30)
	}

	overdue := func(r mcpCallResult) []any {
		require.False(t, r.isError, r.text)
		return r.structured["overdue"].([]any)
	}
	nameOf := func(it any) string { return it.(map[string]any)["contact_name"].(string) }

	t.Run("most overdue first, scoped to the caller", func(t *testing.T) {
		got := overdue(f.call(t, f.alice.ID, "run_cadence_report", map[string]any{}))
		require.Len(t, got, 3)
		assert.Equal(t, "Overdue001 Mcp", nameOf(got[0]))
		assert.Equal(t, "Overdue002 Mcp", nameOf(got[1]))
		assert.Equal(t, "Overdue000 Mcp", nameOf(got[2]))
	})
	t.Run("ownership scoping: other user sees only their own", func(t *testing.T) {
		got := overdue(f.call(t, f.bob.ID, "run_cadence_report", map[string]any{}))
		require.Len(t, got, 2)
		for _, it := range got {
			assert.Contains(t, nameOf(it), "BobOverdue")
		}
	})
	t.Run("limit and offset page the report", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "run_cadence_report", map[string]any{"limit": 1, "offset": 1})
		got := overdue(r)
		require.Len(t, got, 1)
		assert.Equal(t, "Overdue002 Mcp", nameOf(got[0]))
		assert.EqualValues(t, 3, r.structured["total"])
	})
	t.Run("over-max limit is clamped to 100, not rejected", func(t *testing.T) {
		r := f.call(t, f.alice.ID, "run_cadence_report", map[string]any{"limit": 9999})
		assert.False(t, r.isError)
		assert.EqualValues(t, 100, r.structured["limit"])
		assert.EqualValues(t, 25, f.call(t, f.alice.ID, "run_cadence_report", map[string]any{}).structured["limit"])
	})
	t.Run("offset past the end yields an empty page", func(t *testing.T) {
		assert.Empty(t, overdue(f.call(t, f.alice.ID, "run_cadence_report", map[string]any{"offset": 50})))
	})
	t.Run("include_sensitive is accepted", func(t *testing.T) {
		assert.Len(t, overdue(f.call(t, f.alice.ID, "run_cadence_report", map[string]any{"include_sensitive": true})), 3)
	})
	t.Run("a user with no policies gets an empty array, not null", func(t *testing.T) {
		carol := models.User{Username: "carol", Password: "password123", Email: "carol@example.com"}
		require.NoError(t, f.db.Create(&carol).Error)
		r := f.call(t, carol.ID, "run_cadence_report", map[string]any{})
		assert.Equal(t, []any{}, r.structured["overdue"])
	})
}

func TestMCP_UnknownToolIsAnError(t *testing.T) {
	f := newMCPFixture(t)
	status, out := f.rpc(t, f.alice.ID, "tools/call", map[string]any{"name": "delete_contact", "arguments": map[string]any{}})
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, out, "error", "no write tools exist in v1")
}

func TestMCP_ClampHelpers(t *testing.T) {
	assert.Equal(t, 20, mcpClampLimit(0, 20, 50))
	assert.Equal(t, 20, mcpClampLimit(-4, 20, 50))
	assert.Equal(t, 33, mcpClampLimit(33, 20, 50))
	assert.Equal(t, 50, mcpClampLimit(51, 20, 50))
	assert.Equal(t, 0, mcpClampOffset(-1))
	assert.Equal(t, 10, mcpClampOffset(10))
	assert.Equal(t, mcpMaxOffset, mcpClampOffset(mcpMaxOffset+1))
}

func TestMCP_TimelineWindow(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	cutoff, notAfter, err := mcpTimelineWindow("", "", now)
	require.NoError(t, err)
	assert.Equal(t, now, notAfter)
	assert.Equal(t, now.Add(-mcpMaxTimelineSpan), cutoff)

	cutoff, notAfter, err = mcpTimelineWindow("2026-06-01", "2026-06-10", now)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), cutoff)
	assert.True(t, notAfter.After(time.Date(2026, 6, 10, 23, 59, 0, 0, time.UTC)), "bare until date is inclusive of the whole day")
	assert.True(t, notAfter.Before(time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)))

	cutoff, notAfter, err = mcpTimelineWindow("2020-01-01", "2026-06-10", now)
	require.NoError(t, err)
	assert.Equal(t, notAfter.Add(-mcpMaxTimelineSpan), cutoff, "wide span clamps to 366 days")

	cutoff, notAfter, err = mcpTimelineWindow("2025-01-01", "", now)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(mcpMaxTimelineSpan), notAfter)
	assert.Equal(t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), cutoff)

	_, _, err = mcpTimelineWindow("nope", "", now)
	assert.ErrorContains(t, err, "since")
	_, _, err = mcpTimelineWindow("", "nope", now)
	assert.ErrorContains(t, err, "until")
	_, _, err = mcpTimelineWindow("2026-06-10", "2026-06-01", now)
	assert.ErrorContains(t, err, "before")
}
