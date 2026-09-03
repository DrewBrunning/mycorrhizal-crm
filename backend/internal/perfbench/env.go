// Package perfbench is the PERF-02 core-operation benchmark harness (issue
// #469).
//
// The milestone asks that "core operations have measured performance
// characteristics" and that "no critical operation exhibits unacceptable
// behavior at the intended MVP scale". This package produces those
// measurements: it populates one of the PERF-01 representative datasets
// (internal/largedata.Profile — issue #468) into a REAL migrated schema
// (CLAUDE.md backend trap #1), then runs each core operation against a
// connection wrapped in a database.QueryCounter, recording:
//
//   - the query count — the deterministic regression signal. An N+1 that is
//     invisible at 100 contacts is what breaks at 10,000, and it shows up in
//     the count long before the clock (issue #261's philosophy, extended
//     from three scattered *_QueryCountIsBounded tests to a suite).
//   - the result-set size — a second deterministic signal. A pairwise
//     operation (duplicate detection) can be O(1) queries yet O(n^2) output.
//   - wall-clock time — indicative only, NOT a gate: it varies across CI
//     hardware. Recorded in the generated report, never asserted.
//
// growth.go compares the query count and the result size across two dataset
// scales and flags any operation that grows faster than the row count as a
// finding. baseline.go is the committed, diffable record (testdata/baseline.json);
// cmd/perfbench regenerates it and docs/development/perf-benchmarks.md. #471
// turns these numbers into budgets — this ticket only produces them.
package perfbench

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/config"
	"mycorrhizal/controllers"
	"mycorrhizal/database"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/largedata"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EnvOptions configures NewEnv.
type EnvOptions struct {
	// Profile is the PERF-01 dataset shape to populate.
	Profile largedata.Profile
	// WorkDir is a caller-owned directory the SQLite file (and, for
	// destructive operations, per-operation sub-copies) live under. The caller
	// is responsible for removing it.
	WorkDir string
	// OpenMigrated returns a fresh *gorm.DB on a migrated schema at path. nil
	// selects database.InitDB — a real migration run of the hand-written SQL
	// (CLAUDE.md backend trap #1), which cmd/perfbench uses. The tests inject
	// an internal/dbtest template copy instead (schema built once per binary,
	// byte-copied per Env) — a full InitDB per Env blows the CI -race timeout.
	OpenMigrated func(path string) (*gorm.DB, error)
}

// Env is a populated benchmark fixture: a counting DB connection, the seeded
// user, and resolved handles for the interesting contacts (a normal one, a
// dense hub, a pathological record). Build it with NewEnv; release it with
// Close.
type Env struct {
	Profile largedata.Profile
	Cfg     config.Config

	// DB is the connection every measured operation runs on. Its GORM logger
	// is Counter, so Counter.Count() after a call is that call's query tally.
	DB      *gorm.DB
	Counter *database.QueryCounter

	// seedDB is a second connection to the same file, used for fixture
	// population and handle resolution so those queries never land in
	// Counter's tally (the two-connection pattern from
	// controllers/hot_paths_perf_test.go).
	seedDB    *gorm.DB
	dbPath    string
	closeSeed bool // false when an injected opener (internal/dbtest) owns seedDB's lifetime
	dataset   *largedata.ProfileDataset
	router    *gin.Engine
	closers   []func()

	UserID uint

	// NormalContact is an ordinary contact with a handful of associations.
	NormalContact models.Contact
	// HubContact is the block-0 lead — a dense hub (HubFanout out-edges) and
	// the head of the cross-block relationship chain.
	HubContact models.Contact
	// SecondHubContact is a second dense hub that is NOT the chain head, for
	// the "traverse through a hub" measurement.
	SecondHubContact models.Contact
	// PathologicalContact is a manifest record carrying the awkward shapes
	// (duplicate keywords, many repeatable fields) — a slow query is usually
	// the one hitting the pathological record, not the average one.
	PathologicalContact models.Contact
	// CircleName is a populated circle's name, for the list filter measurement.
	CircleName string

	// mergeResolutions is filled by the contact_merge operation's Prepare hook.
	mergeResolutions map[string]string
}

// SeedDB is the non-counting connection. Operation Prepare hooks use it for
// setup writes that must stay out of the query tally.
func (e *Env) SeedDB() *gorm.DB { return e.seedDB }

