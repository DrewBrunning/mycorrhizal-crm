package caldav

import (
	"net/http"
	"strings"

	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler wraps the go-webdav CalDAV handler for use with Gin, mirroring
// carddav.Handler's shape.
type Handler struct {
	backend *Backend
	handler *caldav.Handler
	db      *gorm.DB
}

// NewHandler creates a CalDAV handler.
func NewHandler(db *gorm.DB) *Handler {
	backend := NewBackend(db)
	handler := &caldav.Handler{
		Backend: backend,
		Prefix:  "/caldav",
	}

	return &Handler{
		backend: backend,
		handler: handler,
		db:      db,
	}
}

// GinHandler returns a Gin handler wrapping the CalDAV handler. Authentication
// (Basic Auth: password or a DAV-scoped API token) is done by the shared
// carddav.BasicAuthMiddleware registered on the route group.
func (h *Handler) GinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("userID")
		username, _ := c.Get("username")

		ctx := ContextWithUser(c.Request.Context(), userID.(uint), username.(string), h.db)
		c.Request = c.Request.WithContext(ctx)

		// Principal discovery, mirroring carddav.Handler.GinHandler.
		if strings.HasPrefix(c.Request.URL.Path, "/caldav/principals/") {
			h.servePrincipal(c.Writer, c.Request, username.(string))
			return
		}
		if c.Request.URL.Path == "/caldav/" || c.Request.URL.Path == "/caldav" {
			h.servePrincipal(c.Writer, c.Request, username.(string))
			return
		}

		h.handler.ServeHTTP(c.Writer, c.Request)
	}
}

// servePrincipal handles PROPFIND requests for principal discovery.
func (h *Handler) servePrincipal(w http.ResponseWriter, r *http.Request, username string) {
	principalPath := "/caldav/principals/" + username + "/"
	homeSetPath := "/caldav/calendars/" + username + "/"

	webdav.ServePrincipal(w, r, &webdav.ServePrincipalOptions{
		CurrentUserPrincipalPath: principalPath,
		HomeSets: []webdav.BackendSuppliedHomeSet{
			caldav.NewCalendarHomeSet(homeSetPath),
		},
		Capabilities: []webdav.Capability{
			caldav.CapabilityCalendar,
		},
	})
}

// WellKnownRedirect handles the /.well-known/caldav discovery redirect.
func WellKnownRedirect(c *gin.Context) {
	c.Redirect(http.StatusMovedPermanently, "/caldav/")
}
