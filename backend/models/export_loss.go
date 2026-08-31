package models

import (
	"strings"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
)

// LossReport is one fidelity-loss event surfaced to the user (DATA-02, issue
// #442): which contact lost which field (concept) to which format, and why.
//
// It is NOT a second loss-reporting type. The loss event itself is the
// adapter's contactmodel.Diagnostic — the shared loss-reporting type both
// adapters and the import preview already use — and this struct is the
// transport form of that Diagnostic plus the context that makes a report
// specific: the contact identity (id/name/vcard_uid), the format, and the
// DATA-01 matrix classification (bucket + reason) for the (concept, format)
// pair. Severity/Concept/Message are the Diagnostic's own fields verbatim.
type LossReport struct {
	Format      string `json:"format"`
	ContactID   uint   `json:"contact_id"`
	ContactName string `json:"contact_name"`
	VCardUID    string `json:"vcard_uid"`
	Severity    string `json:"severity"`
	Concept     string `json:"concept"`
	Bucket      string `json:"bucket"` // unsupported | lossy (from the DATA-01 matrix)
	Reason      string `json:"reason"` // from the DATA-01 matrix
	Message     string `json:"message"`
}

// contactDisplayName is the human label a loss report carries so the report
// names which contact lost the field. Matches the list/CSV "First Last"
// convention used across the exporters.
func contactDisplayName(c *Contact) string {
	return strings.TrimSpace(c.Firstname + " " + c.Lastname)
}

// LossReportsFor converts one contact's export diagnostics into user-facing
// loss reports for one format. Only warn diagnostics whose (concept, format)
// resolves to a DATA-01 unsupported/lossy cell become reports — the exact
// bidirectional correspondence the milestone gate asserts (every report is a
// matrix entry, every matrix unsupported/lossy cell is reportable). Info
// diagnostics are not fidelity losses, and adapter diagnostics outside the
// matrix (e.g. the "no Name.Full" warn, which is a degenerate-record warning
// rather than a fidelity loss) are logged but not surfaced as loss reports.
func LossReportsFor(format string, contact *Contact, diags []contactmodel.Diagnostic) []LossReport {
	reports := make([]LossReport, 0, len(diags))
	for _, d := range diags {
		if !strings.EqualFold(d.Severity, "warn") {
			continue
		}
		cell, ok := correspondence.ClassificationFor(d.Concept, correspondence.Format(format))
		if !ok {
			continue
		}
		reports = append(reports, LossReport{
			Format:      format,
			ContactID:   contact.ID,
			ContactName: contactDisplayName(contact),
			VCardUID:    contact.VCardUID,
			Severity:    d.Severity,
			Concept:     d.Concept,
			Bucket:      string(cell.Bucket),
			Reason:      cell.Reason,
			Message:     d.Message,
		})
	}
	return reports
}

// ExportLossPreflightResponse is the body of GET /export/preflight (issue
// #442): what an export with the given format/selection would lose, computed
// without producing the file. Diagnostics is never null on the wire — an empty
// report serializes as `[]`, not `null` (frontend trap #8).
type ExportLossPreflightResponse struct {
	Format       string       `json:"format"`
	ContactCount int          `json:"contact_count"`
	Diagnostics  []LossReport `json:"diagnostics"`
}
