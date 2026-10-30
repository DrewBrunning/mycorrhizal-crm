package routes

// TestQueryScopeMatrix is the fourth authorization surface for this API,
// beside routes/authorization_matrix_test.go (path ids, status code only),
// routes/authorization_matrix_credentials_test.go (non-JWT credentials) and
// routes/ownership_matrix_test.go (identifiers in the request body, #877).
// Issue #2201.
//
// None of those reaches an identifier or scope carried in the *query string*
// (?entity_id=, ?contact_id=, ?vcard_uid=, ?household_id=, ?circle=,
// ?circle_ids=, ?event_id=, ?from=, ?since=, ?include_*=), and none asserts
// that a response contains only in-scope rows. Issue #2196 (the single-contact
// JSContact export returned every contact) slipped through exactly that gap.
// A 2026-10-29 audit found zero cross-user leaks; this test makes that result
// permanent.
//
// ── What each row asserts ─────────────────────────────────────────────────
//
// Two users ("owner" and "intruder") are seeded with structurally identical
// data. Every row names one GET route and the scope query it understands, and
// declares which seed markers a correct response contains (`in`) and which it
// must not (`out`). Markers are either row ids / contact UIDs (uuids) or text
// containing the actor's tag ("OWNQ" / "INTQ"), so a leak is found by scanning
// the RAW response body — not by decoding a shape that a leaky handler could
// change.
//
//  1. Intruder check — the intruder sends the owner's scope values on the
//     intruder's own token; the body must contain nothing of the owner's (no
//     tag text, no owner row id, no owner contact UID). Values the request
//     itself carries are exempt from the scan, because several handlers
//     legitimately echo a supplied id back in an error envelope or in a
//     `circle_ids` field; a leaked row is still caught by its row id and its
//     tag text. A 5xx is also a failure. Rows may add `cross` probes that
//     MIX the intruder's own ids with the owner's.
//  2. Narrowing check — the owner sends the scope; the body must contain the
//     in-scope markers and none of the owner's other-scope markers. The owner's
//     response must also never contain any of the intruder's markers. This is
//     the check that fails against a handler that ignores its scope parameter.
//  3. Positive controls — (a) the unscoped owner request (when the route has a
//     meaningful unscoped form) shows BOTH scopes, so a route that returns
//     nothing cannot pass the narrowing check vacuously; (b) the intruder's own
//     scope on the intruder's own token returns the intruder's markers, so the
//     intruder check is not an empty-body pass either.
//
// ── The completeness guard is the point ──────────────────────────────────
//
// Two independent guards fail in both directions, so a new scope parameter
// lands red until a human classifies it:
//
//   - Handler guard: qsScanQueryReads parses the controllers and middleware
//     packages with go/ast and returns every "Func:param" for which a Gin
//     Query / DefaultQuery / GetQuery / QueryArray / GetQueryArray call has a
//     string-literal name. Each name must be a known scope parameter (probed by
//     a row's `reads`, or listed in qsAllow with a written reason) or a known
//     non-scope parameter (qsNonScope). A name in neither set is `unclassified`;
//     a declared token no handler reads is `stale`.
//   - Spec guard: every GET path in backend/openapi.yaml that declares a
//     scope-named query parameter must have a matrix row for that route and
//     parameter, or a qsAllowSpec entry with a reason.
//
// Rows and allow-listed routes must also be registered GET routes.
//
// Known non-leaks (allow-listed with reasons below or noted here): /search
// echoes `q` back (the test uses a neutral term, not an owner marker);
// /users/directory and /link-field-types accept no scope parameter so they are
// out of this matrix's reach (the directory is covered by the docs issue, the
// built-in "Element/Matrix" link type is a global built-in).

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ── actors ────────────────────────────────────────────────────────────────

type qsActor struct {
	tag   string
	user  models.User
	token string

	c1, c2, c3, cArch, cTomb models.Contact

	circleID1, circleID2     string
	circleName1, circleName2 string
	householdID1             string
	householdID2             string
	tagID1, tagID2           string
	eventID1, eventID2       string

	// m maps a logical marker name ("gift1", "contact2", …) to the string that
	// appears in a response for that row: a uuid, a contact UID, or text
	// carrying the actor's tag.
	m map[string]string
	// scan lists every uuid/UID that identifies this actor's data. The tag
	// text itself is scanned separately.
	scan []string
}

func (a *qsActor) note(name, marker string, isID bool) string {
	a.m[name] = marker
	if isID {
		a.scan = append(a.scan, marker)
	}
	return marker
}

