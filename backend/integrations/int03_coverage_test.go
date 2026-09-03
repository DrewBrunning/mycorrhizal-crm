package integrations

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// int03Coverage maps each OutboundOperations() entry to the backend/services
// test(s) that exercise its *retry safety* (INT-03, issue #466): that a
// transient failure is retried with bounded backoff, that a permanent status
// is not retried, and — the one that matters most — that a retry after an
// ambiguous failure produces exactly one remote effect, not two.
//
// This is the "a new outbound operation without a classification fails the
// check" guard, the same shape as int02Coverage. A test named here must exist
// in backend/services (TestListedRetryTestsExist greps for it).
var int03Coverage = map[string][]string{
	"webhook-delivery": {
		"TestWebhookDelivery_PermanentStatusIsTerminalNotRetried",   // 401/403/404/400 -> failed_permanently, no next_retry_at, event now
		"TestWebhookDelivery_RateLimitedHonorsRetryAfter",           // 429 + Retry-After -> next_retry_at respects the hint
		"TestWebhookDelivery_TransientStatusBacksOffAndIsBounded",   // 5xx -> exp backoff, terminal at maxDeliveryAttempts
		"TestWebhookDelivery_IdempotencyKeyIsStableAcrossRetries",   // the envelope id rides every retry as Idempotency-Key
		"TestWebhookDelivery_RetryProducesExactlyOneAdditionalPost", // no duplicate side effect
		"TestWebhookDelivery_TerminalAtMaxAttempts",                 // pre-existing: bounded budget
	},
	"caldav-push-event": {
		"TestTwoWay_LocalOnlyChangePushesWithIfMatch", // pre-existing: PUT carries If-Match + remote UID
		"TestTwoWay_PushRetryReusesRemoteUIDNoDuplicate",
	},
	"notification-ntfy": {
		"TestNotificationDelivery_InjectedFaultRecordsFailureAndKeepsReminderDue",
		"TestSendReminders_ChannelFailureIsolation",
	},
	"notification-gotify": {
		"TestNotificationDelivery_InjectedFaultRecordsFailureAndKeepsReminderDue",
		"TestSendReminders_ChannelFailureIsolation",
	},
	"notification-webpush": {
		"TestNotificationDelivery_InjectedFaultOnWebPushKeepsSubscription",
		"TestPushSender_RemovesStaleSubscription", // 404/410 -> subscription dropped, not retried forever
	},
	"email-send-resend": {
		"TestSendViaResend_InjectedFaultSurfaces",
		"TestSendReminders_ChannelFailureIsolation", // a failed email leaves the reminder due, retried next run
	},
	"email-send-smtp": {
		"TestSendEmail_InjectedSendFaultSurfaces",
		"TestSendReminders_ChannelFailureIsolation",
	},
}

// TestEveryOutboundOperationHasRetryCoverage fails if an OutboundOperations()
// entry has no int03Coverage row, or a row names an operation that no longer
// exists.
func TestEveryOutboundOperationHasRetryCoverage(t *testing.T) {
	ops := map[string]bool{}
	for _, op := range OutboundOperations() {
		ops[op.ID] = true
		if len(int03Coverage[op.ID]) == 0 {
			t.Errorf("outbound operation %q has no INT-03 retry-safety test in int03Coverage — "+
				"add a services/*_test.go case and list it here", op.ID)
		}
	}
	for id := range int03Coverage {
		if !ops[id] {
			t.Errorf("int03Coverage names %q, which is not in OutboundOperations()", id)
		}
	}
}

// TestListedRetryTestsExist greps backend/services for every test the ledger
// cites, so a rename that orphans a citation fails here.
func TestListedRetryTestsExist(t *testing.T) {
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

	for id, tests := range int03Coverage {
		for _, name := range tests {
			if !defined[name] {
				t.Errorf("int03Coverage[%q] cites %q, which is not defined in backend/services/*_test.go", id, name)
			}
		}
	}
}
