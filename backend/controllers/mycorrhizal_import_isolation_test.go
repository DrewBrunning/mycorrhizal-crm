package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/services"

	"github.com/stretchr/testify/require"
)

// isolateMycorrhizalImportSessions gives the calling test a clean
// process-wide account-bundle session manager and guarantees the next one
// gets the same (issue #1860).
//
// mycorrhizalImportSessions is a package-level singleton keyed by user id, and
// every controller test seeds user id 1. Several tests open the per-user cap's
// worth of sessions and rely on the next test starting empty -- under
// `-count=2` (or a shuffle that runs one of them first) a leftover session made
// TestHandlerGuardMatrix's UploadMycorrhizalBundle return 429 instead of its
// expected 400. Every helper that builds a router capable of reaching the
// account-bundle import handlers calls this; tests that build their own router
// must call it too.
func isolateMycorrhizalImportSessions(t testing.TB) {
	t.Helper()
	mycorrhizalImportSessions = services.NewMycorrhizalImportManager()
	t.Cleanup(func() {
		mycorrhizalImportSessions = services.NewMycorrhizalImportManager()
	})
}

// TestIsolateMycorrhizalImportSessions_SecondUseStartsEmpty is the regression
// test for the -count>1 hygiene gap: a test that fills a user's session cap
// must not leave the next test's first upload answered 429.
func TestIsolateMycorrhizalImportSessions_SecondUseStartsEmpty(t *testing.T) {
	fillSessionCap := func(t *testing.T) {
		_, router := setupRouter(t)
		registerMycorrhizalRoutes(router)

		body := testBundleBytes(t)
		for i := 0; i < services.MaxMycorrhizalImportSessionsPerUser; i++ {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "bundle.json", body))
			require.Equal(t, http.StatusOK, w.Code, "session %d should open", i)
		}
	}

	// Two back-to-back subtests, same seeded user id: without isolation the
	// second one's first upload would already be at the cap.
	t.Run("first", fillSessionCap)
	t.Run("second", fillSessionCap)

	require.Zero(t, mycorrhizalImportSessions.CountActive(1), "sessions must not outlive the test that created them")
}
