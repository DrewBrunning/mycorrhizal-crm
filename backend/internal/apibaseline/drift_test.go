package apibaseline

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContractBaselineHolds is the MAINT-02 (issue #491) drift gate: the live
// spec must be a SUPERSET of the committed frozen baseline. Removing an
// endpoint, dropping a parameter, deleting a schema field, shrinking an enum,
// or narrowing a parameter is a breaking change and fails here; additive
// changes (new endpoints, fields, parameters, enum values) pass without
// touching the baseline.
//
// Regenerate the baseline deliberately — `cd backend && go run
// ./cmd/genapibaseline` — which is the act of declaring a breaking change, not
// a routine build step (see the package doc and docs/breaking-change-policy.md).
func TestContractBaselineHolds(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)

	baseline, err := Read(root)
	require.NoError(t, err, "the committed baseline must parse")

	doc, err := Load(filepath.Join(root, "backend", SpecPath))
	require.NoError(t, err)

	problems := CheckSuperset(baseline, Extract(doc))
	for _, p := range problems {
		t.Errorf("breaking change: %s", p)
	}
}

// TestContractBaselineIsCurrent pins that the committed baseline is not stale
// in the other direction: a change that only ADDS surface must still be
// committed to the baseline, so the snapshot always records what the spec
// currently promises (additive changes are cheap to record — regenerate after
// adding a field or endpoint too, so a later removal of that field is caught).
func TestContractBaselineIsCurrent(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)

	doc, err := Load(filepath.Join(root, "backend", SpecPath))
	require.NoError(t, err)

	// The regenerated baseline must be byte-identical to the committed one.
	regenerated, err := Extract(doc).Marshal()
	require.NoError(t, err)

	path := filepath.Join(root, "backend", "internal", "apibaseline", BaselineFile)
	committed, err := readFileBytes(path)
	require.NoError(t, err, "committed baseline must be readable at %s", path)
	require.Equal(t, string(committed), string(regenerated),
		"the committed baseline is stale — regenerate with `cd backend && go run ./cmd/genapibaseline`")
}

// TestContractBaselineHasContent guards the drift test against silently
// operating on an empty or degenerate baseline (the same vacuous-pass risk the
// route-coverage drift test guards with its sanity floor): a baseline that
// records nothing proves nothing.
func TestContractBaselineHasContent(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)

	baseline, err := Read(root)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(baseline.Operations), 100,
		"baseline records an unexpectedly small operation surface — the extractor may be broken")
	require.GreaterOrEqual(t, len(baseline.Schemas), 50,
		"baseline records an unexpectedly small schema surface — the extractor may be broken")
}

