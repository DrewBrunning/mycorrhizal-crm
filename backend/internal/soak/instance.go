package soak

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/embedded"

	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	"gorm.io/gorm"
)

// Instance is a real server booted in this process on a loopback TCP port
// against a real, migrated SQLite file: the same embedded.Start entry point
// main() uses, so the router, middleware, scheduler (with its boot-time
// catch-up burst), cleanup ticker and /metrics endpoint are production code.
// Only the process boundary differs from the shipped image; see
// docs/development/soak-testing.md for what that does and does not cover.
type Instance struct {
	BaseURL      string
	MetricsToken string
	DBPath       string
	Cfg          *config.Config
	srv          *embedded.Server
}

// StartInstance boots an in-process server rooted in dir.
func StartInstance(ctx context.Context, dir string) (*Instance, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate at-rest key: %w", err)
	}
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		return nil, fmt.Errorf("generate metrics token: %w", err)
	}
	token := "soak-" + hex.EncodeToString(tok) // >= 16 chars

	cfg, err := config.New(func(c *config.Config) {
		c.Deployment = config.DeploymentServer
		c.DBPath = filepath.Join(dir, "soak.db")
		c.JWTSecretKey = "soak-secret-key-that-is-definitely-long-enough"
		c.ProfilePhotoDir = filepath.Join(dir, "photos")
		c.AttachmentsDir = filepath.Join(dir, "attachments")
		c.DataEncryptionKey = base64.StdEncoding.EncodeToString(key)
		c.FrontendURL = "http://localhost:7300"
		c.GinMode = "release"
		c.LogLevel = "fatal"
		c.MetricsToken = token
		// One client IP drives the whole soak, so it shares one bucket; raise
		// the general-API limit the way docker-compose.test.yml does (the
		// auth limiter is left at its production value and exercised as is).
		c.APIRateLimitInterval = time.Millisecond
		c.APIRateLimitBurst = 1_000_000
	})
	if err != nil {
		return nil, fmt.Errorf("build soak config: %w", err)
	}
	// gin prints every route at registration in debug mode; the soak output
	// is a report, not a route dump.
	gin.SetMode(gin.ReleaseMode)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	srv, err := embedded.Start(ctx, cfg, embedded.Options{Listener: ln})
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("start server: %w", err)
	}
	return &Instance{
		BaseURL:      "http://" + ln.Addr().String(),
		MetricsToken: token,
		DBPath:       cfg.DBPath,
		Cfg:          cfg,
		srv:          srv,
	}, nil
}

// DB is the server's own database handle.
func (i *Instance) DB() *gorm.DB { return i.srv.DB() }

// Jobs lists the scheduler's registered jobs.
func (i *Instance) Jobs() []*gocron.Job { return i.srv.Jobs() }

// Stop shuts the server down gracefully.
func (i *Instance) Stop(ctx context.Context) error { return i.srv.Stop(ctx) }
