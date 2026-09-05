package vcard3

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Concept covered: email.
func init() {
	registerExportCoverage("email")
}

func TestExport_Email(t *testing.T) {
	t.Parallel()
	pref := 1
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Emails: []contactmodel.Email{{Address: "Frank_Dawson@Lotus.com", Pref: &pref, Contexts: []string{"work"}}},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropEmail, map[string]string{"TYPE": "INTERNET"}, "Frank_Dawson@Lotus.com")
	rfctest.AssertVCardLine(t, out, PropEmail, map[string]string{"TYPE": "WORK"}, "Frank_Dawson@Lotus.com")
	rfctest.AssertVCardLine(t, out, PropEmail, map[string]string{"TYPE": "PREF"}, "Frank_Dawson@Lotus.com")
}

// TestExport_ContextWithoutV3TokenWarns pins the DATA-03 (issue #443) fix to
// the ctx2type transform: RFC 2426's TYPE vocabulary defines only HOME/WORK,
// so a context like billing/delivery must not be emitted as a TYPE token that
// contextsAndPrefFromTokens would silently drop on import (losing the context
// without a trace AND churning the serialized form across a repeated
// conversion). It must be dropped with a warn diagnostic instead.
func TestExport_ContextWithoutV3TokenWarns(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Emails: []contactmodel.Email{{Address: "billing@example.com", Contexts: []string{"billing"}}},
	}}
	out, diags, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if strings.Contains(string(out), "TYPE=BILLING") {
		t.Errorf("billing context was emitted as a TYPE token the 3.0 importer would silently drop:\n%s", out)
	}
	if !hasWarn(diags, "email") {
		t.Errorf("diags = %+v, want a warn for concept email (billing context dropped)", diags)
	}
	rfctest.AssertVCardLine(t, out, PropEmail, nil, "billing@example.com")
}
