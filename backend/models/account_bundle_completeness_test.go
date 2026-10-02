package models

import (
	"sort"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

// Bundle completeness (issue #1259 acceptance): a model that gains a user
// scope must either be carried by the account bundle or be explicitly excluded
// with a reason. Without this, a new user-authored entity silently never
// enters the bundle and a round-trip loses it.
//
// The bundle represents most entities through AccountBundlePlan sections; a
// few user-scoped models are deliberately outside it (credentials, operational
// bookkeeping, sync ledgers) and must be named here.

// bundleCovered maps a user-scoped model to the bundle section that carries
// it.
var bundleCovered = map[string]string{
	"Activity":              "plan.activities",
	"Attachment":            "attachments (metadata only)",
	"CadencePolicy":         "plan.cadence_policies",
	"Circle":                "plan.circles",
	"CircleMember":          "plan.circles[].member_uids",
	"Contact":               "plan.contacts",
	"ContactTag":            "plan.tags[].contact_uids",
	"ConversationAgenda":    "plan.conversation_agenda",
	"DataDecayPolicy":       "plan.data_decay_policies",
	"FieldDefinition":       "plan.custom_field_definitions",
	"FieldValue":            "plan.custom_field_values",
	"Gift":                  "plan.gifts",
	"Household":             "plan.households",
	"HouseholdMember":       "plan.households[].members",
	"LifeEvent":             "plan.life_events",
	"Note":                  "plan.notes",
	"OccasionEvent":         "plan.occasion_events",
	"OccasionEventAttendee": "plan.occasion_events[].attendees",
	"OccasionObligation":    "plan.occasions",
	"Preference":            "plan.preferences",
	"RelationshipEdge":      "plan.relationships",
	"Reminder":              "plan.reminders",
	"ReminderCompletion":    "plan.reminder_completions",
	"Tag":                   "plan.tags",
}

// bundleExcluded maps a user-scoped model deliberately outside the bundle to
// the reason. Each entry is a decision, not an oversight.
var bundleExcluded = map[string]string{
	"AlertState":                    "global operational alert bookkeeping, not user-authored contact data",
	"ApiToken":                      "credential material; never exported",
	"AuditEvent":                    "tamper-evident audit log; has its own export endpoint",
	"CalendarEventLink":             "external-sync ledger row; re-derived on the destination",
	"CalendarSubscription":          "per-instance external calendar configuration/credential",
	"CardDAVSync":                   "per-instance CardDAV sync state/cursor",
	"ContactShare":                  "cross-user share bookkeeping, not the sharer's own data",
	"ContactSubscription":           "per-instance external contact subscription/credential",
	"ContactSyncConflict":           "sync-conflict notice derived from a CardDAV connection",
	"ContactSyncLink":               "external-sync ledger row; re-derived on the destination",
	"DeviceGrant":                   "device credential/session bookkeeping",
	"DeviceRegistration":            "push device registration (endpoint/credential)",
	"DismissedDuplicatePair":        "per-instance review UI state; not portable data",
	"DismissedHouseholdSuggestion":  "per-instance review UI state; not portable data",
	"ExternalActivity":              "external integration cache; re-fetched on the destination",
	"ExternalIdentity":              "external integration identity mapping; re-derived",
	"Feed":                          "credential material (hashed feed token); never exported",
	"IdempotencyKey":                "request idempotency cache; not portable data",
	"GeoPulseConfig":                "per-instance integration configuration/credential",
	"ImmichConfig":                  "per-instance integration configuration/credential",
	"ImportRun":                     "import history bookkeeping, not user-authored data",
	"ImportSourceLink":              "import idempotency ledger, destination-specific",
	"JobRun":                        "scheduler bookkeeping",
	"LifeEventSuggestionResolution": "per-instance suggestion review state; not portable data",
	"LinkFieldType":                 "per-user field-type registry configured on the destination",
	"NotificationConfig":            "per-instance notification configuration",
	"NotificationDelivery":          "notification delivery bookkeeping",
	"PaperlessConfig":               "per-instance integration configuration/credential",
	"PushSubscription":              "push subscription endpoint/credential",
	"ReachOutCursor":                "per-instance suggestion cursor; not portable data",
	"ReachOutSuggestion":            "computed suggestion; re-derived on the destination",
	"RecoveryCode":                  "2FA credential material; never exported",
	"SeafileConfig":                 "per-instance integration configuration/credential",
	"Session":                       "authentication session; never exported",
	"SystemEvent":                   "system-generated diagnostic timeline; not user-authored data",
	"WebAuthnCredential":            "2FA credential material; never exported",
	"WebDAVConfig":                  "per-instance integration configuration/credential",
	"Webhook":                       "per-instance webhook configuration/credential",
	"WebhookDelivery":               "webhook delivery bookkeeping",
}

// TestAccountBundleCoversEveryUserScopedModel is the completeness net.
func TestAccountBundleCoversEveryUserScopedModel(t *testing.T) {
	db := dbtest.New(t)

	registered := map[string]bool{}
	for _, m := range registeredModels {
		s := parseModel(t, db, m)
		registered[s.Name] = true
		if !modelHasUserScope(s) {
			continue
		}
		_, covered := bundleCovered[s.Name]
		_, excluded := bundleExcluded[s.Name]
		if !covered && !excluded {
			t.Errorf("user-scoped model %s is in neither bundleCovered nor bundleExcluded; "+
				"add a bundle section (and AccountBundlePlan field) or record why it is excluded", s.Name)
		}
	}

	// Anti-rot: every classified name is a real model.
	for name := range bundleCovered {
		require.True(t, registered[name], "bundleCovered entry %q is not a registered model; remove it", name)
	}
	for name := range bundleExcluded {
		require.True(t, registered[name], "bundleExcluded entry %q is not a registered model; remove it", name)
	}
}

// modelHasUserScope reports whether a model is scoped to a user by a user_id
// column.
func modelHasUserScope(s *schema.Schema) bool {
	for _, f := range s.Fields {
		if f.DBName == "user_id" && !f.IgnoreMigration {
			return true
		}
	}
	return false
}

// TestAccountBundleCompleteness_MapsSorted documents the classification size so
// a reviewer notices a silently gutted map.
func TestAccountBundleCompleteness_MapsSorted(t *testing.T) {
	require.Greater(t, len(bundleCovered), 20)
	require.Greater(t, len(bundleExcluded), 20)
	names := make([]string, 0, len(bundleCovered))
	for n := range bundleCovered {
		names = append(names, n)
	}
	sort.Strings(names)
	require.NotEmpty(t, names)
}
