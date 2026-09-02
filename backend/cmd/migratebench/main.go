// Command migratebench is the operator-side measurement harness behind the
// large-dataset migration test (issue #495) and PERF-01 benchmark fixtures
// (issue #468). It builds large databases from the scaled canonical manifest
// (internal/largedata), checkpoints them to a historical migration version,
// and migrates them back to the current schema while sampling wall-clock time,
// peak resident memory, and peak database size — the numbers the milestone's
// "large datasets have been tested" / "documented resource requirements"
// criteria are recorded from. See docs/development/scale-testing.md.
//
// Subcommands:
//
//	seed       (--contacts N | --profile NAME) --db PATH
//	           Build a large CURRENT-schema database at PATH, populated through
//	           the same code paths the REST API uses. --contacts N is the
//	           canonical TEST-02 manifest block-scaled to >= N contacts (one
//	           user, no graph shape — the exact-row-count artifact the
//	           migration test measures over). --profile NAME is a named entry
//	           from the PERF-01 catalogue (internal/largedata: smoke|typical|
//	           large|stress), which adds a user count and a dense-hub/deep-chain
//	           graph on top — see docs/development/scale-profiles.md.
//
//	checkpoint --db PATH --version N --out PATH
//	           Copy every row of a current-schema database into a fresh
//	           database at PATH whose schema is exactly migration version N —
//	           the "large database at release X" artifact a migration path is
//	           measured over.
//
//	measure    --db PATH
//	           Migrate the database at PATH to the current schema in a fresh
//	           process, sampling peak RSS and peak DB size during the run, and
//	           print a `migratebench_report ...` line with duration, peak
//	           memory, peak disk, and from/to versions.
//
//	measure    --db PATH
//	           Migrate the database at PATH to the current schema in a fresh
//	           process (run it as its own command so the reported peak memory
//	           is the migration's alone — the seed's in-memory manifest never
//	           shares a process with it), sampling peak RSS and peak DB size
//	           during the run, and print a `migratebench_report ...` line with
//	           duration, peak memory, peak disk, and from/to versions.
//
// There is deliberately no `run` pipeline subcommand that execs `measure`:
// the security posture bans `os/exec` in non-test packages (ASVS 12.3.6), and
// a same-process in-process measure would report the seed's manifest as the
// migration's peak RSS — a misleading number. The three commands above are the
// workflow; each is its own process.
//
// The memory and disk figures are sampled from the process's own /proc on
// Linux (the platform the documented measurement and CI run on); elsewhere the
// samplers degrade to Go's runtime stats and a zero disk peak.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"mycorrhizal/database"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/largedata"
	"mycorrhizal/internal/schemafixture"
	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// defaultDBPath matches the other CLIs' fallback when SQLITE_DB_PATH is unset.
const defaultDBPath = "mycorrhizal.db"

func main() {
	os.Exit(runCLI(os.Args[1:])) // # pragma: no cover — os.Exit terminates the process; tests exercise runCLI directly
}

// initCLILogger mirrors the server's LOG_LEVEL / LOG_PRETTY env contract so the
// database package's structured logs (including the per-step migration
// progress) reach the terminal.
func initCLILogger() {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	pretty := os.Getenv("LOG_PRETTY") == "true" || os.Getenv("LOG_PRETTY") == "1"
	logger.InitLogger(logger.Config{Level: level, Pretty: pretty})
}

func runCLI(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: migratebench <seed|checkpoint|measure> [flags]")
		return 2
	}
	initCLILogger()

	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "seed":
		return exitCode("seed", runSeed(rest))
	case "checkpoint":
		return exitCode("checkpoint", runCheckpoint(rest))
	case "measure":
		return exitCode("measure", runMeasure(rest))
	default:
		fmt.Fprintln(os.Stderr, "Usage: migratebench <seed|checkpoint|measure> [flags]")
		return 2
	}
}

func exitCode(name string, err error) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "migratebench %s: %v\n", name, err)
		return 1
	}
	return 0
}

