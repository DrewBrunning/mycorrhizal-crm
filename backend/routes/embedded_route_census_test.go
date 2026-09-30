package routes

// TestEmbeddedRouteSurface_Classified is the fail-closed half of ADR 0028's
// embedded mode (issue #1356). The hand-written `disabled` list in
// embedded_mode_test.go only proves that routes someone remembered are absent;
// a NEW network-shaped route registered without `!cfg.IsEmbedded()` would ship
// in the on-device server and nothing would fail (fail-open).
//
// This test derives the embedded route set from the live router, the way
// authorization_matrix_test.go derives the server set, and requires every
// route registered in embedded mode to have a declared row below with a class
// and a reason. A new route therefore FAILS this test until someone reads it
// and classifies it; the reverse direction (a declared row with no registered
// route) is a stale-row failure, so the table cannot rot.
//
// The routers are built with every optional surface switched ON in config
// (CardDAV, CalDAV, METRICS_TOKEN) so a config-conditional route cannot hide
// from the census.
//
// Adding a route: if it is network-shaped (an outbound integration, a public
// unauthenticated endpoint, a login/identity surface, anything that only
// makes sense with a server or a second party), gate it on `!cfg.IsEmbedded()`
// and it will not appear here. If it is meant to be in the on-device server,
// add it to the matching group below with the reason that group states, or a
// new group with its own reason.