// NewEnv builds and populates a benchmark environment for opts.Profile.
func NewEnv(opts EnvOptions) (*Env, error) {
	if opts.WorkDir == "" {
		return nil, fmt.Errorf("perfbench: EnvOptions.WorkDir is required")
	}
	gin.SetMode(gin.ReleaseMode)

	dbPath := filepath.Join(opts.WorkDir, "perfbench.db")
	open := opts.OpenMigrated
	closeSeed := false
	if open == nil { // # pragma: no cover — the cmd's real-migration path; every test injects a dbtest opener
		open = database.InitDB
		closeSeed = true // this Env opened seedDB, so this Env closes it
	}

	seedDB, err := open(dbPath)
	if err != nil { // # pragma: no cover — a fresh temp path always migrates/opens
		return nil, fmt.Errorf("perfbench: opening migrated schema: %w", err)
	}

	base, err := canonicalfixture.Read()
	if err != nil { // # pragma: no cover — the checked-in manifest always loads from inside the repo
		return nil, fmt.Errorf("perfbench: reading canonical manifest: %w", err)
	}

	ds, err := largedata.Populate(seedDB, base, opts.Profile)
	if err != nil { // # pragma: no cover — a catalogue profile always populates a fresh migrated schema
		return nil, fmt.Errorf("perfbench: populating %q profile: %w", opts.Profile.Name, err)
	}

	counter := database.NewQueryCounter()
	countDB, err := database.OpenMigratedFileWithLogger(dbPath, counter)
	if err != nil { // # pragma: no cover — the file seedDB just migrated reopens
		return nil, fmt.Errorf("perfbench: opening counting connection: %w", err)
	}

	photoDir := filepath.Join(opts.WorkDir, "photos")
	if err := os.MkdirAll(photoDir, 0o750); err != nil { // # pragma: no cover — mkdir under a writable temp dir
		return nil, fmt.Errorf("perfbench: creating photo dir: %w", err)
	}

	e := &Env{
		Profile:   opts.Profile,
		Cfg:       config.Config{ProfilePhotoDir: photoDir},
		DB:        countDB,
		Counter:   counter,
		seedDB:    seedDB,
		dbPath:    dbPath,
		closeSeed: closeSeed,
		dataset:   ds,
	}
	e.closers = append(e.closers, closeGorm(countDB))
	if closeSeed { // # pragma: no cover — only set on the cmd's nil-opener path; tests' dbtest opener owns seedDB
		e.closers = append(e.closers, closeGorm(seedDB))
	}

	if err := e.resolveHandles(base); err != nil { // # pragma: no cover — resolveHandles only fails on an empty dataset, which Populate never returns
		e.Close()
		return nil, err
	}
	return e, nil
}

// resolveHandles pins the named contacts (and a circle) from user 0's dataset.
func (e *Env) resolveHandles(base *canonicalfixture.Manifest) error {
	if len(e.dataset.Users) == 0 { // # pragma: no cover — every catalogue profile has Users >= 1
		return fmt.Errorf("perfbench: profile %q populated no users", e.Profile.Name)
	}
	u0 := e.dataset.Users[0]
	e.UserID = u0.User.ID

	// largedata block-scales the manifest: every contact name gains a
	// "_%06d" block suffix (internal/largedata newRewriterSalted). Block 0's
	// lead contact (manifest index 0) is always a hub AND the chain head
	// (internal/largedata.appendGraphShape).
	lead := base.Contacts[0].Name // "ada" in the canonical manifest — live, richest card.
	var ok bool
	if e.HubContact, ok = u0.Contacts[blockName(lead, 0)]; !ok { // # pragma: no cover — block 0's lead is always populated
		return fmt.Errorf("perfbench: hub contact %q not found in populated dataset", blockName(lead, 0))
	}

	// A second hub sits at block (blocks/2) when the profile asks for >= 2
	// hubs and the dataset spans >= 2 blocks; fall back to the chain head.
	e.SecondHubContact = e.HubContact
	if c, ok := u0.Contacts[blockName(lead, secondHubBlock(e.Profile, len(base.Contacts)))]; ok {
		e.SecondHubContact = c
	}

	// A normal contact: manifest index 1 ("bob"), block 0 — an ordinary card
	// with a couple of edges, never a hub.
	if len(base.Contacts) > 1 {
		if c, ok := u0.Contacts[blockName(base.Contacts[1].Name, 0)]; ok {
			e.NormalContact = c
		}
	}
	if e.NormalContact.ID == 0 { // # pragma: no cover — the canonical manifest always has a live second contact
		e.NormalContact = e.HubContact
	}

	// The pathological record: the manifest's "duplicate keywords" contact if
	// present, else the normal one.
	e.PathologicalContact = e.NormalContact
	for _, mc := range base.Contacts {
		if mc.Name == "test07_duplicate_keywords" {
			if c, ok := u0.Contacts[blockName(mc.Name, 0)]; ok {
				e.PathologicalContact = c
			}
		}
	}

	if len(u0.Circles) > 0 {
		e.CircleName = u0.Circles[0].Name
	}
	return nil
}

// Router lazily builds the gin engine every handler-shaped operation runs
// through. Middleware injects the counting DB, the seeded user, and the
// config — the same shape controllers/hot_paths_perf_test.go's
// setupCountingRouter uses, plus the validation middleware the write routes
// carry in routes/routes.go.
func (e *Env) Router() *gin.Engine {
	if e.router != nil {
		return e.router
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", e.DB)
		c.Set("userID", e.UserID)
		c.Set("cfg", e.Cfg)
		c.Next()
	})
	r.GET("/contacts", controllers.GetContacts)
	r.GET("/contacts/:id/detail", controllers.GetContactDetail)
	r.POST("/contacts", middleware.ValidateJSONMiddleware(&models.ContactRecordInput{}), controllers.CreateContact)
	r.PUT("/contacts/:id", middleware.ValidateJSONMiddleware(&models.ContactRecordInput{}), controllers.UpdateContact)
	r.DELETE("/contacts/:id", controllers.DeleteContact)
	r.GET("/dashboard", controllers.GetDashboard)
	r.GET("/search", controllers.SearchAll)
	r.GET("/graph/connections", controllers.GetGraphConnections)
	r.POST("/contacts/merge", middleware.ValidateJSONMiddleware(&models.ContactMergeRequest{}), controllers.CommitContactMerge)
	e.router = r
	return r
}