func qsSeedActor(t *testing.T, db *gorm.DB, cfg *config.Config, name, tag string) *qsActor {
	t.Helper()
	a := &qsActor{tag: tag, m: map[string]string{}}
	a.user = models.User{Username: "qs-" + name, Email: "qs-" + name + "@example.com", Password: "password123"}
	require.NoError(t, db.Create(&a.user).Error)
	tok, err := services.IssueSession(db, a.user, cfg, "", "")
	require.NoError(t, err)
	a.token = tok

	create := func(v any) {
		t.Helper()
		require.NoError(t, db.Create(v).Error)
	}
	contact := func(suffix string, mut func(*models.Contact)) models.Contact {
		c := models.Contact{UserID: a.user.ID, Firstname: tag + "-" + suffix, Lastname: "scopeq"}
		if mut != nil {
			mut(&c)
		}
		create(&c)
		a.note("contact-"+suffix, c.Firstname, false)
		a.note("uid-"+suffix, c.VCardUID, true)
		return c
	}
	a.c1 = contact("c1", func(c *models.Contact) {
		c.Circles = []string{tag + "-legacy1"}
		c.Addresses = []models.ContactAddress{{Type: "home", Street: "1 Scope St", City: "Townsville"}}
	})
	a.c2 = contact("c2", func(c *models.Contact) {
		c.Circles = []string{tag + "-legacy2"}
		c.Addresses = []models.ContactAddress{{Type: "home", Street: "2 Scope St", City: "Townsville"}}
	})
	a.c3 = contact("c3", nil)
	a.cArch = contact("arch", func(c *models.Contact) { c.Archived = true })
	a.cTomb = contact("tomb", nil)
	require.NoError(t, db.Delete(&a.cTomb).Error)

	// circles + legacy names
	mkCircle := func(n int, member models.Contact) (id, name string) {
		c := models.Circle{UserID: a.user.ID, Name: fmt.Sprintf("%s-circle%d", tag, n)}
		create(&c)
		create(&models.CircleMember{CircleID: c.ID, UserID: a.user.ID, MemberVCardUID: member.VCardUID})
		a.note(fmt.Sprintf("circle%d", n), c.ID, true)
		return c.ID, c.Name
	}
	a.circleID1, a.circleName1 = mkCircle(1, a.c1)
	a.circleID2, a.circleName2 = mkCircle(2, a.c2)

	mkHousehold := func(n int, member models.Contact) string {
		h := models.Household{UserID: a.user.ID, Name: fmt.Sprintf("%s-house%d", tag, n), Type: models.HouseholdTypeOther}
		create(&h)
		create(&models.HouseholdMember{HouseholdID: h.ID, UserID: a.user.ID, MemberVCardUID: member.VCardUID})
		a.note(fmt.Sprintf("house%d", n), h.ID, true)
		return h.ID
	}
	a.householdID1 = mkHousehold(1, a.c1)
	a.householdID2 = mkHousehold(2, a.c2)

	mkTag := func(n int, member models.Contact) string {
		tg := models.Tag{UserID: a.user.ID, Name: fmt.Sprintf("%s-tag%d", tag, n)}
		create(&tg)
		create(&models.ContactTag{TagID: tg.ID, UserID: a.user.ID, ContactVCardUID: member.VCardUID})
		a.note(fmt.Sprintf("tag%d", n), tg.ID, true)
		return tg.ID
	}
	a.tagID1 = mkTag(1, a.c1)
	a.tagID2 = mkTag(2, a.c2)

	// per-entity rows: one per scope contact (c1, c2) and one soft-deleted
	// tombstone (on c3, so it never joins a scope-narrowed list).
	type entityRow struct {
		key  string
		make func(entity string, n int) (id string, v any)
	}
	rows := []entityRow{
		{"life", func(e string, n int) (string, any) {
			v := models.LifeEvent{UserID: a.user.ID, EntityID: e, Type: models.LifeEventTypeMoved}
			return "", &v
		}},
		{"agenda", func(e string, n int) (string, any) {
			v := models.ConversationAgenda{UserID: a.user.ID, EntityID: e, Content: fmt.Sprintf("%s-agenda%d", tag, n)}
			return "", &v
		}},
		{"gift", func(e string, n int) (string, any) {
			v := models.Gift{UserID: a.user.ID, EntityID: e}
			return "", &v
		}},
		{"pref", func(e string, n int) (string, any) {
			v := models.Preference{UserID: a.user.ID, EntityID: e, Category: "food", Value: fmt.Sprintf("%s-pref%d", tag, n)}
			return "", &v
		}},
		{"oblig", func(e string, n int) (string, any) {
			v := models.OccasionObligation{UserID: a.user.ID, EntityID: e, Kind: "card", Label: fmt.Sprintf("%s-oblig%d", tag, n)}
			return "", &v
		}},
		{"cadence", func(e string, n int) (string, any) {
			v := models.CadencePolicy{UserID: a.user.ID, EntityID: e, TargetIntervalDays: 30}
			return "", &v
		}},
		{"decay", func(e string, n int) (string, any) {
			v := models.DataDecayPolicy{UserID: a.user.ID, EntityID: e, IntervalDays: 365}
			return "", &v
		}},
	}
	for _, r := range rows {
		for n, e := range map[int]string{1: a.c1.VCardUID, 2: a.c2.VCardUID, 3: a.c3.VCardUID} {
			_, v := r.make(e, n)
			create(v)
			id := idOf(v)
			name := fmt.Sprintf("%s%d", r.key, n)
			if n == 3 {
				name = r.key + "T"
				require.NoError(t, db.Delete(v).Error)
			}
			a.note(name, id, true)
		}
	}

	// dated, active occasion obligations: a normal and a sensitive one of each
	// kind, due in two days, so /occasions/upcoming, the card list and the gift
	// shopping list all have rows and the include_sensitive gate has something
	// to withhold.
	due := time.Now().UTC().AddDate(0, 0, 2)
	month, day := int(due.Month()), due.Day()
	for _, o := range []struct{ key, kind, sens, entity string }{
		{"cardN", "card", "normal", a.c1.VCardUID},
		{"cardS", "card", "private", a.c2.VCardUID},
		{"giftN", "gift", "normal", a.c1.VCardUID},
		{"giftS", "gift", "private", a.c2.VCardUID},
	} {
		ob := models.OccasionObligation{
			UserID: a.user.ID, EntityID: o.entity, Kind: o.kind, Label: tag + "-" + o.key,
			AnchorMonth: &month, AnchorDay: &day, Active: true, Sensitivity: o.sens,
		}
		create(&ob)
		a.note("due-"+o.key, ob.Label, false)
	}

	// audit events (numeric ids, so the marker is the snapshot text)
	for n, e := range map[int]string{1: a.c1.VCardUID, 2: a.c2.VCardUID} {
		ev := models.AuditEvent{
			UserID: a.user.ID, EntityType: "contact", EntityID: e, Operation: "update",
			BeforeSnapshot: fmt.Sprintf(`{"note":"%s-audit%d"}`, tag, n),
		}
		create(&ev)
		a.note(fmt.Sprintf("audit%d", n), fmt.Sprintf("%s-audit%d", tag, n), false)
	}

	// external identity / activity (scoped by contact_id = contact UID)
	for n, e := range map[int]string{1: a.c1.VCardUID, 2: a.c2.VCardUID} {
		ei := models.ExternalIdentity{UserID: a.user.ID, EntityID: e, System: "immich", ExternalID: fmt.Sprintf("%s-person%d", tag, n)}
		create(&ei)
		a.note(fmt.Sprintf("xt-id%d", n), ei.ID, true)
		ea := models.ExternalActivity{UserID: a.user.ID, EntityID: e, SourceSystem: "immich", ExternalID: fmt.Sprintf("%s-asset%d", tag, n), Type: "photo-appearance", OccurredAt: time.Now()}
		create(&ea)
		a.note(fmt.Sprintf("xt-act%d", n), ea.ID, true)
	}

	// relationship edges sharing c3 as the hub: edge13 / edge23.
	for n, src := range map[int]string{1: a.c1.VCardUID, 2: a.c2.VCardUID} {
		e := models.RelationshipEdge{
			UserID: a.user.ID, SourceID: src, TargetID: a.c3.VCardUID,
			Type: "friend_of", Source: models.RelationshipSourceUserConfirmed, Confidence: 1,
			Status: models.RelationshipStatusConfirmed,
		}
		create(&e)
		a.note(fmt.Sprintf("edge%d", n), e.ID, true)
	}

	// unassigned notes + activities (text markers); tombstone each.
	for n := 1; n <= 3; n++ {
		note := models.Note{UserID: a.user.ID, Content: fmt.Sprintf("%s-note%d", tag, n), Date: time.Now()}
		create(&note)
		act := models.Activity{UserID: a.user.ID, Title: fmt.Sprintf("%s-act%d", tag, n), Date: time.Now()}
		create(&act)
		nn, an := fmt.Sprintf("note%d", n), fmt.Sprintf("act%d", n)
		if n == 3 {
			nn, an = "noteT", "actT"
			require.NoError(t, db.Delete(&note).Error)
			require.NoError(t, db.Delete(&act).Error)
		}
		a.note(nn, note.Content, false)
		a.note(an, act.Title, false)
	}

	// A note filed under c1: listed only by GET /notes?scope=all (#2180).
	filedNote := models.Note{UserID: a.user.ID, Content: tag + "-noteF", Date: time.Now(), ContactID: &a.c1.ID}
	create(&filedNote)
	a.note("noteF", filedNote.Content, false)

	// occasion events; ev1 has c1 as attendee so invitee suggestions can
	// exclude it. Third event is the tombstone.
	for n := 1; n <= 3; n++ {
		ev := models.OccasionEvent{UserID: a.user.ID, Title: fmt.Sprintf("%s-event%d", tag, n), StartsAt: time.Now().Add(24 * time.Hour)}
		create(&ev)
		name := fmt.Sprintf("event%d", n)
		switch n {
		case 1:
			a.eventID1 = ev.ID
			create(&models.OccasionEventAttendee{UserID: a.user.ID, EventID: ev.ID, EntityID: a.c1.VCardUID, RSVP: "pending"})
		case 2:
			a.eventID2 = ev.ID
		case 3:
			name = "eventT"
			require.NoError(t, db.Delete(&ev).Error)
		}
		a.note(name, ev.ID, true)
		a.note(name+"-title", ev.Title, false)
	}
	return a
}

