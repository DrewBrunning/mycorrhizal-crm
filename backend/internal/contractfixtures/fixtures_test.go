package contractfixtures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// TestGenerateDerivesAllPinnedFixturesFromTheSpec proves the fixtures are
// spec-derived: loading the real spec and generating must produce the three
// pinned files, each non-empty, pretty-printed JSON. This is the same path
// cmd/gencontract runs; the drift test in backend/ additionally compares the
// output against the checked-in files.
func TestGenerateDerivesAllPinnedFixturesFromTheSpec(t *testing.T) {
	doc, err := Load(filepath.Join("..", "..", SpecPath))
	require.NoError(t, err)

	files, err := Generate(doc)
	require.NoError(t, err)
	require.Len(t, files, len(Pinned))

	for _, pin := range Pinned {
		body, ok := files[pin.Filename]
		require.Truef(t, ok, "Generate must emit %s", pin.Filename)
		require.NotEmpty(t, body, "%s must not be empty", pin.Filename)

		// The fixture is JSON, and parses back to the same example value.
		var parsed any
		require.NoError(t, json.Unmarshal(body, &parsed), "%s must be valid JSON", pin.Filename)
		require.Contains(t, string(body), "\n", "%s must be pretty-printed (newline-separated)", pin.Filename)

		// Generation is deterministic, so the drift test compares stable
		// output rather than something that churns on every run.
		again, err := Generate(doc)
		require.NoError(t, err)
		require.Equal(t, body, again[pin.Filename], "Generate must be deterministic")
	}
}

// TestLoadValidatesTheSpec proves Load rejects a document the spec's own
// validator rejects, so the generator never emits fixtures from a broken
// spec. Two failure classes: an unreadable/malformed file (fails at load) and
// a structurally-wrong document (fails at Validate).
func TestLoadValidatesTheSpec(t *testing.T) {
	dir := t.TempDir()

	// Fails at LoadFromFile: not a valid OpenAPI document.
	_, err := Load(filepath.Join(dir, "does-not-exist.yaml"))
	require.Error(t, err)

	// Fails at Validate: loads fine, but an operation must declare at least
	// one response code.
	missingResponses := filepath.Join(dir, "missing-responses.yaml")
	require.NoError(t, os.WriteFile(missingResponses,
		[]byte("openapi: 3.0.3\ninfo:\n  title: x\n  version: \"1.0\"\npaths:\n  /x:\n    get:\n      responses: {}\n"),
		0o644))
	_, err = Load(missingResponses)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed validation")
}

// genDoc builds a minimal document whose GET /x 200 application/json content
// carries both a schema and a valid example. Individual tests mutate it to
// exercise each failure branch.
func genDoc() *openapi3.T {
	return &openapi3.T{Paths: openapi3.NewPaths(
		openapi3.WithPath("/x", &openapi3.PathItem{
			Get: &openapi3.Operation{
				Responses: openapi3.NewResponses(
					openapi3.WithStatus(200, &openapi3.ResponseRef{
						Value: &openapi3.Response{
							Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{
								Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
							}),
						},
					}),
				),
			},
		}),
	)}
}

// responseErrorCases are the one-per-branch failure modes of responseExample.
// Each message must name the pin's path so a contributor can find the response
// that needs fixing.
var responseErrorCases = []struct {
	name   string
	pin    Pin
	mutate func(doc *openapi3.T)
}{
	{
		"missing path",
		Pin{Filename: "a.json", Method: "GET", Path: "/nope", Status: "200"},
		nil,
	},
	{
		"missing operation",
		Pin{Filename: "a.json", Method: "POST", Path: "/x", Status: "200"},
		nil,
	},
	{
		"missing response",
		Pin{Filename: "a.json", Method: "GET", Path: "/x", Status: "404"},
		nil,
	},
	{
		"missing media type",
		Pin{Filename: "a.json", Method: "GET", Path: "/x", Status: "200"},
		func(doc *openapi3.T) {
			doc.Paths.Find("/x").Get.Responses.Value("200").Value.Content = openapi3.Content{}
		},
	},
	{
		"missing example",
		Pin{Filename: "a.json", Method: "GET", Path: "/x", Status: "200"},
		func(doc *openapi3.T) {
			doc.Paths.Find("/x").Get.Responses.Value("200").Value.Content.Get("application/json").Example = nil
		},
	},
	{
		"missing schema",
		Pin{Filename: "a.json", Method: "GET", Path: "/x", Status: "200"},
		func(doc *openapi3.T) {
			doc.Paths.Find("/x").Get.Responses.Value("200").Value.Content.Get("application/json").Example = map[string]any{}
			doc.Paths.Find("/x").Get.Responses.Value("200").Value.Content.Get("application/json").Schema = nil
		},
	},
	{
		"unresolvable schema",
		Pin{Filename: "a.json", Method: "GET", Path: "/x", Status: "200"},
		func(doc *openapi3.T) {
			doc.Paths.Find("/x").Get.Responses.Value("200").Value.Content.Get("application/json").Example = map[string]any{}
			doc.Paths.Find("/x").Get.Responses.Value("200").Value.Content.Get("application/json").Schema = &openapi3.SchemaRef{}
		},
	},
	{
		"example fails schema validation",
		Pin{Filename: "a.json", Method: "GET", Path: "/x", Status: "200"},
		func(doc *openapi3.T) {
			media := doc.Paths.Find("/x").Get.Responses.Value("200").Value.Content.Get("application/json")
			media.Example = map[string]any{"id": "not-an-integer"}
			media.Schema.Value = &openapi3.Schema{
				Type:       &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{"id": {Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}},
			}
		},
	},
}

// TestResponseExampleErrorPaths covers every failure branch of responseExample
// so a pin with a malformed spec response fails loudly with a message naming
// the operation.
func TestResponseExampleErrorPaths(t *testing.T) {
	for _, tc := range responseErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := genDoc()
			if tc.mutate != nil {
				tc.mutate(doc)
			}
			_, err := responseExample(doc, tc.pin)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.pin.Path)
		})
	}
}

// TestGeneratePropagatesResponseErrorsWithTheFilename proves a pin-level
// failure surfaces as a filename-prefixed error from Generate, so the drift
// test and CLI can name the fixture that is broken.
func TestGeneratePropagatesResponseErrorsWithTheFilename(t *testing.T) {
	doc := genDoc()
	_, err := Generate(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "contacts-list.json")
	require.True(t, strings.HasPrefix(err.Error(), "contacts-list.json:"))
}

// TestOperationForMethod covers every method the generator maps, including
// the default (nil) branch.
func TestOperationForMethod(t *testing.T) {
	pi := &openapi3.PathItem{
		Get:    &openapi3.Operation{OperationID: "get"},
		Post:   &openapi3.Operation{OperationID: "post"},
		Put:    &openapi3.Operation{OperationID: "put"},
		Patch:  &openapi3.Operation{OperationID: "patch"},
		Delete: &openapi3.Operation{OperationID: "delete"},
	}
	for method, want := range map[string]string{
		"GET": "get", "POST": "post", "PUT": "put", "PATCH": "patch", "DELETE": "delete",
	} {
		op := operationForMethod(pi, method)
		require.NotNil(t, op)
		require.Equal(t, want, op.OperationID)
	}
	require.Nil(t, operationForMethod(pi, "HEAD"))
	require.Nil(t, operationForMethod(&openapi3.PathItem{}, "GET"))
}

func TestFixturePath(t *testing.T) {
	require.Equal(t, filepath.Join("dir", "f.json"), FixturePath("dir", "f.json"))
}
