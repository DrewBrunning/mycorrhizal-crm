package perfbench

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"mycorrhizal/controllers"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DataMovementOperation is one bulk operation measured by PERF-03 (issue
// #470): import, export, an FTS rebuild, a backup, a restore, a delete
// cascade, duplicate detection. Unlike a PERF-02 Operation these are not
// iterated — each runs exactly once, against its own byte copy of the fixture
// when it mutates it — and what is recorded is the resource envelope
// (resources.go) rather than a query count.
type DataMovementOperation struct {
	// Name is the stable key in the baseline and the report.
	Name string
	// Category groups the report: "import" | "export" | "maintenance" |
	// "backup" | "restore" | "delete".
	Category string
	// Isolate runs the operation against a fresh byte copy of the populated
	// fixture (Env.forkForDestructive) — set for anything that writes to or
	// deletes from the database.
	Isolate bool
	// Probe arms the concurrent write probe (resources.go) so the report can
	// say whether the operation blocks other writers for its whole duration
	// (an outage) or not (a background task).
	Probe bool
	// ExpectedMemoryGrowth is the worst class the operation's PEAK HEAP is
	// allowed to exhibit as its work size (RowsTouched) grows across profiles.
	// "constant" for anything that streams; "linear" for the operations that
	// are known to assemble their whole result in memory first (the exporters
	// buffer, ParseVCF accumulates every parsed contact). A measured class
	// worse than this fails TestDataMovementAtScale — it is the "will be
	// OOM-killed at scale" signal the ticket asks to surface.
	ExpectedMemoryGrowth GrowthClass
	// FoldResultGrowth folds RowsTouched growth into the memory class for the
	// operations whose output cardinality IS the cost (duplicate detection is
	// O(pairs) to hold).
	FoldResultGrowth bool
	// FreeSpaceNote is the operator-facing one-liner for the report's
	// free-space table: what extra disk this operation needs before it starts.
	FreeSpaceNote string
	// Prepare runs once before the measurement, on e (already the forked copy
	// when Isolate is set). Its writes are not measured.
	Prepare func(e *Env, workDir string) error
	// Run performs the operation and returns the rows it touched and the size
	// of any artifact it produced. workDir is a scratch directory unique to
	// this operation; e.dbPath is the database it runs against.
	Run func(e *Env, workDir string) (rowsTouched int, outputBytes int64, err error)
}

// maxImportContacts caps the synthetic import batch well below the service's
// own MaxVCFContacts ceiling (20000). ParseVCF's within-batch duplicate scan
// is O(n²) in the batch size (see import.vcf's ExpectedMemoryGrowth note), so
// a 15000-row import spends many minutes in the harness for no extra signal.
// 1500 establishes import's memory shape across smoke (150) → typical (900) →
// large (1500) — a 10x span — while keeping the gated run bounded.
const maxImportContacts = 1500

// DataMovementRegistry returns every PERF-03 operation, in a stable order.
// Migration-at-scale is deliberately absent: it is measured by
// TestDataMovementAtScale via internal/schemafixture (which imports testing),
// so the registry — reused by cmd/perfbench — stays testing-free, and issue
// #495 remains the owner of migration depth.
func DataMovementRegistry() []DataMovementOperation {
	return []DataMovementOperation{
		{
			Name:     "import.vcf",
			Category: "import",
			Isolate:  true,
			Probe:    true,
			// SUPER-LINEAR peak memory, and a recorded finding (issue #470's
			// "single most valuable finding"): ParseVCF accumulates every
			// parsed contact and every preview row for the whole file, and its
			// per-row preview compares each row against every prior row of the
			// batch (BuildImportRowPreview's within-batch duplicate scan) —
			// O(n²) work whose allocation grows faster than the batch. Measured
			// heap x63 for a x17 batch at smoke→large. The product caps imports
			// at services.MaxVCFContacts (20000); a genuinely large address book
			// should be chunked. PERF-04 (#471) sets an absolute peak-heap
			// ceiling for it in budgets.json; #725 owns any fix.
			ExpectedMemoryGrowth: GrowthSuperlinear,
			FreeSpaceNote:        "None on disk; peak RAM grows FASTER than the batch — ParseVCF holds the whole parsed batch and its preview pass is O(n²) in the row count.",
			Run: func(e *Env, _ string) (int, int64, error) {
				n := importBatchSize(e)
				vcf := buildImportVCF(n)
				// ParseVCF reads the whole file and accumulates every parsed
				// contact + preview row before returning — the memory shape
				// this finding is about. Persist mirrors ConfirmVCF: one
				// transaction, one Create per row.
				parsed, _, _, err := services.ParseVCF(bytes.NewReader(vcf), e.DB, e.UserID)
				if err != nil {
					return 0, 0, fmt.Errorf("parse: %w", err)
				}
				created := 0
				err = e.DB.Transaction(func(tx *gorm.DB) error {
					for i := range parsed {
						c := *parsed[i].Contact
						c.ID = 0
						c.UserID = e.UserID
						if err := tx.Create(&c).Error; err != nil {
							return err
						}
						created++
					}
					return nil
				})
				return created, int64(len(vcf)), err
			},
		},
		{
			Name:                 "export.bundle",
			Category:             "export",
			ExpectedMemoryGrowth: GrowthLinear, // ExportData builds one bytes.Buffer for the whole multi-section dump.
			FreeSpaceNote:        "None (streamed to the client). Server RAM ≈ the whole serialized bundle — it is buffered before the first byte is sent.",
			Run: func(e *Env, _ string) (int, int64, error) {
				n, err := exportViaDiscard(e, func(c *gin.Context) { controllers.ExportData(c) }, "/export")
				return liveContactCount(e), n, err
			},
		},
		{
			Name:                 "export.vcard4",
			Category:             "export",
			ExpectedMemoryGrowth: GrowthLinear,
			FreeSpaceNote:        "None (streamed to the client). Server RAM ≈ the whole VCF, buffered before send.",
			Run: func(e *Env, _ string) (int, int64, error) {
				n, err := exportViaDiscard(e, func(c *gin.Context) {
					controllers.ExportContactsAsVCF(c, e.Cfg.ProfilePhotoDir)
				}, "/export/vcf")
				return liveContactCount(e), n, err
			},
		},
		{
			Name:                 "export.jscontact",
			Category:             "export",
			ExpectedMemoryGrowth: GrowthLinear,
			FreeSpaceNote:        "None (streamed to the client). Server RAM ≈ the whole JSContact set, buffered before send.",
			Run: func(e *Env, _ string) (int, int64, error) {
				n, err := exportViaDiscard(e, func(c *gin.Context) { controllers.ExportContactsAsJSContact(c) }, "/export/jscontact")
				return liveContactCount(e), n, err
			},
		},
		{
			Name:                 "fts.rebuild",
			Category:             "maintenance",
			Isolate:              true,
			Probe:                true,
			ExpectedMemoryGrowth: GrowthConstant, // INSERT ... SELECT inside SQLite; Go holds nothing.
			FreeSpaceNote:        "Transient: a second copy of the three FTS index rows in the -wal until the single rebuild transaction commits.",
			Run: func(e *Env, _ string) (int, int64, error) {
				stats, err := services.RebuildSearchIndexReport(e.DB)
				return int(stats.Total()), 0, err
			},
		},
		{
			Name:                 "backup.vacuum_into",
			Category:             "backup",
			Probe:                true, // expected: NOT stalled — VACUUM INTO reads through the WAL.
			ExpectedMemoryGrowth: GrowthConstant,
			FreeSpaceNote:        "A full second copy of the database: free space ≥ the live DB size (VACUUM INTO writes the whole file).",
			Run: func(e *Env, workDir string) (int, int64, error) {
				out := filepath.Join(workDir, "snapshot.db")
				if err := database.BackupSnapshot(e.dbPath, out); err != nil {
					return 0, 0, err
				}
				return liveContactCount(e), fileSize(out), nil
			},
		},
		{
			Name:                 "restore.snapshot",
			Category:             "restore",
			ExpectedMemoryGrowth: GrowthConstant,
			FreeSpaceNote:        "A full second copy of the database (the restore target). Photos and attachments are copied separately and may dominate at size — measured here as DB-only.",
			// Prepare produces the snapshot the Run restores, so the snapshot
			// cost is not folded into the restore measurement.
			Prepare: func(e *Env, workDir string) error {
				return database.BackupSnapshot(e.dbPath, filepath.Join(workDir, "snapshot.db"))
			},
			Run: func(_ *Env, workDir string) (int, int64, error) {
				src := filepath.Join(workDir, "snapshot.db")
				dst := filepath.Join(workDir, "restored.db")
				if err := copyFile(src, dst); err != nil {
					return 0, 0, err
				}
				// database.InitDB runs every pending migration — a snapshot
				// that is byte-valid SQLite but a schema behind still restores
				// to a bootable database (services/restore_drill_service.go's
				// own approach).
				restored, err := database.InitDB(dst)
				if err != nil {
					return 0, 0, err
				}
				rows := tableRowCount(restored, "contacts")
				closeGormNow(restored) //nolint:errcheck // scratch restore handle
				return rows, fileSize(dst), nil
			},
		},
		{
			Name:                 "delete_cascade.hub",
			Category:             "delete",
			Isolate:              true,
			Probe:                true,
			ExpectedMemoryGrowth: GrowthConstant, // enumerates dependent tables; deletes are SQL, not loaded.
			FreeSpaceNote:        "None. Runs in one transaction — see the write-lock column for how long other writes queue behind it.",
			Run: func(e *Env, _ string) (int, int64, error) {
				// "Rows touched" here is the size of the fan-out the cascade
				// walks: the hub's relationship edges (which scale with the
				// profile's HubFanout — the growth signal) plus the contact
				// row itself. It is resolved before the delete because the
				// cascade hard-deletes the edges.
				touched := hubEdgeCount(e) + 1
				if _, err := e.expect(e.del(fmt.Sprintf("/contacts/%d", e.HubContact.ID)), http.StatusOK); err != nil {
					return 0, 0, err
				}
				return touched, 0, nil
			},
		},
		{
			Name:                 "duplicates.find_pairs",
			Category:             "maintenance",
			ExpectedMemoryGrowth: GrowthSuperlinear, // O(group^2) pairs held in memory (PERF-02's recorded finding, here at bulk scale).
			FoldResultGrowth:     true,
			FreeSpaceNote:        "None on disk; peak RAM grows with the NUMBER OF PAIRS, which is quadratic in the size of a duplicate cluster.",
			Run: func(e *Env, _ string) (int, int64, error) {
				pairs, err := services.FindDuplicatePairs(e.DB, e.UserID)
				return len(pairs), 0, err
			},
		},
	}
}

// --- helpers -------------------------------------------------------------

// importBatchSize is how many contacts the synthetic import carries for this
// Env's profile. An import is one user loading their address book, so it is
// sized to the profile's PER-USER contact count (not the multi-tenant total),
// capped at what the product accepts.
func importBatchSize(e *Env) int { return capImportSize(e.Profile.Contacts) }

func capImportSize(n int) int {
	if n > maxImportContacts {
		return maxImportContacts
	}
	return n
}

// buildImportVCF renders n minimal-but-valid vCard 3.0 blocks. Each carries a
// name, an org, an address and a unique email/phone so the importer's
// per-row duplicate scan does real work and nothing collides with the fixture.
func buildImportVCF(n int) []byte {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "BEGIN:VCARD\r\nVERSION:3.0\r\n")
		fmt.Fprintf(&b, "FN:Perf Import %06d\r\n", i)
		fmt.Fprintf(&b, "N:Import;Perf%06d;;;\r\n", i)
		fmt.Fprintf(&b, "ORG:Perfbench Import Co\r\n")
		fmt.Fprintf(&b, "EMAIL;TYPE=INTERNET:perf.import.%06d@example.invalid\r\n", i)
		fmt.Fprintf(&b, "TEL;TYPE=CELL:+1206555%06d\r\n", i%1000000)
		fmt.Fprintf(&b, "ADR;TYPE=HOME:;;%d Perf St;Benchtown;WA;98%03d;USA\r\n", i, i%1000)
		fmt.Fprintf(&b, "END:VCARD\r\n")
	}
	return b.Bytes()
}

