package routes

// TestDBFaultReadSweep is the read-route counterpart of TestDBFaultSweep
// (issue #1497 coverage gate). The write sweep only reaches the failure arms of
// mutating handlers; every GET handler has the same
// `if err := tx.Find(...).Error; err != nil { return ErrDatabase(...) }` shape
// and those arms are otherwise unreachable through the HTTP surface (the
// controllers talk to *gorm.DB directly).
//
// For every GET route in faultReadRoutes the sweep records the statements the
// handler issues on the request goroutine (K), then fails statement i for each
// i after the auth middleware's own prefix and requires, for every i, that:
//
//  1. the response is never a panic/hang: it is either the route's normal 2xx
//     (a best-effort enrichment read whose failure is logged and skipped by
//     design) or a well-formed 5xx error envelope whose code is one of the
//     documented server-fault codes;
//  2. the body leaks no SQL or table name (the same leak check the write sweep
//     uses);
//  3. across the whole route, at least one statement failure surfaces as a 5xx
//     (a handler that swallowed *every* database error would return fabricated
//     empty data for an outage).
//
// Reads never mutate, so one seeded database per route serves every injected
// statement.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/fireandforget"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// faultReadRoutes returns the GET paths swept, with the seeded owner's ids
// substituted. Integration routes that need a configured external system
// (immich/paperless/seafile/nextcloud/geopulse) are exercised by their own
// controller tests and are not listed.
func faultReadRoutes(s seeded) []string {
	const v1 = "/api/v1"
	return []string{
		v1 + "/users/enabled-contact-fields",
		v1 + "/users/me",
		v1 + "/users/2fa/status",
		v1 + "/webauthn/credentials",
		v1 + "/users/directory",
		v1 + "/dashboard",
		v1 + "/contacts",
		v1 + "/contacts?q=matrix",
		v1 + "/contacts/circles",
		v1 + "/contacts/random",
		v1 + "/contacts/birthdays",
		v1 + "/contacts/duplicates",
		v1 + "/contacts/map",
		v1 + "/contacts/" + s.contact,
		v1 + "/contacts/" + s.contact + "/briefing",
		v1 + "/contacts/" + s.contact + "/detail",
		v1 + "/contacts/" + s.contact + "/timeline",
		v1 + "/contacts/" + s.contact + "/score",
		v1 + "/contacts/" + s.contact + "/life-event-suggestions",
		v1 + "/contacts/import/history",
		v1 + "/contact-shares/incoming",
		v1 + "/contact-shares/outgoing",
		v1 + "/relationship-edges",
		v1 + "/relationship-edges/" + s.edge,
		v1 + "/contacts/" + s.contact + "/attachments",
		v1 + "/contacts/" + s.contact + "/notes",
		v1 + "/notes/" + s.note,
		v1 + "/notes",
		v1 + "/notes?from_date=2020-01-01&to_date=2099-01-01&search=matrix",
		v1 + "/contacts/" + s.contact + "/activities",
		v1 + "/activities",
		v1 + "/activities/" + s.activity,
		v1 + "/circles",
		v1 + "/circles/" + s.circle,
		v1 + "/link-field-types",
		v1 + "/link-field-types/" + s.linkType,
		v1 + "/households",
		v1 + "/households/" + s.household,
		v1 + "/tags",
		v1 + "/tags/" + s.tag,
		v1 + "/field-definitions",
		v1 + "/field-definitions/" + s.fieldDef,
		v1 + "/contacts/" + s.contact + "/field-values",
		v1 + "/life-events",
		v1 + "/life-events/" + s.lifeEvent,
		v1 + "/conversation-agenda",
		v1 + "/conversation-agenda/" + s.agenda,
		v1 + "/gifts",
		v1 + "/gifts/" + s.gift,
		v1 + "/preferences",
		v1 + "/preferences/" + s.preference,
		v1 + "/occasion-obligations",
		v1 + "/occasion-obligations/" + s.occasionObligation,
		v1 + "/occasions/upcoming",
		v1 + "/occasion-obligations/card-list",
		v1 + "/occasion-obligations/gift-shopping-list",
		v1 + "/occasion-events",
		v1 + "/occasion-events/invitee-suggestions?circle_ids=" + s.circle,
		v1 + "/occasion-events/" + s.occasionEvent,
		v1 + "/cadence-policies/overdue",
		v1 + "/cadence-policies",
		v1 + "/cadence-policies/" + s.cadence,
		v1 + "/reach-out-suggestions",
		v1 + "/reminders",
		v1 + "/reminders/upcoming",
		v1 + "/contacts/" + s.contact + "/reminders",
		v1 + "/reminders/" + s.reminder,
		v1 + "/contacts/" + s.contact + "/reminder-completions",
		v1 + "/export",
		v1 + "/export/jscontact",
		v1 + "/export/preflight?format=vcard4",
		v1 + "/export/account",
		v1 + "/graph",
		v1 + "/graph/connections?from=" + s.contactUID,
		v1 + "/search?q=matrix",
		v1 + "/api-tokens",
		v1 + "/feeds",
		v1 + "/sessions",
		v1 + "/auth/device/grants",
		v1 + "/webhooks",
		v1 + "/webhooks/" + s.webhook,
		v1 + "/webhooks/" + s.webhook + "/deliveries",
		v1 + "/audit",
		v1 + "/audit/export",
		v1 + "/notifications/config",
		v1 + "/notifications/push-subscriptions",
		v1 + "/notifications/devices",
		v1 + "/calendars",
		v1 + "/contact-subscriptions",
		v1 + "/contact-sync-conflicts",
		v1 + "/external-identities",
		v1 + "/external-identities/" + s.extIdentity,
		v1 + "/external-activities",
		v1 + "/external-activities/" + s.extActivity,
		v1 + "/data-decay-policies/overdue",
		v1 + "/data-decay-policies",
		v1 + "/data-decay-policies/" + s.dataDecay,
		v1 + "/admin/users",
		v1 + "/admin/system-events",
		v1 + "/admin/subsystem-health",
		v1 + "/admin/job-runs",
		v1 + "/admin/job-runs/health",
		v1 + "/admin/error-aggregation",
		v1 + "/admin/notification-health",
		v1 + "/admin/diagnostics",
		v1 + "/admin/integrity-check",
		v1 + "/admin/system-status",
	}
}

