package main

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/contractfixtures"
)

// TestContractFixturesMatchSpec pins the shared contract fixtures
// (testdata/contract-fixtures/, consumed by web's contractFixtures.test.ts
// and Android's ContractFixtureTest.kt) to backend/openapi.yaml's response
// examples (issue #266).
//
// The fixtures are generated, not hand-captured — each pinned response's
// `example:` in the spec IS the fixture. So a spec example change without
// regenerating the fixtures fails here, with the exact command to re-run:
//
//	cd backend && go run ./cmd/gencontract
//
// That is the whole point of the issue: a backend contract change is one edit
// to openapi.yaml plus one regeneration, and both client suites react to the
// same regenerated files. The spec's own validator (loadOpenAPIDoc) already
// rejects an example that no longer fits its schema, so a schema change that
// invalidates a fixture is caught twice.
func TestContractFixturesMatchSpec(t *testing.T) {
	doc := loadOpenAPIDoc(t)

	files, err := contractfixtures.Generate(doc)
	if err != nil {
		t.Fatalf("generating fixtures from spec: %v", err)
	}

	fixturesDir := filepath.Join("..", "testdata", "contract-fixtures")
	for _, pin := range contractfixtures.Pinned {
		got, err := os.ReadFile(filepath.Join(fixturesDir, pin.Filename))
		if err != nil {
			t.Errorf("missing checked-in fixture %s: %v (regenerate with `cd backend && go run ./cmd/gencontract`)", pin.Filename, err)
			continue
		}
		want := files[pin.Filename]
		if string(got) != string(want) {
			t.Errorf("%s is stale: its content no longer matches the %s %s %s example in openapi.yaml — "+
				"regenerate with `cd backend && go run ./cmd/gencontract`",
				pin.Filename, pin.Method, pin.Path, pin.Status)
		}
	}
}
