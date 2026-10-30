package controllers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2104 (ADR 0038): the integration controllers (Immich, Seafile,
// Paperless, GeoPulse, Nextcloud/WebDAV, calendar and contact subscriptions,
// notifications, feeds, webhooks) carry a reason_code in the raw JSON so
// clients can localize the message. These tests read the wire body, not the
// Go value, so a dropped .WithReason fails them. wireReason/requireReason live
// in sign_in_reason_codes_test.go.

// abortOn runs fn inside a gin handler and returns the recorded response.
func abortOn(fn func(c *gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/", fn)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	router.ServeHTTP(w, req)
	return w
}

type serviceErrCase struct {
	name   string
	err    error
	status int
	code   string
}

func runServiceErrCases(t *testing.T, abort func(c *gin.Context, err error), cases []serviceErrCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := abortOn(func(c *gin.Context) { abort(c, tc.err) })
			requireReason(t, w, tc.status, tc.code)
		})
	}
}

func TestIntegrationReasonCodes_ServiceErrorMappers(t *testing.T) {
	t.Run("immich", func(t *testing.T) {
		runServiceErrCases(t, abortImmichServiceError, []serviceErrCase{
			{"unauthorized", services.ErrImmichUnauthorized, 400, "immich_api_key_invalid"},
			{"invalid url", services.ErrImmichInvalidURL, 400, "immich_base_url_invalid"},
			{"redirect", services.ErrImmichRedirect, 503, "immich_redirect"},
			{"request failed", &services.ImmichRequestError{Status: "500 Internal Server Error"}, 503, "immich_upstream_error"},
			{"invalid data", services.ErrImmichInvalidData, 503, "immich_response_unparseable"},
			{"unreachable", errors.New("dial tcp: refused"), 503, "immich_unreachable"},
		})
	})
	t.Run("seafile", func(t *testing.T) {
		runServiceErrCases(t, abortSeafileServiceError, []serviceErrCase{
			{"unauthorized", services.ErrSeafileUnauthorized, 400, "seafile_api_token_invalid"},
			{"invalid url", services.ErrSeafileInvalidURL, 400, "seafile_base_url_invalid"},
			{"redirect", services.ErrSeafileRedirect, 503, "seafile_redirect"},
			{"request failed", &services.SeafileRequestError{Status: "500 Internal Server Error"}, 503, "seafile_upstream_error"},
			{"invalid data", services.ErrSeafileInvalidData, 503, "seafile_response_unparseable"},
			{"unreachable", errors.New("dial tcp: refused"), 503, "seafile_unreachable"},
		})
	})
	t.Run("paperless", func(t *testing.T) {
		runServiceErrCases(t, abortPaperlessServiceError, []serviceErrCase{
			{"unauthorized", services.ErrPaperlessUnauthorized, 400, "paperless_api_token_invalid"},
			{"invalid url", services.ErrPaperlessInvalidURL, 400, "paperless_base_url_invalid"},
			{"redirect", services.ErrPaperlessRedirect, 503, "paperless_redirect"},
			{"request failed", &services.PaperlessRequestError{Status: "500 Internal Server Error"}, 503, "paperless_upstream_error"},
			{"invalid data", services.ErrPaperlessInvalidData, 503, "paperless_response_unparseable"},
			{"unreachable", errors.New("dial tcp: refused"), 503, "paperless_unreachable"},
		})
	})
	t.Run("geopulse", func(t *testing.T) {
		runServiceErrCases(t, abortGeoPulseServiceError, []serviceErrCase{
			{"unauthorized", services.ErrGeoPulseUnauthorized, 400, "geopulse_api_token_invalid"},
			{"undecryptable", services.ErrGeoPulseKeyUndecryptable, 400, "geopulse_token_undecryptable"},
			{"invalid url", services.ErrGeoPulseInvalidURL, 400, "geopulse_base_url_invalid"},
			{"redirect", services.ErrGeoPulseRedirect, 503, "geopulse_redirect"},
			{"invalid date", services.ErrGeoPulseInvalidDate, 400, "geopulse_date_invalid"},
			{"request failed", &services.GeoPulseRequestError{Status: "500 Internal Server Error"}, 503, "geopulse_upstream_error"},
			{"invalid data", services.ErrGeoPulseInvalidData, 503, "geopulse_response_unparseable"},
			{"unreachable", errors.New("dial tcp: refused"), 503, "geopulse_unreachable"},
		})
	})
	t.Run("webdav", func(t *testing.T) {
		runServiceErrCases(t, abortWebDAVServiceError, []serviceErrCase{
			{"unauthorized", services.ErrWebDAVUnauthorized, 400, "webdav_app_password_invalid"},
			{"invalid url", services.ErrWebDAVInvalidURL, 400, "webdav_base_url_invalid"},
			{"redirect", services.ErrWebDAVRedirect, 503, "webdav_redirect"},
			{"request failed", &services.WebDAVRequestError{Status: "500 Internal Server Error"}, 503, "webdav_upstream_error"},
			{"invalid data", services.ErrWebDAVInvalidData, 503, "webdav_response_unparseable"},
			{"unreachable", errors.New("dial tcp: refused"), 503, "webdav_unreachable"},
		})
	})
}

