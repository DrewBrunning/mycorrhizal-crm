package services

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // required by the HIBP k-anonymity protocol's own wire format, not used for anything security-sensitive in this app
	"fmt"
	"mycorrhizal/logger"
	"net/http"
	"strings"
	"time"
)

// hibpAPIBaseURL is a package var (not a const) so tests can point it at an
// httptest.Server instead of the real HIBP host, the same pattern
// webhook_service.go's deliveryClient uses for its own package-level
// http.Client.
var hibpAPIBaseURL = "https://api.pwnedpasswords.com"

// hibpClient is a short-timeout client deliberately separate from
// webhook_service.go's deliveryClient (15s) — this call sits inline in a
// registration/password-change request, so it needs to fail fast rather
// than hold the request open.
var hibpClient = &http.Client{Timeout: 5 * time.Second}

// SetHIBPAPIBaseURLForTest points CheckPasswordBreached at a test server for
// the caller's duration, returning a restore func. Test-only seam for the
// controllers package (register/change-password/reset-confirm tests), which
// can't reach the unexported hibpAPIBaseURL var directly — this package's
// own tests set that var directly instead. Deliberately still exported only
// for tests: the var itself stays unexported so production code can't
// accidentally repoint it.
func SetHIBPAPIBaseURLForTest(url string) (restore func()) {
	original := hibpAPIBaseURL
	hibpAPIBaseURL = url
	return func() { hibpAPIBaseURL = original }
}

// CheckPasswordBreached checks password against Have I Been Pwned's
// k-anonymity range API (issue #376, ASVS 2.1.7): only the first 5 hex
// chars of the password's SHA-1 hash are ever sent, never the password
// itself or the full hash — HIBP's API mandates SHA-1 here as a transport
// format, not a security property of anything else in this app (see
// docs/security/asvs-l2.md's P1 for the actual password-hashing algorithm,
// bcrypt).
//
// Deliberately fails open: any network or API error is logged as a warning
// and returns (false, err) rather than blocking the caller — an outage of a
// third-party service must never become an availability dependency for
// registering or changing a password on a self-hosted app. Callers should
// treat a non-nil err the same as "not breached, HIBP unavailable" and
// proceed; this function has already logged the reason.
func CheckPasswordBreached(ctx context.Context, password string) (breached bool, err error) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec // see import comment
	hexSum := strings.ToUpper(fmt.Sprintf("%x", sum))
	prefix, suffix := hexSum[:5], hexSum[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hibpAPIBaseURL+"/range/"+prefix, nil)
	if err != nil {
		logger.Warn().Err(err).Msg("HIBP: failed to build request, allowing password (fail open)")
		return false, err
	}
	// Identifies this app to the API per HIBP's own request; no user data in it.
	req.Header.Set("User-Agent", "MycorrhizalCRM/1.0 HIBPCheck")
	req.Header.Set("Add-Padding", "true") // response-size padding against traffic analysis of the prefix, per HIBP's docs

	resp, err := hibpClient.Do(req)
	if err != nil {
		logger.Warn().Err(err).Msg("HIBP: request failed, allowing password (fail open)")
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("HIBP: unexpected status %d", resp.StatusCode)
		logger.Warn().Err(err).Msg("HIBP: allowing password (fail open)")
		return false, err
	}

	// Response body is "SUFFIX:COUNT\r\n" lines for every hash sharing the
	// sent prefix — scan for our specific suffix rather than parsing the
	// whole body into memory (some prefixes return thousands of lines).
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		lineSuffix, _, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(lineSuffix), suffix) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Warn().Err(err).Msg("HIBP: failed reading response, allowing password (fail open)")
		return false, err
	}

	return false, nil
}
