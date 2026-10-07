package realrelease

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/embedded"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/pquerna/otp/totp"
)

// SeedJWTSecret is the JWT_SECRET_KEY the old instance is run with and Verify
// boots the current server with. It must be the SAME value on both sides: the
// at-rest encryption master key is HKDF-derived from it (atrest.ResolveMasterKey),
// so a different secret would make every encrypted column unreadable and mask
// exactly the bugs this harness exists to find. Synthetic, never a real secret.
const SeedJWTSecret = "realrelease-1489-synthetic-jwt-secret-key-32+chars"

// VerifyOptions locate the install a published release left behind.
type VerifyOptions struct {
	// DBPath is the SQLite file (with its -wal/-shm siblings if the release was
	// not shut down cleanly) the old release wrote.
	DBPath string
	// PhotoDir / AttachDir are the release's static dirs ("" = a fresh temp
	// location beside the database).
	PhotoDir, AttachDir string
	Creds               *Credentials
	// Pre is the old release's own read-back, captured before it was stopped.
	Pre Snapshot
	// Logf receives a line per verified step (may be nil).
	Logf func(format string, args ...any)
}

// VerifyResult is what the upgrade produced.
type VerifyResult struct {
	// Post is the read-back through the CURRENT API after the upgrade.
	Post Snapshot
	// Diffs are the Compare(Pre, Post) findings; empty means the upgrade kept
	// everything the old release's API reported.
	Diffs []string
	// Problems are integrity-checker / login-flow findings.
	Problems []string
}

// OK reports whether the upgrade is clean.
func (r *VerifyResult) OK() bool { return len(r.Diffs) == 0 && len(r.Problems) == 0 }

// freePort asks the kernel for an unused TCP port.
func freePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer func() { _ = l.Close() }()
	_, port, err := net.SplitHostPort(l.Addr().String())
	return port, err
}

// Verify upgrades the install in place by booting the CURRENT server over it
// (embedded.Start: database.InitDB's migrations, then the at-rest, NFC and
// audit-chain startup backfills — the exact production boot path), exercises
// the logins and the old API token, re-captures the data and checks it against
// opts.Pre, then runs the doctor's integrity checks plus the audit-chain
// verification.
func Verify(ctx context.Context, opts VerifyOptions) (*VerifyResult, error) {
	if opts.Creds == nil || opts.Pre == nil {
		return nil, errors.New("realrelease: Verify needs Creds and Pre")
	}
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	cur, err := StartCurrent(ctx, opts.DBPath, opts.PhotoDir, opts.AttachDir)
	if err != nil {
		return nil, err
	}
	defer cur.Stop()
	logf("current server booted over the %s install", opts.DBPath)
	srv, cfg, port := cur.srv, cur.cfg, cur.port

	res := &VerifyResult{}
	problem := func(format string, args ...any) { res.Problems = append(res.Problems, fmt.Sprintf(format, args...)) }

	c := NewClient("http://127.0.0.1:"+port, nil)
	creds := opts.Creds

	// Login with the pre-existing account: the bcrypt hash must survive, and a
	// 2FA-enrolled account must demand its second factor and accept the
	// recovery code the OLD release minted.
	var lr struct {
		TwoFactorRequired bool `json:"two_factor_required"`
	}
	if err := c.call(ctx, http.MethodPost, "/login", map[string]any{"identifier": creds.Username, "password": creds.Password}, &lr); err != nil {
		return nil, fmt.Errorf("pre-existing account cannot log in after the upgrade: %w", err)
	}
	if creds.TOTPSecret != "" {
		if !lr.TwoFactorRequired {
			problem("2FA was enabled before the upgrade but login no longer demands a second factor")
		}
		if len(creds.RecoveryCodes) == 0 {
			return nil, errors.New("credentials carry a TOTP secret but no recovery codes")
		}
		if err := c.call(ctx, http.MethodPost, "/login/2fa", map[string]any{"code": creds.RecoveryCodes[0]}, nil); err != nil {
			return nil, fmt.Errorf("a recovery code minted by the old release is rejected after the upgrade: %w", err)
		}
		logf("login: password + old-release recovery code accepted")
	} else if lr.TwoFactorRequired {
		problem("login demands a second factor but none was enrolled")
	}

	post, err := Capture(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("capturing after the upgrade: %w", err)
	}
	res.Post = post
	res.Diffs = Compare(opts.Pre, post)
	logf("read-back compared: %d difference(s)", len(res.Diffs))

	// The old API token must still authenticate as a bearer (hashing scheme +
	// token_version survive).
	if creds.APIToken != "" {
		bearer := NewClient("http://127.0.0.1:"+port, nil)
		status, _, err := bearer.raw(ctx, http.MethodGet, "/contacts?limit=1", nil, map[string]string{"Authorization": "Bearer " + creds.APIToken})
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			problem("the API token minted by the old release is rejected after the upgrade (HTTP %d)", status)
		}
	}

	if creds.TOTPSecret != "" {
		// A TOTP code one step ahead (the server accepts +-1 step of skew):
		// the enrolment burned the current step, replay protection must not
		// have been reset or broken by the upgrade, but a fresh step works.
		code, err := totp.GenerateCode(creds.TOTPSecret, time.Now().Add(30*time.Second))
		if err != nil {
			return nil, err
		}
		totpClient := NewClient("http://127.0.0.1:"+port, nil)
		if err := totpClient.call(ctx, http.MethodPost, "/login", map[string]any{"identifier": creds.Username, "password": creds.Password}, nil); err != nil {
			return nil, err
		}
		if err := totpClient.call(ctx, http.MethodPost, "/login/2fa", map[string]any{"code": code}, nil); err != nil {
			problem("the TOTP secret enrolled on the old release no longer validates after the upgrade: %v", err)
		} else {
			logf("login: TOTP from the old release's secret accepted")
		}
		// A consumed recovery code stays consumed.
		reuse := NewClient("http://127.0.0.1:"+port, nil)
		if err := reuse.call(ctx, http.MethodPost, "/login", map[string]any{"identifier": creds.Username, "password": creds.Password}, nil); err != nil {
			return nil, err
		}
		if err := reuse.call(ctx, http.MethodPost, "/login/2fa", map[string]any{"code": creds.RecoveryCodes[0]}, nil); err == nil {
			problem("a recovery code consumed before the upgrade was accepted a second time")
		}
	}

	// The doctor's checks, against the live upgraded database.
	db := srv.DB()
	storage, sErr := services.RunStorageIntegrityChecks(db)
	if sErr != nil || !storage.OK {
		problem("storage integrity (PRAGMA integrity_check/foreign_key_check) failed: %v %s", sErr, storage.Detail())
	}
	data, dErr := services.RunDataIntegrityChecks(ctx, db, *cfg)
	if dErr != nil {
		problem("data integrity checks could not run: %v", dErr)
	} else if !data.OK {
		problem("data integrity checks found violations: %+v", data)
	}
	gaps, gErr := models.VerifyAuditChain(db)
	switch {
	case gErr != nil:
		problem("audit chain verification could not run: %v", gErr)
	case len(gaps) > 0:
		problem("audit hash chain is broken after the upgrade at event %d: %s", gaps[0].EventID, gaps[0].Message)
	}
	latest, err := database.LatestMigrationVersion()
	if err != nil { // # pragma: no cover — the embedded migration FS always parses
		return nil, err
	}
	var version uint
	var dirty bool
	row := db.Raw("SELECT version, dirty FROM schema_migrations LIMIT 1").Row()
	if err := row.Scan(&version, &dirty); err != nil {
		problem("reading schema_migrations: %v", err)
	} else if version != latest || dirty {
		problem("schema_migrations is at version %d dirty=%v, want %d clean", version, dirty, latest)
	}
	return res, nil
}