// idOf returns the string primary key of the seeded uuid-keyed models.
func idOf(v any) string {
	switch x := v.(type) {
	case *models.LifeEvent:
		return x.ID
	case *models.ConversationAgenda:
		return x.ID
	case *models.Gift:
		return x.ID
	case *models.Preference:
		return x.ID
	case *models.OccasionObligation:
		return x.ID
	case *models.CadencePolicy:
		return x.ID
	case *models.DataDecayPolicy:
		return x.ID
	}
	panic(fmt.Sprintf("idOf: unsupported %T", v))
}

// ── rows ──────────────────────────────────────────────────────────────────

type qsRow struct {
	name string
	path string // registered GET path, no path params

	// params are the scope query parameters this row exercises (spec guard);
	// reads are the "Func:param" handler reads it vouches for (handler guard).
	params []string
	reads  []string

	// query is the scoped query string for an actor (no leading '?').
	query func(a *qsActor) string
	// in / out are the markers a scoped response must / must not contain.
	in  func(a *qsActor) []string
	out func(a *qsActor) []string
	// baseIn / baseOut describe the unscoped (empty query) owner response;
	// nil baseIn skips the baseline control.
	baseIn func(a *qsActor) []string
	// baseQuery is the unscoped query for routes that need a non-scope
	// parameter (e.g. a format) even without a scope.
	baseQuery string
	baseOut   func(a *qsActor) []string
	// cross returns extra intruder queries that mix the attacker's ids with the
	// victim's (default: just the victim's scoped query).
	cross func(victim, attacker *qsActor) []string
	// skipIntruderPositive is for routes where the intruder's own scope cannot
	// produce a marker (explained in the field's reason).
	skipIntruderPositive string
}

func qsKeys(a *qsActor, names ...string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		v, ok := a.m[n]
		if !ok {
			panic("qsKeys: unknown marker " + n)
		}
		out = append(out, v)
	}
	return out
}

func qsMk(names ...string) func(a *qsActor) []string {
	return func(a *qsActor) []string { return qsKeys(a, names...) }
}

func qsSinceCursor() string {
	return controllers.EncodeCursor(time.Now().Add(-time.Hour), "0")
}

