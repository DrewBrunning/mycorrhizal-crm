package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mycorrhizal/database"
)

// ---------------------------------------------------------------------------
// 7. PII-inventory ↔ schema drift (issue #1316)
// ---------------------------------------------------------------------------
//
// docs/security/pii-inventory.md walks the schema table by table. It drifted
// once already (22 migrated tables — several holding personal data — were
// never mentioned), because nothing tied it to the migrations. This check
// migrates a scratch database with the app's own migration chain and requires
// every resulting table to appear in the inventory: as a code span in a table
// row or heading (a bare mention in prose is not an inventory entry), or in
// the reasoned exclusion list below.

const piiInventoryPath = "docs/security/pii-inventory.md"

// piiExcludedTables are tables that are not inventory rows, each with the
// reason. Adding a table here needs a reason that says why it can never hold
// personal data — "no PII" belongs in the inventory itself (the doc lists the
// no-PII operational tables with that verdict), so this list is for tables
// that are not application data stores at all.
var piiExcludedTables = map[string]string{
	"schema_migrations": "golang-migrate's own version bookkeeping (one row: version + dirty flag)",
	"sqlite_sequence":   "SQLite's internal AUTOINCREMENT counter table",
}

// ftsShadowTable matches the storage tables SQLite creates behind an FTS5
// virtual table. The virtual table itself (contacts_fts, notes_fts,
// activities_fts) is inventoried in §3.1; its shadow tables are the same
// data, not separate stores.
var ftsShadowTable = regexp.MustCompile(`_fts_(config|content|data|docsize|idx)$`)

var codeSpan = regexp.MustCompile("`([^`]+)`")
var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// migratedTables returns every table in a freshly migrated database.
func migratedTables() ([]string, error) {
	dir, err := os.MkdirTemp("", "docscheck-schema-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	db, err := database.InitDB(filepath.Join(dir, "schema.db"))
	if err != nil {
		return nil, fmt.Errorf("migrating scratch database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	defer func() { _ = sqlDB.Close() }()
	var names []string
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name").Scan(&names).Error; err != nil {
		return nil, err
	}
	return names, nil
}

// inventoriedNames returns every identifier that appears inside a code span on
// a markdown table row or heading line of the inventory. `notes.content` and
// `users.password` therefore inventory `notes` and `users`.
func inventoriedNames(doc string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "|") && !strings.HasPrefix(t, "#") {
			continue
		}
		for _, m := range codeSpan.FindAllStringSubmatch(t, -1) {
			for _, id := range identifier.FindAllString(m[1], -1) {
				out[id] = true
			}
		}
	}
	return out
}

func checkPIIInventory(root string) []string {
	// #nosec G304 -- root is the repo root from findRepoRoot, the leaf is a constant
	doc, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(piiInventoryPath)))
	if err != nil {
		return nil // no inventory in this tree (a fixture): nothing to hold to the schema
	}
	tables, err := migratedTables()
	if err != nil {
		return []string{"pii-inventory: could not derive the migrated schema: " + err.Error()}
	}
	return piiInventoryFindings(tables, string(doc))
}

// piiInventoryFindings is the pure comparison, split out so tests can drive it
// with a synthetic table list.
func piiInventoryFindings(tables []string, doc string) []string {
	have := inventoriedNames(doc)
	var findings []string
	for _, tbl := range tables {
		if _, ok := piiExcludedTables[tbl]; ok || ftsShadowTable.MatchString(tbl) || have[tbl] {
			continue
		}
		findings = append(findings, fmt.Sprintf(
			"pii-inventory: migrated table %q is not in %s — add a row (`%s` in a table row or heading) stating its personal data, necessity and retention/deletion, or, if it is not an application data store, add it to piiExcludedTables in cmd/docscheck/pii_inventory.go with a reason (issue #1316)",
			tbl, piiInventoryPath, tbl))
	}
	sort.Strings(findings)
	return findings
}
