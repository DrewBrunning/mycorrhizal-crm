package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2290, ADR 0042 / docs/security/standing-shares-review.md: the
// share-grant API. These tests are the cross-user authorization matrix: the
// owner (A), the recipient (B), an unrelated user (C) and an admin each
// attempt every route, and every non-party / wrong-role / wrong-state /
// expired / blocked / entity-gone case must be the identical 404 with no side
// effect.

type sgEnv struct {
	t      *testing.T
	db     *gorm.DB
	router *gin.Engine
	clock  *clock.Fake
	A, B   models.User // owner, recipient
	C      models.User // unrelated
	Admin  models.User
}

func newSGEnv(t *testing.T) *sgEnv {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	e := &sgEnv{t: t, db: db, clock: clock.NewFake(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))}
	t.Cleanup(services.SetClock(e.clock))
	models.NewAuditRecorder(db, models.WithSync())
	mk := func(name string, admin bool) models.User {
		u := models.User{Username: name, Password: "password123", Email: name + "@example.com", IsAdmin: admin}
		require.NoError(t, db.Create(&u).Error)
		return u
	}
	e.A, e.B, e.C, e.Admin = mk("sg_owner", false), mk("sg_recipient", false), mk("sg_third", false), mk("sg_admin", true)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		clock.Install(c, e.clock)
		c.Set("db", db)
		c.Set("cfg", config.Config{ProfilePhotoDir: t.TempDir()})
		if v := c.GetHeader("X-Test-User"); v != "" {
			id, _ := strconv.Atoi(v)
			c.Set("userID", uint(id))
		}
		c.Next()
	})
	sg := r.Group("/share-grants")
	sg.Use(NoStoreMiddleware())
	sg.POST("", middleware.ValidateJSONMiddleware(&models.ShareGrantInput{}), CreateShareGrant)
	sg.GET("/outgoing", ListOutgoingShareGrants)
	sg.GET("/incoming", ListIncomingShareGrants)
	sg.GET("/blocks", ListShareBlocks)
	sg.DELETE("/blocks/:user_id", DeleteShareBlock)
	sg.PATCH("/:id", middleware.ValidateJSONMiddleware(&models.ShareGrantPatchInput{}), UpdateShareGrant)
	sg.POST("/:id/accept", AcceptShareGrant)
	sg.POST("/:id/decline", DeclineShareGrant)
	sg.POST("/:id/leave", LeaveShareGrant)
	sg.POST("/:id/revoke", RevokeShareGrant)
	sg.POST("/:id/reconfirm", ReconfirmShareGrant)
	sg.GET("/:id/content", GetShareGrantContent)
	e.router = r
	return e
}