func buildQueryScopeRows() []qsRow {
	var rows []qsRow

	entityRow := func(route, fn, key string) {
		rows = append(rows, qsRow{
			name: route + " ?entity_id=", path: "/api/v1/" + route,
			params: []string{"entity_id"}, reads: []string{fn + ":entity_id"},
			query:   func(a *qsActor) string { return "entity_id=" + a.c1.VCardUID },
			in:      qsMk(key + "1"),
			out:     qsMk(key+"2", key+"T"),
			baseIn:  qsMk(key+"1", key+"2"),
			baseOut: qsMk(key + "T"),
		})
	}
	entityRow("life-events", "ListLifeEvents", "life")
	entityRow("conversation-agenda", "ListConversationAgenda", "agenda")
	entityRow("gifts", "ListGifts", "gift")
	entityRow("preferences", "ListPreferences", "pref")
	entityRow("occasion-obligations", "ListOccasionObligations", "oblig")
	entityRow("cadence-policies", "ListCadencePolicies", "cadence")
	entityRow("data-decay-policies", "ListDataDecayPolicies", "decay")

	rows = append(rows,
		qsRow{
			name: "audit ?entity_type=&entity_id=", path: "/api/v1/audit",
			params: []string{"entity_id", "entity_type"}, reads: []string{"ListAuditEvents:entity_id", "ListAuditEvents:entity_type"},
			query:  func(a *qsActor) string { return "entity_type=contact&entity_id=" + a.c1.VCardUID },
			in:     qsMk("audit1"),
			out:    qsMk("audit2"),
			baseIn: qsMk("audit1", "audit2"),
		},
		qsRow{
			name: "external-identities ?contact_id=", path: "/api/v1/external-identities",
			params: []string{"contact_id"}, reads: []string{"ListExternalIdentities:contact_id"},
			query:  func(a *qsActor) string { return "contact_id=" + a.c1.VCardUID },
			in:     qsMk("xt-id1"),
			out:    qsMk("xt-id2"),
			baseIn: qsMk("xt-id1", "xt-id2"),
		},
		qsRow{
			name: "external-activities ?contact_id=", path: "/api/v1/external-activities",
			params: []string{"contact_id"}, reads: []string{"ListExternalActivities:contact_id"},
			query:  func(a *qsActor) string { return "contact_id=" + a.c1.VCardUID },
			in:     qsMk("xt-act1"),
			out:    qsMk("xt-act2"),
			baseIn: qsMk("xt-act1", "xt-act2"),
		},
		qsRow{
			name: "relationship-edges ?contact_id=", path: "/api/v1/relationship-edges",
			params: []string{"contact_id"}, reads: []string{"ListRelationshipEdges:contact_id"},
			query:  func(a *qsActor) string { return "contact_id=" + a.c1.VCardUID },
			in:     qsMk("edge1"),
			out:    qsMk("edge2"),
			baseIn: qsMk("edge1", "edge2"),
		},
	)

	// ── /contacts ───────────────────────────────────────────────────────
	rows = append(rows,
		qsRow{
			name: "contacts ?vcard_uid=", path: "/api/v1/contacts",
			params: []string{"vcard_uid"}, reads: []string{"GetContacts:vcard_uid"},
			query:  func(a *qsActor) string { return "vcard_uid=" + a.c1.VCardUID },
			in:     qsMk("contact-c1"),
			out:    qsMk("contact-c2", "contact-c3"),
			baseIn: qsMk("contact-c1", "contact-c2", "contact-c3"),
		},
		qsRow{
			name: "contacts ?circle=", path: "/api/v1/contacts",
			params: []string{"circle"}, reads: []string{"GetContacts:circle"},
			query:  func(a *qsActor) string { return "circle=" + a.circleName1 },
			in:     qsMk("contact-c1"),
			out:    qsMk("contact-c2", "contact-c3"),
			baseIn: qsMk("contact-c1", "contact-c2", "contact-c3"),
		},
		qsRow{
			name: "contacts ?circle_legacy=", path: "/api/v1/contacts",
			params: []string{"circle_legacy"}, reads: []string{"GetContacts:circle_legacy"},
			query:  func(a *qsActor) string { return "circle_legacy=" + a.tag + "-legacy1" },
			in:     qsMk("contact-c1"),
			out:    qsMk("contact-c2", "contact-c3"),
			baseIn: qsMk("contact-c1", "contact-c2", "contact-c3"),
		},
		qsRow{
			name: "contacts ?include_archived=", path: "/api/v1/contacts",
			params: []string{"include_archived"}, reads: []string{"GetContacts:include_archived"},
			query:   func(a *qsActor) string { return "include_archived=true" },
			in:      qsMk("contact-c1", "contact-arch"),
			baseIn:  qsMk("contact-c1"),
			baseOut: qsMk("contact-arch"),
		},
		qsRow{
			name: "contacts ?vcard_uid=&include_archived=", path: "/api/v1/contacts",
			params: []string{"vcard_uid", "include_archived"}, reads: []string{"GetContacts:include_archived", "GetContacts:vcard_uid"},
			query:  func(a *qsActor) string { return "include_archived=true&vcard_uid=" + a.cArch.VCardUID },
			in:     qsMk("contact-arch"),
			out:    qsMk("contact-c1", "contact-c2"),
			baseIn: nil,
		},
	)

	// ── ?since= change feeds ────────────────────────────────────────────
	since := func(route, key string) {
		rows = append(rows, qsRow{
			name: route + " ?since=", path: "/api/v1/" + route,
			params: []string{"since"}, reads: []string{"cursorParams:since"},
			query:   func(a *qsActor) string { return "since=" + qsSinceCursor() },
			in:      qsMk(key+"1", key+"2", key+"T"),
			baseIn:  qsMk(key+"1", key+"2"),
			baseOut: qsMk(key + "T"),
		})
	}
	rows = append(rows, qsRow{
		name: "contacts ?since=", path: "/api/v1/contacts",
		params: []string{"since"}, reads: []string{"cursorParams:since"},
		query:   func(a *qsActor) string { return "since=" + qsSinceCursor() },
		in:      qsMk("contact-c1", "contact-c2", "contact-c3", "contact-tomb"),
		baseIn:  qsMk("contact-c1", "contact-c2", "contact-c3"),
		baseOut: qsMk("contact-tomb"),
	})
	since("notes", "note")
	since("activities", "act")
	since("life-events", "life")
	since("conversation-agenda", "agenda")
	since("gifts", "gift")
	since("preferences", "pref")
	since("occasion-obligations", "oblig")
	since("occasion-events", "event")
	since("cadence-policies", "cadence")
	since("data-decay-policies", "decay")

	// ── exports ─────────────────────────────────────────────────────────
	for _, ex := range []struct{ route, fn string }{
		{"export/vcf", "ExportContactsAsVCF"},
		{"export/jscontact", "ExportContactsAsJSContact"},
		{"export/preflight", "ExportPreflight"},
	} {
		// The preflight endpoint requires an explicit format on every call.
		pre, baseQ := "", ""
		baseIn := qsMk("contact-c1", "contact-c2", "contact-c3")
		if ex.route == "export/preflight" {
			pre, baseQ = "format=vcard4&", "format=vcard4"
			// preflight only reports contacts with export diagnostics (c1, c2 carry
			// legacy circles); c3 has none.
			baseIn = qsMk("contact-c1", "contact-c2")
		}
		rows = append(rows, qsRow{
			baseQuery: baseQ,
			name:      ex.route + " ?vcard_uid=", path: "/api/v1/" + ex.route,
			params: []string{"vcard_uid"}, reads: []string{ex.fn + ":vcard_uid"},
			query:  func(a *qsActor) string { return pre + "vcard_uid=" + a.c1.VCardUID },
			in:     qsMk("contact-c1"),
			out:    qsMk("contact-c2", "contact-c3"),
			baseIn: baseIn,
		}, qsRow{
			name: ex.route + " ?include_sensitive=", path: "/api/v1/" + ex.route,
			params: []string{"include_sensitive"}, reads: []string{"parseExportFieldSelection:include_sensitive"},
			query: func(a *qsActor) string { return pre + "include_sensitive=true&vcard_uid=" + a.c1.VCardUID },
			in:    qsMk("contact-c1"),
			out:   qsMk("contact-c2"),
		})
	}

	// ── graph ───────────────────────────────────────────────────────────
	rows = append(rows,
		qsRow{
			name: "graph ?from=", path: "/api/v1/graph",
			params: []string{"from"}, reads: []string{"resolveGraphScope:from"},
			query:  func(a *qsActor) string { return "depth=1&from=" + a.c1.VCardUID },
			in:     qsMk("contact-c1", "contact-c3"),
			out:    qsMk("contact-c2"),
			baseIn: qsMk("contact-c1", "contact-c2", "contact-c3"),
		},
		qsRow{
			name: "graph/connections ?from=", path: "/api/v1/graph/connections",
			params: []string{"from"}, reads: []string{"GetGraphConnections:from"},
			query: func(a *qsActor) string { return "depth=1&from=" + a.c1.VCardUID },
			in:    qsMk("contact-c3"),
			out:   qsMk("contact-c2"),
		},
		qsRow{
			name: "search ?household_id=", path: "/api/v1/search",
			params: []string{"household_id"}, reads: []string{"SearchAll:household_id"},
			query:  func(a *qsActor) string { return "q=scopeq&household_id=" + a.householdID1 },
			in:     qsMk("contact-c1"),
			out:    qsMk("contact-c2"),
			baseIn: nil,
		},
	)

	// ── occasions ───────────────────────────────────────────────────────
	rows = append(rows,
		qsRow{
			name: "occasion-events/invitee-suggestions ?circle_ids=", path: "/api/v1/occasion-events/invitee-suggestions",
			params: []string{"circle_ids"}, reads: []string{"GetInviteeSuggestions:circle_ids"},
			query: func(a *qsActor) string { return "circle_ids=" + a.circleID1 },
			in:    qsMk("contact-c1"),
			out:   qsMk("contact-c2"),
			cross: func(v, atk *qsActor) []string {
				return []string{
					"circle_ids=" + v.circleID1,
					"circle_ids=" + atk.circleID1 + "," + v.circleID1,
					"circle_ids=" + v.circleID1 + "," + v.circleID2,
				}
			},
		},
		qsRow{
			name: "occasion-events/invitee-suggestions ?event_id=", path: "/api/v1/occasion-events/invitee-suggestions",
			params: []string{"event_id"}, reads: []string{"GetInviteeSuggestions:event_id"},
			// event1 already has c1 as attendee, so with both circles c1 is
			// excluded and only c2 is suggested.
			query: func(a *qsActor) string {
				return "circle_ids=" + a.circleID1 + "," + a.circleID2 + "&event_id=" + a.eventID1
			},
			in:  qsMk("contact-c2"),
			out: qsMk("contact-c1"),
			cross: func(v, atk *qsActor) []string {
				return []string{
					"circle_ids=" + atk.circleID1 + "&event_id=" + v.eventID1,
					"circle_ids=" + v.circleID1 + "&event_id=" + v.eventID1,
				}
			},
		},
	)

	// ── include_* flags ─────────────────────────────────────────────────
	rows = append(rows,
		qsRow{
			name: "circles ?include_members=", path: "/api/v1/circles",
			params: []string{"include_members"}, reads: []string{"ListCircles:include_members"},
			query:   func(a *qsActor) string { return "include_members=true" },
			in:      qsMk("circle1", "circle2", "uid-c1", "uid-c2"),
			baseIn:  qsMk("circle1", "circle2"),
			baseOut: qsMk("uid-c1", "uid-c2"),
		},
		qsRow{
			name: "households ?include_members=", path: "/api/v1/households",
			params: []string{"include_members"}, reads: []string{"ListHouseholds:include_members"},
			query:   func(a *qsActor) string { return "include_members=true" },
			in:      qsMk("house1", "house2", "uid-c1", "uid-c2"),
			baseIn:  qsMk("house1", "house2"),
			baseOut: qsMk("uid-c1", "uid-c2"),
		},
		qsRow{
			name: "tags ?include_contacts=", path: "/api/v1/tags",
			params: []string{"include_contacts"}, reads: []string{"ListTags:include_contacts"},
			query:   func(a *qsActor) string { return "include_contacts=true" },
			in:      qsMk("tag1", "tag2", "uid-c1", "uid-c2"),
			baseIn:  qsMk("tag1", "tag2"),
			baseOut: qsMk("uid-c1", "uid-c2"),
		},
		qsRow{
			name: "activities ?include=contacts", path: "/api/v1/activities",
			params: []string{"include"}, reads: []string{"GetActivities:include"},
			query:  func(a *qsActor) string { return "include=contacts" },
			in:     qsMk("act1", "act2"),
			baseIn: qsMk("act1", "act2"),
		},
		qsRow{
			name: "notes ?scope=all&include=contacts", path: "/api/v1/notes",
			params: []string{"include", "scope"}, reads: []string{"GetUnassignedNotes:include", "GetUnassignedNotes:scope"},
			query:   func(a *qsActor) string { return "scope=all&include=contacts" },
			in:      qsMk("note1", "note2", "noteF"),
			out:     qsMk("noteT"),
			baseIn:  qsMk("note1", "note2"),
			baseOut: qsMk("noteF"),
		},
		qsRow{
			name: "audit/export ?include_snapshots=", path: "/api/v1/audit/export",
			params: []string{"include_snapshots"}, reads: []string{"ExportAuditLog:include_snapshots"},
			query:   func(a *qsActor) string { return "include_snapshots=true" },
			in:      qsMk("audit1", "audit2"),
			baseIn:  nil,
			baseOut: nil,
		},
		qsRow{
			name: "occasions/upcoming ?include_sensitive=", path: "/api/v1/occasions/upcoming",
			params: []string{"include_sensitive"}, reads: []string{"GetUpcomingOccasions:include_sensitive"},
			query:   func(a *qsActor) string { return "include_sensitive=true" },
			in:      qsMk("due-cardN", "due-giftN", "due-cardS", "due-giftS"),
			baseIn:  qsMk("due-cardN", "due-giftN"),
			baseOut: qsMk("due-cardS", "due-giftS"),
		},
		qsRow{
			name: "occasion-obligations/card-list ?include_sensitive=", path: "/api/v1/occasion-obligations/card-list",
			params: []string{"include_sensitive"}, reads: []string{"GetOccasionCardListCSV:include_sensitive"},
			query:   func(a *qsActor) string { return "include_sensitive=true" },
			in:      qsMk("due-cardN", "due-cardS"),
			baseIn:  qsMk("due-cardN"),
			baseOut: qsMk("due-cardS"),
		},
		qsRow{
			name: "occasion-obligations/gift-shopping-list ?include_sensitive=", path: "/api/v1/occasion-obligations/gift-shopping-list",
			params: []string{"include_sensitive"}, reads: []string{"GetGiftShoppingList:include_sensitive"},
			query:   func(a *qsActor) string { return "include_sensitive=true" },
			in:      qsMk("due-giftN", "due-giftS"),
			baseIn:  qsMk("due-giftN"),
			baseOut: qsMk("due-giftS"),
		},
	)
	return rows
}

