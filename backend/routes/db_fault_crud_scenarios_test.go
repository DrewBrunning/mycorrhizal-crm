package routes

// Additional TestDBFaultSweep scenarios (issue #1497 coverage gate): the CRUD
// write handlers of the entity controllers that the original curated list does
// not reach. Each scenario is a single request; TestDBFaultSweep fails every
// statement it issues in turn and requires an error envelope, no SQL leak and
// an unchanged database for each.

import (
	"net/http"
	"strconv"
	"time"

	"mycorrhizal/models"

	"testing"

	"github.com/stretchr/testify/require"
)

func faultCRUDScenarios() []faultScenario {
	when := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	const v1 = "/api/v1"

	// scenario seeds the standard resource set and builds the request from it.
	sc := func(name string, build func(r seeded) faultReq) faultScenario {
		return faultScenario{name, func(t *testing.T, e *faultEnv) faultReq {
			return build(seedResources(t, e.db, e.owner.ID))
		}}
	}

	return []faultScenario{
		sc("POST /notes", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/notes", map[string]any{"content": "n", "date": when}}
		}),
		sc("POST /contacts/:id/notes", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/contacts/" + r.contact + "/notes", map[string]any{"content": "n", "date": when}}
		}),
		sc("PUT /notes/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/notes/" + r.note, map[string]any{"content": "edited", "date": when}}
		}),
		sc("POST /activities", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/activities", map[string]any{"title": "a", "date": when, "contact_ids": []uint{mustUint(r.contact)}}}
		}),
		sc("PUT /activities/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/activities/" + r.activity, map[string]any{"title": "edited", "date": when}}
		}),
		sc("DELETE /activities/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/activities/" + r.activity}
		}),
		sc("POST /circles", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/circles", map[string]any{"name": "new circle"}}
		}),
		sc("PUT /circles/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/circles/" + r.circle, map[string]any{"name": "renamed"}}
		}),
		sc("POST /households", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/households", map[string]any{"name": "new", "type": "other"}}
		}),
		sc("PUT /households/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/households/" + r.household, map[string]any{"name": "renamed", "type": "roommates"}}
		}),
		sc("POST /tags", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/tags", map[string]any{"name": "new tag"}}
		}),
		sc("PUT /tags/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/tags/" + r.tag, map[string]any{"name": "renamed"}}
		}),
		sc("POST /field-definitions", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/field-definitions", map[string]any{"label": "L", "key": "fault_key", "type": "string"}}
		}),
		sc("PUT /field-definitions/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/field-definitions/" + r.fieldDef, map[string]any{"label": "L2", "key": "matrix_custom", "type": "string"}}
		}),
		sc("DELETE /field-definitions/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/field-definitions/" + r.fieldDef}
		}),
		sc("POST /life-events", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/life-events", map[string]any{"entity_id": r.contactUID, "type": "moved", "description": "d"}}
		}),
		sc("PUT /life-events/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/life-events/" + r.lifeEvent, map[string]any{"entity_id": r.contactUID, "type": "moved", "description": "edited"}}
		}),
		sc("DELETE /life-events/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/life-events/" + r.lifeEvent}
		}),
		sc("POST /conversation-agenda", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/conversation-agenda", map[string]any{"entity_id": r.contactUID, "content": "talk"}}
		}),
		sc("PUT /conversation-agenda/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/conversation-agenda/" + r.agenda, map[string]any{"entity_id": r.contactUID, "content": "edited"}}
		}),
		sc("DELETE /conversation-agenda/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/conversation-agenda/" + r.agenda}
		}),
		sc("POST /gifts", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/gifts", map[string]any{"entity_id": r.contactUID, "description": "book"}}
		}),
		sc("PUT /gifts/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/gifts/" + r.gift, map[string]any{"entity_id": r.contactUID, "description": "edited"}}
		}),
		sc("DELETE /gifts/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/gifts/" + r.gift}
		}),
		sc("POST /preferences", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/preferences", map[string]any{"entity_id": r.contactUID, "category": "food", "value": "tacos"}}
		}),
		sc("PUT /preferences/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/preferences/" + r.preference, map[string]any{"entity_id": r.contactUID, "category": "food", "value": "edited"}}
		}),
		sc("DELETE /preferences/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/preferences/" + r.preference}
		}),
		sc("POST /occasion-obligations", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/occasion-obligations", map[string]any{"entity_id": r.contactUID, "kind": "card", "label": "xmas"}}
		}),
		sc("PUT /occasion-obligations/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/occasion-obligations/" + r.occasionObligation, map[string]any{"entity_id": r.contactUID, "kind": "card", "label": "edited"}}
		}),
		sc("DELETE /occasion-obligations/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/occasion-obligations/" + r.occasionObligation}
		}),
		sc("POST /occasion-events", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/occasion-events", map[string]any{"title": "party", "starts_at": when}}
		}),
		sc("PUT /occasion-events/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/occasion-events/" + r.occasionEvent, map[string]any{"title": "edited", "starts_at": when}}
		}),
		sc("DELETE /occasion-events/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/occasion-events/" + r.occasionEvent}
		}),
		sc("POST /occasion-events/:id/attendees", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/occasion-events/" + r.occasionEvent + "/attendees", map[string]any{"entity_id": r.contactUID}}
		}),
		sc("POST /cadence-policies", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/cadence-policies", map[string]any{"entity_id": r.contactUID, "target_interval_days": 14}}
		}),
		sc("PUT /cadence-policies/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/cadence-policies/" + r.cadence, map[string]any{"entity_id": r.contactUID, "target_interval_days": 21}}
		}),
		sc("DELETE /cadence-policies/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/cadence-policies/" + r.cadence}
		}),
		sc("POST /data-decay-policies", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/data-decay-policies", map[string]any{"entity_id": r.contactUID, "interval_days": 90}}
		}),
		sc("PUT /data-decay-policies/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/data-decay-policies/" + r.dataDecay, map[string]any{"entity_id": r.contactUID, "interval_days": 120}}
		}),
		sc("DELETE /data-decay-policies/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/data-decay-policies/" + r.dataDecay}
		}),
		sc("POST /webhooks", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/webhooks", map[string]any{"name": "hook", "url": "https://example.com/h", "events": []string{"contact.created"}, "is_active": true}}
		}),
		sc("PUT /webhooks/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/webhooks/" + r.webhook, map[string]any{"name": "renamed", "url": "https://example.com/h", "events": []string{"contact.created"}, "is_active": true}}
		}),
		sc("DELETE /webhooks/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/webhooks/" + r.webhook}
		}),
		sc("POST /external-identities", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/external-identities", map[string]any{"entity_id": r.contactUID, "system": "paperless", "external_id": "ext-1"}}
		}),
		sc("PUT /external-identities/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/external-identities/" + r.extIdentity, map[string]any{"entity_id": r.contactUID, "system": "immich", "external_id": "ext-2"}}
		}),
		sc("DELETE /external-identities/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/external-identities/" + r.extIdentity}
		}),
		sc("POST /external-activities", func(r seeded) faultReq {
			return faultReq{http.MethodPost, v1 + "/external-activities", map[string]any{"entity_id": r.contactUID, "source_system": "immich", "external_id": "x1", "type": "photo", "occurred_at": when}}
		}),
		sc("PUT /external-activities/:id", func(r seeded) faultReq {
			return faultReq{http.MethodPut, v1 + "/external-activities/" + r.extActivity, map[string]any{"entity_id": r.contactUID, "source_system": "immich", "external_id": "matrix-asset-1", "type": "photo2", "occurred_at": when}}
		}),
		sc("DELETE /external-activities/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/external-activities/" + r.extActivity}
		}),
		sc("DELETE /relationship-edges/:id", func(r seeded) faultReq {
			return faultReq{method: http.MethodDelete, path: v1 + "/relationship-edges/" + r.edge}
		}),
		{"POST /relationship-edges", func(t *testing.T, e *faultEnv) faultReq {
			c, other := seedRichContact(t, e)
			return faultReq{http.MethodPost, v1 + "/relationship-edges", models.RelationshipEdgeInput{SourceID: c.VCardUID, TargetID: other.VCardUID, Type: "mentor_of"}}
		}},
	}
}

