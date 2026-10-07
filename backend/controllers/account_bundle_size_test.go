package controllers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/internal/logtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exportBundleOnce(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/export/account", nil))
	return w
}

// TestExportAccountBundle_SizeCap_Boundary (issue #1313) pins both sides of the
// export limit on the real migrated schema: a bundle of exactly the limit is
// served AND imports; one byte over is a structured 507 before any download.
func TestExportAccountBundle_SizeCap_Boundary(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Account bundle exceeds the import size limit; export refused; Request error")
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	for _, n := range []string{"Ada", "Bea", "Cyd"} {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: n, Lastname: "Big"}).Error)
	}
	router.GET("/export/account", ExportAccountBundle)
	registerMycorrhizalRoutes(router)

	probe := exportBundleOnce(t, router)
	require.Equal(t, http.StatusOK, probe.Code, probe.Body.String())
	size := int64(probe.Body.Len())
	assert.Equal(t, strconv.FormatInt(services.MaxMycorrhizalBundleSize, 10), probe.Header().Get("X-Mycorrhizal-Bundle-Max-Bytes"))

	prev := accountBundleMaxBytes
	t.Cleanup(func() { accountBundleMaxBytes = prev })

	t.Run("at the limit exports and imports", func(t *testing.T) {
		accountBundleMaxBytes = size
		w := exportBundleOnce(t, router)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, strconv.FormatInt(size, 10), w.Header().Get("X-Mycorrhizal-Bundle-Max-Bytes"))

		up := httptest.NewRecorder()
		router.ServeHTTP(up, newFileUploadRequest(t, "/import/mycorrhizal/upload", "bundle.json", w.Body.Bytes()))
		assert.Equal(t, http.StatusOK, up.Code, up.Body.String())
	})

	t.Run("one byte over is a structured 507 with the sizes", func(t *testing.T) {
		accountBundleMaxBytes = size - 1
		w := exportBundleOnce(t, router)
		require.Equal(t, http.StatusInsufficientStorage, w.Code)
		assert.Empty(t, w.Header().Get("Content-Disposition"), "no download is offered")
		env := decodeError(t, w)
		assert.Equal(t, "INSUFFICIENT_STORAGE", env.Error.Code)
		assert.Contains(t, env.Error.Message, "over the")
		assert.Contains(t, env.Error.Message, "CSV/vCard")
		assert.Equal(t, "export:account", env.Error.Details["operation"])
		assert.Equal(t, "validation", env.Error.Details["category"])
		assert.EqualValues(t, size, env.Error.Details["bundle_bytes"])
		assert.EqualValues(t, size-1, env.Error.Details["limit_bytes"])
	})
}

// TestMycorrhizalUpload_AtAndOverProductionLimit (issue #1313) uploads a real
// bundle padded to exactly the production limit through the same body-limit
// middleware the route uses. The multipart envelope makes the request larger
// than the file, so this fails if the route cap ever drops back to the bare
// file limit — the exact mismatch that dead-ended a maximal export.
func TestMycorrhizalUpload_AtAndOverProductionLimit(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Mycorrhizal bundle upload rejected")
	_, router := setupRouter(t)
	registerMycorrhizalRoutes(router) // resets the session manager
	router.POST("/limited/upload", middleware.BodySizeLimitMiddleware(services.MycorrhizalUploadBodyLimit), UploadMycorrhizalBundle)

	pad := func(n int64) []byte {
		b := testBundleBytes(t)
		return append(b, bytes.Repeat([]byte(" "), int(n)-len(b))...)
	}

	t.Run("exactly MaxMycorrhizalBundleSize is accepted", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/limited/upload", "b.json", pad(services.MaxMycorrhizalBundleSize)))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("one byte over is refused with the limit named", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/limited/upload", "b.json", pad(services.MaxMycorrhizalBundleSize+1)))
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, errorReason(decodeError(t, w)), "64 MiB")
	})
}
