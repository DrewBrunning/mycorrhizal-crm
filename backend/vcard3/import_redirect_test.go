package vcard3

import "testing"

// Concepts covered (import): anniversary.wedding and related. Both are
// adapter-level redirects the export side emits as X-ANNIVERSARY and AGENT
// (see exportAddresses / exportAgentRelated); importAnniversaries and
// importAgentRelated read them back, so the coverage gate — which keys on the
// effective v3 bucket — requires registered import coverage for them.
func init() {
	registerImportCoverage("anniversary.wedding", "related")
}

const redirectImportVCF = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"FN:Frank Dawson\n" +
	"X-ANNIVERSARY:2001-06-10\n" +
	"AGENT:urn:uuid:1234\n" +
	"END:VCARD\n"

func TestImport_AnniversaryWedding(t *testing.T) {
	t.Parallel()
	rec, _, err := (Adapter{}).Import([]byte(redirectImportVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var found bool
	for _, a := range rec.Card.Anniversaries {
		if a.Kind == "wedding" {
			found = true
		}
	}
	if !found {
		t.Errorf("anniversaries = %+v, want a wedding entry from X-ANNIVERSARY", rec.Card.Anniversaries)
	}
}

func TestImport_RelatedAgent(t *testing.T) {
	t.Parallel()
	rec, _, err := (Adapter{}).Import([]byte(redirectImportVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.RelatedTo) != 1 || rec.Card.RelatedTo[0].Target != "urn:uuid:1234" {
		t.Errorf("RelatedTo = %+v, want the AGENT target urn:uuid:1234", rec.Card.RelatedTo)
	}
}
