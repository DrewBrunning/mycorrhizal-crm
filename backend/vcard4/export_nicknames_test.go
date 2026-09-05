package vcard4

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

func init() {
	registerExportCoverage("nickname")
}

func TestExport_Nickname(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Nicknames: []contactmodel.Nickname{{Name: "Johnny", Contexts: []string{"work"}, Pref: intPtr(1)}},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, "NICKNAME", map[string]string{"PREF": "1", "TYPE": "work"}, "Johnny")
}

// TestExport_NicknameSkipsEmpty pins the DATA-03 (issue #443) fix: an empty
// nickname name is dropped by importNicknames and ignored by the semantic
// comparison, so emitting it would produce a degenerate "NICKNAME:" line that
// never survives a re-import — churning the serialized form across a repeated
// conversion.
func TestExport_NicknameSkipsEmpty(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Nicknames: []contactmodel.Nickname{{Name: ""}, {Name: "Bobby"}},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, "NICKNAME", nil, "Bobby")
	card, err := parseVCardForTest(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(card["NICKNAME"]) != 1 {
		t.Errorf("want exactly one NICKNAME line (the empty nickname must be skipped), got %d:\n%s", len(card["NICKNAME"]), out)
	}
}