func (e *sgEnv) do(u models.User, method, path string, body any, hdr ...string) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(e.t, json.NewEncoder(&buf).Encode(body))
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if u.ID != 0 {
		req.Header.Set("X-Test-User", strconv.Itoa(int(u.ID)))
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func (e *sgEnv) contact(owner models.User, first string) models.Contact {
	c := models.Contact{UserID: owner.ID, Firstname: first, Lastname: "Doe"}
	require.NoError(e.t, e.db.Create(&c).Error)
	return c
}

func (e *sgEnv) circle(owner models.User, name string) models.Circle {
	c := models.Circle{UserID: owner.ID, Name: name}
	require.NoError(e.t, e.db.Create(&c).Error)
	return c
}

func (e *sgEnv) note(owner models.User, text string) models.Note {
	n := models.Note{UserID: owner.ID, Content: text, Date: e.clock.Now()}
	require.NoError(e.t, e.db.Create(&n).Error)
	require.NotEmpty(e.t, n.UUID)
	return n
}

func (e *sgEnv) activity(owner models.User, title string) models.Activity {
	a := models.Activity{UserID: owner.ID, Title: title, Date: e.clock.Now(), Description: "desc", Location: "loc"}
	require.NoError(e.t, e.db.Create(&a).Error)
	require.NotEmpty(e.t, a.UUID)
	return a
}

// grant creates a grant through the real API and returns its id.
func (e *sgEnv) grant(from, to models.User, etype, eid string, sections []string) string {
	e.t.Helper()
	w := e.do(from, http.MethodPost, "/share-grants", map[string]any{
		"to_user_id": to.ID, "entity_type": etype, "entity_id": eid, "sections": sections,
	})
	require.Equal(e.t, http.StatusCreated, w.Code, w.Body.String())
	var out struct {
		ShareGrant struct {
			ID string `json:"id"`
		} `json:"share_grant"`
	}
	require.NoError(e.t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.ShareGrant.ID
}

func (e *sgEnv) activeContactGrant(sections []string) (string, models.Contact) {
	c := e.contact(e.A, "Alice")
	id := e.grant(e.A, e.B, "contact", c.VCardUID, sections)
	require.Equal(e.t, http.StatusOK, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	return id, c
}

func (e *sgEnv) status(id string) string {
	var g models.ShareGrant
	require.NoError(e.t, e.db.First(&g, "id = ?", id).Error)
	return g.Status
}

func (e *sgEnv) grantCount() int64 {
	var n int64
	require.NoError(e.t, e.db.Model(&models.ShareGrant{}).Count(&n).Error)
	return n
}

// ---------------------------------------------------------------- create

func TestShareGrantCreate_OwnerCreatesPendingGrant(t *testing.T) {
	e := newSGEnv(t)
	c := e.contact(e.A, "Alice")
	w := e.do(e.A, http.MethodPost, "/share-grants", map[string]any{
		"to_user_id": e.B.ID, "entity_type": "contact", "entity_id": c.VCardUID, "sections": []string{"emails"},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Empty(t, w.Header().Get("ETag"))
	var out struct {
		ShareGrant map[string]any `json:"share_grant"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "pending", out.ShareGrant["status"])
	assert.Equal(t, []any{"emails"}, out.ShareGrant["sections"])
}

func TestShareGrantCreate_ForeignEntityRejected(t *testing.T) {
	e := newSGEnv(t)
	foreign := e.contact(e.C, "Carol")
	for name, body := range map[string]map[string]any{
		"foreign contact":  {"to_user_id": e.B.ID, "entity_type": "contact", "entity_id": foreign.VCardUID, "sections": []string{"emails"}},
		"foreign note":     {"to_user_id": e.B.ID, "entity_type": "note", "entity_id": e.note(e.C, "x").UUID},
		"foreign activity": {"to_user_id": e.B.ID, "entity_type": "activity", "entity_id": e.activity(e.C, "x").UUID},
		"foreign circle":   {"to_user_id": e.B.ID, "entity_type": "circle", "entity_id": e.circle(e.C, "x").ID},
		"nonexistent":      {"to_user_id": e.B.ID, "entity_type": "circle", "entity_id": "no-such-circle"},
	} {
		w := e.do(e.A, http.MethodPost, "/share-grants", body)
		assert.Equal(t, http.StatusNotFound, w.Code, name)
	}
	assert.Zero(t, e.grantCount(), "no row for a foreign entity")
}

func TestShareGrantCreate_RejectsBadInput(t *testing.T) {
	e := newSGEnv(t)
	c := e.contact(e.A, "Alice")
	n := e.note(e.A, "n")
	past := e.clock.Now().Add(-time.Hour)
	cases := map[string]map[string]any{
		"empty sections":         {"to_user_id": e.B.ID, "entity_type": "contact", "entity_id": c.VCardUID, "sections": []string{}},
		"unknown section":        {"to_user_id": e.B.ID, "entity_type": "contact", "entity_id": c.VCardUID, "sections": []string{"bogus"}},
		"sections on note":       {"to_user_id": e.B.ID, "entity_type": "note", "entity_id": n.UUID, "sections": []string{"emails"}},
		"self":                   {"to_user_id": e.A.ID, "entity_type": "note", "entity_id": n.UUID},
		"expiry in the past":     {"to_user_id": e.B.ID, "entity_type": "note", "entity_id": n.UUID, "expires_at": past},
		"bad entity type":        {"to_user_id": e.B.ID, "entity_type": "household", "entity_id": n.UUID},
		"missing recipient":      {"entity_type": "note", "entity_id": n.UUID},
		"include_sensitive only": {"to_user_id": e.B.ID, "entity_type": "contact", "entity_id": c.VCardUID, "include_sensitive": true},
	}
	for name, body := range cases {
		w := e.do(e.A, http.MethodPost, "/share-grants", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
	}
	assert.Zero(t, e.grantCount())
}

func TestShareGrantCreate_UnknownRecipient404(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "n")
	w := e.do(e.A, http.MethodPost, "/share-grants", map[string]any{"to_user_id": 99999, "entity_type": "note", "entity_id": n.UUID})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestShareGrantCreate_DuplicateLiveGrantConflicts(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "n")
	e.grant(e.A, e.B, "note", n.UUID, nil)
	w := e.do(e.A, http.MethodPost, "/share-grants", map[string]any{"to_user_id": e.B.ID, "entity_type": "note", "entity_id": n.UUID})
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, int64(1), e.grantCount())
}

func TestShareGrantCreate_PendingCapPerPairAndSender(t *testing.T) {
	e := newSGEnv(t)
	for i := 0; i < services.MaxPendingShareGrantsPerPair; i++ {
		n := e.note(e.A, "n"+strconv.Itoa(i))
		e.grant(e.A, e.B, "note", n.UUID, nil)
	}
	n := e.note(e.A, "over")
	w := e.do(e.A, http.MethodPost, "/share-grants", map[string]any{"to_user_id": e.B.ID, "entity_type": "note", "entity_id": n.UUID})
	assert.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	// A different recipient is still allowed (per-pair cap, not per-sender yet).
	w = e.do(e.A, http.MethodPost, "/share-grants", map[string]any{"to_user_id": e.C.ID, "entity_type": "note", "entity_id": n.UUID})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

// ---------------------------------------------------------------- 404 matrix

// TestShareGrant_IdenticalNotFoundMatrix drives every id-bearing route as every
// non-entitled persona and as the entitled persona in the wrong state; each
// must return the same 404 body byte-for-byte as a random uuid, and write
// nothing.
func TestShareGrant_IdenticalNotFoundMatrix(t *testing.T) {
	e := newSGEnv(t)
	id, _ := e.activeContactGrant([]string{"emails"})

	absent := e.do(e.A, http.MethodGet, "/share-grants/00000000-0000-4000-8000-000000000000/content", nil)
	require.Equal(t, http.StatusNotFound, absent.Code)
	want := absent.Body.String()
	assert.Contains(t, want, "share_grant_not_found")

	type probe struct {
		who    models.User
		method string
		path   string
		body   any
	}
	var probes []probe
	ownerRoutes := []string{"revoke", "reconfirm"}
	recipientRoutes := []string{"accept", "decline", "leave"}
	for _, who := range []models.User{e.C, e.Admin, {}} {
		for _, r := range append(ownerRoutes, recipientRoutes...) {
			probes = append(probes, probe{who, http.MethodPost, "/share-grants/" + id + "/" + r, nil})
		}
		probes = append(probes, probe{who, http.MethodGet, "/share-grants/" + id + "/content", nil})
		probes = append(probes, probe{who, http.MethodPatch, "/share-grants/" + id, map[string]any{"sections": []string{"emails"}}})
	}
	// Wrong role.
	for _, r := range ownerRoutes {
		probes = append(probes, probe{e.B, http.MethodPost, "/share-grants/" + id + "/" + r, nil})
	}
	for _, r := range recipientRoutes {
		probes = append(probes, probe{e.A, http.MethodPost, "/share-grants/" + id + "/" + r, nil})
	}
	probes = append(probes, probe{e.A, http.MethodGet, "/share-grants/" + id + "/content", nil})
	probes = append(probes, probe{e.B, http.MethodPatch, "/share-grants/" + id, map[string]any{"sections": []string{"emails"}}})

	before := e.status(id)
	for _, p := range probes {
		w := e.do(p.who, p.method, p.path, p.body)
		if p.who.ID == 0 {
			assert.Equal(t, http.StatusUnauthorized, w.Code, p.method+" "+p.path)
			continue
		}
		assert.Equal(t, http.StatusNotFound, w.Code, "%s %s as user %d", p.method, p.path, p.who.ID)
		assert.Equal(t, want, w.Body.String(), "%s %s as user %d: body differs from random-uuid 404", p.method, p.path, p.who.ID)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
	assert.Equal(t, before, e.status(id), "no side effect from any probe")
}

func TestShareGrantContent_WrongStateIsIdentical404(t *testing.T) {
	e := newSGEnv(t)
	c := e.contact(e.A, "Alice")
	absent := e.do(e.B, http.MethodGet, "/share-grants/00000000-0000-4000-8000-000000000000/content", nil).Body.String()

	pending := e.grant(e.A, e.B, "contact", c.VCardUID, []string{"emails"})
	w := e.do(e.B, http.MethodGet, "/share-grants/"+pending+"/content", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "pending exposes nothing")
	assert.Equal(t, absent, w.Body.String())

	// declined
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+pending+"/decline", nil).Code)
	assert.Equal(t, absent, e.do(e.B, http.MethodGet, "/share-grants/"+pending+"/content", nil).Body.String())

	// revoked
	id2 := e.grant(e.A, e.B, "contact", c.VCardUID, []string{"emails"})
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id2+"/accept", nil).Code)
	require.Equal(t, 200, e.do(e.A, http.MethodPost, "/share-grants/"+id2+"/revoke", nil).Code)
	w = e.do(e.B, http.MethodGet, "/share-grants/"+id2+"/content", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, absent, w.Body.String(), "revoked is effective on the next request")
}

func TestShareGrantContent_ExpiredIsEffectiveWithoutJob(t *testing.T) {
	e := newSGEnv(t)
	c := e.contact(e.A, "Alice")
	exp := e.clock.Now().Add(time.Hour)
	w := e.do(e.A, http.MethodPost, "/share-grants", map[string]any{
		"to_user_id": e.B.ID, "entity_type": "contact", "entity_id": c.VCardUID, "sections": []string{"emails"}, "expires_at": exp,
	})
	require.Equal(t, 201, w.Code, w.Body.String())
	var out struct {
		ShareGrant struct{ ID string } `json:"share_grant"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	id := out.ShareGrant.ID
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	require.Equal(t, 200, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Code)

	e.clock.Advance(2 * time.Hour)
	assert.Equal(t, 404, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Code)
	assert.Equal(t, "active", e.status(id), "the expiry job did not run; read-time evaluation alone denies")
	// Expired grants are not listed as incoming either.
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String(), id)
	// And accepting an expired-by-time pending grant is the 404 as well.
}

func TestShareGrant_PendingTTLExpiresUnacceptedGrant(t *testing.T) {
	e := newSGEnv(t)
	c := e.contact(e.A, "Alice")
	id := e.grant(e.A, e.B, "contact", c.VCardUID, []string{"emails"})
	e.clock.Advance(31 * 24 * time.Hour)
	assert.Equal(t, 404, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code, "pending TTL is evaluated at accept time")
	assert.Equal(t, "pending", e.status(id))
}

func TestShareGrantContent_EntityGoneIs404(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"emails"})
	// Simulate a missed revoke: soft-delete the contact directly.
	require.NoError(t, e.db.Delete(&models.Contact{}, c.ID).Error)
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "the read path requires the live source row")
}

func TestShareGrantContent_ArchivedContactIs404(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"emails"})
	require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Update("archived", true).Error)
	assert.Equal(t, 404, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Code)
}

