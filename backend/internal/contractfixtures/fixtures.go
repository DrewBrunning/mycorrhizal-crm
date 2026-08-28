// Package contractfixtures generates the shared web+Android contract-test
// fixtures from backend/openapi.yaml's response examples (issue #266).
//
// Issue #257 shipped a small set of *hand-captured* real responses under
// testdata/contract-fixtures/ that both client suites parse: web via a direct
// TS import (frontend/src/api/contractFixtures.test.ts), Android via a
// resources.srcDirs entry (android/core/network/.../ContractFixtureTest.kt).
// This package replaces the hand-capture with a spec-derived single source of
// truth: each pinned response's `example:` in the OpenAPI spec IS the fixture,
// pretty-printed verbatim into the shared directory. A backend spec change is
// now "edit the example in openapi.yaml, run `go run ./cmd/gencontract`", and
// both client suites react to the regenerated files — the maintenance-churn
// loop the issue existed to break.
//
// The spec's own validator (openapi_test.go's loadOpenAPIDoc, which runs
// doc.Validate) already rejects an example that does not fit its schema; this
// package re-validates with VisitJSON as a belt-and-suspenders so the
// generator cannot emit a fixture the schema contradicts.
//
// Fixtures live in testdata/contract-fixtures/ (already the shared location
// both clients point at) rather than docs/contracts/ — moving them would
// churn both clients' wiring for no behavioral gain, and testdata/ is the
// conventional home for checked-in test fixtures.
package contractfixtures

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"
)

// Pin is one checked-in fixture file and the spec response it derives from.
type Pin struct {
	Filename string // filename under FixturesDir
	Method   string // HTTP method
	Path     string // spec path (already in {param} form)
	Status   string // response status code
}

// Pinned are the contract fixtures the web and Android suites share. Adding a
// pinned response means: add an `example:` to that response in
// backend/openapi.yaml, add a Pin here, regenerate
// (`cd backend && go run ./cmd/gencontract`), and wire the new file into both
// client suites.
var Pinned = []Pin{
	{Filename: "contacts-list.json", Method: "GET", Path: "/contacts", Status: "200"},
	{Filename: "contact-detail.json", Method: "GET", Path: "/contacts/{id}/detail", Status: "200"},
	{Filename: "dashboard.json", Method: "GET", Path: "/dashboard", Status: "200"},
}

// FixturesDir is the shared checked-in fixture directory, repo-relative.
const FixturesDir = "testdata/contract-fixtures"

// SpecPath is the OpenAPI document the fixtures derive from, backend-relative.
const SpecPath = "openapi.yaml"

// Load parses and validates the spec. The doc is validated (which includes
// example-vs-schema checks in kin-openapi) so the generator and the drift
// test never operate on a broken document.
func Load(specPath string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", specPath, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("%s failed validation: %w", specPath, err)
	}
	return doc, nil
}

// Generate extracts every pinned response's example from doc, validates it
// against that response's schema, and returns the pretty-printed JSON for
// each fixture filename.
func Generate(doc *openapi3.T) (map[string][]byte, error) {
	out := make(map[string][]byte, len(Pinned))
	for _, pin := range Pinned {
		example, err := responseExample(doc, pin)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pin.Filename, err)
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(example); err != nil {
			// An example always comes from a YAML-parsed spec document
			// (objects/arrays/scalars), which json.Encoder.Encode can never
			// fail on, so no test can reach this.
			return nil, fmt.Errorf("%s: encoding example: %w", pin.Filename, err) // # pragma: no cover
		}
		out[pin.Filename] = buf.Bytes()
	}
	return out, nil
}

// responseExample resolves a pin's operation → response → application/json
// media type, returns its example, and verifies the example fits the response
// schema.
func responseExample(doc *openapi3.T, pin Pin) (any, error) {
	pi := doc.Paths.Find(pin.Path)
	if pi == nil {
		return nil, fmt.Errorf("spec has no path %s", pin.Path)
	}
	op := operationForMethod(pi, pin.Method)
	if op == nil {
		return nil, fmt.Errorf("spec has no %s operation for %s", pin.Method, pin.Path)
	}
	respRef := op.Responses.Value(pin.Status)
	if respRef == nil {
		return nil, fmt.Errorf("spec %s %s has no %s response", pin.Method, pin.Path, pin.Status)
	}
	media := respRef.Value.Content.Get("application/json")
	if media == nil {
		return nil, fmt.Errorf("spec %s %s %s has no application/json content", pin.Method, pin.Path, pin.Status)
	}
	if media.Example == nil {
		return nil, fmt.Errorf("spec %s %s %s has no example — add one to %s", pin.Method, pin.Path, pin.Status, SpecPath)
	}
	if media.Schema == nil || media.Schema.Value == nil {
		return nil, fmt.Errorf("spec %s %s %s has no resolvable schema", pin.Method, pin.Path, pin.Status)
	}
	if err := media.Schema.Value.VisitJSON(media.Example); err != nil {
		return nil, fmt.Errorf("spec %s %s %s example fails schema validation: %w", pin.Method, pin.Path, pin.Status, err)
	}
	return media.Example, nil
}

// operationForMethod returns the PathItem's operation for the given HTTP
// method, or nil if the path item has none.
func operationForMethod(pi *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case "GET":
		return pi.Get
	case "POST":
		return pi.Post
	case "PUT":
		return pi.Put
	case "PATCH":
		return pi.Patch
	case "DELETE":
		return pi.Delete
	}
	return nil
}

// FixturePath returns the absolute path of a fixture filename under dir.
func FixturePath(dir, filename string) string {
	return filepath.Join(dir, filename)
}
