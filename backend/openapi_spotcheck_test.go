package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/routes"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestOpenAPIResponseSpotCheck (T8) boots the real router against a real
// migrated schema, drives a few endpoints end to end, and validates every
// response body against the OpenAPI spec with openapi3filter — proving the
// documented schemas are truthful, not aspirational.
func TestOpenAPIResponseSpotCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := openAPITestConfig()
	cfg.DBPath = filepath.Join(t.TempDir(), "spot.db")

	db := dbtest.NewAt(t, cfg.DBPath)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	routes.RegisterRoutes(router, cfg, db, nil)

	doc := loadOpenAPIDoc(t)

	validateResponse := func(t *testing.T, rr *httptest.ResponseRecorder, method, path string, pathParams map[string]string) {
		t.Helper()
		pi := doc.Paths.Find(path)
		require.NotNil(t, pi, "spec has no path %s", path)
		op := operationForMethod(pi, method)
		require.NotNil(t, op, "spec has no %s %s", method, path)

		rv := &openapi3filter.RequestValidationInput{
			Request:    httptest.NewRequest(method, "http://spot.local"+path, nil),
			Route:      &routers.Route{Spec: doc, Path: path, PathItem: pi, Method: method, Operation: op},
			PathParams: pathParams,
		}
		res := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: rv,
			Status:                 rr.Code,
			Header:                 rr.Header(),
		}
		res.SetBodyBytes(rr.Body.Bytes())
		err := openapi3filter.ValidateResponse(context.Background(), res)
		require.NoError(t, err, "response for %s %s does not match the spec: %s", method, path, rr.Body.String())
	}

	// 1. Register a user (the first user becomes admin), then log in to get
	// the auth_token cookie.
	regBody := `{"username":"spot","email":"spot@example.com","password":"correct-horse-battery-staple-9"}`
	regReq := httptest.NewRequest("POST", "/api/v1/register", bytes.NewBufferString(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regResp := httptest.NewRecorder()
	router.ServeHTTP(regResp, regReq)
	require.Equal(t, 201, regResp.Code, regResp.Body.String())
	validateResponse(t, regResp, "POST", "/register", nil)

	loginBody := `{"identifier":"spot","password":"correct-horse-battery-staple-9"}`
	loginReq := httptest.NewRequest("POST", "/api/v1/login", bytes.NewBufferString(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	require.Equal(t, 200, loginResp.Code, loginResp.Body.String())
	validateResponse(t, loginResp, "POST", "/login", nil)
	cookie := ""
	for _, c := range loginResp.Result().Cookies() {
		if c.Name == "auth_token" {
			cookie = c.String()
		}
	}
	require.NotEmpty(t, cookie, "login must set the auth_token cookie")

	// 2. Create a contact, then fetch it and list contacts.
	createContact := `{"gender":"female","card":{"name":{"components":[{"kind":"given","value":"Ada"}]}}}`
	ccReq := httptest.NewRequest("POST", "/api/v1/contacts", bytes.NewBufferString(createContact))
	ccReq.Header.Set("Content-Type", "application/json")
	ccReq.Header.Set("Cookie", cookie)
	ccResp := httptest.NewRecorder()
	router.ServeHTTP(ccResp, ccReq)
	require.Equal(t, 201, ccResp.Code, ccResp.Body.String())
	validateResponse(t, ccResp, "POST", "/contacts", nil)

	var created struct {
		Contact struct {
			ID  uint   `json:"id"`
			UID string `json:"uid"`
		} `json:"contact"`
	}
	require.NoError(t, json.Unmarshal(ccResp.Body.Bytes(), &created))
	require.NotZero(t, created.Contact.ID)

	id := fmt.Sprintf("%d", created.Contact.ID)
	getReq := httptest.NewRequest("GET", "/api/v1/contacts/"+id, nil)
	getReq.Header.Set("Cookie", cookie)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	require.Equal(t, 200, getResp.Code, getResp.Body.String())
	validateResponse(t, getResp, "GET", "/contacts/{id}", map[string]string{"id": id})

	listReq := httptest.NewRequest("GET", "/api/v1/contacts", nil)
	listReq.Header.Set("Cookie", cookie)
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, listReq)
	require.Equal(t, 200, listResp.Code, listResp.Body.String())
	validateResponse(t, listResp, "GET", "/contacts", nil)

	// 3. Create a relationship edge (the create-vs-update envelope asymmetry
	// the spec calls out), targeting a second real contact.
	targetBody := `{"card":{"name":{"components":[{"kind":"given","value":"Grace"}]}}}`
	targetReq := httptest.NewRequest("POST", "/api/v1/contacts", bytes.NewBufferString(targetBody))
	targetReq.Header.Set("Content-Type", "application/json")
	targetReq.Header.Set("Cookie", cookie)
	targetResp := httptest.NewRecorder()
	router.ServeHTTP(targetResp, targetReq)
	require.Equal(t, 201, targetResp.Code, targetResp.Body.String())

	var target struct {
		Contact struct {
			UID string `json:"uid"`
		} `json:"contact"`
	}
	require.NoError(t, json.Unmarshal(targetResp.Body.Bytes(), &target))

	edgeBody, _ := json.Marshal(map[string]any{
		"source_id": created.Contact.UID,
		"target_id": target.Contact.UID,
		"type":      "friend_of",
	})
	edgeReq := httptest.NewRequest("POST", "/api/v1/relationship-edges", bytes.NewBuffer(edgeBody))
	edgeReq.Header.Set("Content-Type", "application/json")
	edgeReq.Header.Set("Cookie", cookie)
	edgeResp := httptest.NewRecorder()
	router.ServeHTTP(edgeResp, edgeReq)
	require.Equal(t, 201, edgeResp.Code, edgeResp.Body.String())
	validateResponse(t, edgeResp, "POST", "/relationship-edges", nil)

	// 4. Bulk import round-trip (T57's documented contract): a JSON batch of
	// neutral Card/CRM records through /contacts/import/records, then a VCF
	// confirm, then a REPLAY of that confirm — which must still be a 200 with
	// a spec-valid ImportResult (idempotent no-op), not a 404/500.
	// "Grace" is the target contact created in step 3, so row 1's preview
	// carries a non-null duplicate_match (exercising that nullable schema
	// branch) while "Linus" (row 0) exercises the null branch. Both rows are
	// confirmed as "add" so the round-trip is a pure contract check.
	recordsBody := `{"records":[` +
		`{"card":{"name":{"components":[{"kind":"given","value":"Linus"}]}}},` +
		`{"card":{"name":{"components":[{"kind":"given","value":"Grace"}]}}}]}`
	recReq := httptest.NewRequest("POST", "/api/v1/contacts/import/records", bytes.NewBufferString(recordsBody))
	recReq.Header.Set("Content-Type", "application/json")
	recReq.Header.Set("Cookie", cookie)
	recResp := httptest.NewRecorder()
	router.ServeHTTP(recResp, recReq)
	require.Equal(t, 200, recResp.Code, recResp.Body.String())
	validateResponse(t, recResp, "POST", "/contacts/import/records", nil)

	var preview struct {
		SessionID string `json:"session_id"`
	}
	require.NoError(t, json.Unmarshal(recResp.Body.Bytes(), &preview))
	require.NotEmpty(t, preview.SessionID)

	confirmBody, _ := json.Marshal(map[string]any{
		"session_id": preview.SessionID,
		"actions": []map[string]any{
			{"row_index": 0, "action": "add"},
			{"row_index": 1, "action": "add"},
		},
	})
	confirmReq := httptest.NewRequest("POST", "/api/v1/contacts/import/vcf/confirm", bytes.NewBuffer(confirmBody))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmReq.Header.Set("Cookie", cookie)
	confirmResp := httptest.NewRecorder()
	router.ServeHTTP(confirmResp, confirmReq)
	require.Equal(t, 200, confirmResp.Code, confirmResp.Body.String())
	validateResponse(t, confirmResp, "POST", "/contacts/import/vcf/confirm", nil)

	// Replay the same confirm: the session is consumed but the result must be
	// returned verbatim (idempotent), proving the documented contract holds.
	replayReq := httptest.NewRequest("POST", "/api/v1/contacts/import/vcf/confirm", bytes.NewBuffer(confirmBody))
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set("Cookie", cookie)
	replayResp := httptest.NewRecorder()
	router.ServeHTTP(replayResp, replayReq)
	require.Equal(t, 200, replayResp.Code, "a retried confirm of a consumed session must replay, not 404: %s", replayResp.Body.String())
	validateResponse(t, replayResp, "POST", "/contacts/import/vcf/confirm", nil)
	require.JSONEq(t, confirmResp.Body.String(), replayResp.Body.String(), "the replayed result must be byte-identical to the original")

	// 5. The health surface (public, unversioned, path-level server override).
	// Deep /health can be 200 healthy or degraded depending on scheduled-job
	// state; both are spec-valid HealthResponse bodies.
	for _, hp := range []string{"/health", "/health/live", "/health/ready"} {
		hr := httptest.NewRecorder()
		router.ServeHTTP(hr, httptest.NewRequest("GET", hp, nil))
		require.Contains(t, []int{200, 503}, hr.Code, "%s -> %s", hp, hr.Body.String())
		validateResponse(t, hr, "GET", hp, nil)
	}

	// 6. The admin diagnostics sweep (issue #423). The first registered user
	// is an admin, so the session cookie admits the admin-gated route; the
	// response body — a mixture of ok/warning/error checks, since the test
	// config's storage dirs are not provisioned — must validate against the
	// documented DiagnosticsResponse schema.
	diagReq := httptest.NewRequest("GET", "/api/v1/admin/diagnostics", nil)
	diagReq.Header.Set("Cookie", cookie)
	diagResp := httptest.NewRecorder()
	router.ServeHTTP(diagResp, diagReq)
	require.Equal(t, 200, diagResp.Code, diagResp.Body.String())
	validateResponse(t, diagResp, "GET", "/admin/diagnostics", nil)
}
