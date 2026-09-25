package tsapi

import (
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/contractfixtures"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func schemaDoc(schemas map[string]*openapi3.SchemaRef) *openapi3.T {
	return &openapi3.T{Components: &openapi3.Components{Schemas: schemas}}
}

func val(s *openapi3.Schema) *openapi3.SchemaRef { return &openapi3.SchemaRef{Value: s} }

func typ(t string) *openapi3.Types { return &openapi3.Types{t} }

func TestGenerateRealSpecIsDeterministic(t *testing.T) {
	doc, err := contractfixtures.Load(filepath.Join("..", "..", contractfixtures.SpecPath))
	require.NoError(t, err)
	a, err := Generate(doc)
	require.NoError(t, err)
	b, err := Generate(doc)
	require.NoError(t, err)
	require.Equal(t, a, b)
	require.True(t, strings.HasPrefix(string(a), Header))
	require.Contains(t, string(a), "export type Note = {")
}

// TestGenerateMapping pins every OpenAPI→TS mapping rule the package doc
// promises, on a synthetic document.
func TestGenerateMapping(t *testing.T) {
	nullableStr := &openapi3.Schema{Type: typ("string"), Nullable: true}
	doc := schemaDoc(map[string]*openapi3.SchemaRef{
		"Base": val(&openapi3.Schema{Type: typ("object"), Properties: openapi3.Schemas{"id": val(&openapi3.Schema{Type: typ("integer")})}}),
		"Obj": val(&openapi3.Schema{
			Type:     typ("object"),
			Required: []string{"req"},
			Properties: openapi3.Schemas{
				"req":        val(&openapi3.Schema{Type: typ("string")}),
				"opt":        val(nullableStr),
				"num":        val(&openapi3.Schema{Type: typ("number")}),
				"flag":       val(&openapi3.Schema{Type: typ("boolean")}),
				"kind":       val(&openapi3.Schema{Type: typ("string"), Enum: []any{"a", "b'c", nil}, Nullable: true}),
				"level":      val(&openapi3.Schema{Type: typ("integer"), Enum: []any{float64(1), float64(2)}}),
				"list":       val(&openapi3.Schema{Type: typ("array"), Items: &openapi3.SchemaRef{Ref: "#/components/schemas/Base"}}),
				"enumList":   val(&openapi3.Schema{Type: typ("array"), Items: val(&openapi3.Schema{Type: typ("string"), Enum: []any{"x", "y"}})}),
				"free":       val(&openapi3.Schema{Type: typ("object")}),
				"bag":        val(&openapi3.Schema{Type: typ("object"), AdditionalProperties: openapi3.AdditionalProperties{Schema: val(&openapi3.Schema{Type: typ("integer")})}}),
				"any":        val(&openapi3.Schema{}),
				"merged":     val(&openapi3.Schema{AllOf: openapi3.SchemaRefs{{Ref: "#/components/schemas/Base"}}, Nullable: true}),
				"either":     val(&openapi3.Schema{OneOf: openapi3.SchemaRefs{{Ref: "#/components/schemas/Base"}, val(&openapi3.Schema{Type: typ("string")})}}),
				"some":       val(&openapi3.Schema{AnyOf: openapi3.SchemaRefs{val(&openapi3.Schema{Type: typ("string"), Enum: []any{"p", "q"}}), val(&openapi3.Schema{Type: typ("integer")})}}),
				"weird-name": val(&openapi3.Schema{Type: typ("string")}),
				"nested": val(&openapi3.Schema{Type: typ("object"), Properties: openapi3.Schemas{
					"inner": val(&openapi3.Schema{Type: typ("boolean")}),
				}}),
			},
		}),
		"Both": val(&openapi3.Schema{AllOf: openapi3.SchemaRefs{
			{Ref: "#/components/schemas/Base"},
			val(&openapi3.Schema{Type: typ("object"), Properties: openapi3.Schemas{"x": val(&openapi3.Schema{Type: typ("string")})}}),
		}, Nullable: true}),
	})
	out, err := Generate(doc)
	require.NoError(t, err)
	s := string(out)
	for _, want := range []string{
		"export type Base = {\n  id?: number;\n};",
		"  req: string;\n",
		"  opt?: string | null;\n",
		"  num?: number;\n",
		"  flag?: boolean;\n",
		"  kind?: 'a' | 'b\\'c' | null;\n",
		"  level?: 1 | 2;\n",
		"  list?: Base[];\n",
		"  enumList?: ('x' | 'y')[];\n",
		"  free?: Record<string, unknown>;\n",
		"  bag?: Record<string, number>;\n",
		"  any?: unknown;\n",
		"  merged?: Base | null;\n",
		"  either?: Base | string;\n",
		"  some?: ('p' | 'q') | number;\n",
		"  'weird-name'?: string;\n",
		"  nested?: {\n    inner?: boolean;\n  };\n",
		"export type Both = (Base & {\n  x?: string;\n}) | null;",
	} {
		require.Contains(t, s, want)
	}
	// Sorted, deterministic order.
	require.Less(t, strings.Index(s, "export type Base"), strings.Index(s, "export type Both"))
	require.Less(t, strings.Index(s, "export type Both"), strings.Index(s, "export type Obj"))
}

func TestGenerateErrors(t *testing.T) {
	cases := map[string]*openapi3.T{
		"no schemas":        {},
		"bad name":          schemaDoc(map[string]*openapi3.SchemaRef{"bad-name": val(&openapi3.Schema{})}),
		"unresolved":        schemaDoc(map[string]*openapi3.SchemaRef{"X": {}}),
		"external ref":      schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{Type: typ("array"), Items: &openapi3.SchemaRef{Ref: "other.yaml#/X"}})}),
		"unresolved inline": schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{Type: typ("array"), Items: &openapi3.SchemaRef{}})}),
		"multi type":        schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{Type: &openapi3.Types{"string", "null"}})}),
		"unknown type":      schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{Type: typ("file")})}),
		"bad enum":          schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{Enum: []any{[]any{1}}})}),
		"bad required": schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{
			Type: typ("object"), Required: []string{"missing"},
			Properties: openapi3.Schemas{"a": val(&openapi3.Schema{Type: typ("string")})},
		})}),
		"bad property": schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{
			Type: typ("object"), Properties: openapi3.Schemas{"a": val(&openapi3.Schema{Type: typ("file")})},
		})}),
		"bad allOf":      schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{AllOf: openapi3.SchemaRefs{val(&openapi3.Schema{Type: typ("file")})}})}),
		"bad additional": schemaDoc(map[string]*openapi3.SchemaRef{"X": val(&openapi3.Schema{Type: typ("object"), AdditionalProperties: openapi3.AdditionalProperties{Schema: val(&openapi3.Schema{Type: typ("file")})}})}),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Generate(doc)
			require.Error(t, err)
		})
	}
}

func TestRenderRefNil(t *testing.T) {
	got, err := renderRef(nil, 0)
	require.NoError(t, err)
	require.Equal(t, "unknown", got)
}

func TestHasTopLevelOperator(t *testing.T) {
	require.False(t, hasTopLevelOperator("{ a: 'x' | 'y' }", '|'))
	require.False(t, hasTopLevelOperator("'a|b'", '|'))
	require.False(t, hasTopLevelOperator(`'it\'s|'`, '|'))
	require.True(t, hasTopLevelOperator("A | B", '|'))
	require.False(t, hasTopLevelOperator("Record<string, A | B>", '|'))
}

func TestLiteral(t *testing.T) {
	got, err := literal(true)
	require.NoError(t, err)
	require.Equal(t, "true", got)
}