// runSeed populates a large current-schema database — either an ad-hoc
// --contacts count (one user, no graph shape: the exact-row-count artifact the
// migration test measures over) or a named --profile from the PERF-01
// catalogue (internal/largedata, multi-user + dense-hub/deep-chain graph:
// docs/development/scale-profiles.md). Exactly one of the two is required.
func runSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	contacts := fs.Int("contacts", 0, "target contact count (rounded up to a whole manifest block); mutually exclusive with --profile")
	profileName := fs.String("profile", "", "PERF-01 dataset profile: "+profileNames()+"; mutually exclusive with --contacts")
	dbPath := fs.String("db", "", "output database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("--db is required")
	}
	if (*contacts > 0) == (*profileName != "") {
		return fmt.Errorf("exactly one of --contacts or --profile is required")
	}
	var profile largedata.Profile
	if *profileName != "" {
		p, ok := largedata.ProfileByName(*profileName)
		if !ok {
			return fmt.Errorf("unknown --profile %q (have %s)", *profileName, profileNames())
		}
		profile = p
	}
	_ = os.Remove(*dbPath)

	m, err := canonicalfixture.Read()
	if err != nil { // # pragma: no cover — the committed manifest always resolves under the repo root
		return err
	}

	start := time.Now()
	db, err := database.InitDB(*dbPath)
	if err != nil { // # pragma: no cover — a fresh path always migrates; the migration tests exercise the failure vocabulary
		return err
	}

	var summary string
	if *profileName != "" {
		ds, err := largedata.Populate(db, m, profile)
		if err != nil { // # pragma: no cover — a catalogue profile always populates a fresh migrated DB; largedata tests exercise the failure vocabulary
			_ = sqlClose(db)
			return err
		}
		summary = fmt.Sprintf("profile=%s users=%d contacts=%d", profile.Name, len(ds.Users), ds.ContactCount())
	} else {
		scaled, err := largedata.Scale(m, *contacts)
		if err != nil { // # pragma: no cover — a positive --contacts and a manifest that just loaded cannot fail Scale
			_ = sqlClose(db)
			return err
		}
		if _, err := canonicalfixture.Populate(db, scaled); err != nil { // # pragma: no cover — a scaled manifest that validated against a migrated DB always populates; largedata tests exercise the failure vocabulary
			_ = sqlClose(db)
			return err
		}
		summary = fmt.Sprintf("contacts=%d blocks=%d", len(scaled.Contacts), len(scaled.Contacts)/largedata.BlocksOfManifest)
	}

	sqlDB, err := db.DB()
	if err != nil { // # pragma: no cover — DB() fails only on a closed handle, which was just opened
		return err
	}
	// Fold the WAL into the main file so the checkpoint phase's plain file copy
	// is a complete database.
	if _, err := sqlDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil { // # pragma: no cover — a freshly-written database always accepts a checkpoint
		return err
	}
	if err := sqlDB.Close(); err != nil { // # pragma: no cover — closing an open handle does not fail
		return err
	}

	size, err := fileSize(*dbPath)
	if err != nil { // # pragma: no cover — the file was just written
		return err
	}
	fmt.Printf("migratebench seed: %s db_bytes=%d duration_ms=%d\n",
		summary, size, time.Since(start).Milliseconds())
	return nil
}

// profileNames is the catalogue's names as a "a|b|c" string for flag help and
// error messages.
func profileNames() string {
	names := make([]string, 0, len(largedata.Profiles()))
	for _, p := range largedata.Profiles() {
		names = append(names, p.Name)
	}
	return strings.Join(names, "|")
}

// runCheckpoint transplants a current-schema database's rows into a fresh
// database at a historical migration version.
func runCheckpoint(args []string) error {
	fs := flag.NewFlagSet("checkpoint", flag.ContinueOnError)
	dbPath := fs.String("db", "", "current-schema source database")
	version := fs.Uint("version", database.SupportedUpgradeFloorVersion, "migration version to build")
	outPath := fs.String("out", "", "output database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *outPath == "" {
		return fmt.Errorf("--db and --out are required")
	}
	if *version == 0 {
		return fmt.Errorf("--version must be >= 1")
	}
	if _, err := os.Stat(*outPath); err == nil {
		return fmt.Errorf("--out %s already exists; refusing to overwrite", *outPath)
	}

	start := time.Now()
	src, err := database.OpenMigratedFile(*dbPath)
	if err != nil { // # pragma: no cover — the source was just written by seed and the migration tests cover the open failure vocabulary
		return err
	}
	if err := schemafixture.TransplantDataToVersion(src, *version, *outPath); err != nil { // # pragma: no cover — the source data came from a real migrated DB; schemafixture tests cover the failure vocabulary
		_ = sqlClose(src)
		return err
	}
	if err := sqlClose(src); err != nil { // # pragma: no cover — closing an open handle does not fail
		return err
	}

	size, err := fileSize(*outPath)
	if err != nil { // # pragma: no cover — the file was just written
		return err
	}
	fmt.Printf("migratebench checkpoint: from=current to=%d db_bytes=%d duration_ms=%d\n",
		*version, size, time.Since(start).Milliseconds())
	return nil
}

