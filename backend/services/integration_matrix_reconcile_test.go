package services

import (
	"testing"
	"time"

	"mycorrhizal/integrations"
)

// TestIntegrationMatrixTimeoutsMatchCode is issue #464's "How to verify" bullet
// "Each has a documented timeout ... and the code matches it". The INT-01 matrix
// (backend/integrations) declares a per-request timeout per integration; this
// test — living in package services so it can see the unexported timeout
// constants — asserts the declared value is the one the client actually uses.
//
// Every integration now wires an explicit timeout (INT-02, issue #465 closed the
// email-resend / email-smtp / oidc gaps), so wantTimeout is exhaustive.
func TestIntegrationMatrixTimeoutsMatchCode(t *testing.T) {
	wantTimeout := map[string]time.Duration{
		"carddav":      contactSyncRequestTimeout,
		"caldav":       calendarRequestTimeout,
		"immich":       immichRequestTimeout,
		"paperless":    paperlessRequestTimeout,
		"seafile":      seafileRequestTimeout,
		"webdav":       webdavRequestTimeout,
		"webhooks":     deliveryClient.Timeout,
		"ntfy":         deliveryClient.Timeout, // clientFor(cfg) hands back deliveryClient / guardedDeliveryClient
		"gotify":       deliveryClient.Timeout,
		"webpush":      deliveryClient.Timeout,
		"email-resend": resendRequestTimeout,
		"email-smtp":   smtpDialTimeout,
		"oidc":         oidcRequestTimeout,
		"hibp":         hibpClient.Timeout,
		"update-check": updateCheckTimeout,
	}

	// Both guarded and unguarded webhook delivery clients must share the timeout
	// the matrix documents, or the "15s" claim is only half true.
	if guardedDeliveryClient.Timeout != deliveryClient.Timeout {
		t.Errorf("guardedDeliveryClient.Timeout = %s, deliveryClient.Timeout = %s — matrix documents one value",
			guardedDeliveryClient.Timeout, deliveryClient.Timeout)
	}

	for _, in := range integrations.Registry() {
		want, checked := wantTimeout[in.ID]
		if !checked {
			if in.Timeout != 0 {
				t.Errorf("%s: matrix declares Timeout %s but this test has no code binding for it — add one to wantTimeout", in.ID, in.Timeout)
			}
			continue
		}
		if in.Timeout != want {
			t.Errorf("%s: matrix declares Timeout %s, code uses %s — regenerate the matrix or fix the drift", in.ID, in.Timeout, want)
		}
	}

	// Guard against an integration silently dropping out of the registry.
	for id := range wantTimeout {
		if _, ok := integrations.ByID(id); !ok {
			t.Errorf("wantTimeout has %q but integrations.Registry() does not", id)
		}
	}
}
