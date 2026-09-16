package vcard4

import (
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
)

// jCardPropToField is fieldToJCardProp's inverse (adapter.go), used when
// re-emitting Passthrough.VCard entries on export. It is core to the
// ADR-backed "preserve unknown properties" guarantee: a regression here is a
// silent data-loss bug for round-tripped contacts, not just a coverage gap.
// These are direct unit tests of the function itself (unlike
// import_passthrough_test.go/export_passthrough_test.go, which exercise it
// indirectly through Adapter{}.Export) so each branch of its value-decoding
// and param type-switch can be pinned precisely.

func TestJCardPropToField_Name(t *testing.T) {
	t.Parallel()
	name, f := jCardPropToField(contactmodel.JCardProp{Name: "clientpidmap", Value: json.RawMessage(`""`)})
	if name != "CLIENTPIDMAP" {
		t.Errorf("name = %q, want CLIENTPIDMAP", name)
	}
	if f == nil {
		t.Fatalf("f = nil")
	}
}

func TestJCardPropToField_ValueDecoding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		rawValue  json.RawMessage
		wantValue string
	}{
		// A jCard string value unmarshals cleanly.
		{name: "json_string", rawValue: json.RawMessage(`"hello"`), wantValue: "hello"},
		{name: "json_string_with_escapes", rawValue: json.RawMessage(`"a\"b"`), wantValue: `a"b`},
		// Not a JSON string: json.Unmarshal into a string fails, so the
		// fallback branch stores the raw bytes verbatim instead.
		{name: "json_number_fallback", rawValue: json.RawMessage(`42`), wantValue: "42"},
		{name: "json_array_fallback", rawValue: json.RawMessage(`["a","b"]`), wantValue: `["a","b"]`},
		{name: "json_object_fallback", rawValue: json.RawMessage(`{"a":1}`), wantValue: `{"a":1}`},
		{name: "malformed_json_fallback", rawValue: json.RawMessage(`not-json`), wantValue: "not-json"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, f := jCardPropToField(contactmodel.JCardProp{Name: "x", Value: tt.rawValue})
			if f.Value != tt.wantValue {
				t.Errorf("f.Value = %q, want %q", f.Value, tt.wantValue)
			}
		})
	}
}

func TestJCardPropToField_GroupParam(t *testing.T) {
	t.Parallel()

	t.Run("string_group_sets_field_group", func(t *testing.T) {
		t.Parallel()
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"group": "g1"},
		})
		if f.Group != "g1" {
			t.Errorf("f.Group = %q, want g1", f.Group)
		}
		if len(f.Params) != 0 {
			t.Errorf("f.Params = %+v, want empty (group must not also become a param)", f.Params)
		}
	})

	t.Run("group_key_is_case_insensitive", func(t *testing.T) {
		t.Parallel()
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"GROUP": "g2"},
		})
		if f.Group != "g2" {
			t.Errorf("f.Group = %q, want g2", f.Group)
		}
	})

	t.Run("non_string_group_value_is_dropped", func(t *testing.T) {
		t.Parallel()
		// The type assertion v.(string) fails for a non-string group value;
		// the loop iteration still `continue`s (skipping the param-map
		// path entirely) rather than falling through to store it as an
		// ordinary parameter.
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"group": true},
		})
		if f.Group != "" {
			t.Errorf("f.Group = %q, want empty (non-string group value must be dropped, not stringified)", f.Group)
		}
		if len(f.Params) != 0 {
			t.Errorf("f.Params = %+v, want empty", f.Params)
		}
	})
}

func TestJCardPropToField_ParamTypeSwitch(t *testing.T) {
	t.Parallel()

	t.Run("string_param", func(t *testing.T) {
		t.Parallel()
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"pid": "1.1"},
		})
		got := f.Params["PID"]
		if len(got) != 1 || got[0] != "1.1" {
			t.Errorf("f.Params[PID] = %v, want [1.1]", got)
		}
	})

	t.Run("string_slice_param", func(t *testing.T) {
		t.Parallel()
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"type": []string{"home", "work"}},
		})
		got := f.Params["TYPE"]
		if len(got) != 2 || got[0] != "home" || got[1] != "work" {
			t.Errorf("f.Params[TYPE] = %v, want [home work]", got)
		}
	})

	t.Run("any_slice_param_all_strings", func(t *testing.T) {
		t.Parallel()
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"foo": []any{"a", "b"}},
		})
		got := f.Params["FOO"]
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("f.Params[FOO] = %v, want [a b]", got)
		}
	})

	t.Run("any_slice_param_skips_non_string_items", func(t *testing.T) {
		t.Parallel()
		// A []any item that fails the `item.(string)` assertion (e.g. a
		// decoded JSON number) is silently skipped rather than stringified
		// or causing an error.
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"foo": []any{"a", float64(7), "b"}},
		})
		got := f.Params["FOO"]
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("f.Params[FOO] = %v, want [a b] (numeric item dropped)", got)
		}
	})

	t.Run("unsupported_param_value_type_is_dropped", func(t *testing.T) {
		t.Parallel()
		// A param value that is none of string/[]string/[]any (e.g. a bare
		// float64, which is what a decoded JSON number becomes) matches no
		// switch case: f.Params is still initialized (non-nil) but the key
		// itself must never appear.
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"foo": float64(42)},
		})
		if _, ok := f.Params["FOO"]; ok {
			t.Errorf("f.Params[FOO] = %v, want key absent for an unsupported param value type", f.Params["FOO"])
		}
	})

	t.Run("param_name_uppercased", func(t *testing.T) {
		t.Parallel()
		_, f := jCardPropToField(contactmodel.JCardProp{
			Name:   "x",
			Value:  json.RawMessage(`""`),
			Params: map[string]any{"language": "en"},
		})
		got := f.Params["LANGUAGE"]
		if len(got) != 1 || got[0] != "en" {
			t.Errorf("f.Params[LANGUAGE] = %v, want [en]", got)
		}
	})
}
