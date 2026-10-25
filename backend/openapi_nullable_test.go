package main

import (
	"strconv"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestOpenAPINullableRequiresType pins the OAS 3.0 rule that `nullable` only
// takes effect when `type` is explicit. A nullable composed schema
// (`nullable: true` + allOf, no type) is what Spectral's
// oas3-valid-media-example rule caught on the target-closeness example; vacuum
// does not check examples, so this small structural guard keeps that regression
// out of the spec. Adding `type: object` to the composed schema is the fix.
func TestOpenAPINullableRequiresType(t *testing.T) {
	doc := loadOpenAPIDoc(t)

	visited := map[*openapi3.Schema]bool{}
	var walk func(t *testing.T, where string, s *openapi3.SchemaRef)
	walk = func(t *testing.T, where string, s *openapi3.SchemaRef) {
		t.Helper()
		if s == nil || s.Value == nil {
			return
		}
		v := s.Value
		if visited[v] {
			return
		}
		visited[v] = true

		if v.Nullable && v.Type == nil {
			t.Errorf("%s: nullable is set without an explicit type (OAS 3.0 ignores nullable unless type is present)", where)
		}
		for name, p := range v.Properties {
			walk(t, where+".properties."+name, p)
		}
		if v.Items != nil {
			walk(t, where+".items", v.Items)
		}
		for i, sub := range v.AllOf {
			walk(t, where+".allOf["+strconv.Itoa(i)+"]", sub)
		}
		for i, sub := range v.AnyOf {
			walk(t, where+".anyOf["+strconv.Itoa(i)+"]", sub)
		}
		for i, sub := range v.OneOf {
			walk(t, where+".oneOf["+strconv.Itoa(i)+"]", sub)
		}
	}

	for name, s := range doc.Components.Schemas {
		walk(t, "components.schemas."+name, s)
	}
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for method, op := range item.Operations() {
			if op == nil {
				continue
			}
			where := method + " " + path
			for _, p := range op.Parameters {
				if p != nil && p.Value != nil {
					walk(t, where+".parameter."+p.Value.Name, p.Value.Schema)
				}
			}
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for mt, media := range op.RequestBody.Value.Content {
					walk(t, where+".requestBody."+mt, media.Schema)
				}
			}
			for status, resp := range op.Responses.Map() {
				if resp == nil || resp.Value == nil {
					continue
				}
				for mt, media := range resp.Value.Content {
					walk(t, where+"."+status+"."+mt, media.Schema)
				}
			}
		}
	}
}