// ContactCount is the total live+tombstoned contacts populated across every
// user — the row-count scale growth.go compares operations against.
func (e *Env) ContactCount() int { return e.dataset.ContactCount() }

// forkForDestructive returns a second Env backed by a byte copy of this Env's
// (already populated) database, under workDir. It is how a destructive
// operation (merge, delete cascade) gets its own fixture WITHOUT paying a
// second migrate + populate — the dominant cost under CI -race. The copy has
// identical rows and IDs, so the parent's resolved handles carry over
// verbatim. The parent must not be mutated between forking and measuring.
func (e *Env) forkForDestructive(workDir string) (*Env, error) {
	// Fold the WAL back into the main file so a plain copy is complete and
	// consistent (the internal/dbtest template trick).
	sqlDB, err := e.seedDB.DB()
	if err != nil { // # pragma: no cover — e.seedDB is a live handle
		return nil, err
	}
	if _, err := sqlDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil { // # pragma: no cover — checkpoint on a healthy WAL db
		return nil, err
	}

	dst := filepath.Join(workDir, "fork.db")
	if err := copyFile(e.dbPath, dst); err != nil { // # pragma: no cover — copy within a writable temp tree
		return nil, err
	}

	counter := database.NewQueryCounter()
	countDB, err := database.OpenMigratedFileWithLogger(dst, counter)
	if err != nil { // # pragma: no cover — reopening a just-copied migrated file
		return nil, err
	}
	seedDB, err := database.OpenMigratedFile(dst)
	if err != nil { // # pragma: no cover — see above
		_ = closeGormNow(countDB)
		return nil, err
	}

	photoDir := filepath.Join(workDir, "photos")
	if err := os.MkdirAll(photoDir, 0o750); err != nil { // # pragma: no cover — mkdir under a writable temp dir
		_ = closeGormNow(countDB)
		_ = closeGormNow(seedDB)
		return nil, err
	}

	f := &Env{
		Profile:             e.Profile,
		Cfg:                 config.Config{ProfilePhotoDir: photoDir},
		DB:                  countDB,
		Counter:             counter,
		seedDB:              seedDB,
		dbPath:              dst,
		closeSeed:           true,
		dataset:             e.dataset,
		UserID:              e.UserID,
		NormalContact:       e.NormalContact,
		HubContact:          e.HubContact,
		SecondHubContact:    e.SecondHubContact,
		PathologicalContact: e.PathologicalContact,
		CircleName:          e.CircleName,
	}
	f.closers = []func(){closeGorm(countDB), closeGorm(seedDB)}
	return f, nil
}

// Close releases every connection this Env opened.
func (e *Env) Close() {
	for i := len(e.closers) - 1; i >= 0; i-- {
		e.closers[i]()
	}
	e.closers = nil
}

func closeGorm(db *gorm.DB) func() {
	return func() { _ = closeGormNow(db) }
}

func closeGormNow(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil { // # pragma: no cover — db is always a live handle here
		return err
	}
	return sqlDB.Close()
}

// copyFile is a minimal file copy for forkForDestructive (mirrors the
// unexported one in internal/dbtest).
func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src is this package's own temp db path, never request input
	if err != nil {         // # pragma: no cover — src was just written by Populate
		return err
	}
	defer in.Close()
	out, err := os.Create(dst) // #nosec G304 -- dst is a caller t.TempDir()/os.MkdirTemp path
	if err != nil {            // # pragma: no cover — dst dir exists and is writable
		return err
	}
	if _, err := io.Copy(out, in); err != nil { // # pragma: no cover — local temp-fs copy
		_ = out.Close()
		return err
	}
	return out.Close()
}

// blockName is largedata's contact-name re-keying: "<manifest name>_<block>".
func blockName(manifestName string, block int) string {
	return fmt.Sprintf("%s_%06d", manifestName, block)
}

// secondHubBlock mirrors internal/largedata.appendGraphShape's hub placement
// for the second hub (h == 1): hubBlock = (h * blocks) / hubs. It returns 0
// (the chain head, a safe fallback) when the profile has fewer than two hubs
// or the dataset is a single block.
func secondHubBlock(p largedata.Profile, manifestContacts int) int {
	blocks := (p.Contacts + manifestContacts - 1) / manifestContacts
	hubs := p.Hubs
	if hubs > blocks {
		hubs = blocks
	}
	if hubs < 2 || blocks < 2 {
		return 0
	}
	return blocks / hubs
}