// qsAllow lists "Func:param" handler reads that are scope-named but are NOT
// probed by a row, each with the recorded reason.
var qsAllow = map[string]string{
	"includeSensitiveRequested:include_sensitive": "only reached from the POST geocode handlers (no GET route reads it); sensitive-address filtering is covered by controllers/contact_address_controller_test.go",
	"ListSystemEvents:ids":                        "admin-only route (non-admins get 403 in authorization_matrix_test.go); system events are instance-wide by design",
	"ListSystemEvents:since":                      "admin-only route, instance-wide by design (see ListSystemEvents:ids)",
	"ListSystemEvents:correlation_id":             "admin-only route, instance-wide by design (see ListSystemEvents:ids)",
	"ListJobRuns:since":                           "admin-only route; job runs are instance-wide by design",
	"ServeFeed:token":                             "feed capability token, not a user-supplied scope: the credential authenticates the owner; covered by authorization_matrix_credentials_test.go",
	"parseEventWindow:from":                       "RFC 3339 date lower bound, not an identifier; rows are user_id-scoped",
	"GetMonicaImportStatus:session_id":            "in-memory import sessions keyed by (user_id, session_id); creating one needs an outbound Monica client, so the cross-user lookup is covered at controller/service level (controllers/*import*_test.go, services/import_session_test.go)",
	"GetMonicaImportPreview:session_id":           "see GetMonicaImportStatus:session_id",
	"CancelMonicaImport:session_id":               "see GetMonicaImportStatus:session_id",
	"GetMeerkatImportStatus:session_id":           "see GetMonicaImportStatus:session_id",
	"GetMeerkatImportPreview:session_id":          "see GetMonicaImportStatus:session_id",
	"CancelMeerkatImport:session_id":              "see GetMonicaImportStatus:session_id",
	"GetMycorrhizalImportStatus:session_id":       "see GetMonicaImportStatus:session_id",
	"GetMycorrhizalImportPreview:session_id":      "see GetMonicaImportStatus:session_id",
	"CancelMycorrhizalImport:session_id":          "see GetMonicaImportStatus:session_id",
}

