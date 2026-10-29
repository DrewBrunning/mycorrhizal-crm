package controllers

import (
	"net/http"
	"strconv"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
)

// Issue #2102: every "Authentication required" arm of the preference and
// password handlers (no username, an empty username, a username that no longer
// exists) carries auth_required.
func TestAccountReasonCodes_AuthRequiredArms(t *testing.T) {
	_, router := setupRouter(t)
	handlers := map[string]gin.HandlerFunc{
		"language": UpdateLanguage,
		"date":     UpdateDateFormat,
		"password": func(c *gin.Context) {
			c.Set("validated", &models.ChangePasswordInput{CurrentPassword: strongPassword, NewPassword: strongPasswordAnother})
			ChangePassword(c, &config.Config{})
		},
	}
	bodies := map[string]string{
		"language": `{"language":"en"}`,
		"date":     `{"date_format":"iso"}`,
		"password": `{}`,
	}
	n := 0
	for name, h := range handlers {
		for _, username := range []any{nil, "", "ghost-user"} {
			n++
			path := "/" + name + "/" + strconv.Itoa(n)
			router.POST(path, func(c *gin.Context) {
				if username != nil {
					c.Set("username", username)
				}
				h(c)
			})
			requireReason(t, serveRaw(router, "POST", path, bodies[name]), http.StatusUnauthorized, "auth_required")
		}
	}
}
