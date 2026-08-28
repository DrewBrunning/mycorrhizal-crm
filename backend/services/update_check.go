// Update-availability check (issue #650): when UPDATE_CHECK_ENABLED is on, the
// admin system-status endpoint asks the GitHub releases API whether a newer
// release of this project exists and compares it against
// buildinfo.Get().Version. Off by default and, when on, strictly
// informational — nothing here blocks or errors any surface it feeds.
//
// The outbound call is the operator's decision, so the flag defaults off and
// no request is ever made unless it is set (see docs/security/asvs-l2.md's P6
// and .env.example).
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/httputil"
	"mycorrhizal/logger"
)

// updateCheckAPIBaseURL is the repo whose latest release we compare against.
// A package var (not a const) so tests can point it at an httptest.Server.
var updateCheckAPIBaseURL = "https://api.github.com/repos/DrewBrunning/mycorrhizal-crm"

// updateCheckClient is the SSRF-guarded client used for the outbound call.
// Built via newUpdateCheckClient rather than a literal so tests can swap it
// for one that dials a local stub.
var updateCheckClient = newUpdateCheckClient()

// updateCheckTimeout bounds a single outbound request. The surface this feeds
// is admin-gated and read-only, so it fails fast rather than holding an
// operator's snapshot open. The context deadline applies regardless of which
// client was injected, so tests keep their own (unbounded) transport.
const updateCheckTimeout = 3 * time.Second

// updateCheckCacheTTL bounds how often the outbound lookup is recomputed. A
// package var (not a const) so tests can zero or shrink it. Errors are never
// cached — a failed lookup retries on the next call.
var updateCheckCacheTTL = 6 * time.Hour

// newUpdateCheckClient builds the SSRF-guarded client. Every connection goes
// through httputil.SafeDialContext, which re-resolves the host and pins a
// public address at dial time, so a DNS-rebinding or redirect-to-internal
// attack cannot reach loopback/private networks from the server (ASVS 5.2.6).
func newUpdateCheckClient() *http.Client {
	return &http.Client{
		Timeout: updateCheckTimeout,
		Transport: &http.Transport{
			DialContext: httputil.SafeDialContext(
				errors.New("update check: could not resolve host"),
				errors.New("update check: access to internal IP addresses is not allowed"),
			),
		},
	}
}

// updateCheckCache memoizes the last successful lookup (tag + fetch time),
// in the shape of deepHealthCache: a mutex-guarded package var with a TTL.
var updateCheckCache struct {
	mu  sync.Mutex
	at  time.Time
	tag string
}

// ResetUpdateCheckCache clears the memoized lookup. Test helper.
func ResetUpdateCheckCache() {
	updateCheckCache.mu.Lock()
	updateCheckCache.at = time.Time{}
	updateCheckCache.tag = ""
	updateCheckCache.mu.Unlock()
}

// SetUpdateCheckCacheTTL overrides the cache window (0 disables caching).
// Test seam; returns the previous value so a test can restore it.
func SetUpdateCheckCacheTTL(d time.Duration) time.Duration {
	prev := updateCheckCacheTTL
	updateCheckCacheTTL = d
	return prev
}

// SetUpdateCheckForTest repoints the update check at a test server and swaps
// in a client that can dial it, returning a restore func. Test-only seam for
// the controllers package, which cannot reach the unexported vars directly;
// this package's own tests set the vars directly instead. Deliberately still
// exported only for tests.
func SetUpdateCheckForTest(baseURL string, client *http.Client) (restore func()) {
	origURL := updateCheckAPIBaseURL
	origClient := updateCheckClient
	updateCheckAPIBaseURL = baseURL
	updateCheckClient = client
	return func() {
		updateCheckAPIBaseURL = origURL
		updateCheckClient = origClient
	}
}

// UpdateCheckStatus is the `update` block on the admin system-status
// response (issue #650). `enabled` is the config flag; the rest is only
// populated when a lookup actually succeeded. `update_available` is omitted
// when there is no comparison to report (disabled, or latest unknown), so a
// disabled block marshals as exactly {"enabled": false}.
type UpdateCheckStatus struct {
	Enabled         bool       `json:"enabled"`
	Current         string     `json:"current,omitempty"`
	Latest          string     `json:"latest,omitempty"`
	UpdateAvailable bool       `json:"update_available,omitempty"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
}

// BuildUpdateCheckStatus assembles the update block. When the flag is off it
// returns {enabled: false} and makes NO outbound call. When on, any lookup
// error (network, non-200, unparseable) leaves latest empty and
// update_available false — "unknown" — and never propagates to the caller.
func BuildUpdateCheckStatus(ctx context.Context, cfg config.Config) UpdateCheckStatus {
	if !cfg.UpdateCheckEnabled {
		return UpdateCheckStatus{Enabled: false}
	}

	out := UpdateCheckStatus{Enabled: true, Current: buildinfo.Get().Version}
	tag, err := latestRelease(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("update check: latest release lookup failed")
		return out
	}
	now := time.Now().UTC()
	out.Latest = tag
	out.CheckedAt = &now
	out.UpdateAvailable = isUpdateAvailable(out.Current, tag)
	return out
}

// latestRelease returns the latest published release tag, memoized for
// updateCheckCacheTTL. On any error it returns ("", err) and caches nothing,
// so a later call retries.
func latestRelease(ctx context.Context) (string, error) {
	updateCheckCache.mu.Lock()
	defer updateCheckCache.mu.Unlock()

	if !updateCheckCache.at.IsZero() && time.Since(updateCheckCache.at) < updateCheckCacheTTL {
		return updateCheckCache.tag, nil
	}

	tag, err := fetchLatestRelease(ctx)
	if err != nil {
		return "", err
	}
	updateCheckCache.at = time.Now()
	updateCheckCache.tag = tag
	return tag, nil
}

// fetchLatestRelease performs the actual GET against the GitHub releases API
// and parses tag_name only; everything else in the payload is ignored.
func fetchLatestRelease(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateCheckAPIBaseURL+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MycorrhizalCRM/1.0 UpdateCheck")

	resp, err := updateCheckClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update check: unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.TagName == "" {
		return "", errors.New("update check: empty tag_name in response")
	}
	return payload.TagName, nil
}

// isUpdateAvailable reports whether a newer release exists, using a real
// semver comparison. A non-release running version ("dev", empty, or anything
// that does not parse) is never "behind", and an unparseable latest tag is
// likewise "unknown" rather than available — latest is still reported.
func isUpdateAvailable(current, latest string) bool {
	if !semver.IsValid(current) || !semver.IsValid(latest) {
		return false
	}
	return semver.Compare(latest, current) > 0
}
