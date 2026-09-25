package main

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/tsapi"
)

// TestGeneratedTSTypesMatchSpec pins frontend/src/generated/openapi.ts (the
// TypeScript mirror of openapi.yaml's components.schemas) to the spec. The
// file is generated, never hand-edited: a schema change without regenerating
// fails here with the exact command to re-run. The frontend's
// src/api/contractConformance.ts then checks the hand-written API types
// against the regenerated file at `tsc --noEmit` time.
func TestGeneratedTSTypesMatchSpec(t *testing.T) {
	doc := loadOpenAPIDoc(t)

	want, err := tsapi.Generate(doc)
	if err != nil {
		t.Fatalf("generating TS types from spec: %v", err)
	}
	got, err := os.ReadFile(filepath.Join("..", tsapi.OutputPath))
	if err != nil {
		t.Fatalf("missing checked-in %s: %v (regenerate with `cd backend && go run ./cmd/gentsapi`)", tsapi.OutputPath, err)
	}
	if string(got) != string(want) {
		t.Errorf("%s is stale: it no longer matches openapi.yaml components.schemas — "+
			"regenerate with `cd backend && go run ./cmd/gentsapi`", tsapi.OutputPath)
	}
}
