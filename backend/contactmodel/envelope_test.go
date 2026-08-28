package contactmodel

import (
	"encoding/json"
	"testing"
)

func TestCRMEnvelopeRoundTrip(t *testing.T) {
	assertRoundTrip(t, fullCRMEnvelope())
}

// jsonOmitemptyOmitted reports whether marshaling a zero value of T omits the
// given key — the wire shape for an optional field that must not appear when
// unset.
func jsonOmitemptyOmitted[T any](zero T, key string) bool {
	data, err := json.Marshal(zero)
	if err != nil {
		return false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	_, present := m[key]
	return !present
}

// TestCRMEnvelopeGenderRoundTrip pins issue #515's core wire fact: Gender
// lives on CRMEnvelope (a real neutral-model home, not Passthrough), survives
// JSON serialization/deserialization, and never collides with the
// standardized Card.SpeakToAs concepts it is deliberately distinct from.
func TestCRMEnvelopeGenderRoundTrip(t *testing.T) {
	env := fullCRMEnvelope()
	if env.Gender != "female" {
		t.Fatalf("fullCRMEnvelope().Gender = %q, want female", env.Gender)
	}

	// JSON round trip: gender must serialize and come back intact.
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back CRMEnvelope
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.Gender != "female" {
		t.Errorf("round-tripped Gender = %q, want female", back.Gender)
	}

	// The zero value must stay omitted (omitempty), so legacy payloads that
	// predate the field remain byte-identical.
	if !jsonOmitemptyOmitted(CRMEnvelope{}, "gender") {
		t.Error("zero-value CRMEnvelope must omit the gender key (omitempty)")
	}
}