import (
	"sort"
	"testing"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type embeddedRouteClass string

const (
	// embeddedClassLocal: the CRM's own single-user data surface. No network
	// peer, no second party.
	embeddedClassLocal embeddedRouteClass = "local"
	// embeddedClassProbe: unauthenticated readiness/liveness endpoints the
	// embedded host's handshake depends on (backend/embedded).
	embeddedClassProbe embeddedRouteClass = "probe"
	// embeddedClassPublic: an unauthenticated route that is not a probe.
	embeddedClassPublic embeddedRouteClass = "public"
	// embeddedClassOutbound: a user-configured outbound integration or fetch.
	// Present in embedded mode today because ADR 0028 Decision 2's disabled
	// set does not list it; each is authenticated and SSRF-guarded.
	embeddedClassOutbound embeddedRouteClass = "outbound"
	// embeddedClassAdmin: multi-user administration or cross-user visibility.
	embeddedClassAdmin embeddedRouteClass = "admin"
	// embeddedClassOperator: a server-operator surface.
	embeddedClassOperator embeddedRouteClass = "operator"
)

const (
	embeddedReasonLocal = "CRM's own single-user data surface behind session auth; the on-device server exists to serve exactly this."
	embeddedReasonProbe = "Unauthenticated health/readiness probe; the embedded host's start-up handshake and the client capability gate (GET /health) read it. Exposes no user data."
	// FLAGGED (#1356): reviewed, kept registered, but not clearly intended.
	embeddedReasonPublic = "FLAGGED: public unauthenticated route (rate-limited, no data). Inherited from server mode; it does not serve a purpose without a login form. Not in ADR 0028 Decision 2's disabled set; a candidate to gate."
	// FLAGGED (#1356)
	embeddedReasonOutbound = "FLAGGED: outbound integration / remote fetch, authenticated and SSRF-guarded, but NOT in ADR 0028 Decision 2's disabled set and not hidden by the capability-gated UI beyond notification channels. Kept registered so this gate lands without a behavior change; whether the on-device server should reach the network is an open decision."
	// FLAGGED (#1356)
	embeddedReasonAdmin = "FLAGGED: multi-user administration / cross-user directory. The embedded deployment is single-user; these have no purpose there (README-developer.md says the client hides them). Not gated server-side today; a candidate to gate."
	// FLAGGED (#1356)
	embeddedReasonOperator = "FLAGGED: Prometheus scrape route, only registered when METRICS_TOKEN is set, which the embedded host never sets. This census sets it deliberately, so a host that started setting it would expose a scrape endpoint; a candidate to gate."
)

type embeddedRouteRow struct {
	class  embeddedRouteClass
	reason string
	routes []string
}

// embeddedRouteRows is the committed classification. Every route present in
// embedded mode appears in exactly one row.
var embeddedRouteRows = []embeddedRouteRow{
	{class: embeddedClassLocal, reason: embeddedReasonLocal, routes: []string{
		"DELETE /api/v1/account",
		"DELETE /api/v1/activities/:id",
		"DELETE /api/v1/attachments/:id",
		"DELETE /api/v1/cadence-policies/:id",
		"DELETE /api/v1/circles/:id",
		"DELETE /api/v1/circles/:id/members/:vcard_uid",
		"DELETE /api/v1/contacts/:id",
		"DELETE /api/v1/conversation-agenda/:id",
		"DELETE /api/v1/data-decay-policies/:id",
		"DELETE /api/v1/external-activities/:id",
		"DELETE /api/v1/external-identities/:id",
		"DELETE /api/v1/field-definitions/:id",
		"DELETE /api/v1/gifts/:id",
		"DELETE /api/v1/households/:id",
		"DELETE /api/v1/households/:id/members/:vcard_uid",
		"DELETE /api/v1/life-events/:id",
		"DELETE /api/v1/link-field-types/:id",
		"DELETE /api/v1/notes/:id",
		"DELETE /api/v1/occasion-events/:id",
		"DELETE /api/v1/occasion-events/:id/attendees/:vcard_uid",
		"DELETE /api/v1/occasion-obligations/:id",
		"DELETE /api/v1/preferences/:id",
		"DELETE /api/v1/relationship-edges/:id",
		"DELETE /api/v1/reminder-completions/:id",
		"DELETE /api/v1/reminders/:id",
		"DELETE /api/v1/sessions",
		"DELETE /api/v1/sessions/:id",
		"DELETE /api/v1/tags/:id",
		"DELETE /api/v1/tags/:id/contacts/:vcard_uid",
		"GET /api/v1/activities",
		"GET /api/v1/activities/:id",
		"GET /api/v1/attachments/:id/download",
		"GET /api/v1/audit",
		"GET /api/v1/audit/export",
		"GET /api/v1/cadence-policies",
		"GET /api/v1/cadence-policies/:id",
		"GET /api/v1/cadence-policies/overdue",
		"GET /api/v1/circles",
		"GET /api/v1/circles/:id",
		"GET /api/v1/contacts",
		"GET /api/v1/contacts/:id",
		"GET /api/v1/contacts/:id/activities",
		"GET /api/v1/contacts/:id/attachments",
		"GET /api/v1/contacts/:id/briefing",
		"GET /api/v1/contacts/:id/detail",
		"GET /api/v1/contacts/:id/field-values",
		"GET /api/v1/contacts/:id/life-event-suggestions",
		"GET /api/v1/contacts/:id/notes",
		"GET /api/v1/contacts/:id/profile_picture",
		"GET /api/v1/contacts/:id/reminder-completions",
		"GET /api/v1/contacts/:id/reminders",
		"GET /api/v1/contacts/:id/score",
		"GET /api/v1/contacts/:id/timeline",
		"GET /api/v1/contacts/birthdays",
		"GET /api/v1/contacts/circles",
		"GET /api/v1/contacts/duplicates",
		"GET /api/v1/contacts/import/history",
		"GET /api/v1/contacts/import/meerkat/preview",
		"GET /api/v1/contacts/import/meerkat/status",
		"GET /api/v1/contacts/random",
		"GET /api/v1/conversation-agenda",
		"GET /api/v1/conversation-agenda/:id",
		"GET /api/v1/dashboard",
		"GET /api/v1/data-decay-policies",
		"GET /api/v1/data-decay-policies/:id",
		"GET /api/v1/data-decay-policies/overdue",
		"GET /api/v1/export",
		"GET /api/v1/export/account",
		"GET /api/v1/export/jscontact",
		"GET /api/v1/export/preflight",
		"GET /api/v1/export/vcf",
		"GET /api/v1/external-activities",
		"GET /api/v1/external-activities/:id",
		"GET /api/v1/external-identities",
		"GET /api/v1/external-identities/:id",
		"GET /api/v1/field-definitions",
		"GET /api/v1/field-definitions/:id",
		"GET /api/v1/gifts",
		"GET /api/v1/gifts/:id",
		"GET /api/v1/graph",
		"GET /api/v1/graph/connections",
		"GET /api/v1/households",
		"GET /api/v1/households/:id",
		"GET /api/v1/import/mycorrhizal/preview",
		"GET /api/v1/import/mycorrhizal/status",
		"GET /api/v1/life-events",
		"GET /api/v1/life-events/:id",
		"GET /api/v1/link-field-types",
		"GET /api/v1/link-field-types/:id",
		"GET /api/v1/notes",
		"GET /api/v1/notes/:id",
		"GET /api/v1/occasion-events",
		"GET /api/v1/occasion-events/:id",
		"GET /api/v1/occasion-events/invitee-suggestions",
		"GET /api/v1/occasion-obligations",
		"GET /api/v1/occasion-obligations/:id",
		"GET /api/v1/occasion-obligations/card-list",
		"GET /api/v1/occasion-obligations/gift-shopping-list",
		"GET /api/v1/occasions/upcoming",
		"GET /api/v1/preferences",
		"GET /api/v1/preferences/:id",
		"GET /api/v1/reach-out-suggestions",
		"GET /api/v1/relationship-edges",
		"GET /api/v1/relationship-edges/:id",
		"GET /api/v1/reminders",
		"GET /api/v1/reminders/:id",
		"GET /api/v1/reminders/upcoming",
		"GET /api/v1/search",
		"GET /api/v1/sessions",
		"GET /api/v1/tags",
		"GET /api/v1/tags/:id",
		"GET /api/v1/users/enabled-contact-fields",
		"GET /api/v1/users/me",
		"PATCH /api/v1/conversation-agenda/:id/discuss",
		"PATCH /api/v1/households/:id/members/:vcard_uid",
		"PATCH /api/v1/relationship-edges/:id/accept",
		"PATCH /api/v1/users/date-format",
		"PATCH /api/v1/users/enabled-contact-fields",
		"PATCH /api/v1/users/language",
		"PATCH /api/v1/users/me/self-contact",
		"POST /api/v1/activities",
		"POST /api/v1/audit/:id/undo",
		"POST /api/v1/cadence-policies",
		"POST /api/v1/circles",
		"POST /api/v1/circles/:id/members",
		"POST /api/v1/contacts",
		"POST /api/v1/contacts/:id/archive",
		"POST /api/v1/contacts/:id/attachments",
		"POST /api/v1/contacts/:id/favorite",
		"POST /api/v1/contacts/:id/notes",
		"POST /api/v1/contacts/:id/profile_picture",
		"POST /api/v1/contacts/:id/reminders",
		"POST /api/v1/contacts/:id/unarchive",
		"POST /api/v1/contacts/:id/unfavorite",
		"POST /api/v1/contacts/address-suggestions",
		"POST /api/v1/contacts/address-suggestions/apply",
		"POST /api/v1/contacts/bulk",
		"POST /api/v1/contacts/duplicates/dismiss",
		"POST /api/v1/contacts/import/confirm",
		"POST /api/v1/contacts/import/jscontact/upload",
		"POST /api/v1/contacts/import/meerkat/cancel",
		"POST /api/v1/contacts/import/meerkat/confirm",
		"POST /api/v1/contacts/import/meerkat/upload",
		"POST /api/v1/contacts/import/preview",
		"POST /api/v1/contacts/import/records",
		"POST /api/v1/contacts/import/upload",
		"POST /api/v1/contacts/import/vcf/confirm",
		"POST /api/v1/contacts/import/vcf/upload",
		"POST /api/v1/contacts/merge",
		"POST /api/v1/contacts/merge/preview",
		"POST /api/v1/conversation-agenda",
		"POST /api/v1/data-decay-policies",
		"POST /api/v1/data-decay-policies/:id/verify",
		"POST /api/v1/external-activities",
		"POST /api/v1/external-identities",
		"POST /api/v1/field-definitions",
		"POST /api/v1/gifts",
		"POST /api/v1/households",
		"POST /api/v1/households/:id/members",
		"POST /api/v1/households/:id/suggest-relationships",
		"POST /api/v1/households/suggest-addresses",
		"POST /api/v1/households/suggestions/accept",
		"POST /api/v1/households/suggestions/dismiss",
		"POST /api/v1/import/mycorrhizal/cancel",
		"POST /api/v1/import/mycorrhizal/confirm",
		"POST /api/v1/import/mycorrhizal/fetch",
		"POST /api/v1/import/mycorrhizal/upload",
		"POST /api/v1/life-event-suggestions/resolve",
		"POST /api/v1/life-events",
		"POST /api/v1/link-field-types",
		"POST /api/v1/logout",
		"POST /api/v1/notes",
		"POST /api/v1/occasion-events",
		"POST /api/v1/occasion-events/:id/attendees",
		"POST /api/v1/occasion-obligations",
		"POST /api/v1/preferences",
		"POST /api/v1/reach-out-suggestions/:id/dismiss",
		"POST /api/v1/relationship-edges",
		"POST /api/v1/relationship-edges/suggest",
		"POST /api/v1/reminders/:id/complete",
		"POST /api/v1/tags",
		"POST /api/v1/tags/:id/contacts",
		"POST /api/v1/users/change-password",
		"PUT /api/v1/activities/:id",
		"PUT /api/v1/cadence-policies/:id",
		"PUT /api/v1/circles/:id",
		"PUT /api/v1/contacts/:id",
		"PUT /api/v1/contacts/:id/field-values",
		"PUT /api/v1/conversation-agenda/:id",
		"PUT /api/v1/data-decay-policies/:id",
		"PUT /api/v1/external-activities/:id",
		"PUT /api/v1/external-identities/:id",
		"PUT /api/v1/field-definitions/:id",
		"PUT /api/v1/field-definitions/reorder",
		"PUT /api/v1/gifts/:id",
		"PUT /api/v1/households/:id",
		"PUT /api/v1/life-events/:id",
		"PUT /api/v1/link-field-types/:id",
		"PUT /api/v1/link-field-types/reorder",
		"PUT /api/v1/notes/:id",
		"PUT /api/v1/occasion-events/:id",
		"PUT /api/v1/occasion-events/:id/attendees/:vcard_uid",
		"PUT /api/v1/occasion-obligations/:id",
		"PUT /api/v1/preferences/:id",
		"PUT /api/v1/relationship-edges/:id",
		"PUT /api/v1/reminders/:id",
		"PUT /api/v1/tags/:id",
	}},
	{class: embeddedClassProbe, reason: embeddedReasonProbe, routes: []string{
		"GET /health",
		"GET /health/live",
		"GET /health/ready",
	}},
	{class: embeddedClassPublic, reason: embeddedReasonPublic, routes: []string{
		"POST /api/v1/check-password-strength",
	}},
	{class: embeddedClassOutbound, reason: embeddedReasonOutbound, routes: []string{
		"DELETE /api/v1/calendars/:id",
		"DELETE /api/v1/contact-subscriptions/:id",
		"DELETE /api/v1/immich/config",
		"DELETE /api/v1/immich/contacts/:vcard_uid/link",
		"DELETE /api/v1/nextcloud/config",
		"DELETE /api/v1/nextcloud/contacts/:vcard_uid/links/:identity_id",
		"DELETE /api/v1/paperless/config",
		"DELETE /api/v1/paperless/contacts/:vcard_uid/links/:identity_id",
		"DELETE /api/v1/seafile/config",
		"DELETE /api/v1/seafile/contacts/:vcard_uid/links/:identity_id",
		"GET /api/v1/calendars",
		"GET /api/v1/contact-subscriptions",
		"GET /api/v1/contact-sync-conflicts",
		"GET /api/v1/contacts/import/monica/preview",
		"GET /api/v1/contacts/import/monica/status",
		"GET /api/v1/immich/config",
		"GET /api/v1/immich/contacts/:vcard_uid/assets",
		"GET /api/v1/immich/contacts/:vcard_uid/assets/:asset_id/image",
		"GET /api/v1/immich/contacts/:vcard_uid/summary",
		"GET /api/v1/immich/contacts/:vcard_uid/thumbnail",
		"GET /api/v1/immich/people",
		"GET /api/v1/nextcloud/config",
		"GET /api/v1/nextcloud/dir",
		"GET /api/v1/notifications/config",
		"GET /api/v1/paperless/config",
		"GET /api/v1/paperless/documents",
		"GET /api/v1/proxy/image",
		"GET /api/v1/seafile/config",
		"GET /api/v1/seafile/libraries",
		"GET /api/v1/seafile/libraries/:repo_id/dir",
		"POST /api/v1/calendars",
		"POST /api/v1/calendars/:id/sync",
		"POST /api/v1/contact-subscriptions",
		"POST /api/v1/contact-subscriptions/:id/sync",
		"POST /api/v1/contact-sync-conflicts/:id/dismiss",
		"POST /api/v1/contact-sync-conflicts/:id/restore",
		"POST /api/v1/contacts/import/meerkat/fetch",
		"POST /api/v1/contacts/import/monica/cancel",
		"POST /api/v1/contacts/import/monica/confirm",
		"POST /api/v1/contacts/import/monica/connect",
		"POST /api/v1/contacts/import/monica/fetch",
		"POST /api/v1/immich/contacts/:vcard_uid/link",
		"POST /api/v1/immich/sync",
		"POST /api/v1/immich/test-connection",
		"POST /api/v1/nextcloud/contacts/:vcard_uid/link",
		"POST /api/v1/nextcloud/test-connection",
		"POST /api/v1/notifications/config/test",
		"POST /api/v1/paperless/contacts/:vcard_uid/link",
		"POST /api/v1/paperless/test-connection",
		"POST /api/v1/seafile/contacts/:vcard_uid/link",
		"POST /api/v1/seafile/test-connection",
		"PUT /api/v1/calendars/:id",
		"PUT /api/v1/contact-subscriptions/:id",
		"PUT /api/v1/immich/config",
		"PUT /api/v1/nextcloud/config",
		"PUT /api/v1/notifications/config",
		"PUT /api/v1/paperless/config",
		"PUT /api/v1/seafile/config",
	}},
	{class: embeddedClassAdmin, reason: embeddedReasonAdmin, routes: []string{
		"DELETE /api/v1/admin/users/:id",
		"GET /api/v1/admin/diagnostics",
		"GET /api/v1/admin/error-aggregation",
		"GET /api/v1/admin/integrity-check",
		"GET /api/v1/admin/job-runs",
		"GET /api/v1/admin/job-runs/health",
		"GET /api/v1/admin/notification-health",
		"GET /api/v1/admin/subsystem-health",
		"GET /api/v1/admin/system-events",
		"GET /api/v1/admin/system-status",
		"GET /api/v1/admin/users",
		"GET /api/v1/admin/users/:id",
		"GET /api/v1/users/directory",
		"PATCH /api/v1/admin/users/:id",
		"POST /api/v1/admin/contacts/rebuild-derived",
		"POST /api/v1/admin/search/rebuild",
		"POST /api/v1/admin/trigger-purge",
		"POST /api/v1/admin/trigger-reminders",
		"POST /api/v1/admin/users",
		"POST /api/v1/admin/users/:id/reset-2fa",
	}},
	{class: embeddedClassOperator, reason: embeddedReasonOperator, routes: []string{
		"GET /metrics",
	}},
}

// embeddedRouteTable builds a router with every optional surface enabled and
// returns its sorted "METHOD path" set.
func embeddedRouteTable(t *testing.T, mode string) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	cfg := testConfig()
	cfg.Deployment = mode
	cfg.CardDAVEnabled = true
	cfg.CalDAVEnabled = true
	cfg.MetricsToken = "census-token"

	RegisterRoutes(router, cfg, db, nil)
	return routeSet(t, router)
}