var faultServerCodes = map[string]bool{
	apperrors.ErrCodeDatabase: true,
	apperrors.ErrCodeInternal: true,
	apperrors.ErrCodeExternal: true,
}

func TestDBFaultReadSweep(t *testing.T) {
	probe := newFaultEnv(t)
	routes := faultReadRoutes(seedResources(t, probe.db, probe.owner.ID))

	for idx, label0 := range routes {
		path := label0
		t.Run("GET "+path, func(t *testing.T) {
			e := newFaultEnv(t)
			res := seedResources(t, e.db, e.owner.ID)
			require.NoError(t, e.db.Model(&e.owner).Update("is_admin", true).Error)
			// Resource ids (UUIDs) differ per database: re-derive this route's
			// path against the env's own seed.
			req := faultReq{method: http.MethodGet, path: faultReadRoutes(res)[idx]}

			// Warm-up: a few GETs lazily provision rows on first read (e.g.
			// /users/me creates the self contact), which would shift the
			// statement count between the recorded run and the injected ones.
			e.do(t, req)
			fireandforget.Wait()
			e.inj.Record()
			w := e.do(t, req)
			e.inj.Disarm()
			fireandforget.Wait()
			require.Equalf(t, http.StatusOK, w.Code, "happy path must succeed: %s", w.Body.String())
			stmts := e.inj.Statements()
			require.GreaterOrEqual(t, len(stmts), len(faultAuthPrefix))
			for i, want := range faultAuthPrefix {
				require.Equal(t, want, stmts[i].String(), "AuthMiddleware's leading statements changed; update faultAuthPrefix")
			}
			tables := underscoreTables(t, e.db)

			surfaced := 0
			for i := len(faultAuthPrefix) + 1; i <= len(stmts); i++ {
				e.inj.FailNth(i)
				w := e.do(t, req)
				fired, ok := e.inj.Fired()
				e.inj.Disarm()
				fireandforget.Wait()
				label := fmt.Sprintf("statement %d/%d (%s)", i, len(stmts), stmts[i-1])
				require.Truef(t, ok, "%s: injection did not fire (statement count drifted)", label)
				require.Equalf(t, stmts[i-1], fired, "%s: statement order drifted", label)

				if w.Code >= 200 && w.Code < 300 {
					continue // best-effort read, failure logged and skipped
				}
				assert.Emptyf(t, faultLeak(w.Body.String(), tables), "%s: body leaks internals: %s", label, w.Body.String())
				surfaced++
				var env struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				require.NoErrorf(t, json.Unmarshal(w.Body.Bytes(), &env), "%s: HTTP %d body is not JSON: %s", label, w.Code, w.Body.String())
				assert.Truef(t, w.Code >= 500 && w.Code < 600, "%s: a database fault must be a 5xx, got %d: %s", label, w.Code, w.Body.String())
				assert.Truef(t, faultServerCodes[env.Error.Code], "%s: error code %q is not a server-fault code: %s", label, env.Error.Code, w.Body.String())
				assert.NotEmptyf(t, env.Error.Message, "%s: empty error message", label)
			}
			if len(stmts) > len(faultAuthPrefix) {
				assert.Positivef(t, surfaced, "no statement failure of %s surfaced as an error: the handler masks a database outage", path)
			}
		})
	}
}
