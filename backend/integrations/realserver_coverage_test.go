package integrations

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// realServerEntry records, for one Registry() integration, how its *success*
// protocol is proven against the real upstream (issue #1490): either the
// real-server tests that do it, or a reasoned exclusion. Exactly one of the
// two. This is the same "a new integration cannot be added without a
// decision" guard as int02Coverage / int03Coverage, aimed at the opposite
// failure: INT-02 proves what we do when the other side fails; this proves we
// speak its protocol when it succeeds, and that the test is not a hand-written
// fake agreeing with our own reading of the API (ADR-0003's shared
// misconception).
type realServerEntry struct {
	// Tests are Test function names. A name resolves in backend/integrations/
	// realserver/*_test.go unless it carries a "services:" prefix, which
	// resolves in backend/services/*_test.go (the pre-existing CardDAV
	// reference-server suite).
	Tests []string
	// Exclusion is why no real-server test exists. Must be a real reason
	// (>= minExclusionLen chars), not a placeholder.
	Exclusion string
}

const minExclusionLen = 60

var realServerCoverage = map[string]realServerEntry{
	"carddav": {Tests: []string{"services:TestCardDAVReferenceServer_RoundTrip"}}, // carddav-e2e.yml: Radicale, Baikal, Nextcloud
	"caldav": {Exclusion: "No real CalDAV server job yet: carddav-e2e.yml's Radicale/Baikal/Nextcloud legs already serve CalDAV but only the CardDAV " +
		"round trip is wired against them. The client's iCalendar parsing is differential-tested against golang-ical in CI; a real calendar-query/PUT " +
		"round trip is the follow-up."},
	"immich":    {Tests: []string{"TestImmich_ConnectionAndPickerFlowAgainstRealServer", "TestImmich_ErrorsMapToSentinels"}},
	"paperless": {Tests: []string{"TestPaperless_PickerFlowAgainstRealServer", "TestPaperless_ErrorsMapToSentinels"}},
	"seafile":   {Tests: []string{"TestSeafile_ConnectionAndBrowseAgainstRealServer", "TestSeafile_ErrorsMapToSentinels"}},
	"webdav":    {Tests: []string{"TestNextcloudWebDAV_BrowseListsRealFilesAndFolders", "TestNextcloudWebDAV_ErrorsMapToSentinels"}},
	"ntfy":      {Tests: []string{"TestNtfy_DeliveredMessageIsReadableFromServer", "TestNtfy_RejectedRequestIsAnError"}},
	"gotify":    {Tests: []string{"TestGotify_DeliveredMessageIsReadableFromServer", "TestGotify_WrongTokenIsRejected"}},
	"oidc": {Tests: []string{
		"TestOIDC_AuthCodePKCELoginAgainstRealIdP",
		"TestOIDC_PKCEVerifierIsEnforcedByRealIdP",
		"TestOIDC_RPInitiatedLogoutEndsTheIdPSession",
		"TestOIDC_SigningKeyRotationIsPickedUp",
	}},
	"email-smtp": {Tests: []string{"TestSMTP_SendEmailIsReceivedByRealServer"}},

	"geocoder": {Exclusion: "The geocoder is a hosted, keyed, rate-limited commercial API (MapTiler-style); there is no self-hostable image that is the same " +
		"service, and CI hammering it would breach its terms and need a secret. The recorded-response fixtures in services tests stand in."},
	"geopulse": {Exclusion: "GeoPulse is a niche self-hosted app with no small official image that boots without a populated location-history database " +
		"and an upstream Immich; a real-server leg would cost a whole bespoke seeding harness for one optional enrichment. The fake stays."},
	"webhooks": {Exclusion: "Webhooks are outbound POSTs to a receiver the *user* runs; there is no upstream server whose protocol we could misread. The " +
		"receiver contract (headers, HMAC signature, Idempotency-Key) is ours and is pinned in webhook_delivery_test.go and docs."},
	"webpush": {Exclusion: "Web Push goes to browser-vendor push services (Mozilla autopush, FCM, APNs) that cannot be self-hosted in CI; the wire format " +
		"(RFC 8030/8291/8292) is implemented by webpush-go and the VAPID/encryption path is exercised by the service tests."},
	"email-resend": {Exclusion: "Resend is a hosted vendor API needing a live account key and sending real mail; it cannot run in a self-contained CI job. " +
		"The SMTP transport, which shares the message builder, has a real-server test."},
	"hibp": {Exclusion: "Have I Been Pwned's range API is a hosted third-party service; the k-anonymity request/response shape is documented and stable, " +
		"the call is opt-in and fail-open, and hitting it from CI nightly adds rate-limit risk for no new signal."},
	"update-check": {Exclusion: "The update check reads GitHub's releases API, a hosted service we cannot stand up; it is opt-in, fail-safe, and a response " +
		"change degrades to 'unknown' (BuildUpdateCheckStatus), never to a wrong 'update available'."},
}