// runMeasure migrates a database to the current schema while sampling peak RSS
// and peak DB size, and prints the report line. Run as its own process (the
// `run` pipeline execs it) so the memory figure is the migration's, not the
// seed's.
func runMeasure(args []string) error {
	fs := flag.NewFlagSet("measure", flag.ContinueOnError)
	dbPath := fs.String("db", "", "database path (SQLITE_DB_PATH fallback)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		*dbPath = os.Getenv("SQLITE_DB_PATH")
	}
	if *dbPath == "" { // # pragma: no cover — the default-path branch is the CLI fallback when neither --db nor SQLITE_DB_PATH is set; tests always set one
		*dbPath = defaultDBPath
	}

	beforeVersion, _, ok := migrationState(*dbPath)
	if !ok {
		return fmt.Errorf("measure: %s has no schema_migrations row (build one with seed + checkpoint first)", *dbPath)
	}

	latest, err := database.LatestMigrationVersion()
	if err != nil { // # pragma: no cover — the embedded migrations FS always has at least one migration
		return fmt.Errorf("resolve latest migration version: %w", err)
	}

	start := time.Now()
	sampler := startSampler(*dbPath)
	// MigrateUpTo, not MigrateUp: this harness measures the migration's own
	// peak RSS/disk, and MigrateUp now takes the mandatory pre-migration
	// backup (issue #530) — a full VACUUM INTO in the sampled window would
	// distort the figure and, on a re-run, collide. The set of migrations
	// applied to reach `latest` is identical.
	err = database.MigrateUpTo(*dbPath, latest)
	s := sampler()
	if err != nil { // # pragma: no cover — a migration failure is the fail-closed vocabulary the chaos job and schemafixture tests exercise, not a CLI branch
		return fmt.Errorf("migration failed: %w", err)
	}

	afterVersion, dirty, ok := migrationState(*dbPath)
	if !ok { // # pragma: no cover — a migration that just ran writes the version row
		return fmt.Errorf("measure: no schema_migrations row after migration")
	}
	integrity, err := database.IntegrityCheck(*dbPath) // raw-sql: GORM's slow-SQL logger would pollute the report stream on a large DB
	if err != nil {                                    // # pragma: no cover — a database the migration just verified always answers integrity_check
		return err
	}
	finalSize := dbBytes(*dbPath)

	fmt.Printf("migratebench_report from_version=%d to_version=%d duration_ms=%d peak_rss_kb=%d peak_db_bytes=%d initial_db_bytes=%d peak_extra_db_bytes=%d final_db_bytes=%d integrity=%s dirty=%v\n",
		beforeVersion, afterVersion, time.Since(start).Milliseconds(),
		s.peakRSSKb, s.peakDB, s.initialDB, s.peakDB-s.initialDB, finalSize, integrity, dirty)
	return nil
}

// sample is the accumulated measurement for one migration run.
type sample struct {
	peakRSSKb int64
	peakDB    int64
	initialDB int64
}

// startSampler begins a goroutine sampling the process's resident memory and
// the database's on-disk size (main + -wal + -shm) until the returned stop
// func is called. It samples immediately, then every 20 ms, and once more at
// stop, so even a sub-50ms migration still records a peak. The db-size peak
// captures SQLite's transient table rebuilds — a migration that grows the file
// halfway before collapsing it back shows up as peak additional disk, not just
// the final size.
func startSampler(dbPath string) func() sample {
	initial := dbBytes(dbPath)
	var (
		mu      sync.Mutex
		peakRSS int64
		peakDB  int64
		done    = make(chan struct{})
		wg      sync.WaitGroup
	)
	record := func() {
		if rss := residentKB(); rss > 0 {
			mu.Lock()
			if rss > peakRSS {
				peakRSS = rss
			}
			mu.Unlock()
		}
		if sz := dbBytes(dbPath); sz > 0 {
			mu.Lock()
			if sz > peakDB {
				peakDB = sz
			}
			mu.Unlock()
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		record()
		for {
			select {
			case <-done:
				record() // final sample: a fast migration may have finished between ticks
				return
			case <-time.After(20 * time.Millisecond):
				record()
			}
		}
	}()
	return func() sample {
		close(done)
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		return sample{peakRSSKb: peakRSS, peakDB: peakDB, initialDB: initial}
	}
}

// residentKB returns the process's current resident memory in kB from /proc on
// Linux, or 0 when unavailable (the documented measurement runs on Linux; the
// fallback exists so the CLI still works elsewhere).
func residentKB() int64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil { // # pragma: no cover — /proc always exists on the platforms the measurement runs on
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		kb := ms.Sys / 1024
		if kb > uint64(math.MaxInt64) { // # pragma: no cover — a process cannot map more than 2^63 bytes of address space
			return math.MaxInt64
		}
		return int64(kb)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return kb
				}
			}
		}
	}
	return 0 // # pragma: no cover — a Linux /proc/self/status always carries a parseable VmRSS line
}

// dbBytes returns the total on-disk size of the database and its WAL sidecars.
func dbBytes(path string) int64 {
	var total int64
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	return total
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil { // # pragma: no cover — the caller just created the file
		return 0, err
	}
	return fi.Size(), nil
}

// migrationState reads the applied version/dirty flag of a database file.
func migrationState(path string) (version uint, dirty bool, ok bool) {
	version, dirty, ok, _ = database.MigrationVersion(path)
	return version, dirty, ok
}

func sqlClose(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil { // # pragma: no cover — DB() fails only on a closed handle, which callers never pass
		return err
	}
	return sqlDB.Close()
}
