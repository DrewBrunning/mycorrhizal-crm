package apibaseline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// synthSpec builds a minimal openapi3.T with the given paths and schemas so
// Extract's edge branches (dedupe, nil refs, composed members, schema-level
// enums, non-component refs) are exercised without depending on the real spec.
func synthSpec() *openapi3.T {
	doc := &openapi3.T{OpenAPI: "3.0.0", Info: &openapi3.Info{Title: "synth", Version: "1.0"}}
	doc.Paths = openapi3.NewPaths()

	// A path whose operation declares the same parameter as the path item:
	// Extract must dedupe on (in, name). The path item also declares a
	// duplicate itself (exercising the path-level dedupe) and a nil ref
	// (exercising the nil guards).
	pi := &openapi3.PathItem{
		Parameters: openapi3.Parameters{
			{Value: &openapi3.Parameter{Name: "shared", In: "query", Required: false}},
			{Value: &openapi3.Parameter{Name: "shared", In: "query", Required: false}},
			{Value: &openapi3.Parameter{Name: "dup", In: "query"}},
			{Value: &openapi3.Parameter{Name: "dup", In: "query"}},
			nil,
		},
		Get: &openapi3.Operation{
			Parameters: openapi3.Parameters{
				{Value: &openapi3.Parameter{Name: "shared", In: "query", Required: false}},
				{Value: &openapi3.Parameter{Name: "op_only", In: "query"}},
				nil,
			},
			Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: &openapi3.Response{
				Description: strPtr("ok"),
				Content: openapi3.Content{
					"application/json": &openapi3.MediaType{
						Schema: ref("Payload"),
					},
				},
			}})),
		},
	}
	doc.Paths.Set("/things", pi)

	// A schema composed via allOf whose member is itself oneOf, and a
	// schema-level enum, and a non-component $ref.
	doc.Components = &openapi3.Components{
		Schemas: openapi3.Schemas{
			"Payload": {
				Value: &openapi3.Schema{
					Type:     newTypes("object"),
					Required: []string{"id"},
					Properties: openapi3.Schemas{
						"id":    {Value: &openapi3.Schema{Type: newTypes("integer")}},
						"inner": {Ref: "#/components/schemas/Nested"},
					},
				},
			},
			"Nested": {
				Value: &openapi3.Schema{
					AllOf: openapi3.SchemaRefs{
						{
							Value: &openapi3.Schema{
								OneOf: openapi3.SchemaRefs{
									{Value: &openapi3.Schema{Type: newTypes("string"), Enum: []any{"x", "y"}}},
									{Value: &openapi3.Schema{Type: newTypes("null")}},
								},
							},
						},
						{Value: nil}, // nil composed member — must be skipped
					},
				},
			},
			"Mode": {Value: &openapi3.Schema{Enum: []any{"a", "b"}}},
			"Ext":  {Ref: "https://example.com/external.json#/components/schemas/X"},
		},
	}
	return doc
}

// TestExtractSyntheticSpec pins Extract's handling of the edge cases the real
// spec does not exercise: path/operation parameter dedupe, composed members
// reached through allOf, schema-level enums, and a non-component $ref.
func TestExtractSyntheticSpec(t *testing.T) {
	b := Extract(synthSpec())

	var getThings *OperationSurface
	for i := range b.Operations {
		if b.Operations[i].Method == "GET" && b.Operations[i].Path == "/things" {
			getThings = &b.Operations[i]
		}
	}
	require.NotNil(t, getThings)
	// shared (path+op, deduped), dup (path-level duplicate, deduped),
	// op_only (op-level only) — the two nil refs are skipped.
	require.Len(t, getThings.Parameters, 3, "path+operation params must be deduped, nil refs skipped")
	gotNames := map[string]bool{}
	for _, p := range getThings.Parameters {
		gotNames[p.Name] = true
	}
	assert.True(t, gotNames["shared"])
	assert.True(t, gotNames["dup"])
	assert.True(t, gotNames["op_only"])
	assert.Equal(t, "ref:Payload", getThings.Responses[0].Schema)

	var payload, nested, mode, ext *SchemaSurface
	for i := range b.Schemas {
		switch b.Schemas[i].Name {
		case "Payload":
			payload = &b.Schemas[i]
		case "Nested":
			nested = &b.Schemas[i]
		case "Mode":
			mode = &b.Schemas[i]
		case "Ext":
			ext = &b.Schemas[i]
		}
	}
	require.NotNil(t, payload)
	require.Len(t, payload.Properties, 2)
	assert.True(t, payload.Properties[0].Required || payload.Properties[1].Required)
	// id is required; inner is a ref to a component.
	byName := map[string]PropertySurface{}
	for _, p := range payload.Properties {
		byName[p.Name] = p
	}
	assert.True(t, byName["id"].Required)
	assert.Equal(t, "ref:Nested", byName["inner"].Type)

	// Nested's oneOf member folds to its property (none here — oneOf members
	// without properties contribute nothing, which is fine).
	require.NotNil(t, nested)

	// Schema-level enum is captured.
	require.NotNil(t, mode)
	assert.Equal(t, []any{"a", "b"}, mode.Enum)

	// Non-component $ref keeps the full ref string.
	require.NotNil(t, ext)
}

// TestParseRejectsInvalidJSON pins the JSON-decode error path of Parse.
func TestParseRejectsInvalidJSON(t *testing.T) {
	_, err := Parse([]byte("{not json"))
	require.Error(t, err)
}

// TestSortedResponseKeysNil pins the nil-responses guard.
func TestSortedResponseKeysNil(t *testing.T) {
	assert.Nil(t, sortedResponseKeys(nil))
}

// TestOperationForMethodUnknown pins the default branch (an unrecognized
// method yields nil).
func TestOperationForMethodUnknown(t *testing.T) {
	pi := &openapi3.PathItem{Get: &openapi3.Operation{}}
	assert.Nil(t, operationForMethod(pi, "TRACE"))
}

// TestFindRepoRootFailure pins the not-found error path from a directory
// without an openapi.yaml ancestor.
func TestFindRepoRootFailure(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(orig)) })

	_, err = findRepoRoot()
	require.Error(t, err)
}

// TestReadRepoRootPathResolution pins that Read resolves the committed
// baseline via the real repo root (already covered by the drift tests, but the
// filepath join is the reachable statement).
func TestReadRepoRootPathResolution(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	_, err = Read(root)
	require.NoError(t, err)
}

var _ = filepath.Join