var testFuncDecl = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

func definedTests(t *testing.T, dir string) map[string]bool {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	defined := map[string]bool{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for _, m := range testFuncDecl.FindAllStringSubmatch(string(body), -1) {
			defined[m[1]] = true
		}
	}
	return defined
}

// TestEveryIntegrationHasRealServerCoverageOrExclusion fails when a Registry()
// integration has neither a real-server test nor a reasoned exclusion (or has
// both, or a stub reason) — the #1490 guard.
func TestEveryIntegrationHasRealServerCoverageOrExclusion(t *testing.T) {
	for _, in := range Registry() {
		e, ok := realServerCoverage[in.ID]
		if !ok {
			t.Errorf("integration %q has no realServerCoverage row — add a real-server test in "+
				"backend/integrations/realserver (and its server to .github/workflows/integration-real-servers.yml) "+
				"or a reasoned Exclusion", in.ID)
			continue
		}
		switch {
		case len(e.Tests) > 0 && e.Exclusion != "":
			t.Errorf("realServerCoverage[%q] has both Tests and an Exclusion; pick one", in.ID)
		case len(e.Tests) == 0 && len(strings.TrimSpace(e.Exclusion)) < minExclusionLen:
			t.Errorf("realServerCoverage[%q] has no Tests and an Exclusion shorter than %d chars — "+
				"state the actual reason a real server cannot be used", in.ID, minExclusionLen)
		}
	}
	for id := range realServerCoverage {
		if _, ok := ByID(id); !ok {
			t.Errorf("realServerCoverage names %q, which is not in Registry()", id)
		}
	}
}

// TestRealServerTestsExist resolves every cited test name, so a rename or
// deletion that orphans the citation fails here instead of silently dropping
// the guarantee.
func TestRealServerTestsExist(t *testing.T) {
	rs := definedTests(t, filepath.Join("realserver"))
	svc := definedTests(t, filepath.Join("..", "services"))
	for id, e := range realServerCoverage {
		for _, name := range e.Tests {
			if bare, ok := strings.CutPrefix(name, "services:"); ok {
				if !svc[bare] {
					t.Errorf("realServerCoverage[%q] cites %q, not defined in backend/services/*_test.go", id, bare)
				}
				continue
			}
			if !rs[name] {
				t.Errorf("realServerCoverage[%q] cites %q, not defined in backend/integrations/realserver/*_test.go", id, name)
			}
		}
	}
}

// TestRealServerWorkflowProvisionsEveryServer is the "cannot silently skip in
// CI" structural check: every MYCORRHIZAL_RS_* variable the realserver tests
// read must be set by integration-real-servers.yml, and that workflow must
// export MYCORRHIZAL_REQUIRE_REFERENCES so a missing server fails (citest.
// SkipOrRequire) rather than skipping green.
func TestRealServerWorkflowProvisionsEveryServer(t *testing.T) {
	wf, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "integration-real-servers.yml"))
	if err != nil {
		t.Fatalf("reading workflow: %v", err)
	}
	workflow := string(wf)
	if !strings.Contains(workflow, "MYCORRHIZAL_REQUIRE_REFERENCES") {
		t.Error("integration-real-servers.yml must set MYCORRHIZAL_REQUIRE_REFERENCES so an unprovisioned server fails instead of skipping")
	}

	envRE := regexp.MustCompile(`"(MYCORRHIZAL_RS_[A-Z_]+)"`)
	ents, err := os.ReadDir("realserver")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join("realserver", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range envRE.FindAllStringSubmatch(string(body), -1) {
			seen[m[1]] = true
		}
	}
	if len(seen) == 0 {
		t.Fatal("found no MYCORRHIZAL_RS_* variables in backend/integrations/realserver — the scan is broken")
	}
	for env := range seen {
		if !strings.Contains(workflow, env) {
			t.Errorf("realserver tests read %s but integration-real-servers.yml never sets it — the CI leg would fail/skip", env)
		}
	}
}
