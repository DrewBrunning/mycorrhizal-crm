package contactgen

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
	"mycorrhizal/internal/canonicalfixture"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// cardJSONTags maps Card Go field names to their JSON tags (the keys
// SupportedCardFields uses), derived from the struct so the two cannot drift.
func cardJSONTags() map[string]string {
	typ := reflect.TypeOf(contactmodel.Card{})
	m := make(map[string]string, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			m[f.Name] = tag
		}
	}
	return m
}

func populatedCardFields(c contactmodel.Card) []string {
	v := reflect.ValueOf(c)
	typ := v.Type()
	var out []string
	for i := 0; i < v.NumField(); i++ {
		if isZero(v.Field(i)) {
			continue
		}
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			out = append(out, tag)
		}
	}
	return out
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

// TestSupportedSurfaceMatchesCorrespondenceOracle pins the generator's shape
// to the correspondence oracle (the model description TEST-02's manifest is
// validated against by the TEST-03 consumer). A concept the table declares
// with a Card.* neutral path must be one this generator can populate — a new
// concept fails here until the generator learns it, so no round-trip-relevant
// field can silently go untested by every generative property.
func TestSupportedSurfaceMatchesCorrespondenceOracle(t *testing.T) {
	jsonTag := cardJSONTags()
	supported := make(map[string]bool, len(SupportedCardFields))
	for _, f := range SupportedCardFields {
		supported[f] = true
	}

	for _, row := range correspondence.Load() {
		if !strings.HasPrefix(row.NeutralPath, "Card.") {
			continue
		}
		rest := strings.TrimPrefix(row.NeutralPath, "Card.")
		fieldName := rest
		if idx := strings.IndexAny(rest, ".["); idx >= 0 {
			fieldName = rest[:idx]
		}
		tag, ok := jsonTag[fieldName]
		require.True(t, ok, "correspondence concept %q (%s) names unknown Card field %q", row.ConceptID, row.NeutralPath, fieldName)
		require.True(t, supported[tag],
			"correspondence concept %q (%s) has no generator support: add %q to SupportedCardFields or the generator will silently never test it",
			row.ConceptID, row.NeutralPath, tag)
	}
}

// TestManifestSurfaceIsGeneratorCoverable asserts the "cannot drift into
// describing different models" direction that matters most: every Card field
// any TEST-02 manifest contact populates is one this generator can produce.
// A fixture record the generator cannot express means a generative suite and
// the fixture have drifted apart.
func TestManifestSurfaceIsGeneratorCoverable(t *testing.T) {
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	supported := make(map[string]bool, len(SupportedCardFields))
	for _, f := range SupportedCardFields {
		supported[f] = true
	}

	for _, entry := range m.Contacts {
		for _, f := range populatedCardFields(entry.Card) {
			require.True(t, supported[f],
				"manifest contact %q populates card field %q, which the generator cannot produce (generator and fixture drifted)",
				entry.Name, f)
		}
	}
}

// TestRecord_ExercisesEverySupportedField proves the generator is not
// decorative: across generated records every SupportedCardFields entry is
// actually populated. Accumulated across the whole check run (a single
// unlucky draw must not fail the test), so with RAPID_CHECKS checks the
// probability any field is never drawn is negligible.
func TestRecord_ExercisesEverySupportedField(t *testing.T) {
	var (
		mu   sync.Mutex
		seen = map[string]bool{}
	)
	t.Run("surface", rapid.MakeCheck(func(t *rapid.T) {
		rec := Record(t)
		for _, f := range populatedCardFields(rec.Card) {
			mu.Lock()
			seen[f] = true
			mu.Unlock()
		}
	}))
	for _, f := range SupportedCardFields {
		require.True(t, seen[f], "generated records never populated supported card field %q", f)
	}
}

// TestRecord_WellFormed pins the structural invariants generated records must
// hold for every serialized format to accept them without a hard error:
// an AnniversaryDate carries exactly one of partial/timestamp, timestamps are
// RFC3339, and partial-date components stay in range.
func TestRecord_WellFormed(t *testing.T) {
	t.Run("anniversary", rapid.MakeCheck(func(t *rapid.T) {
		rec := Record(t)
		for _, a := range rec.Card.Anniversaries {
			if (a.Date.Partial == nil) == (a.Date.Timestamp == nil) {
				t.Fatalf("anniversary date must carry exactly one of partial/timestamp, got %+v", a.Date)
			}
			if p := a.Date.Partial; p != nil {
				if p.Year != nil && (*p.Year < 0 || *p.Year > 9999) {
					t.Fatalf("partial year out of range: %d", *p.Year)
				}
				if p.Month != nil && (*p.Month < 1 || *p.Month > 12) {
					t.Fatalf("partial month out of range: %d", *p.Month)
				}
				if p.Day != nil && (*p.Day < 1 || *p.Day > 31) {
					t.Fatalf("partial day out of range: %d", *p.Day)
				}
			}
		}
	}))
	t.Run("timestamp", rapid.MakeCheck(func(t *rapid.T) {
		ts := Timestamp(t)
		if _, err := time.Parse(time.RFC3339, ts.UTC); err != nil {
			t.Fatalf("generated timestamp %q is not RFC3339: %v", ts.UTC, err)
		}
	}))
}

// TestRecord_JSONMarshalable asserts every generated record survives the
// neutral model's own JSON round trip — a shape the wire/serializer chokes on
// would fail downstream consumers before the format adapters even run.
func TestRecord_JSONMarshalable(t *testing.T) {
	t.Run("json", rapid.MakeCheck(func(t *rapid.T) {
		rec := Record(t)
		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("generated record is not JSON-marshalable: %v\n%+v", err, rec)
		}
		var back contactmodel.Record
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("generated record is not JSON-unmarshalable: %v", err)
		}
	}))
}

// TestText_ExercisesPathologyClasses asserts the free-text generator covers
// every data pathology the TEST-02 manifest documents (README "Data
// pathologies"): empty, whitespace-only, ordinary, Unicode (combining marks /
// RTL / emoji) and very long strings. Accumulated across the check run for
// the same reason as TestRecord_ExercisesEverySupportedField.
func TestText_ExercisesPathologyClasses(t *testing.T) {
	var (
		mu                 sync.Mutex
		empty, ws, long, u bool
	)
	t.Run("text", rapid.MakeCheck(func(t *rapid.T) {
		s := Text(t)
		mu.Lock()
		defer mu.Unlock()
		if s == "" {
			empty = true
		}
		if s != "" && strings.TrimSpace(s) == "" {
			ws = true
		}
		if utf8.RuneCountInString(s) >= 300 {
			long = true
		}
		if !isASCII(s) {
			u = true
		}
	}))
	require.True(t, empty, "Text never generated an empty string")
	require.True(t, ws, "Text never generated a whitespace-only string")
	require.True(t, long, "Text never generated a very long (>=300 rune) string")
	require.True(t, u, "Text never generated a Unicode string")
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// TestRecords_UIDsAreDistinct pins the contract the DB properties rely on:
// Records returns non-empty, mutually-distinct card UIDs, so Populate cannot
// trip the partial unique VCardUID index.
func TestRecords_UIDsAreDistinct(t *testing.T) {
	t.Run("distinct", rapid.MakeCheck(func(t *rapid.T) {
		recs := Records(t, 8)
		seen := map[string]bool{}
		for _, r := range recs {
			if r.Card.UID == "" {
				t.Fatalf("Records returned a record with an empty UID")
			}
			if seen[r.Card.UID] {
				t.Fatalf("Records returned duplicate UID %q", r.Card.UID)
			}
			seen[r.Card.UID] = true
		}
	}))
}