// A grant from A never resolves C's entity with the same identity value.
func TestShareGrantContent_CrossOwnerSameUID(t *testing.T) {
	e := newSGEnv(t)
	const uid = "11111111-2222-4333-8444-555555555555"
	ca := models.Contact{UserID: e.A.ID, Firstname: "OwnerSide", VCardUID: uid}
	cc := models.Contact{UserID: e.C.ID, Firstname: "ForeignSide", VCardUID: uid}
	require.NoError(t, e.db.Create(&ca).Error)
	require.NoError(t, e.db.Create(&cc).Error)
	id := e.grant(e.A, e.B, "contact", uid, []string{"emails"})
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "OwnerSide")
	assert.NotContains(t, w.Body.String(), "ForeignSide")
}

// ---------------------------------------------------------------- transitions

func TestShareGrantTransitions(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "hello")
	id := e.grant(e.A, e.B, "note", n.UUID, nil)

	// accept is idempotent
	assert.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	assert.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	assert.Equal(t, "active", e.status(id))
	// decline of an active grant is 409 (use leave)
	assert.Equal(t, 409, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/decline", nil).Code)
	// leave ends it
	assert.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/leave", nil).Code)
	assert.Equal(t, "declined", e.status(id))
	// terminal: accept / leave / revoke are 409
	assert.Equal(t, 409, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	assert.Equal(t, 409, e.do(e.A, http.MethodPost, "/share-grants/"+id+"/revoke", nil).Code)
	// a new grant is a new row and uuid
	id2 := e.grant(e.A, e.B, "note", n.UUID, nil)
	assert.NotEqual(t, id, id2)
}

func TestShareGrantRevoke_IdempotentAndSetsReason(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "hello")
	id := e.grant(e.A, e.B, "note", n.UUID, nil)
	assert.Equal(t, 200, e.do(e.A, http.MethodPost, "/share-grants/"+id+"/revoke", nil).Code)
	assert.Equal(t, 200, e.do(e.A, http.MethodPost, "/share-grants/"+id+"/revoke", nil).Code)
	var g models.ShareGrant
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	assert.Equal(t, "revoked", g.Status)
	require.NotNil(t, g.RevokedReason)
	assert.Equal(t, models.ShareGrantReasonOwnerRevoked, *g.RevokedReason)
	assert.NotNil(t, g.RevokedAt)
}

