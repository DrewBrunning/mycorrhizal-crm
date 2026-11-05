package integrations

import (
	"path/filepath"
	"testing"
)

// int02Coverage maps each Registry() integration to the test(s) that exercise
// its external failure behavior (INT-02, issue #465). The bar per the ticket is
// "every integration × every applicable failure mode has a test with a declared
// expected outcome" — this ledger is what makes an integration added without
// that coverage a build failure, the same shape as
// TestEveryOutboundClientIsClassified.
//
// A test named here must exist in backend/services, backend/meerkatapi or
// backend/monica (TestListedFailureTestsExist greps for it). Where the behavior is covered by a pre-existing suite, that
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
	"geocoder": {
		"TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged",
		"TestIntegrationClient_StatusMappingIsStable",
		"TestIntegrationClient_BlackHoleHostIsBounded",
		"TestGeocoderClient_MalformedBodiesAreInvalidData",
		"TestGeocoderClient_RateLimitedIsDistinct",
	},
	"paperless": {
		"TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged",
		"TestIntegrationClient_StatusMappingIsStable",
		"TestIntegrationClient_BlackHoleHostIsBounded",
		"TestIntegrationClient_ConnectionRefusedIsFast",
	},
	"geopulse": {
		"TestIntegrationClient_InjectedFaultCrossesBoundaryUnchanged",
		"TestIntegrationClient_StatusMappingIsStable",
		"TestIntegrationClient_BlackHoleHostIsBounded",
		"TestGeoPulseSuggestions_PhotoFailureDegradesNotFails", // request-scoped: a failed photo call degrades, a failed timeline call fails
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
	// The two import-assistant clients live in their own packages, not under
	// services; TestListedFailureTestsExist resolves names in all three.
	"meerkat-api": {
		"TestUnreachableServer",                          // meerkatapi: connection refused -> ErrUnreachable
		"TestMeerkatClient_HungServerFailsAsUnreachable", // meerkatapi: a hung server is bounded by the client timeout
		"TestMeerkatClient_RequestTimeoutIsWired",        // meerkatapi: the 30s deadline the row states
		"TestUnauthorizedToken",                          // meerkatapi: 401 -> ErrUnauthorized
		"TestLogin_LockedAccountIsNotRetried",            // meerkatapi: login 429 -> ErrLocked, not retried
		"TestInvalidJSONAndOversizeBody",                 // meerkatapi: malformed / oversized body -> ErrInvalidData
		"TestRateLimitedAfterRetriesIsUnreachable",       // meerkatapi: 429 honours Retry-After, then fails
		"TestServerErrorsRetryThenFail",                  // meerkatapi: bounded retry budget on 5xx
		"TestGetJSON_StatusMapping",                      // meerkatapi: 404 -> ErrNotFound and the rest of the status table
		"TestPrivateAddressBlockedWhenConfigured",        // meerkatapi: MEERKAT_BLOCK_PRIVATE_URLS guard
		"TestRedirectsAreNotFollowed",                    // meerkatapi: redirects never followed
	},
	"monica": {
		"TestGetUnreachableHost",                        // monica: connection refused -> ErrUnreachable
		"TestMonicaClient_HungServerFailsAsUnreachable", // monica: a hung server is bounded by the client timeout
		"TestMonicaClient_RequestTimeoutIsWired",        // monica: the 30s deadline the row states
		"TestGetUnauthorized",                           // monica: 401 -> ErrUnauthorized
		"TestGetInvalidJSON",                            // monica: malformed body -> ErrInvalidData
		"TestHandleResponseClassifiesStatuses",          // monica: 403/404/429/5xx classification
		"TestGetRetriesOn429",                           // monica: 429 honours Retry-After
		"TestGetGivesUpAfterMaxRetries",                 // monica: bounded retry budget
		"TestBlockPrivateRefusesLoopback",               // monica: MONICA_BLOCK_PRIVATE_URLS guard
		"TestFetchAvatarErrors",                         // monica: avatar failure paths
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

// failureTestDirs are the packages int02Coverage may cite tests from: the
// services suites plus the two import-assistant client packages.
var failureTestDirs = []string{
	filepath.Join("..", "services"),
	filepath.Join("..", "meerkatapi"),
	filepath.Join("..", "monica"),
}

// TestListedFailureTestsExist greps backend/services, backend/meerkatapi and
// backend/monica for every test named in the ledger, so a rename that orphans
// a citation fails here rather than silently weakening the guarantee.
func TestListedFailureTestsExist(t *testing.T) {
	defined := map[string]bool{}
	for _, dir := range failureTestDirs {
		for name := range definedTests(t, dir) {
			defined[name] = true
		}
	}

	seen := map[string]bool{}
	for id, tests := range int02Coverage {
		for _, name := range tests {
			seen[name] = true
			if !defined[name] {
				t.Errorf("int02Coverage[%q] cites %q, which is not defined in backend/services, meerkatapi or monica *_test.go", id, name)
			}
		}
	}
	if len(seen) < 15 {
		t.Errorf("ledger cites only %d distinct tests — suspiciously thin for %d integrations", len(seen), len(Registry()))
	}
}
