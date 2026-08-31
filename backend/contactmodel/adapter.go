package contactmodel

// Diagnostic is a non-fatal data-handling event (see degradation policy). It
// is the shared loss-reporting type (DATA-02, issue #442): both the export
// loss reports and the import preview diagnostics carry its JSON shape
// verbatim — the wire tags keep the two surfaces on one shape rather than a
// parallel one.
type Diagnostic struct {
	Severity string `json:"severity"`          // "warn" | "info"
	Concept  string `json:"concept,omitempty"` // correspondence concept_id, or ""
	Message  string `json:"message"`
}

// Importer parses one serialized format into the neutral model.
// It MUST NOT return an error for unmappable/unknown data — it preserves it
// (passthrough) and appends a Diagnostic. errors are reserved for malformed input
// (bytes that are not valid instances of the format at all).
type Importer interface {
	Import(raw []byte) (*Record, []Diagnostic, error)
}

// Exporter renders the neutral model into one serialized format.
// Same rule: never error on a field that has no home in the target format —
// drop-with-warning or passthrough, and append a Diagnostic.
type Exporter interface {
	Export(r *Record) ([]byte, []Diagnostic, error)
}