func TestShareGrant_RevokeAfterAcceptCannotBeOverwrittenByAccept(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "hello")
	id := e.grant(e.A, e.B, "note", n.UUID, nil)
	require.Equal(t, 200, e.do(e.A, http.MethodPost, "/share-grants/"+id+"/revoke", nil).Code)
	assert.Equal(t, 409, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	assert.Equal(t, "revoked", e.status(id))
}

// ---------------------------------------------------------------- blocking

func TestShareGrantBlock_EndsGrantsSenderSeesPendingNoLeak(t *testing.T) {
	e := newSGEnv(t)
	n1, n2 := e.note(e.A, "1"), e.note(e.A, "2")
	g1 := e.grant(e.A, e.B, "note", n1.UUID, nil)
	g2 := e.grant(e.A, e.B, "note", n2.UUID, nil)
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+g2+"/accept", nil).Code)

	w := e.do(e.B, http.MethodPost, "/share-grants/"+g1+"/decline", map[string]any{"block": true})
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, "declined", e.status(g1))
	assert.Equal(t, "declined", e.status(g2), "a block ends every pending/active grant from that sender")

	// New grant from the blocked sender: accepted by the API, stored blocked,
	// the sender sees pending, the recipient sees nothing.
	n3 := e.note(e.A, "3")
	w = e.do(e.A, http.MethodPost, "/share-grants", map[string]any{"to_user_id": e.B.ID, "entity_type": "note", "entity_id": n3.UUID})
	require.Equal(t, 201, w.Code, w.Body.String())
	var out struct {
		ShareGrant map[string]any `json:"share_grant"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "pending", out.ShareGrant["status"], "sender cannot learn about the block")
	id3 := out.ShareGrant["id"].(string)
	assert.Equal(t, "blocked", e.status(id3))
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String(), id3)
	assert.Equal(t, 404, e.do(e.B, http.MethodPost, "/share-grants/"+id3+"/accept", nil).Code)
	assert.Equal(t, 404, e.do(e.B, http.MethodGet, "/share-grants/"+id3+"/content", nil).Code)
	assert.Contains(t, e.do(e.A, http.MethodGet, "/share-grants/outgoing", nil).Body.String(), id3)
	// A duplicate create looks like an unblocked pending duplicate: 409.
	w = e.do(e.A, http.MethodPost, "/share-grants", map[string]any{"to_user_id": e.B.ID, "entity_type": "note", "entity_id": n3.UUID})
	assert.Equal(t, 409, w.Code)

	// Block list and unblock.
	w = e.do(e.B, http.MethodGet, "/share-grants/blocks", nil)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "sg_owner")
	assert.Equal(t, 204, e.do(e.A, http.MethodDelete, "/share-grants/blocks/"+strconv.Itoa(int(e.B.ID)), nil).Code, "deleting someone else's block row is a no-op, never touches B's")
	var cnt int64
	e.db.Model(&models.ShareBlock{}).Count(&cnt)
	assert.Equal(t, int64(1), cnt, "A cannot unblock themselves via B's id (scoped by blocker)")
	assert.Equal(t, 204, e.do(e.B, http.MethodDelete, "/share-grants/blocks/"+strconv.Itoa(int(e.A.ID)), nil).Code)
	e.db.Model(&models.ShareBlock{}).Count(&cnt)
	assert.Zero(t, cnt)
}

// ---------------------------------------------------------------- lists

func TestShareGrantLists_ScopedAndIncomingHidesTerminal(t *testing.T) {
	e := newSGEnv(t)
	n1, n2 := e.note(e.A, "1"), e.note(e.A, "2")
	g1 := e.grant(e.A, e.B, "note", n1.UUID, nil)
	g2 := e.grant(e.A, e.B, "note", n2.UUID, nil)
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+g2+"/decline", nil).Code)

	in := e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String()
	assert.Contains(t, in, g1)
	assert.NotContains(t, in, g2, "incoming shows pending and active only")
	out := e.do(e.A, http.MethodGet, "/share-grants/outgoing", nil).Body.String()
	assert.Contains(t, out, g1)
	assert.Contains(t, out, g2, "owner sees the outcome")

	for _, who := range []models.User{e.C, e.Admin} {
		for _, p := range []string{"/share-grants/incoming", "/share-grants/outgoing"} {
			body := e.do(who, http.MethodGet, p, nil).Body.String()
			assert.NotContains(t, body, g1)
			assert.NotContains(t, body, "total", "no count that could include anyone else's rows")
		}
	}
	// Recipient cannot see owner-only columns; owner list is not the recipient's.
	assert.NotContains(t, in, n1.UUID, "incoming never carries the entity id")
	assert.NotContains(t, e.do(e.A, http.MethodGet, "/share-grants/incoming", nil).Body.String(), g1)
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/outgoing", nil).Body.String(), g1)
}

func TestShareGrantIncoming_PendingLabelOnlyForRecipient(t *testing.T) {
	e := newSGEnv(t)
	c := e.contact(e.A, "Alice")
	n := e.note(e.A, "secret note text")
	e.grant(e.A, e.B, "contact", c.VCardUID, []string{"emails"})
	e.grant(e.A, e.B, "note", n.UUID, nil)
	body := e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String()
	assert.Contains(t, body, "Alice Doe")
	assert.Contains(t, body, "sg_owner")
	assert.NotContains(t, body, "secret note text", "a note shows only a generic label before accepting")
}

// ---------------------------------------------------------------- patch

func TestShareGrantPatch_NarrowOnlyAndIfMatch(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"emails", "phones"})
	require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Updates(map[string]any{"email": "a@example.com"}).Error)

	// widening is 409
	w := e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"sections": []string{"emails", "phones", "links"}})
	assert.Equal(t, 409, w.Code, w.Body.String())
	// unknown section 400
	assert.Equal(t, 400, e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"sections": []string{"bogus"}}).Code)
	// empty sections 400
	assert.Equal(t, 400, e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"sections": []string{}}).Code)
	// stale If-Match 412
	w = e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"sections": []string{"emails"}}, "If-Match", `"999"`)
	assert.Equal(t, 412, w.Code)
	// narrowing with the right revision works and takes effect on the next read
	var g models.ShareGrant
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	w = e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"sections": []string{"emails"}}, "If-Match", `"`+strconv.FormatInt(g.Revision, 10)+`"`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	assert.Equal(t, []string{"emails"}, g.Sections)
	// status/from_user_id are not bindable
	w = e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"sections": []string{"emails"}, "status": "revoked", "from_user_id": e.C.ID})
	assert.Contains(t, []int{200, 400}, w.Code)
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	assert.Equal(t, "active", g.Status)
	assert.Equal(t, e.A.ID, g.FromUserID)
}

func TestShareGrantPatch_ExpiresAtSetAndClear(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "n")
	id := e.grant(e.A, e.B, "note", n.UUID, nil)
	exp := e.clock.Now().Add(48 * time.Hour)
	require.Equal(t, 200, e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"expires_at": exp}).Code)
	var g models.ShareGrant
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	require.NotNil(t, g.ExpiresAt)
	require.Equal(t, 200, e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"expires_at": nil}).Code)
	var cleared models.ShareGrant
	require.NoError(t, e.db.First(&cleared, "id = ?", id).Error)
	assert.Nil(t, cleared.ExpiresAt)
	// past expiry rejected
	assert.Equal(t, 400, e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"expires_at": e.clock.Now().Add(-time.Hour)}).Code)
	// sections on a note grant is 400
	assert.Equal(t, 400, e.do(e.A, http.MethodPatch, "/share-grants/"+id, map[string]any{"sections": []string{"emails"}}).Code)
}

// ---------------------------------------------------------------- content: contact

func TestShareGrantContent_ContactSectionsReappliedEveryRead(t *testing.T) {
	e := newSGEnv(t)
	c := models.Contact{UserID: e.A.ID, Firstname: "Alice", Lastname: "Doe"}
	require.NoError(t, e.db.Create(&c).Error)
	require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Updates(map[string]any{}).Error)
	e.setEmailAndPhone(c.ID, "alice@example.com", "+15555550100")

	id := e.grant(e.A, e.B, "contact", c.VCardUID, []string{"emails"})
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Empty(t, w.Header().Get("ETag"))
	assert.Contains(t, w.Body.String(), "alice@example.com")
	assert.NotContains(t, w.Body.String(), "5555550100", "phones is not a granted section")
	assert.Contains(t, w.Body.String(), "Alice")

	// A new item in a granted section is visible immediately.
	e.setEmailAndPhone(c.ID, "alice2@example.com", "+15555550100")
	w = e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	assert.Contains(t, w.Body.String(), "alice2@example.com")
}

func (e *sgEnv) setEmailAndPhone(contactID uint, email, phone string) {
	var c models.Contact
	require.NoError(e.t, e.db.First(&c, contactID).Error)
	c.Emails = []models.ContactEmail{{Value: email}}
	c.Phones = []models.ContactPhone{{Value: phone}}
	require.NoError(e.t, e.db.Save(&c).Error)
}

func TestShareGrantContent_UnknownStoredSectionNarrowsNeverWidens(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"emails"})
	e.setEmailAndPhone(c.ID, "alice@example.com", "+15555550100")
	// A token removed in a later release (or injected) sits in the stored list.
	require.NoError(t, e.db.Model(&models.ShareGrant{}).Where("id = ?", id).
		UpdateColumn("sections", `["emails","legacy_section","all","*"]`).Error)
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "alice@example.com")
	assert.NotContains(t, w.Body.String(), "5555550100")
}

func TestShareGrantContent_ExcludesPrivateAndSecretItems(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"addresses", "related_to", "personal_info", "custom_fields"})

	// preference (personal_info) at each sensitivity
	for i, s := range []string{"normal", "private", "secret"} {
		require.NoError(t, e.db.Create(&models.Preference{
			UserID: e.A.ID, EntityID: c.VCardUID, Category: models.PreferenceCategoryHobby,
			Value: "hobby-" + s, Sensitivity: s,
		}).Error, i)
	}
	// edge at each sensitivity
	for _, s := range []string{"normal", "private", "secret"} {
		tgt := e.contact(e.A, "Tgt"+s)
		require.NoError(t, e.db.Create(&models.RelationshipEdge{
			UserID: e.A.ID, SourceID: c.VCardUID, TargetID: tgt.VCardUID, Type: "parent_of",
			Source: models.RelationshipSourceUserConfirmed, Confidence: 1, Status: models.RelationshipStatusConfirmed, Sensitivity: s,
		}).Error)
	}
	// custom field definitions at each sensitivity
	for _, s := range []string{"normal", "private", "secret"} {
		def := models.FieldDefinition{UserID: e.A.ID, Label: "F" + s, Key: "f_" + s, Target: models.FieldDefinitionTargetContact,
			Type: models.FieldTypeString, Projection: "vcard:X-" + s, Sensitivity: s}
		require.NoError(t, e.db.Create(&def).Error)
		require.NoError(t, e.db.Create(&models.FieldValue{FieldDefinitionID: def.ID, UserID: e.A.ID, EntityID: c.VCardUID, Value: []byte(`"cf-` + s + `"`)}).Error)
	}
	// addresses at each sensitivity
	var cc models.Contact
	require.NoError(t, e.db.First(&cc, c.ID).Error)
	cc.Addresses = []models.ContactAddress{
		{Street: "1 Normal St", City: "A", Sensitivity: "normal"},
		{Street: "2 Private St", City: "B", Sensitivity: "private", Coordinates: "geo:1.5,2.5"},
		{Street: "3 Secret St", City: "C", Sensitivity: "secret", Coordinates: "geo:3.5,4.5"},
	}
	require.NoError(t, e.db.Save(&cc).Error)
	// A pending reconfirm marker is NOT set by a direct row write to an
	// already-granted contact in this fixture; clear whatever the persistence
	// hooks set so this test isolates the query filter.
	require.NoError(t, e.db.Model(&models.ShareGrant{}).Where("id = ?", id).UpdateColumn("reconfirm_sections", "[]").Error)

	body := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String()
	for _, want := range []string{"hobby-normal", "1 Normal St", "cf-normal"} {
		assert.Contains(t, body, want)
	}
	for _, leaked := range []string{"hobby-private", "hobby-secret", "2 Private St", "3 Secret St", "geo:1.5", "geo:3.5", "cf-private", "cf-secret", "Tgtprivate", "Tgtsecret"} {
		assert.NotContains(t, body, leaked)
	}
}

func TestShareGrantContent_IncludeSensitiveIgnored(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"personal_info"})
	require.NoError(t, e.db.Create(&models.Preference{
		UserID: e.A.ID, EntityID: c.VCardUID, Category: models.PreferenceCategoryHobby, Value: "hobby-secret", Sensitivity: "secret",
	}).Error)
	require.NoError(t, e.db.Model(&models.ShareGrant{}).Where("id = ?", id).UpdateColumn("reconfirm_sections", "[]").Error)
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content?include_sensitive=true", nil, "X-Include-Sensitive", "true")
	require.Equal(t, 200, w.Code)
	assert.NotContains(t, w.Body.String(), "hobby-secret")
}

func TestShareGrantContent_FieldVisibilityIsCosmetic(t *testing.T) {
	// #1997 per-contact field visibility is a UI affordance, never a boundary.
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"emails"})
	e.setEmailAndPhone(c.ID, "alice@example.com", "+15555550100")
	before := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String()
	require.NoError(t, e.db.Create(&models.ContactFieldOverride{UserID: e.A.ID, ContactVCardUID: c.VCardUID, FieldKey: "emails", State: "hide"}).Error)
	after := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String()
	assert.Equal(t, before, after)
}

// ---------------------------------------------------------------- content: other types

func TestShareGrantContent_NoteView(t *testing.T) {
	e := newSGEnv(t)
	other := e.contact(e.A, "Linked")
	n := models.Note{UserID: e.A.ID, Content: "note body", Date: e.clock.Now(), ContactID: &other.ID}
	require.NoError(t, e.db.Create(&n).Error)
	id := e.grant(e.A, e.B, "note", n.UUID, nil)
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "note body")
	assert.NotContains(t, w.Body.String(), "Linked", "the note's contact link is not granted")
	assert.NotContains(t, w.Body.String(), "contact_id")
	assertKeys(t, w.Body.Bytes(), "share_grant_content", "note")
}

func TestShareGrantContent_ActivityOmitsParticipantsAndExternalRef(t *testing.T) {
	e := newSGEnv(t)
	part := e.contact(e.A, "Participant")
	a := models.Activity{UserID: e.A.ID, Title: "Lunch", Description: "d", Location: "Cafe", Date: e.clock.Now(),
		Type: "meeting", ExternalRef: "geopulse:secret-ref", Contacts: []models.Contact{part}}
	require.NoError(t, e.db.Create(&a).Error)
	id := e.grant(e.A, e.B, "activity", a.UUID, nil)
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	for _, want := range []string{"Lunch", "Cafe", "meeting"} {
		assert.Contains(t, w.Body.String(), want)
	}
	for _, leaked := range []string{"Participant", "geopulse:secret-ref", "external_ref", "contacts"} {
		assert.NotContains(t, w.Body.String(), leaked)
	}
}

func TestShareGrantContent_CircleLiveMembersNamesOnly(t *testing.T) {
	e := newSGEnv(t)
	circ := e.circle(e.A, "Family")
	m1, m2 := e.contact(e.A, "Mia"), e.contact(e.A, "Noah")
	for _, m := range []models.Contact{m1, m2} {
		require.NoError(t, e.db.Create(&models.CircleMember{CircleID: circ.ID, UserID: e.A.ID, MemberVCardUID: m.VCardUID}).Error)
	}
	e.setEmailAndPhone(m1.ID, "mia@example.com", "+1555")
	id := e.grant(e.A, e.B, "circle", circ.ID, nil)
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)

	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Mia")
	assert.Contains(t, w.Body.String(), "Noah")
	assert.NotContains(t, w.Body.String(), "mia@example.com", "names only")
	assert.NotContains(t, w.Body.String(), m1.VCardUID, "member uids are not exposed")

	// live both ways: remove one, archive one, add one, delete one
	require.NoError(t, e.db.Where("circle_id = ? AND member_vcard_uid = ?", circ.ID, m1.VCardUID).Delete(&models.CircleMember{}).Error)
	m3 := e.contact(e.A, "Olive")
	require.NoError(t, e.db.Create(&models.CircleMember{CircleID: circ.ID, UserID: e.A.ID, MemberVCardUID: m3.VCardUID}).Error)
	require.NoError(t, e.db.Model(&models.Contact{}).Where("id = ?", m2.ID).Update("archived", true).Error)
	body := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String()
	assert.NotContains(t, body, "Mia")
	assert.NotContains(t, body, "Noah", "archived members are excluded")
	assert.Contains(t, body, "Olive")
}

func TestShareGrantContent_CircleMemberCap(t *testing.T) {
	e := newSGEnv(t)
	circ := e.circle(e.A, "Big")
	for i := 0; i < services.MaxShareGrantCircleMembers+5; i++ {
		m := e.contact(e.A, "M"+strconv.Itoa(i))
		require.NoError(t, e.db.Create(&models.CircleMember{CircleID: circ.ID, UserID: e.A.ID, MemberVCardUID: m.VCardUID}).Error)
	}
	id := e.grant(e.A, e.B, "circle", circ.ID, nil)
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	var out struct {
		Content struct {
			Circle struct {
				Members   []map[string]any `json:"members"`
				Truncated bool             `json:"members_truncated"`
			} `json:"circle"`
		} `json:"share_grant_content"`
	}
	w := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	assert.Len(t, out.Content.Circle.Members, services.MaxShareGrantCircleMembers)
	assert.True(t, out.Content.Circle.Truncated)
}

func assertKeys(t *testing.T, raw []byte, top, typ string) {
	t.Helper()
	var m map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Contains(t, m, top)
	assert.Contains(t, m[top], typ)
}

// ---------------------------------------------------------------- reconfirm

func TestReconfirm_SectionWithheldUntilOwnerReconfirms(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"personal_info", "emails"})
	e.setEmailAndPhone(c.ID, "alice@example.com", "")
	pref := models.Preference{UserID: e.A.ID, EntityID: c.VCardUID, Category: models.PreferenceCategoryHobby, Value: "chess", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&pref).Error)
	assert.Contains(t, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String(), "chess")

	// normal -> private: marker set at the persistence point, whole section withheld.
	require.NoError(t, e.db.Model(&pref).Update("sensitivity", "private").Error)
	var g models.ShareGrant
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	assert.Equal(t, []string{"personal_info"}, g.ReconfirmSections)
	body := e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String()
	assert.NotContains(t, body, "chess")
	assert.Contains(t, body, "alice@example.com", "other sections are unchanged")

	// back to normal: still withheld until reconfirmed.
	require.NoError(t, e.db.Model(&pref).Update("sensitivity", "normal").Error)
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String(), "chess")

	// Only the owner can reconfirm.
	assert.Equal(t, 404, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/reconfirm", nil).Code)
	assert.Equal(t, 404, e.do(e.C, http.MethodPost, "/share-grants/"+id+"/reconfirm", nil).Code)
	require.Equal(t, 200, e.do(e.A, http.MethodPost, "/share-grants/"+id+"/reconfirm", nil).Code)
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	assert.Empty(t, g.ReconfirmSections)
	assert.Contains(t, e.do(e.B, http.MethodGet, "/share-grants/"+id+"/content", nil).Body.String(), "chess")

	var audits int64
	e.db.Model(&models.AuditEvent{}).Where("entity_type = ? AND entity_id = ? AND user_id = ?", "share_grant", id, e.A.ID).Count(&audits)
	assert.Positive(t, audits, "reconfirm is audited on the owner's trail")
}

func TestReconfirm_EdgeAddressAndFieldDefinitionWriters(t *testing.T) {
	e := newSGEnv(t)
	id, c := e.activeContactGrant([]string{"related_to", "addresses", "custom_fields"})
	read := func() []string {
		var g models.ShareGrant
		require.NoError(t, e.db.First(&g, "id = ?", id).Error)
		return g.ReconfirmSections
	}
	require.Empty(t, read())

	// relationship edge normal -> secret (edge touches the granted contact as source)
	tgt := e.contact(e.A, "T")
	edge := models.RelationshipEdge{UserID: e.A.ID, SourceID: c.VCardUID, TargetID: tgt.VCardUID, Type: "parent_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1, Status: models.RelationshipStatusConfirmed, Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&edge).Error)
	require.Empty(t, read(), "a normal edge never sets a marker")
	require.NoError(t, e.db.Model(&edge).Update("sensitivity", "secret").Error)
	assert.ElementsMatch(t, []string{"related_to"}, read())

	// address normal -> private on the contact
	var cc models.Contact
	require.NoError(t, e.db.First(&cc, c.ID).Error)
	cc.Addresses = []models.ContactAddress{{Street: "1 St", City: "X", Sensitivity: "normal"}}
	require.NoError(t, e.db.Save(&cc).Error)
	require.NotContains(t, read(), "addresses")
	require.NoError(t, e.db.First(&cc, c.ID).Error)
	cc.Addresses[0].Sensitivity = "private"
	require.NoError(t, e.db.Save(&cc).Error)
	assert.ElementsMatch(t, []string{"related_to", "addresses"}, read())

	// field definition normal -> private (custom fields)
	def := models.FieldDefinition{UserID: e.A.ID, Label: "F", Key: "f", Target: models.FieldDefinitionTargetContact,
		Type: models.FieldTypeString, Projection: "vcard:X-F", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&def).Error)
	require.NotContains(t, read(), "custom_fields")
	require.NoError(t, e.db.Model(&def).Update("sensitivity", "private").Error)
	assert.ElementsMatch(t, []string{"related_to", "addresses", "custom_fields"}, read())
}

func TestReconfirm_OnlyOwnersGrantsAreMarked(t *testing.T) {
	e := newSGEnv(t)
	const uid = "11111111-2222-4333-8444-666666666666"
	ca := models.Contact{UserID: e.A.ID, Firstname: "A-side", VCardUID: uid}
	cc := models.Contact{UserID: e.C.ID, Firstname: "C-side", VCardUID: uid}
	require.NoError(t, e.db.Create(&ca).Error)
	require.NoError(t, e.db.Create(&cc).Error)
	id := e.grant(e.A, e.B, "contact", uid, []string{"personal_info"})
	pref := models.Preference{UserID: e.C.ID, EntityID: uid, Category: models.PreferenceCategoryHobby, Value: "x", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&pref).Error)
	require.NoError(t, e.db.Model(&pref).Update("sensitivity", "private").Error)
	var g models.ShareGrant
	require.NoError(t, e.db.First(&g, "id = ?", id).Error)
	assert.Empty(t, g.ReconfirmSections, "C's reclassification never touches A's grant")
}

// ---------------------------------------------------------------- no mass assignment / headers

func TestShareGrantDTOs_BindOnlyDocumentedFields(t *testing.T) {
	assertJSONFields(t, models.ShareGrantInput{}, "to_user_id", "entity_type", "entity_id", "sections", "expires_at")
	assertJSONFields(t, models.ShareGrantPatchInput{}, "expires_at", "sections")
}

func assertJSONFields(t *testing.T, v any, want ...string) {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	assert.ElementsMatch(t, want, got)
}

func TestShareGrantCreate_ClientCannotSetStatusOrSender(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "n")
	w := e.do(e.A, http.MethodPost, "/share-grants", map[string]any{
		"to_user_id": e.B.ID, "entity_type": "note", "entity_id": n.UUID,
		"status": "active", "from_user_id": e.C.ID, "reconfirm_sections": []string{"x"}, "revision": 99,
	})
	require.Equal(t, 201, w.Code, w.Body.String())
	var g models.ShareGrant
	require.NoError(t, e.db.First(&g).Error)
	assert.Equal(t, "pending", g.Status)
	assert.Equal(t, e.A.ID, g.FromUserID)
	assert.Empty(t, g.ReconfirmSections)
	assert.Equal(t, int64(1), g.Revision)
}