// requireNoDuplicateFaultNames guards the merged scenario list: subtests and
// allowlist keys are keyed by scenario name.
func requireNoDuplicateFaultNames(t *testing.T, all []faultScenario) {
	t.Helper()
	seen := map[string]bool{}
	for _, s := range all {
		require.Falsef(t, seen[s.name], "duplicate fault scenario name %q", s.name)
		seen[s.name] = true
	}
}

func mustUint(s string) uint {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return uint(n)
}

// faultReasonPostCommitRead marks a 2xx outcome whose failed statement is the
// post-commit response read (issue #1663): the write is already committed, so
// the handler answers 2xx from the committed in-memory entity (warn-logged)
// and the sweep additionally asserts the write persisted.
const faultReasonPostCommitRead = "post-commit response read: the write is committed, so a failed re-read answers 2xx with the committed in-memory entity and a warn log, never 5xx (#1663)"

// crudAllowlist are the 2xx outcomes (best-effort side effects) of the CRUD
// scenarios.
func crudAllowlist() map[string]string {
	m := map[string]string{}
	for k, v := range map[string]string{
		"POST /activities|query activities#1":      faultReasonAudit,
		"POST /activities|query contacts#2":        faultReasonAudit,
		"PUT /notes/:id|query notes#2":             faultReasonAudit,
		"PUT /activities/:id|query activities#2":   faultReasonAudit,
		"PUT /circles/:id|query circles#2":         faultReasonAudit,
		"PUT /households/:id|query households#2":   faultReasonAudit,
		"PUT /tags/:id|query tags#2":               faultReasonAudit,
		"PUT /life-events/:id|query life_events#2": faultReasonAudit,
		"PUT /gifts/:id|query gifts#2":             faultReasonAudit,

		"PUT /activities/:id|query activities#3":        faultReasonPostCommitRead,
		"PUT /activities/:id|query activity_contacts#2": faultReasonPostCommitRead,
		"POST /cadence-policies|query activities#1":     faultReasonPostCommitRead,
		"PUT /cadence-policies/:id|query activities#1":  faultReasonPostCommitRead,
		// Issue #1736 / #1733: the health derivation also reads given gifts and
		// the ambient flag/household (both scanned rows), post-commit like the
		// activities read above.
		"POST /cadence-policies|row c#1":    faultReasonPostCommitRead,
		"POST /cadence-policies|row c#2":    faultReasonPostCommitRead,
		"PUT /cadence-policies/:id|row c#1": faultReasonPostCommitRead,
		"PUT /cadence-policies/:id|row c#2": faultReasonPostCommitRead,
	} {
		m[k] = v
	}
	return m
}