// TestCheckSupersetDetectsEachBreakClass is the hand-verified unit test for
// the detector itself: each breaking-change class (operation removal,
// parameter removal, parameter narrowing, request-body change, response
// removal, response schema change, schema removal, field removal, field type
// change, field required-narrowing, enum shrink) must produce a diagnostic,
// and each additive change (new endpoint, new parameter, new response, new
// field, new enum value, field widening) must pass.
func TestCheckSupersetDetectsEachBreakClass(t *testing.T) {
	base := &Baseline{
		Version: BaselineVersion,
		Operations: []OperationSurface{
			{
				Method: "GET", Path: "/contacts",
				Parameters: []ParameterSurface{
					{Name: "search", In: "query", Required: false},
					{Name: "limit", In: "query", Required: true},
				},
				Responses: []ResponseSurface{
					{Status: "200", Schema: "ref:ContactSummary"},
					{Status: "401"},
				},
			},
			{
				Method: "POST", Path: "/contacts",
				RequestBody: "ref:ContactRecordInput",
				Responses:   []ResponseSurface{{Status: "201", Schema: "ref:ContactSummary"}},
			},
		},
		Schemas: []SchemaSurface{
			{
				Name: "ContactSummary",
				Properties: []PropertySurface{
					{Name: "id", Type: "integer", Required: true},
					{Name: "firstname", Type: "string"},
					{Name: "archived", Type: "boolean", Required: true},
				},
			},
			{Name: "Mode", Enum: []any{"a", "b"}},
		},
	}

	clone := func() *Baseline {
		b, err := Parse(mustMarshal(t, base))
		require.NoError(t, err)
		return b
	}

	t.Run("identical surface passes", func(t *testing.T) {
		assert.Empty(t, CheckSuperset(base, clone()))
	})

	t.Run("operation removal is flagged", func(t *testing.T) {
		cur := clone()
		cur.Operations = cur.Operations[1:]
		assert.Contains(t, CheckSuperset(base, cur), "operation GET /contacts was removed")
	})

	t.Run("parameter removal is flagged", func(t *testing.T) {
		cur := clone()
		cur.Operations[0].Parameters = []ParameterSurface{
			{Name: "limit", In: "query", Required: true},
		}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "parameter query search on GET /contacts was removed")
	})

	t.Run("parameter narrowing is flagged", func(t *testing.T) {
		cur := clone()
		cur.Operations[0].Parameters = []ParameterSurface{
			{Name: "search", In: "query", Required: true},
			{Name: "limit", In: "query", Required: true},
		}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "parameter query search on GET /contacts became required (narrowing)")
	})

	t.Run("parameter widening passes", func(t *testing.T) {
		cur := clone()
		cur.Operations[0].Parameters = []ParameterSurface{
			{Name: "search", In: "query"},
			{Name: "limit", In: "query"},
		}
		assert.Empty(t, CheckSuperset(base, cur))
	})

	t.Run("request body schema change is flagged", func(t *testing.T) {
		cur := clone()
		// Find the POST /contacts operation (index 1 after clone sorts back in
		// the same order — both baseline and current parse identically).
		for i := range cur.Operations {
			if cur.Operations[i].Method == "POST" && cur.Operations[i].Path == "/contacts" {
				cur.Operations[i].RequestBody = "ref:SomethingElse"
			}
		}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "request body schema on POST /contacts changed from ref:ContactRecordInput to ref:SomethingElse")
	})

	t.Run("response removal is flagged", func(t *testing.T) {
		cur := clone()
		cur.Operations[0].Responses = []ResponseSurface{{Status: "200", Schema: "ref:ContactSummary"}}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "response 401 on GET /contacts was removed")
	})

	t.Run("response schema change is flagged", func(t *testing.T) {
		cur := clone()
		cur.Operations[0].Responses = []ResponseSurface{
			{Status: "200", Schema: "ref:ContactList"},
			{Status: "401"},
		}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "response 200 schema on GET /contacts changed from ref:ContactSummary to ref:ContactList")
	})

	t.Run("schema removal is flagged", func(t *testing.T) {
		cur := clone()
		cur.Schemas = cur.Schemas[1:]
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "schema ContactSummary was removed")
	})

	t.Run("field removal is flagged", func(t *testing.T) {
		cur := clone()
		cur.Schemas[0].Properties = []PropertySurface{
			{Name: "id", Type: "integer", Required: true},
			{Name: "firstname", Type: "string"},
		}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "field ContactSummary.archived was removed")
	})

	t.Run("field type change is flagged", func(t *testing.T) {
		cur := clone()
		cur.Schemas[0].Properties = []PropertySurface{
			{Name: "id", Type: "string", Required: true},
			{Name: "firstname", Type: "string"},
			{Name: "archived", Type: "boolean", Required: true},
		}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "field ContactSummary.id type changed from integer to string")
	})

	t.Run("field required-narrowing is flagged", func(t *testing.T) {
		cur := clone()
		cur.Schemas[0].Properties = []PropertySurface{
			{Name: "id", Type: "integer", Required: true},
			{Name: "firstname", Type: "string", Required: true},
			{Name: "archived", Type: "boolean", Required: true},
		}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "field ContactSummary.firstname became required (narrowing)")
	})

	t.Run("field widening passes", func(t *testing.T) {
		cur := clone()
		cur.Schemas[0].Properties = []PropertySurface{
			{Name: "id", Type: "integer"},
			{Name: "firstname", Type: "string"},
			{Name: "archived", Type: "boolean", Required: true},
		}
		assert.Empty(t, CheckSuperset(base, cur))
	})

	t.Run("enum shrink is flagged", func(t *testing.T) {
		cur := clone()
		cur.Schemas[1].Enum = []any{"a"}
		msgs := CheckSuperset(base, cur)
		assert.Contains(t, msgs, "enum value b on schema Mode was removed")
	})

	t.Run("additive changes pass", func(t *testing.T) {
		cur := clone()
		cur.Operations = append(cur.Operations, OperationSurface{Method: "GET", Path: "/brand-new"})
		cur.Operations[0].Parameters = append(cur.Operations[0].Parameters, ParameterSurface{Name: "new_param", In: "query"})
		cur.Operations[0].Responses = append(cur.Operations[0].Responses, ResponseSurface{Status: "429"})
		cur.Schemas = append(cur.Schemas, SchemaSurface{Name: "NewSchema", Properties: []PropertySurface{{Name: "x", Type: "string"}}})
		cur.Schemas[0].Properties = append(cur.Schemas[0].Properties, PropertySurface{Name: "new_field", Type: "string"})
		cur.Schemas[1].Enum = append(cur.Schemas[1].Enum, "c")
		assert.Empty(t, CheckSuperset(base, cur))
	})
}