// qsAllowSpec lists "<openapi path>|<param>" GET parameters that the OpenAPI
// spec declares with a scope-ish name but that are not row-probed, each with
// the recorded reason.
var qsAllowSpec = map[string]string{
	"/admin/job-runs|since":                       "admin-only, instance-wide by design (qsAllow ListJobRuns:since)",
	"/admin/system-events|correlation_id":         "admin-only, instance-wide by design (qsAllow ListSystemEvents:correlation_id)",
	"/admin/system-events|ids":                    "admin-only, instance-wide by design (qsAllow ListSystemEvents:ids)",
	"/admin/system-events|since":                  "admin-only, instance-wide by design (qsAllow ListSystemEvents:since)",
	"/contacts/import/meerkat/preview|session_id": "in-memory import session keyed by (user_id, session_id); see qsAllow GetMonicaImportStatus:session_id",
	"/contacts/import/meerkat/status|session_id":  "in-memory import session keyed by (user_id, session_id); see qsAllow GetMonicaImportStatus:session_id",
	"/contacts/import/monica/preview|session_id":  "in-memory import session keyed by (user_id, session_id); see qsAllow GetMonicaImportStatus:session_id",
	"/contacts/import/monica/status|session_id":   "in-memory import session keyed by (user_id, session_id); see qsAllow GetMonicaImportStatus:session_id",
	"/feeds/atom|token":                           "capability token, not a scope (qsAllow ServeFeed:token)",
	"/import/mycorrhizal/preview|session_id":      "in-memory import session keyed by (user_id, session_id); see qsAllow GetMonicaImportStatus:session_id",
	"/import/mycorrhizal/status|session_id":       "in-memory import session keyed by (user_id, session_id); see qsAllow GetMonicaImportStatus:session_id",
	"/occasion-events|from":                       "date bound, not an identifier (qsAllow parseEventWindow:from)",
}

