package contactmodel

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNameComponentRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullNameComponent())
}

func TestNameRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullName())
}

func TestNicknameRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullNickname())
}

func TestOrgUnitRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullOrgUnit())
}

func TestOrganizationRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullOrganization())
}

func TestTitleRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullTitle())
}

func TestEmailRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullEmail())
}

func TestPhoneRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullPhone())
}

func TestOnlineServiceRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullOnlineService())
}

func TestAddressComponentRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullAddressComponent())
}

func TestAddressRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullAddress())
}

func TestPartialDateRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullPartialDate())
}

func TestAnniversaryDateRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullAnniversaryDate())
}

func TestAnniversaryRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullAnniversary())
}

func TestSpeakToAsRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullSpeakToAs())
}

func TestGrammaticalGenderRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullGrammaticalGender())
}

func TestPronounsRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullPronouns())
}

func TestPersonalInfoRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullPersonalInfo())
}

func TestAuthorRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullAuthor())
}

func TestNoteRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullNote())
}

func TestResourceRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullResource())
}

func TestLanguagePrefRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullLanguagePref())
}

func TestRelationRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullRelation())
}

func TestTimestampRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullTimestamp())
}

func TestCardRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullCard())
}

// TestRecordRoundTrip verifies the JSON-serialized parts of Record (Card,
// Envelope, Passthrough) survive a round trip. Record.UID and Record.ETag are
// deliberately `json:"-"` (persistence-layer identity, not part of the Card
// payload preamble) so they are — by design — NOT expected to
// survive a JSON marshal/unmarshal cycle; a plain reflect.DeepEqual of the
// whole literal would therefore incorrectly fail on those two fields alone.
func TestRecordRoundTrip(t *testing.T) {
	t.Parallel()
	original := fullRecord()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\njson: %s", err, data)
	}

	if got.UID != "" || got.ETag != "" {
		t.Errorf("expected UID/ETag to NOT survive round-trip (json:\"-\"), got UID=%q ETag=%q", got.UID, got.ETag)
	}

	// Zero out the intentionally-non-serialized fields before comparing the
	// rest, since those are covered by the assertion above instead.
	original.UID, original.ETag = "", ""

	if !reflect.DeepEqual(original, got) {
		t.Errorf("round-trip mismatch (excluding UID/ETag)\n original: %#v\n got:      %#v\n json:     %s", original, got, data)
	}
}