// setEnv sets the variables and returns a func restoring the previous values.
func setEnv(vars map[string]string) func() {
	type prev struct {
		val string
		set bool
	}
	old := map[string]prev{}
	for k, v := range vars {
		pv, ok := os.LookupEnv(k)
		old[k] = prev{pv, ok}
		_ = os.Setenv(k, v)
	}
	return func() {
		for k, p := range old {
			if p.set {
				_ = os.Setenv(k, p.val)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
}

// Current is the CURRENT server booted over a data directory.
type Current struct {
	srv     *embedded.Server
	cfg     *config.Config
	port    string
	restore func()
}

// BaseURL is the address the server listens on.
func (c *Current) BaseURL() string { return "http://127.0.0.1:" + c.port }

// Stop shuts the server down and restores the process environment.
func (c *Current) Stop() {
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = c.srv.Stop(stopCtx)
	c.restore()
}

// StartCurrent boots the current server (embedded.Start: migrations, then the
// at-rest/NFC/audit-chain startup backfills) over dbPath with SeedJWTSecret.
// photoDir/attachDir default to siblings of the database.
func StartCurrent(ctx context.Context, dbPath, photoDir, attachDir string) (*Current, error) {
	base := filepath.Dir(dbPath)
	if photoDir == "" {
		photoDir = filepath.Join(base, "photos")
	}
	if attachDir == "" {
		attachDir = filepath.Join(base, "attachments")
	}
	for _, d := range []string{photoDir, attachDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, err
		}
	}
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("finding a free port: %w", err)
	}
	restore := setEnv(map[string]string{
		"SQLITE_DB_PATH":             dbPath,
		"JWT_SECRET_KEY":             SeedJWTSecret,
		"PROFILE_PHOTO_DIR":          photoDir,
		"ATTACHMENTS_DIR":            attachDir,
		"FRONTEND_URL":               "http://127.0.0.1:" + port,
		"PORT":                       port,
		"DISABLE_REGISTRATION":       "false",
		"API_RATE_LIMIT_INTERVAL_MS": "1",
		"API_RATE_LIMIT_BURST":       "100000",
		"DB_RESTORE_DRILL_ENABLED":   "false",
		"LOG_LEVEL":                  "error",
		"LOG_PRETTY":                 "false",
	})
	cfg := config.LoadConfig()
	srv, err := embedded.Start(ctx, cfg, embedded.Options{})
	if err != nil {
		restore()
		return nil, fmt.Errorf("the current server failed to boot over the data: %w", err)
	}
	return &Current{srv: srv, cfg: cfg, port: port, restore: restore}, nil
}