// qsNonScope are query parameter names the handlers read that carry no
// identifier or scope: pagination, sort, free-text search, formats, dates and
// the like. A name in neither this set nor the scope rules is `unclassified`.
var qsNonScope = map[string]string{
	"limit": "page size", "order": "sort direction", "cursor": "opaque pagination cursor (scoped by user_id; tampered/foreign cursors covered by the pagination matrix)",
	"sort": "sort key", "search": "free-text filter", "q": "free-text search term", "query": "free-text filter",
	"fromDate": "date lower bound", "toDate": "date upper bound", "to": "date upper bound", "until": "time upper bound",
	"page": "offset page number", "favorites": "boolean list filter", "archived": "boolean list filter",
	"has_contact_info": "boolean list filter", "includes": "relationship-expansion list on /contacts (selects response shape of caller's own rows)",
	"legacy": "boolean format switch", "type": "type filter", "bucket": "timeline bucket", "kind": "kind filter",
	"days": "look-ahead window", "status": "status filter", "system": "source-system filter", "sections": "export field picker",
	"version": "export format version", "format": "export format", "full": "graph scope switch (caller's graph either way)",
	"depth": "graph depth", "relation": "relation-token filter", "state": "OAuth state", "code": "OAuth code",
	"code_challenge": "PKCE", "code_challenge_method": "PKCE", "client": "OIDC client kind", "error": "OIDC error echo",
	"email": "auth flow", "skip": "pagination offset", "url": "image proxy target URL (SSRF-guarded, not a row scope)",
	"path": "WebDAV path in the caller's configured remote", "thumbnail": "boolean image variant", "window_hours": "admin aggregation window",
	"component": "admin filter", "severity": "admin filter", "event_type": "admin filter", "job_name": "admin filter",
	"result": "admin filter", "date": "geopulse day", "timezone": "geopulse zone",
}

// ── AST scan of handler query reads ───────────────────────────────────────

func qsScanQueryReads(t *testing.T) map[string]string {
	t.Helper()
	methods := map[string]bool{"Query": true, "DefaultQuery": true, "GetQuery": true, "QueryArray": true, "GetQueryArray": true}
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, dir := range []string{"../controllers", "../middleware"} {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			f, perr := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
			require.NoErrorf(t, perr, "parsing %s", e.Name())
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok || len(call.Args) == 0 {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || !methods[sel.Sel.Name] {
						return true
					}
					lit, ok := call.Args[0].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return true
					}
					param := strings.Trim(lit.Value, `"`)
					out[fd.Name.Name+":"+param] = fmt.Sprintf("%s:%d", e.Name(), fset.Position(call.Pos()).Line)
					return true
				})
			}
		}
	}
	require.NotEmpty(t, out, "AST scan found zero query reads — the scanner is broken")
	return out
}

func qsIsScopeName(name string) bool {
	switch name {
	case "since", "circle", "circle_legacy", "circle_ids", "from", "ids", "token", "include", "scope":
		return true
	}
	return strings.HasPrefix(name, "include_") ||
		strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "_ids") ||
		strings.HasSuffix(name, "_uid") || strings.HasSuffix(name, "_uids") ||
		name == "entity_type"
}

// ── spec discovery ────────────────────────────────────────────────────────

