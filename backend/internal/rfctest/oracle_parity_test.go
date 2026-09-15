package rfctest

import (
	"os"
	"path/filepath"
	"testing"
)

// goldenFixturesDirRel is docs/golden-fixtures/ relative to this package
// (backend/internal/rfctest -> repo root is three levels up). It cannot be
// embedded like fixtures/ since it lives outside the backend module tree.
const goldenFixturesDirRel = "../../../docs/golden-fixtures"

// TestEmbeddedFixturesMatchGoldenOracle pins ADR-0003's claim that
// fixtures/ is docs/golden-fixtures/ "copied VERBATIM" — the adapters read
// the embedded copy at test time, so nothing catches the two drifting apart
// on their own (issue #977). SOURCES.md is excluded: the two directories'
// provenance tables deliberately differ in content and structure (the
// embedded copy documents a second layer of per-concept fixtures that have
// no golden/RFC-verbatim counterpart), only the fixture files themselves are
// required to be byte-identical.
func TestEmbeddedFixturesMatchGoldenOracle(t *testing.T) {
	entries, err := os.ReadDir(goldenFixturesDirRel)
	if err != nil {
		t.Fatalf("reading %s: %v", goldenFixturesDirRel, err)
	}

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SOURCES.md" {
			continue
		}
		name := entry.Name()
		checked++

		want, err := os.ReadFile(filepath.Join(goldenFixturesDirRel, name))
		if err != nil {
			t.Errorf("%s: reading golden oracle copy: %v", name, err)
			continue
		}

		got, err := fixturesFS.ReadFile("fixtures/" + name)
		if err != nil {
			t.Errorf("%s: present in docs/golden-fixtures/ but not embedded at backend/internal/rfctest/fixtures/ (%v) — ADR-0003 requires the embedded copy to exist and be byte-identical", name, err)
			continue
		}

		if string(got) != string(want) {
			t.Errorf("%s: backend/internal/rfctest/fixtures/%s has drifted from the docs/golden-fixtures/ oracle — ADR-0003 requires golden fixtures to be copied verbatim, never hand-edited", name, name)
		}
	}

	if checked == 0 {
		t.Fatalf("found no fixture files under %s; test is not exercising anything", goldenFixturesDirRel)
	}
}