func TestIntegrationReasonCodes_UpstreamStatusParam(t *testing.T) {
	w := abortOn(func(c *gin.Context) {
		abortImmichServiceError(c, &services.ImmichRequestError{Status: "502 Bad Gateway"})
	})
	code, params := wireReason(t, w)
	assert.Equal(t, "immich_upstream_error", code)
	assert.Equal(t, map[string]any{"status": "502 Bad Gateway"}, params)
	assert.Contains(t, w.Body.String(), "Immich returned an error (502 Bad Gateway)")
}

func TestIntegrationReasonCodes_ImmichSVGProxy(t *testing.T) {
	w := abortOn(func(c *gin.Context) { writeImmichImage(c, []byte("<svg/>"), "image/svg+xml") })
	requireReason(t, w, http.StatusBadRequest, "immich_svg_unsupported")
}

func TestIntegrationReasonCodes_CalendarSyncErrors(t *testing.T) {
	for _, tc := range []serviceErrCase{
		{"invalid url", services.ErrCalendarInvalidURL, 400, "calendar_url_invalid"},
		{"unauthorized", services.ErrCalendarUnauthorized, 503, "calendar_auth_failed"},
		{"not found", services.ErrCalendarNotFound, 503, "calendar_not_found_at_url"},
		{"private", services.ErrCalendarPrivateAddress, 503, "calendar_private_address"},
		{"too large", services.ErrCalendarTooLarge, 503, "calendar_response_too_large"},
		{"invalid data", services.ErrCalendarInvalidData, 503, "calendar_data_unparseable"},
		{"unreachable", services.ErrCalendarUnreachable, 400, "calendar_unreachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := abortOn(func(c *gin.Context) { apperrors.AbortWithError(c, calendarSyncError(tc.err)) })
			requireReason(t, w, tc.status, tc.code)
		})
	}
}

func TestIntegrationReasonCodes_ContactSyncErrors(t *testing.T) {
	for _, tc := range []serviceErrCase{
		{"invalid url", services.ErrContactSyncInvalidURL, 400, "contact_subscription_url_invalid"},
		{"unauthorized", services.ErrContactSyncUnauthorized, 503, "contact_subscription_auth_failed"},
		{"not found", services.ErrContactSyncNotFound, 503, "contact_subscription_not_found_at_url"},
		{"private", services.ErrContactSyncPrivateAddress, 503, "contact_subscription_private_address"},
		{"too large", services.ErrContactSyncTooLarge, 503, "contact_subscription_response_too_large"},
		{"invalid data", services.ErrContactSyncInvalidData, 503, "contact_subscription_data_unparseable"},
		{"unreachable", services.ErrContactSyncUnreachable, 400, "contact_subscription_unreachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := abortOn(func(c *gin.Context) { apperrors.AbortWithError(c, contactSyncError(tc.err)) })
			requireReason(t, w, tc.status, tc.code)
		})
	}
}

