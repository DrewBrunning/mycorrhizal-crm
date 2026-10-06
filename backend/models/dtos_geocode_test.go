package models

import "testing"

func TestGeocodeDraftInput_ToContactAddress(t *testing.T) {
	in := &GeocodeDraftInput{
		Street:      "1 Main St",
		City:        "Springfield",
		Region:      "IL",
		Postal:      "62701",
		Country:     "US",
		Sensitivity: "private",
	}
	got := in.ToContactAddress()
	want := ContactAddress{
		Street:      "1 Main St",
		City:        "Springfield",
		Region:      "IL",
		Postal:      "62701",
		Country:     "US",
		Sensitivity: "private",
	}
	if got != want {
		t.Fatalf("ToContactAddress() = %+v, want %+v", got, want)
	}
	// A draft lookup never invents an id or a coordinate; those stay empty so
	// the handler cannot accidentally persist a resolved point from the body.
	if got.ID != "" || got.Coordinates != "" {
		t.Fatalf("draft must not invent id/coordinates: %+v", got)
	}
}
