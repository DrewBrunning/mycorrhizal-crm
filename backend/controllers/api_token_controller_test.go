package controllers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListApiTokens_Empty(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest("GET", "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Tokens []models.ApiTokenResponse `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Empty(t, body.Tokens)
}

func TestListApiTokens_ReturnsScopedTokens(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	// Token for a second user — must NOT appear in the response
	other := models.User{Username: "other", Email: "other@example.com", Password: "x"}
	db.Create(&other)

	db.Create(&models.ApiToken{UserID: user.ID, Name: "my-token", TokenHash: "hash1"})
	db.Create(&models.ApiToken{UserID: other.ID, Name: "other-token", TokenHash: "hash2"})

	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest("GET", "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Tokens []models.ApiTokenResponse `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Tokens, 1)
	assert.Equal(t, "my-token", body.Tokens[0].Name)
}

func TestListApiTokens_OrderedByCreatedAtDesc(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	db.Create(&models.ApiToken{UserID: user.ID, Name: "first", TokenHash: "hash-first"})
	db.Create(&models.ApiToken{UserID: user.ID, Name: "second", TokenHash: "hash-second"})

	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest("GET", "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Tokens []models.ApiTokenResponse `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Tokens, 2)
	assert.Equal(t, "second", body.Tokens[0].Name)
	assert.Equal(t, "first", body.Tokens[1].Name)
}

func TestListApiTokens_RevokedTokensAreIncluded(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	now := time.Now()
	db.Create(&models.ApiToken{UserID: user.ID, Name: "active", TokenHash: "h1"})
	db.Create(&models.ApiToken{UserID: user.ID, Name: "revoked", TokenHash: "h2", RevokedAt: &now})

	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest("GET", "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Tokens []models.ApiTokenResponse `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Tokens, 2)

	var revokedFound bool
	for _, tok := range body.Tokens {
		if tok.Name == "revoked" {
			assert.NotNil(t, tok.RevokedAt)
			revokedFound = true
		}
	}
	assert.True(t, revokedFound)
}

func TestCreateApiToken_Success(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.POST("/api-tokens", withValidated(func() any { return &models.ApiTokenInput{} }), CreateApiToken)

	input := models.ApiTokenInput{Name: "ci-token"}
	body, _ := json.Marshal(input)

	req, _ := http.NewRequest("POST", "/api-tokens", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp models.ApiTokenCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ci-token", resp.Name)
	assert.NotZero(t, resp.ID)
	assert.Contains(t, resp.Token, "mycorrhizal_")
	assert.Nil(t, resp.LastUsedAt)
	assert.Nil(t, resp.RevokedAt)

	// Issue #413: a new token always gets a bounded lifetime -- NULL is only
	// for legacy rows predating the column, never for one just created.
	require.NotNil(t, resp.ExpiresAt)
	assert.WithinDuration(t,
		time.Now().Add(time.Duration(models.DefaultApiTokenExpiryDays)*24*time.Hour),
		*resp.ExpiresAt, 5*time.Second)

	// Plaintext is never stored; only the SHA-256 hash is persisted
	var stored models.ApiToken
	require.NoError(t, db.First(&stored, resp.ID).Error)
	expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(resp.Token)))
	assert.Equal(t, expectedHash, stored.TokenHash)
	assert.NotContains(t, stored.TokenHash, "mycorrhizal_")
}

func TestCreateApiToken_DefaultScopeIsFull(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.POST("/api-tokens", withValidated(func() any { return &models.ApiTokenInput{} }), CreateApiToken)

	input := models.ApiTokenInput{Name: "no-scope-given"}
	body, _ := json.Marshal(input)

	req, _ := http.NewRequest("POST", "/api-tokens", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp models.ApiTokenCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "full", resp.Scope)

	var stored models.ApiToken
	require.NoError(t, db.First(&stored, resp.ID).Error)
	assert.Equal(t, "full", stored.Scope)
}

func TestCreateApiToken_CardDAVScope(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.POST("/api-tokens", withValidated(func() any { return &models.ApiTokenInput{} }), CreateApiToken)

	input := models.ApiTokenInput{Name: "sync-device", Scope: "carddav"}
	body, _ := json.Marshal(input)

	req, _ := http.NewRequest("POST", "/api-tokens", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp models.ApiTokenCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "carddav", resp.Scope)

	var stored models.ApiToken
	require.NoError(t, db.First(&stored, resp.ID).Error)
	assert.Equal(t, "carddav", stored.Scope)
}

func TestCreateApiToken_InvalidScopeRejected(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)

	body, _ := json.Marshal(map[string]string{"name": "bad-scope", "scope": "admin"})
	req, _ := http.NewRequest("POST", "/api-tokens", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateApiToken_MissingName(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)

	body, _ := json.Marshal(map[string]string{"name": ""})
	req, _ := http.NewRequest("POST", "/api-tokens", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRevokeApiToken_Success(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	token := models.ApiToken{UserID: user.ID, Name: "to-revoke", TokenHash: "somehash"}
	db.Create(&token)

	router.DELETE("/api-tokens/:id", RevokeApiToken)

	req, _ := http.NewRequest("DELETE", "/api-tokens/"+strconv.Itoa(int(token.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "Token revoked successfully", resp["message"])

	var updated models.ApiToken
	db.First(&updated, token.ID)
	assert.NotNil(t, updated.RevokedAt)
	assert.WithinDuration(t, time.Now(), *updated.RevokedAt, 5*time.Second)
}

func TestRevokeApiToken_NotFound(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.DELETE("/api-tokens/:id", RevokeApiToken)

	req, _ := http.NewRequest("DELETE", "/api-tokens/9999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRevokeApiToken_WrongUser(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	// setupRouter seeds a user and sets its ID in the context.
	// Create a second user and put their token in the DB.
	other := models.User{Username: "attacker", Email: "attacker@example.com", Password: "x"}
	db.Create(&other)

	token := models.ApiToken{UserID: other.ID, Name: "victim-token", TokenHash: "victimhash"}
	db.Create(&token)

	router.DELETE("/api-tokens/:id", RevokeApiToken)

	req, _ := http.NewRequest("DELETE", "/api-tokens/"+strconv.Itoa(int(token.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Must not find a token belonging to a different user
	assert.Equal(t, http.StatusNotFound, w.Code)

	var unchanged models.ApiToken
	db.First(&unchanged, token.ID)
	assert.Nil(t, unchanged.RevokedAt)
}

func TestRevokeApiToken_InvalidID(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.DELETE("/api-tokens/:id", RevokeApiToken)

	req, _ := http.NewRequest("DELETE", "/api-tokens/not-a-number", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- RevokeAllApiTokens (issue #413) ---

func TestRevokeAllApiTokens_OnlyAffectsCallersOwnTokens(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	other := models.User{Username: "other-revokeall", Email: "other-revokeall@example.com", Password: "x"}
	db.Create(&other)

	mine1 := models.ApiToken{UserID: user.ID, Name: "mine1", TokenHash: "h1"}
	mine2 := models.ApiToken{UserID: user.ID, Name: "mine2", TokenHash: "h2"}
	theirs := models.ApiToken{UserID: other.ID, Name: "theirs", TokenHash: "h3"}
	require.NoError(t, db.Create(&mine1).Error)
	require.NoError(t, db.Create(&mine2).Error)
	require.NoError(t, db.Create(&theirs).Error)

	router.POST("/api-tokens/revoke-all", RevokeAllApiTokens)

	req, _ := http.NewRequest("POST", "/api-tokens/revoke-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Revoked int64 `json:"revoked"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 2, body.Revoked)

	var reloadedMine1, reloadedMine2, reloadedTheirs models.ApiToken
	db.First(&reloadedMine1, mine1.ID)
	db.First(&reloadedMine2, mine2.ID)
	db.First(&reloadedTheirs, theirs.ID)
	assert.NotNil(t, reloadedMine1.RevokedAt)
	assert.NotNil(t, reloadedMine2.RevokedAt)
	assert.Nil(t, reloadedTheirs.RevokedAt, "another user's token must be untouched")
}

func TestRevokeAllApiTokens_AlreadyRevokedTokensUnaffectedAndUncounted(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	priorRevoke := time.Now().Add(-24 * time.Hour)
	alreadyRevoked := models.ApiToken{UserID: user.ID, Name: "already", TokenHash: "h1", RevokedAt: &priorRevoke}
	active := models.ApiToken{UserID: user.ID, Name: "active", TokenHash: "h2"}
	require.NoError(t, db.Create(&alreadyRevoked).Error)
	require.NoError(t, db.Create(&active).Error)

	router.POST("/api-tokens/revoke-all", RevokeAllApiTokens)

	req, _ := http.NewRequest("POST", "/api-tokens/revoke-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Revoked int64 `json:"revoked"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 1, body.Revoked, "the already-revoked token must not be recounted")

	var reloaded models.ApiToken
	db.First(&reloaded, alreadyRevoked.ID)
	assert.WithinDuration(t, priorRevoke, *reloaded.RevokedAt, time.Second, "revoked_at must not be overwritten")
}

// --- RotateApiToken (issue #413) ---

func TestRotateApiToken_Success(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	old := models.ApiToken{UserID: user.ID, Name: "sync-device", TokenHash: "oldhash", Scope: "carddav"}
	require.NoError(t, db.Create(&old).Error)

	router.POST("/api-tokens/:id/rotate", RotateApiToken)

	req, _ := http.NewRequest("POST", "/api-tokens/"+strconv.Itoa(int(old.ID))+"/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp models.ApiTokenCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEqual(t, old.ID, resp.ID, "rotate must mint a new row, not reuse the old id")
	assert.Equal(t, "sync-device", resp.Name, "name carries over")
	assert.Equal(t, "carddav", resp.Scope, "scope carries over")
	assert.Contains(t, resp.Token, "mycorrhizal_")
	require.NotNil(t, resp.ExpiresAt)

	var reloadedOld models.ApiToken
	db.First(&reloadedOld, old.ID)
	assert.NotNil(t, reloadedOld.RevokedAt, "the old token must be revoked")

	var newRow models.ApiToken
	require.NoError(t, db.First(&newRow, resp.ID).Error)
	expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(resp.Token)))
	assert.Equal(t, expectedHash, newRow.TokenHash)
}

func TestRotateApiToken_WrongUser(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	other := models.User{Username: "attacker-rotate", Email: "attacker-rotate@example.com", Password: "x"}
	db.Create(&other)

	token := models.ApiToken{UserID: other.ID, Name: "victim-token", TokenHash: "victimhash"}
	db.Create(&token)

	router.POST("/api-tokens/:id/rotate", RotateApiToken)

	req, _ := http.NewRequest("POST", "/api-tokens/"+strconv.Itoa(int(token.ID))+"/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var unchanged models.ApiToken
	db.First(&unchanged, token.ID)
	assert.Nil(t, unchanged.RevokedAt, "a rotate attempt on someone else's token must not revoke it")
}

func TestRotateApiToken_AlreadyRevoked(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	var user models.User
	db.First(&user)

	revokedAt := time.Now()
	token := models.ApiToken{UserID: user.ID, Name: "dead", TokenHash: "deadhash", RevokedAt: &revokedAt}
	require.NoError(t, db.Create(&token).Error)

	router.POST("/api-tokens/:id/rotate", RotateApiToken)

	req, _ := http.NewRequest("POST", "/api-tokens/"+strconv.Itoa(int(token.ID))+"/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRotateApiToken_InvalidID(t *testing.T) {
	db, router := setupRouter()
	db.AutoMigrate(&models.ApiToken{})

	router.POST("/api-tokens/:id/rotate", RotateApiToken)

	req, _ := http.NewRequest("POST", "/api-tokens/not-a-number/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