func TestEmbeddedRouteSurface_Classified(t *testing.T) {
	embedded := embeddedRouteTable(t, config.DeploymentEmbedded)
	server := embeddedRouteTable(t, config.DeploymentServer)

	declared := map[string]embeddedRouteRow{}
	for _, row := range embeddedRouteRows {
		require.NotEmpty(t, row.reason, "every group needs a reason")
		require.NotEmpty(t, row.class, "every group needs a class")
		for _, r := range row.routes {
			_, dup := declared[r]
			require.Falsef(t, dup, "%s is declared twice", r)
			declared[r] = row
		}
	}

	var unclassified, stale, notInServer []string
	for r := range embedded {
		if _, ok := declared[r]; !ok {
			unclassified = append(unclassified, r)
		}
		if !server[r] {
			notInServer = append(notInServer, r)
		}
	}
	for r := range declared {
		if !embedded[r] {
			stale = append(stale, r)
		}
	}
	sort.Strings(unclassified)
	sort.Strings(stale)
	sort.Strings(notInServer)

	require.Emptyf(t, unclassified,
		"routes registered in embedded mode with no declared classification in embeddedRouteRows. "+
			"If a route is network-shaped, gate it on !cfg.IsEmbedded(); otherwise classify it (issue #1356): %v", unclassified)
	require.Emptyf(t, stale,
		"embeddedRouteRows lists routes that embedded mode no longer registers (dead entries): %v", stale)
	require.Emptyf(t, notInServer,
		"embedded mode registers routes server mode does not; embedded must only ever remove surface: %v", notInServer)
}

// TestEmbeddedRouteSurface_DisabledSetNeverClassified pins that nothing
// classified as reachable in embedded mode is also a route the existing
// hand-written disabled list (TestRegisterRoutes_EmbeddedOmitsNetworkSurfaces)
// says must be absent: the two lists can never disagree silently.
func TestEmbeddedRouteSurface_DisabledSetNeverClassified(t *testing.T) {
	for _, r := range []string{
		"POST /api/v1/login",
		"POST /api/v1/register",
		"GET /api/v1/api-tokens",
		"GET /api/v1/webhooks",
		"GET /.well-known/carddav",
		"GET /.well-known/assetlinks.json",
	} {
		for _, row := range embeddedRouteRows {
			for _, declared := range row.routes {
				require.NotEqualf(t, r, declared, "%s is network-only and must not be classified as embedded-reachable", r)
			}
		}
	}
}
