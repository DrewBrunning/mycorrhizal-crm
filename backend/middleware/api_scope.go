package middleware

import (
	"net/http"
	"strings"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
)

const apiTokenContextKey = "apiToken"

// RequireAPIScopeForMethod enforces the REST half of a token's scope set:
// safe methods need api:read, mutations need api:write. Non-token callers
// (cookie sessions) pass through untouched.
func RequireAPIScopeForMethod() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := APITokenFromContext(c)
		if !ok {
			c.Next()
			return
		}

		required := models.TokenScopeAPIWrite
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			required = models.TokenScopeAPIRead
		}
		requireTokenScope(c, token, required)
	}
}

// RequireAPIScope requires one named scope. It is used for the MCP surface,
// whose POST requests are read-only but are not part of the REST scope pair.
func RequireAPIScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := APITokenFromContext(c)
		if !ok {
			c.Next()
			return
		}
		requireTokenScope(c, token, scope)
	}
}

// credentialManagementExactPaths and credentialManagementPathPrefixes are the
// single production classification of routes that mint or manage credentials.
// A new route in this class must be added here; the authorization matrix reads
// the same predicate so there is no second hand-maintained list.
var (
	credentialManagementExactPaths = map[string]bool{
		"/api/v1/account":                   true,
		"/api/v1/users/change-password":     true,
		"/api/v1/users/dav-password-policy": true,
		"/api/v1/feeds":                     true,
	}
	credentialManagementPathPrefixes = []string{
		"/api/v1/users/2fa/",
		"/api/v1/api-tokens",
		"/api/v1/sessions",
		"/api/v1/webauthn/register",
		"/api/v1/webauthn/assert",
		"/api/v1/webauthn/credentials",
		"/api/v1/auth/device/grants",
		"/api/v1/feeds/",
	}
)

// IsCredentialManagementRoute reports whether a matched route pattern belongs
// to the credential/identity-management surface (issue #2010).
func IsCredentialManagementRoute(fullPath string) bool {
	if fullPath == "/api/v1/feeds/atom" {
		return false
	}
	if credentialManagementExactPaths[fullPath] {
		return true
	}
	for _, prefix := range credentialManagementPathPrefixes {
		if strings.HasPrefix(fullPath, prefix) {
			return true
		}
	}
	return false
}

// RequireInteractiveSessionForCredentials rejects API-token callers on every
// credential/identity-management route. A leaked token must not be able to
// enroll a second factor, create a device grant, or mint a replacement token.
func RequireInteractiveSessionForCredentials() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isAPIToken, _ := c.Get("isAPIToken"); isAPIToken == true && IsCredentialManagementRoute(c.FullPath()) {
			apperrors.AbortWithError(c, apperrors.ErrForbidden(
				"This endpoint requires an interactive session; API tokens cannot manage credentials"))
			return
		}
		c.Next()
	}
}

// APITokenFromContext returns the validated token AuthMiddleware attached to
// the request, if any.
func APITokenFromContext(c *gin.Context) (*models.ApiToken, bool) {
	value, exists := c.Get(apiTokenContextKey)
	if !exists {
		return nil, false
	}
	token, ok := value.(*models.ApiToken)
	return token, ok
}

func requireTokenScope(c *gin.Context, token *models.ApiToken, scope string) {
	if token.HasScope(scope) {
		c.Next()
		return
	}
	apperrors.AbortWithError(c, apperrors.ErrForbidden(
		"This token does not have the "+scope+" scope required for this request"))
}
