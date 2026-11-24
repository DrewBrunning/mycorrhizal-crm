package services

import (
	"testing"

	"mycorrhizal/models"
)

// TestImportVCardBlock_FNOnlyName is the regression test for issue #2913:
// RFC 6350 makes FN required and N optional, but applyName only recovered a
// flat name from N components (or projection values derived from those same
// components), so an FN-only card imported with no name — the row failed
// "First name is required" or fell back to the email (#2186). FN's first
// space splits first name from the rest, the same convention
// carddav/vcard_mapper.go uses.
func TestImportVCardBlock_FNOnlyName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		vcf       string
		wantFirst string
		wantLast  string
	}{
		{
			name:      "FN with a first and last name",
			vcf:       "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Fn Only\r\nEND:VCARD\r\n",
			wantFirst: "Fn",
			wantLast:  "Only",
		},
		{
			name:      "single-word FN sets only the first name",
			vcf:       "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Prince\r\nEND:VCARD\r\n",
			wantFirst: "Prince",
			wantLast:  "",
		},
		{
			name:      "structured N still wins over FN",
			vcf:       "BEGIN:VCARD\r\nVERSION:4.0\r\nN:Component;Given;;;\r\nFN:Full Name\r\nEND:VCARD\r\n",
			wantFirst: "Given",
			wantLast:  "Component",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			record, _, err := ImportVCardBlock([]byte(tc.vcf))
			if err != nil {
				t.Fatalf("ImportVCardBlock: %v", err)
			}

			contact := &models.Contact{}
			models.ApplyRecordToContact(contact, record, "")

			if contact.Firstname != tc.wantFirst {
				t.Errorf("Firstname = %q, want %q", contact.Firstname, tc.wantFirst)
			}
			if contact.Lastname != tc.wantLast {
				t.Errorf("Lastname = %q, want %q", contact.Lastname, tc.wantLast)
			}
			if got := ValidateImportedContact(contact); len(got) != 0 {
				t.Errorf("ValidateImportedContact = %v, want no errors", got)
			}
		})
	}
}
