package services

import (
	"net/http"

	"mycorrhizal/internal/faults"
)

// Integration failure-injection seam names (issue #434). Each names a
// faultingRoundTripper wired onto one integration client's transport, so an
// armed fault replaces every request that client makes — exactly how an
// unreachable / auth-expired / deleted-remotely upstream presents to every
// caller. Documented in docs/development/fault-injection.md; the external-fault
// job can arm the same names against a live process.
const (
	faultPaperlessRequest     = "services.paperless.request"
	faultSeafileRequest       = "services.seafile.request"
	faultWebDAVRequest        = "services.webdav.request"
	faultContactSyncRequest   = "services.contactsync.request"
	faultCalendarSyncRequest  = "services.calendarsync.request"
	faultWebhookDelivery      = "services.webhook.delivery"
	faultNotificationDelivery = "services.notification.delivery"
)

// faultingRoundTripper injects a named failure-injection seam ahead of an
// integration client's real transport. Unarmed it is a single read-lock map
// lookup and delegates unchanged; armed, the fault's error is returned in
// place of the round trip. One seam covers every request method the client
// has, without test-only branches in the request code.
type faultingRoundTripper struct {
	name string
	base http.RoundTripper
}

func (rt faultingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := faults.Hook(rt.name); err != nil {
		return nil, err
	}
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
