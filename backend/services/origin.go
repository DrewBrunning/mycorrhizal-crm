package services

import (
	"net/url"
	"strings"
)

// SameOrigin reports whether two normalized base URLs share scheme, host and
// port (case-insensitive; default ports are equated). An unparseable URL counts
// as a different origin (fail closed).
//
// It backs the credential-exfiltration guard shared by every per-user
// integration that stores a write-only secret next to a base URL (GeoPulse,
// Immich, Paperless, Seafile, Nextcloud, calendar and contact subscriptions):
// an update that leaves the secret empty ("keep the stored one") while moving
// the URL to a different origin is refused, otherwise a hijacked session could
// point the saved secret at an attacker host and press "Test connection".
// A path-only change on the same origin keeps the stored secret.
func SameOrigin(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) &&
		strings.EqualFold(ua.Hostname(), ub.Hostname()) &&
		originEffectivePort(ua) == originEffectivePort(ub)
}

func originEffectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

// isRedirectStatus reports whether an integration client received a 3xx. The
// per-user integration clients never follow redirects (CheckRedirect returns
// http.ErrUseLastResponse), so the 3xx reaches their status switch and is
// mapped to the integration's Err<Name>Redirect sentinel.
func isRedirectStatus(code int) bool {
	return code >= 300 && code < 400
}
