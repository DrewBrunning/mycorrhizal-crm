package carddav

import (
	"mycorrhizal/internal/clock"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// BasicAuthMiddleware provides HTTP Basic Authentication for CardDAV
// It supports both username and email as the login identifier
// Includes account-based rate limiting to prevent brute force attacks
func BasicAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		username, password, ok := c.Request.BasicAuth()
		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="CardDAV"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// Normalize identifier for consistent rate limiting
		identifier := strings.ToLower(username)
		clientIP := c.ClientIP()

		// Check login rate limiting before attempting authentication. Issue
		// #867: keyed on (identifier, client IP), so a failed run only denies
		// the source that caused it — it shares the AccountRateLimiter with the
		// interactive-login path.
		accountLimiter := middleware.GetAccountRateLimiter()
		if isLocked, remainingSecs := accountLimiter.IsLoginLocked(identifier, clientIP); isLocked {
			logger.Warn().
				Str("identifier", identifier).
				Str("ip", c.ClientIP()).
				Int("retry_after", remainingSecs).
				Msg("CardDAV auth blocked: account temporarily locked")
			c.Header("WWW-Authenticate", `Basic realm="CardDAV"`)
			c.Header("Retry-After", strconv.Itoa(remainingSecs))
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}

		db := c.MustGet("db").(*gorm.DB)
		var user models.User

		// Try to find user by username or email
		err := db.Where("username = ? OR email = ?", identifier, identifier).First(&user).Error
		if err != nil {
			// Burn the same bcrypt cost as a real comparison to not reveal if an account exists (issue #862)
			services.SpendDummyPasswordHash(password)
			// Record failed attempt even for non-existent users to prevent enumeration
			accountLimiter.RecordLoginFailure(identifier, clientIP)
			logger.Warn().
				Str("identifier", identifier).
				Str("ip", c.ClientIP()).
				Msg("CardDAV auth failed: user not found")
			c.Header("WWW-Authenticate", `Basic realm="CardDAV"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		surface := "CardDAV"
		if strings.HasPrefix(c.Request.URL.Path, "/caldav") {
			surface = "CalDAV"
		}

		// Validate password, falling back to an API token/app password if the
		// account password doesn't match. The account password itself is
		// allowed only when the user has opted in AND has no second factor
		// enrolled (issue #2010). OIDC users have Password "" and therefore
		// always use an app password.
		passwordOK := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil
		passwordDeniedReason := ""
		if passwordOK {
			var allowed bool
			allowed, passwordDeniedReason = davAccountPasswordAllowed(db, &user)
			passwordOK = allowed
		}

		var matchedToken *models.ApiToken
		if !passwordOK {
			if apiToken, ok := middleware.LookupAPIToken(db, password, clock.FromContext(c).Now()); ok && apiToken.UserID == user.ID {
				matchedToken = apiToken
			}
		}

		if matchedToken != nil {
			requiredScope := models.TokenScopeCardDAV
			if surface == "CalDAV" {
				requiredScope = models.TokenScopeCalDAV
			}
			if !matchedToken.HasScope(requiredScope) {
				logger.Warn().
					Str("username", user.Username).
					Uint("user_id", user.ID).
					Str("surface", surface).
					Msg("DAV auth rejected: app password lacks the surface scope")
				c.Header("X-Mycorrhizal-Auth-Error", "token-scope-mismatch")
				c.String(http.StatusForbidden, "This app password is not scoped for "+surface+".")
				c.Abort()
				return
			}
		}

		if !passwordOK && matchedToken == nil {
			if passwordDeniedReason != "" {
				logger.Warn().
					Str("username", user.Username).
					Uint("user_id", user.ID).
					Str("surface", surface).
					Str("reason", passwordDeniedReason).
					Msg("DAV auth rejected: account password is not allowed")
				c.Header("WWW-Authenticate", `Basic realm="`+surface+` - use an app password"`)
				c.Header("X-Mycorrhizal-Auth-Error", "account-password-not-allowed-for-dav")
				c.String(http.StatusForbidden, "Account password cannot be used for CardDAV/CalDAV. Create an app password in Settings.")
				c.Abort()
				return
			}
			// Record failed attempt for password/token mismatch
			isLocked, _ := accountLimiter.RecordLoginFailure(identifier, clientIP)
			logger.Warn().
				Str("identifier", identifier).
				Str("ip", c.ClientIP()).
				Uint("user_id", user.ID).
				Bool("now_locked", isLocked).
				Msg("CardDAV auth failed: invalid password")
			c.Header("WWW-Authenticate", `Basic realm="CardDAV"`)
			if isLocked {
				c.AbortWithStatus(http.StatusTooManyRequests)
			} else {
				c.AbortWithStatus(http.StatusUnauthorized)
			}
			return
		}

		// Successful login - clear any failed attempt tracking for this (identifier, IP)
		accountLimiter.RecordLoginSuccess(identifier, clientIP)

		// Set user info in context for downstream handlers
		c.Set("userID", user.ID)
		c.Set("username", user.Username)
		c.Set("user", &user)

		if matchedToken != nil {
			middleware.TouchAPIToken(db, matchedToken.ID, clock.FromContext(c).Now())
		}

		c.Next()
	}
}

// davAccountPasswordAllowed implements the account-password half of issue
// #2010. The setting is ignored once a second factor is enrolled, and a
// failure to check that state fails closed.
func davAccountPasswordAllowed(db *gorm.DB, user *models.User) (bool, string) {
	methods, err := services.SecondFactorMethods(db, *user)
	if err != nil {
		logger.Error().Err(err).Uint("user_id", user.ID).Msg("DAV auth: could not check second factors")
		return false, "second-factor-check-failed"
	}
	if len(methods) > 0 {
		return false, "second-factor"
	}
	if !user.AllowDAVPassword {
		return false, "account-password-disabled"
	}
	return true, ""
}