// discardResponseWriter is an http.ResponseWriter that throws the body away
// and counts its length. The export controllers assemble the whole response
// in an internal bytes.Buffer before writing it — measuring them through an
// httptest.ResponseRecorder would double that buffer in the recorder and
// confuse the peak-memory reading, which is the entire thing PERF-03 measures.
type discardResponseWriter struct {
	header http.Header
	n      int64
	status int
}

func (w *discardResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *discardResponseWriter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	return len(p), nil
}

func (w *discardResponseWriter) WriteHeader(status int) { w.status = status }

// exportViaDiscard invokes one export controller against e's counting DB with
// a body-discarding writer, and returns the byte length it would have sent.
func exportViaDiscard(e *Env, handler func(*gin.Context), path string) (int64, error) {
	w := &discardResponseWriter{}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	c.Set("db", e.DB)
	c.Set("userID", e.UserID)
	c.Set("cfg", e.Cfg)
	handler(c)
	if w.status != 0 && w.status != http.StatusOK {
		return w.n, fmt.Errorf("export %s: status %d", path, w.status)
	}
	return w.n, nil
}

// liveContactCount is the number of non-deleted contacts owned by e's user —
// the "rows touched" figure for an export.
func liveContactCount(e *Env) int {
	var n int64
	e.seedDB.Model(&models.Contact{}).Where("user_id = ?", e.UserID).Count(&n)
	return int(n)
}

// hubEdgeCount is how many relationship edges name e's hub contact as either
// endpoint — the fan-out DeleteContact's cascade repoints/removes, and the
// figure that scales with a profile's HubFanout.
func hubEdgeCount(e *Env) int {
	uid := e.HubContact.VCardUID
	var n int64
	e.seedDB.Table("relationship_edges").
		Where("source_id = ? OR target_id = ?", uid, uid).
		Count(&n)
	return int(n)
}

// fileSize is the byte size of one file, or 0 if it is missing.
func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// tableRowCount counts every row in a table on db (unscoped raw count).
func tableRowCount(db *gorm.DB, table string) int {
	var n int64
	db.Table(table).Count(&n)
	return int(n)
}