func TestIntegrationReasonCodes_FirstConnectNeedsCredential(t *testing.T) {
	t.Run("immich", func(t *testing.T) {
		router := immichTestRouter(t, seedImmichControllerDB(t))
		w := immichDoJSON(t, router, "PUT", "/immich/config", models.ImmichConfigInput{BaseURL: "https://immich.example"})
		requireReason(t, w, http.StatusBadRequest, "immich_api_key_required")
	})
	t.Run("seafile", func(t *testing.T) {
		router := seafileTestRouter(t, seedSeafileControllerDB(t))
		w := seafileDoJSON(t, router, "PUT", "/seafile/config", models.SeafileConfigInput{BaseURL: "https://seafile.example"})
		requireReason(t, w, http.StatusBadRequest, "seafile_api_token_required")
	})
	t.Run("paperless", func(t *testing.T) {
		router := paperlessTestRouter(t, seedPaperlessControllerDB(t))
		w := paperlessDoJSON(t, router, "PUT", "/paperless/config", models.PaperlessConfigInput{BaseURL: "https://paperless.example"})
		requireReason(t, w, http.StatusBadRequest, "paperless_api_token_required")
	})
	t.Run("webdav", func(t *testing.T) {
		router := webdavTestRouter(t, seedWebDAVControllerDB(t))
		w := webdavDoJSON(t, router, "PUT", "/nextcloud/config", models.WebDAVConfigInput{BaseURL: "https://nc.example"})
		requireReason(t, w, http.StatusBadRequest, "webdav_app_password_required")
	})
	t.Run("geopulse", func(t *testing.T) {
		db, user := seedGeoPulseControllerDB(t)
		router := geopulseTestRouter(t, db, user.ID, config.Config{})
		w := geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://geopulse.example"})
		requireReason(t, w, http.StatusBadRequest, "geopulse_api_token_required")
	})
}

func TestIntegrationReasonCodes_LinkRequiresIdentifier(t *testing.T) {
	blank := "   "
	t.Run("immich", func(t *testing.T) {
		db := seedImmichControllerDB(t)
		router := immichTestRouter(t, db)
		uid := firstContactUID(t, db)
		w := immichDoJSON(t, router, "POST", "/immich/contacts/"+uid+"/link", map[string]any{"person_id": blank})
		requireReason(t, w, http.StatusBadRequest, "immich_person_id_required")
		// An empty body fails struct validation first.
		w = immichDoJSON(t, router, "POST", "/immich/contacts/"+uid+"/link", map[string]any{})
		requireReason(t, w, http.StatusBadRequest, "integration_request_invalid")
	})
	t.Run("seafile", func(t *testing.T) {
		db := seedSeafileControllerDB(t)
		router := seafileTestRouter(t, db)
		uid := firstContactUID(t, db)
		w := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+uid+"/link", map[string]any{"repo_id": blank, "path": "/a", "name": "a", "type": "file"})
		requireReason(t, w, http.StatusBadRequest, "seafile_repo_and_path_required")
		w = seafileDoJSON(t, router, "POST", "/seafile/contacts/"+uid+"/link", map[string]any{})
		requireReason(t, w, http.StatusBadRequest, "integration_request_invalid")
	})
	t.Run("paperless", func(t *testing.T) {
		db := seedPaperlessControllerDB(t)
		router := paperlessTestRouter(t, db)
		uid := firstContactUID(t, db)
		w := paperlessDoJSON(t, router, "POST", "/paperless/contacts/"+uid+"/link", map[string]any{"document_id": blank})
		requireReason(t, w, http.StatusBadRequest, "paperless_document_id_required")
		w = paperlessDoJSON(t, router, "POST", "/paperless/contacts/"+uid+"/link", map[string]any{})
		requireReason(t, w, http.StatusBadRequest, "integration_request_invalid")
	})
	t.Run("webdav", func(t *testing.T) {
		db := seedWebDAVControllerDB(t)
		router := webdavTestRouter(t, db)
		uid := firstContactUID(t, db)
		w := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+uid+"/link", map[string]any{"path": blank, "name": "a", "type": "file"})
		requireReason(t, w, http.StatusBadRequest, "webdav_path_required")
		w = webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+uid+"/link", map[string]any{})
		requireReason(t, w, http.StatusBadRequest, "integration_request_invalid")
	})
}

func TestIntegrationReasonCodes_GeoPulseSuggestionsDate(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})
	connectGeoPulseController(t, router, "https://geopulse.example")
	w := geopulseDo(t, router, "GET", "/geopulse/suggestions", nil)
	requireReason(t, w, http.StatusBadRequest, "geopulse_date_required")
}

func TestIntegrationReasonCodes_MessageStaysEnglish(t *testing.T) {
	w := abortOn(func(c *gin.Context) { abortSeafileServiceError(c, services.ErrSeafileInvalidURL) })
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Seafile base URL is invalid")
}

func TestIntegrationReasonCodes_NotificationURLInvalid(t *testing.T) {
	_, router := setupRouter(t)
	router.PUT("/notifications/config", withValidated(func() any { return &models.NotificationConfigInput{} }), SaveNotificationConfig)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, rawRequest("PUT", "/notifications/config", `{"ntfy_url":"ftp://ntfy.example.com","ntfy_topic":"alerts"}`, ""))
	requireReason(t, w, http.StatusBadRequest, "notification_url_invalid")
}

func firstContactUID(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)
	return contact.VCardUID
}
