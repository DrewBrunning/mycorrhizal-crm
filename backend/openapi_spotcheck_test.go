package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
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

	db, err := database.InitDB(cfg.DBPath)
	require.NoError(t, err)

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

	// 4. GET /health (public, unversioned, path-level server override).
	healthReq := httptest.NewRequest("GET", "/health", nil)
	healthResp := httptest.NewRecorder()
	router.ServeHTTP(healthResp, healthReq)
	require.Equal(t, 200, healthResp.Code, healthResp.Body.String())
	validateResponse(t, healthResp, "GET", "/health", nil)
}
