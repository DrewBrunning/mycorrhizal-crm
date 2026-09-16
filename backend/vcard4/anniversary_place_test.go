package vcard4

import (
	"strings"
	"testing"

	vcard "github.com/emersion/go-vcard"

	"mycorrhizal/contactmodel"
)

// placeTextToAddress/addressToPlaceValue implement the place_text transform
// for anniversary.place.birth/death (BIRTHPLACE/DEATHPLACE <-> Address).
// import_anniversaries_test.go/export_anniversaries_test.go already exercise
// the plain-text and geo:-coordinates branches through the full
// Adapter{}.Import/Export path; these are direct unit tests of the two
// helpers themselves, covering the remaining branches: a non-geo: VALUE=uri
// scheme (passthrough, not a neutral field) and the lossy
// components-only join on export.

func TestPlaceTextToAddress(t *testing.T) {
	t.Parallel()

	t.Run("plain_text_becomes_full", func(t *testing.T) {
		t.Parallel()
		var diags []contactmodel.Diagnostic
		var pt []contactmodel.JCardProp
		f := &vcard.Field{Value: "Babies'R'Us Hospital"}
		addr := placeTextToAddress(PropBirthplace, f, &diags, &pt)
		if addr == nil || addr.Full != "Babies'R'Us Hospital" {
			t.Errorf("addr = %+v, want Full = Babies'R'Us Hospital", addr)
		}
		if len(diags) != 0 {
			t.Errorf("diags = %+v, want none", diags)
		}
		if len(pt) != 0 {
			t.Errorf("pt = %+v, want none", pt)
		}
	})

	t.Run("value_uri_geo_becomes_coordinates", func(t *testing.T) {
		t.Parallel()
		var diags []contactmodel.Diagnostic
		var pt []contactmodel.JCardProp
		f := &vcard.Field{Value: "geo:41.7325,-49.9469", Params: vcard.Params{}}
		f.Params.Add(ParamValue, "uri")
		addr := placeTextToAddress(PropDeathplace, f, &diags, &pt)
		if addr == nil || addr.Coordinates != "geo:41.7325,-49.9469" {
			t.Errorf("addr = %+v, want Coordinates = geo:41.7325,-49.9469", addr)
		}
		if len(diags) != 0 {
			t.Errorf("diags = %+v, want none", diags)
		}
		if len(pt) != 0 {
			t.Errorf("pt = %+v, want none", pt)
		}
	})

	t.Run("value_uri_geo_is_case_insensitive", func(t *testing.T) {
		t.Parallel()
		var diags []contactmodel.Diagnostic
		var pt []contactmodel.JCardProp
		f := &vcard.Field{Value: "geo:1,2", Params: vcard.Params{}}
		f.Params.Add(ParamValue, "URI")
		addr := placeTextToAddress(PropBirthplace, f, &diags, &pt)
		if addr == nil || addr.Coordinates != "geo:1,2" {
			t.Errorf("addr = %+v, want Coordinates = geo:1,2", addr)
		}
	})

	t.Run("value_uri_non_geo_scheme_goes_to_passthrough", func(t *testing.T) {
		t.Parallel()
		var diags []contactmodel.Diagnostic
		var pt []contactmodel.JCardProp
		f := &vcard.Field{Value: "https://example.com/hospital", Params: vcard.Params{}}
		f.Params.Add(ParamValue, "uri")
		addr := placeTextToAddress(PropBirthplace, f, &diags, &pt)
		if addr != nil {
			t.Errorf("addr = %+v, want nil (no neutral field for a non-geo: URI)", addr)
		}
		if len(pt) != 1 || !strings.EqualFold(pt[0].Name, PropBirthplace) {
			t.Fatalf("pt = %+v, want one passthrough entry named %s", pt, PropBirthplace)
		}
		if len(diags) != 1 || diags[0].Severity != "info" {
			t.Errorf("diags = %+v, want one info diagnostic", diags)
		}
	})
}

func TestAddressToPlaceValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		addr          *contactmodel.Address
		wantValue     string
		wantValueParm string
	}{
		{name: "nil_address", addr: nil, wantValue: "", wantValueParm: ""},
		{
			name:          "full_takes_priority",
			addr:          &contactmodel.Address{Full: "City Hospital", Coordinates: "geo:1,2"},
			wantValue:     "City Hospital",
			wantValueParm: "",
		},
		{
			name:          "coordinates_when_no_full",
			addr:          &contactmodel.Address{Coordinates: "geo:41.7325,-49.9469"},
			wantValue:     "geo:41.7325,-49.9469",
			wantValueParm: "uri",
		},
		{
			name: "components_joined_lossily_when_no_full_or_coordinates",
			addr: &contactmodel.Address{Components: []contactmodel.AddressComponent{
				{Kind: "locality", Value: "Southampton"},
				{Kind: "country", Value: "UK"},
			}},
			wantValue:     "Southampton, UK",
			wantValueParm: "",
		},
		{
			name: "components_skip_empty_values",
			addr: &contactmodel.Address{Components: []contactmodel.AddressComponent{
				{Kind: "locality", Value: ""},
				{Kind: "country", Value: "UK"},
			}},
			wantValue:     "UK",
			wantValueParm: "",
		},
		{
			name:          "nothing_set",
			addr:          &contactmodel.Address{},
			wantValue:     "",
			wantValueParm: "",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotValue, gotParm := addressToPlaceValue(tt.addr)
			if gotValue != tt.wantValue || gotParm != tt.wantValueParm {
				t.Errorf("addressToPlaceValue(%+v) = (%q, %q), want (%q, %q)",
					tt.addr, gotValue, gotParm, tt.wantValue, tt.wantValueParm)
			}
		})
	}
}
