package integrations

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// int02Coverage maps each Registry() integration to the test(s) that exercise
// its external failure behavior (INT-02, issue #465). The bar per the ticket is
// "every integration × every applicable failure mode has a test with a declared
// expected outcome" — this ledger is what makes an integration added without
// that coverage a build failure, the same shape as
// TestEveryOutboundClientIsClassified.
//
// A test named here must exist in backend/services (TestListedFailureTestsExist
// greps for it). Where the behavior is covered by a pre-existing suite, that
// suite is cited rather than duplicated.
var int02Coverage = map[string][]string{
	"carddav": {
		"TestContactSync_RequestFailureIsDefinedAndObservable", // fail recorded, lock released, local data safe, sync_failed event
		"TestSync_HungRemoteIsBoundedByContext",                // a hung remote is bounded, not a job-stall
	},
	"caldav": {
		"TestCalendarSync_RequestFailureIsDefinedAndObservable",
	},
	"immich": {
		"TestImmichInjectedRequestFaultCrossesBoundaryUnchanged", // pre-existing #434 seam suite
		"TestImmichInjectedFaultReachesServiceDiagnostics",
	},
	"paperless": {
		"TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged",
		"TestIntegrationClient_StatusMappingIsStable",
		"TestIntegrationClient_BlackHoleHostIsBounded",
		"TestIntegrationClient_ConnectionRefusedIsFast",
	},
	"seafile": {
		"TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged",
		"TestIntegrationClient_StatusMappingIsStable",
		"TestIntegrationClient_BlackHoleHostIsBounded",
	},
	"webdav": {
		"TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged",
		"TestIntegrationClient_StatusMappingIsStable",
		"TestIntegrationClient_BlackHoleHostIsBounded",
	},
	"webhooks": {
		"TestWebhookDelivery_InjectedFaultRecordsAndSchedulesRetry",
		"TestWebhookDelivery_TerminalAtMaxAttempts", // bounded: no retry past maxDeliveryAttempts
		"TestDeliverWebhookConnectionErrorSchedulesRetry",
		"TestGuardedClientBlocksInternalAddresses",
	},
	"ntfy": {
		"TestNotificationDelivery_InjectedFaultRecordsFailureAndKeepsReminderDue",
		"TestSendReminders_ChannelFailureIsolation",
	},
	"gotify": {
		"TestNotificationDelivery_InjectedFaultRecordsFailureAndKeepsReminderDue",
		"TestSendReminders_ChannelFailureIsolation",
	},
	"webpush": {
		"TestNotificationDelivery_InjectedFaultOnWebPushKeepsSubscription", // sendPushMessage seam: failed, subscription kept
		"TestPushSender_RemovesStaleSubscription",                          // pre-existing: 404/410 → subscription dropped
		"TestSendReminders_PushPrivateAddressBlocked",
	},
	"email-resend": {
		"TestSendViaResend_InjectedFaultSurfaces",
		"TestResendHTTPClient_HasExplicitTimeout",
	},
	"email-smtp": {
		"TestSendViaSMTP_StalledServerIsBounded", // the finding fix: a hung MX cannot block forever
		"TestSendViaSMTP_DialTimeoutIsBounded",
		"TestSendEmail_InjectedSendFaultSurfaces",
	},
	"oidc": {
		"TestOIDCClient_BlocksPrivateAddressWhenEnabled", // the finding fix: guarded dialer
		"TestOIDCClient_TimeoutIsWired",
		"TestExchangeAndVerify_TokenEndpointError", // pre-existing: token-endpoint failure path
	},
	"hibp": {
		"TestCheckPasswordBreached_FailsOpenOnServerError",     // pre-existing: fail-open on 5xx
		"TestCheckPasswordBreached_FailsOpenOnUnreachableHost", // pre-existing: fail-open on unreachable
	},
	"update-check": {
		"TestBuildUpdateCheckStatus_Enabled_StubErrorIsUnknownNotFailure", // error → "unknown", never a spurious "available"
		"TestLatestRelease_ErrorsAreNotCached",                            // a failed lookup is retried, not memoized
		"TestLatestRelease_GarbageBodyReturnsError",
	},
}

// TestEveryIntegrationHasFailureBehaviorCoverage fails if a Registry() entry
// has no INT-02 ledger row — the "a new integration added without a test"
// structural guard.
func TestEveryIntegrationHasFailureBehaviorCoverage(t *testing.T) {
	for _, in := range Registry() {
		tests := int02Coverage[in.ID]
		if len(tests) == 0 {
			t.Errorf("integration %q has no INT-02 failure-behavior test in int02Coverage — "+
				"add a services/*_failure_behavior_test.go case and list it here", in.ID)
		}
	}
	for id := range int02Coverage {
		if _, ok := ByID(id); !ok {
			t.Errorf("int02Coverage names %q, which is not in Registry()", id)
		}
	}
}

// TestListedFailureTestsExist greps backend/services for every test named in
// the ledger, so a rename that orphans a citation fails here rather than
// silently weakening the guarantee.
func TestListedFailureTestsExist(t *testing.T) {
	dir := filepath.Join("..", "services")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	defined := map[string]bool{}
	funcDecl := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for _, m := range funcDecl.FindAllStringSubmatch(string(body), -1) {
			defined[m[1]] = true
		}
	}

	seen := map[string]bool{}
	for id, tests := range int02Coverage {
		for _, name := range tests {
			seen[name] = true
			if !defined[name] {
				t.Errorf("int02Coverage[%q] cites %q, which is not defined in backend/services/*_test.go", id, name)
			}
		}
	}
	if len(seen) < 15 {
		t.Errorf("ledger cites only %d distinct tests — suspiciously thin for %d integrations", len(seen), len(Registry()))
	}
}