// qsSpecScopeParams returns "<openapi path>|<param>" for every GET operation
// in openapi.yaml that declares a scope-named query parameter.
func qsSpecScopeParams(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("../openapi.yaml")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	resolve := func(v any) map[string]any {
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
	out := map[string]bool{}
	paths, _ := doc["paths"].(map[string]any)
	for p, item := range paths {
		itemMap, _ := item.(map[string]any)
		get := resolve(itemMap["get"])
		if get == nil {
			continue
		}
		params := []any{}
		if ps, ok := itemMap["parameters"].([]any); ok {
			params = append(params, ps...)
		}
		if ps, ok := get["parameters"].([]any); ok {
			params = append(params, ps...)
		}
		for _, raw := range params {
			pm := resolve(raw)
			if pm == nil || pm["in"] != "query" {
				continue
			}
			name, _ := pm["name"].(string)
			if qsIsScopeName(name) {
				out[p+"|"+name] = true
			}
		}
	}
	require.NotEmpty(t, out, "openapi scan found zero scope parameters — the scanner is broken")
	return out
}

// ── harness ───────────────────────────────────────────────────────────────

type qsHarness struct {
	t      *testing.T
	router http.Handler
}

func (h *qsHarness) get(token, path, query string) (int, string) {
	h.t.Helper()
	target := path
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = uniqueTestClientIP() + ":1234"
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// leaks returns the first marker of victim found in body, ignoring identifiers
// the request itself carried (a handler may echo a supplied id back).
func qsLeaks(body, requestQuery string, victim *qsActor) (string, bool) {
	if strings.Contains(body, victim.tag) {
		return victim.tag, true
	}
	for _, id := range victim.scan {
		if id == "" || strings.Contains(requestQuery, id) {
			continue
		}
		if strings.Contains(body, id) {
			return id, true
		}
	}
	return "", false
}

func qsDescribe(a *qsActor, markers []string) string {
	names := []string{}
	for _, mk := range markers {
		for k, v := range a.m {
			if v == mk {
				names = append(names, k)
				break
			}
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func TestQueryScopeMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := dbtest.New(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	cfg := &config.Config{
		JWTSecretKey:     "query-scope-matrix-test-secret-key-long-enough",
		JWTExpiryHours:   96,
		ProfilePhotoDir:  t.TempDir(),
		FrontendURL:      "http://localhost:5173",
		Port:             "7300",
		ReminderTime:     "12:00",
		ReminderTimezone: "UTC",
		// ?since= feeds 410 when the cursor predates the retention window; a
		// zero value would make every cursor "older".
		DeleteRetentionDays: 30,
	}
	middleware.ConfigureAPIRateLimiter(time.Microsecond, 1_000_000)

	owner := qsSeedActor(t, db, cfg, "owner", "OWNQ")
	intruder := qsSeedActor(t, db, cfg, "intruder", "INTQ")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	RegisterRoutes(router, cfg, db, nil)

	rows := buildQueryScopeRows()

	// ── completeness guard ──────────────────────────────────────────────
	registered := map[string]bool{}
	for _, r := range router.Routes() {
		if r.Method == http.MethodGet {
			registered[r.Path] = true
		}
	}
	for _, r := range rows {
		require.Truef(t, registered[r.path], "row %q: %s is not a registered GET route (renamed or removed?)", r.name, r.path)
		require.NotNilf(t, r.query, "row %q has no query", r.name)
		require.NotEmptyf(t, r.params, "row %q declares no scope params", r.name)
		require.NotEmptyf(t, r.reads, "row %q vouches for no handler reads", r.name)
	}

	// handler guard
	scanned := qsScanQueryReads(t)
	vouched := map[string]bool{}
	for _, r := range rows {
		for _, tok := range r.reads {
			vouched[tok] = true
		}
	}
	var unclassified, unprobed, stale []string
	for tok, where := range scanned {
		param := tok[strings.Index(tok, ":")+1:]
		if !qsIsScopeName(param) {
			if _, ok := qsNonScope[param]; !ok {
				unclassified = append(unclassified, tok+"  ("+where+")")
			}
			continue
		}
		if !vouched[tok] {
			if _, ok := qsAllow[tok]; !ok {
				unprobed = append(unprobed, tok+"  ("+where+")")
			}
		}
	}
	for tok := range vouched {
		if _, ok := scanned[tok]; !ok {
			stale = append(stale, "row reads "+tok)
		}
	}
	for tok := range qsAllow {
		if _, ok := scanned[tok]; !ok {
			stale = append(stale, "qsAllow "+tok)
		}
		require.NotEmptyf(t, qsAllow[tok], "qsAllow %q needs a reason", tok)
	}
	for tok := range qsNonScope {
		require.NotEmptyf(t, qsNonScope[tok], "qsNonScope %q needs a reason", tok)
	}
	sort.Strings(unclassified)
	sort.Strings(unprobed)
	sort.Strings(stale)
	require.Emptyf(t, unclassified,
		"handlers read a query parameter that is neither a scope parameter nor in qsNonScope — classify it:\n  %s", strings.Join(unclassified, "\n  "))
	require.Emptyf(t, unprobed,
		"handlers read a scope query parameter with no matrix row — add a qsRow whose `reads` names it, or a qsAllow entry with a reason:\n  %s", strings.Join(unprobed, "\n  "))
	require.Emptyf(t, stale,
		"declared reads that no handler performs any more (renamed or removed?):\n  %s", strings.Join(stale, "\n  "))

	// spec guard
	covered := map[string]bool{}
	for _, r := range rows {
		spec := strings.TrimPrefix(r.path, "/api/v1")
		for _, p := range r.params {
			covered[spec+"|"+p] = true
		}
	}
	var specMissing []string
	for key := range qsSpecScopeParams(t) {
		if covered[key] {
			continue
		}
		if reason, ok := qsAllowSpec[key]; ok {
			require.NotEmptyf(t, reason, "qsAllowSpec %q needs a reason", key)
			continue
		}
		// An allow-listed handler read also excuses the matching spec entry.
		specMissing = append(specMissing, key)
	}
	sort.Strings(specMissing)
	require.Emptyf(t, specMissing,
		"openapi.yaml declares a scope query parameter with no matrix row (or qsAllowSpec reason):\n  %s", strings.Join(specMissing, "\n  "))

	// ── run the rows ────────────────────────────────────────────────────
	h := &qsHarness{t: t, router: router}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			h.t = t

			// 1. intruder check
			victimQueries := []string{r.query(owner)}
			if r.cross != nil {
				victimQueries = r.cross(owner, intruder)
			}
			for _, q := range victimQueries {
				st, body := h.get(intruder.token, r.path, q)
				require.Lessf(t, st, 500, "INTRUDER %s?%s -> %d (%s)", r.path, q, st, body)
				if leaked, bad := qsLeaks(body, q, owner); bad {
					t.Fatalf("LEAK: intruder GET %s?%s -> %d contained owner marker %q\nbody: %s", r.path, q, st, leaked, body)
				}
			}

			// 3b. intruder positive control: the intruder's own scope works.
			if r.in != nil && r.skipIntruderPositive == "" {
				q := r.query(intruder)
				st, body := h.get(intruder.token, r.path, q)
				require.Equalf(t, http.StatusOK, st, "intruder positive control GET %s?%s -> %d (%s)", r.path, q, st, body)
				for _, mk := range r.in(intruder) {
					require.Containsf(t, body, mk, "intruder positive control GET %s?%s: missing own marker %q (vacuous intruder check)", r.path, q, mk)
				}
			}

			// 2. narrowing check
			q := r.query(owner)
			st, body := h.get(owner.token, r.path, q)
			require.Equalf(t, http.StatusOK, st, "owner GET %s?%s -> %d (%s)", r.path, q, st, body)
			if r.in != nil {
				for _, mk := range r.in(owner) {
					require.Containsf(t, body, mk, "owner GET %s?%s: in-scope marker %q (%s) missing — positive control failed", r.path, q, mk, qsDescribe(owner, []string{mk}))
				}
			}
			if r.out != nil {
				for _, mk := range r.out(owner) {
					require.NotContainsf(t, body, mk, "NARROWING: owner GET %s?%s returned out-of-scope marker %q (%s) — scope parameter ignored?", r.path, q, mk, qsDescribe(owner, []string{mk}))
				}
			}
			if leaked, bad := qsLeaks(body, q, intruder); bad {
				t.Fatalf("LEAK: owner GET %s?%s contained intruder marker %q\nbody: %s", r.path, q, leaked, body)
			}

			// 3a. unscoped baseline: both scopes visible, so narrowing is real.
			if r.baseIn != nil {
				bst, bbody := h.get(owner.token, r.path, r.baseQuery)
				require.Equalf(t, http.StatusOK, bst, "owner baseline GET %s -> %d (%s)", r.path, bst, bbody)
				for _, mk := range r.baseIn(owner) {
					require.Containsf(t, bbody, mk, "baseline GET %s: marker %q (%s) missing — a narrowed pass could be vacuous", r.path, mk, qsDescribe(owner, []string{mk}))
				}
				if r.baseOut != nil {
					for _, mk := range r.baseOut(owner) {
						require.NotContainsf(t, bbody, mk, "baseline GET %s unexpectedly contains %q (%s)", r.path, mk, qsDescribe(owner, []string{mk}))
					}
				}
				if leaked, bad := qsLeaks(bbody, "", intruder); bad {
					t.Fatalf("LEAK: owner baseline GET %s contained intruder marker %q", r.path, leaked)
				}
			}
		})
	}
}
