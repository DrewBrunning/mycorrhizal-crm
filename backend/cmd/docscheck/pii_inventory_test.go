package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPIIInventoryFindings(t *testing.T) {
	doc := "# Inventory\n\n" +
		"| Store | Personal data |\n|---|---|\n" +
		"| `contacts` (+ nested json) | names |\n" +
		"| `users.password` | bcrypt hash |\n" +
		"### 3.1 Search (`contacts_fts`)\n" +
		"Prose mentioning `orphan_table` is not an inventory row.\n" +
		"\n## Changelog\n\n| Date | Change |\n|---|---|\n| 2026-01-01 | added `history_only` |\n"
	tables := []string{
		"contacts", "users", "contacts_fts", "contacts_fts_data", "contacts_fts_idx",
		"schema_migrations", "sqlite_sequence", "orphan_table", "brand_new", "history_only",
	}
	got := strings.Join(piiInventoryFindings(tables, doc), "\n")
	for _, want := range []string{`"orphan_table"`, `"brand_new"`, `"history_only"`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected a finding for %s, got:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{`"contacts"`, `"users"`, `"contacts_fts"`, `"contacts_fts_data"`, `"schema_migrations"`, `"sqlite_sequence"`} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected finding for %s:\n%s", unwanted, got)
		}
	}
}

// TestPIIInventoryMatchesMigratedSchema is the real gate: the committed
// inventory covers every table the migration chain creates.
func TestPIIInventoryMatchesMigratedSchema(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got := checkPIIInventory(root); len(got) != 0 {
		t.Fatalf("pii-inventory drifted from the schema:\n%s", strings.Join(got, "\n"))
	}
}

func TestCheckPIIInventorySkipsTreeWithoutInventory(t *testing.T) {
	if got := checkPIIInventory(t.TempDir()); got != nil {
		t.Fatalf("a tree with no inventory should yield no findings, got %v", got)
	}
}

func TestCheckPIIInventoryReportsMissingTables(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "security")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pii-inventory.md"), []byte("# empty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(checkPIIInventory(root), "\n")
	if !strings.Contains(got, `"contacts"`) || !strings.Contains(got, `"webauthn_credentials"`) {
		t.Fatalf("an empty inventory must flag every migrated table, got:\n%s", got)
	}
}
