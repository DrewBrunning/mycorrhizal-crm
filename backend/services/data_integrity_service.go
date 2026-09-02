package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mycorrhizal/attachments"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/photostore"

	"gorm.io/gorm"
)

// Data-integrity checker (DB-01, issue #460). This is the application-invariant
// pass that sits beside the storage-level PRAGMA pass in
// db_integrity_service.go: PRAGMA integrity_check answers "are the pages and
// indexes intact", this answers "is the data meaningful". A database can pass
// every PRAGMA while holding a RelationshipEdge pointing at a deleted Contact,
// a FieldValue whose definition is gone, or an Attachment row whose file
// vanished.
//
// Every invariant here is one row of docs/adrs/0012-canonical-database-
// invariants.md, cited by its stable ID (INV-D1 .. INV-D8, INV-A5). The
// application invariants INV-A1..A4/A6 are operation-level (durability,
// atomicity, idempotency, convergence) and are not checkable against a
// database at rest — they belong to the fault-injection suite (#434/#494).
//
// DETECTION ONLY. This file never writes. Repair is a separate, explicit,
// operator-invoked path (data_integrity_repair.go, dry-run by default) so a
// repair built on a misunderstood invariant cannot silently destroy data
// (issue #460 point 4, gate #538).
//
// Scoping: the sweep is instance-wide (an operator runs it), but every query
// correlates on user_id — an edge owned by user A is only ever checked against
// user A's contacts, so a reference that happens to match another user's row
// is still reported as the violation it is (CLAUDE.md item 5, no IDOR).

// Finding severity. A violation is a real logical hole; info is a fact worth
// surfacing that is not, on its own, wrong (an audit row legitimately outlives
// the soft-deleted entity it describes). Only violations flip Report.OK,
// record a failed OperationalCheckResult, or fire the alert.
const (
	IntegritySeverityViolation = "violation"
	IntegritySeverityInfo      = "info"
)

// IntegrityFinding is one class of problem found by one check, aggregated per
// owning user. Detail is secret-free (counts and table/column names only) so
// the whole report is safe to return on an admin endpoint and log verbatim.
type IntegrityFinding struct {
	// Invariant is the ADR 0012 identifier, e.g. "INV-D1".
	Invariant string `json:"invariant"`
	// Check is a stable slug identifying the specific probe, e.g.
	// "relationship_edge.endpoint_missing" — the key #494's per-invariant
	// tests and the repair path match on.
	Check string `json:"check"`
	// Severity is IntegritySeverity{Violation,Info}.
	Severity string `json:"severity"`
	// UserID is the owner of the affected rows; 0 for an instance-wide check
	// (the relation-type registry, the FTS row counts).
	UserID uint `json:"user_id,omitempty"`
	// Count is how many rows/references are affected for this (check, user).
	Count int `json:"count"`
	// Detail is a short, secret-free human message.
	Detail string `json:"detail"`
	// Repairable is true when data_integrity_repair.go can safely remove the
	// affected rows (a truly-orphaned hard-delete join/edge row). Never true
	// for a reference to a merely soft-deleted entity, canonical-record
	// corruption, or a missing file.
	Repairable bool `json:"repairable"`
}

// DataIntegrityReport is the full application-invariant sweep output.
type DataIntegrityReport struct {
	Timestamp string             `json:"timestamp"`
	OK        bool               `json:"ok"`
	Findings  []IntegrityFinding `json:"findings"`
}

// violationCount returns how many findings are real violations (info findings
// excluded) — the number the scheduled path and the alert condition act on.
func (r DataIntegrityReport) violationCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == IntegritySeverityViolation {
			n++
		}
	}
	return n
}

// dataIntegrityCheck is one named probe. Returning an error means the probe
// itself could not run (a broken query, an unreadable directory) — distinct
// from a clean run that found violations, exactly as the storage pass
// separates OpCheckStatusError from OpCheckStatusFailed.
type dataIntegrityCheck struct {
	name string
	fn   func(ctx context.Context, db *gorm.DB, cfg config.Config) ([]IntegrityFinding, error)
}

// dataIntegrityChecks is the registry. Adding an invariant to ADR 0012 means
// adding a row here and a per-invariant test (#494).
func dataIntegrityChecks() []dataIntegrityCheck {
	return []dataIntegrityCheck{
		{"relationship_endpoints", checkRelationshipEndpoints},    // INV-D1 / INV-D7
		{"relationship_type_registry", checkRelationshipRegistry}, // INV-D2
		{"orphaned_contact_refs", checkOrphanedContactRefs},       // INV-D3 / INV-D7
		{"dangling_external_refs", checkDanglingExternalRefs},     // INV-D4
		{"dangling_field_values", checkDanglingFieldValues},       // INV-D4
		{"missing_files", checkMissingFiles},                      // INV-D4
		{"vanished_audit_refs", checkVanishedAuditRefs},           // INV-D4 (info)
		{"required_fields", checkRequiredFields},                  // INV-D5 / INV-D6
		{"canonical_records", checkCanonicalRecords},              // INV-D8
		{"derived_indexes", checkDerivedIndexes},                  // INV-A5
		{"derived_contact_columns", checkDerivedContactColumns},   // INV-A5 (issue #497)
	}
}

// RunDataIntegrityChecks executes every registered probe and folds the results
// into one report. It is read-only. The returned error is non-nil only when a
// probe could not complete; violations found by a probe that ran cleanly are
// in the report, and Report.OK is false.
func RunDataIntegrityChecks(ctx context.Context, db *gorm.DB, cfg config.Config) (DataIntegrityReport, error) {
	report := DataIntegrityReport{Timestamp: time.Now().UTC().Format(time.RFC3339)}
	var runErrs []string

	for _, c := range dataIntegrityChecks() {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		findings, err := c.fn(ctx, db, cfg)
		if err != nil {
			runErrs = append(runErrs, c.name+": "+err.Error())
			continue
		}
		report.Findings = append(report.Findings, findings...)
	}

	sortFindings(report.Findings)
	report.OK = report.violationCount() == 0 && len(runErrs) == 0

	if len(runErrs) > 0 {
		return report, fmt.Errorf("data integrity: %s", strings.Join(runErrs, "; "))
	}
	return report, nil
}

func sortFindings(f []IntegrityFinding) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].Check != f[j].Check {
			return f[i].Check < f[j].Check
		}
		return f[i].UserID < f[j].UserID
	})
}

// ---------------------------------------------------------------------------
// INV-D1 / INV-D7 — relationship edges reference real, live, owned contacts
// ---------------------------------------------------------------------------

// contactRefRow is the per-user aggregate the reference probes scan into.
type contactRefRow struct {
	UserID            uint
	Missing           int
	SoftDeleted       int
	SoftDeletedActive int // subset of SoftDeleted that is still authoritative
}

func checkRelationshipEndpoints(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	// One pass over both endpoints. A soft-deleted contact still has its row,
	// so a plain model query (deleted_at IS NULL scope) would hide exactly the
	// INV-D7 case; raw SQL keeps both visible.
	const q = `
WITH endpoints AS (
    SELECT id AS edge_id, user_id, status, source_id AS cid FROM relationship_edges
    UNION ALL
    SELECT id, user_id, status, target_id FROM relationship_edges
)
SELECT e.user_id AS user_id,
       SUM(CASE WHEN c.id IS NULL THEN 1 ELSE 0 END) AS missing,
       SUM(CASE WHEN c.id IS NOT NULL AND c.deleted_at IS NOT NULL THEN 1 ELSE 0 END) AS soft_deleted,
       SUM(CASE WHEN c.id IS NOT NULL AND c.deleted_at IS NOT NULL AND e.status = ? THEN 1 ELSE 0 END) AS soft_deleted_active
FROM endpoints e
LEFT JOIN contacts c ON c.vcard_uid = e.cid AND c.user_id = e.user_id
GROUP BY e.user_id
HAVING missing > 0 OR soft_deleted > 0`

	var rows []contactRefRow
	if err := db.WithContext(ctx).Raw(q, models.RelationshipStatusConfirmed).Scan(&rows).Error; err != nil {
		return nil, err
	}

	var out []IntegrityFinding
	for _, r := range rows {
		if r.Missing > 0 {
			out = append(out, IntegrityFinding{
				Invariant: "INV-D1", Check: "relationship_edge.endpoint_missing",
				Severity: IntegritySeverityViolation, UserID: r.UserID, Count: r.Missing,
				Detail:     fmt.Sprintf("%d relationship-edge endpoint(s) reference a contact that no longer exists", r.Missing),
				Repairable: true,
			})
		}
		if r.SoftDeleted > 0 {
			detail := fmt.Sprintf("%d relationship-edge endpoint(s) reference a soft-deleted contact", r.SoftDeleted)
			if r.SoftDeletedActive > 0 {
				detail += fmt.Sprintf(" (%d on a confirmed edge — a deleted contact still an active relationship target)", r.SoftDeletedActive)
			}
			out = append(out, IntegrityFinding{
				Invariant: "INV-D7", Check: "relationship_edge.endpoint_soft_deleted",
				Severity: IntegritySeverityViolation, UserID: r.UserID, Count: r.SoftDeleted,
				Detail:     detail,
				Repairable: false, // the contact may yet be undeleted
			})
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// INV-D2 — reciprocal relationships are consistent / derivable
// ---------------------------------------------------------------------------

func checkRelationshipRegistry(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	var out []IntegrityFinding

	// The registry must be a total involution: inverse(inverse(t)) == t for
	// every known token. A broken entry makes an edge's reciprocal
	// underivable.
	var broken []string
	for _, t := range models.KnownRelationTypes() {
		if models.InverseRelationType(models.InverseRelationType(t)) != t {
			broken = append(broken, t)
		}
	}
	if len(broken) > 0 {
		sort.Strings(broken)
		out = append(out, IntegrityFinding{
			Invariant: "INV-D2", Check: "relationship_type.registry_inconsistent",
			Severity: IntegritySeverityViolation, Count: len(broken),
			Detail:     "relation-type registry inverse round-trip fails for: " + strings.Join(broken, ", "),
			Repairable: false, // a code defect, not a data defect
		})
	}

	// Every stored edge type must be a registered token — otherwise its
	// reciprocal cannot be derived and it is invisible to projection.
	type typeRow struct {
		UserID uint
		Type   string
		N      int
	}
	var rows []typeRow
	if err := db.WithContext(ctx).
		Raw(`SELECT user_id, type, COUNT(*) AS n FROM relationship_edges GROUP BY user_id, type`).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	perUser := map[uint]int{}
	unknown := map[uint][]string{}
	for _, r := range rows {
		if !models.IsKnownRelationType(r.Type) {
			perUser[r.UserID] += r.N
			unknown[r.UserID] = append(unknown[r.UserID], r.Type)
		}
	}
	for uid, n := range perUser {
		sort.Strings(unknown[uid])
		out = append(out, IntegrityFinding{
			Invariant: "INV-D2", Check: "relationship_edge.unknown_type",
			Severity: IntegritySeverityViolation, UserID: uid, Count: n,
			Detail:     fmt.Sprintf("%d edge(s) use a relation type not in the registry: %s", n, strings.Join(unknown[uid], ", ")),
			Repairable: false,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// INV-D3 / INV-D7 — no join row points at a gone or deleted contact
// ---------------------------------------------------------------------------

// contactRefTarget names a table/column whose value is a Contact.VCardUID with
// no foreign key backing it (the graph endpoints). table and col are
// compile-time constants from this file only — never request input.
type contactRefTarget struct {
	table      string
	col        string
	extraWhere string // optional, constant
	invariant  string // invariant for the "missing" case
	missCheck  string
	softCheck  string
	repairable bool
}

func (t contactRefTarget) scan(ctx context.Context, db *gorm.DB) ([]contactRefRow, error) {
	where := ""
	if t.extraWhere != "" {
		where = " WHERE " + t.extraWhere
	}
	// #nosec G201 -- t.table/t.col/t.extraWhere are constants defined in this
	// file; the only dynamic value (the join) is a bound parameter.
	q := fmt.Sprintf(`
SELECT j.user_id AS user_id,
       SUM(CASE WHEN c.id IS NULL THEN 1 ELSE 0 END) AS missing,
       SUM(CASE WHEN c.id IS NOT NULL AND c.deleted_at IS NOT NULL THEN 1 ELSE 0 END) AS soft_deleted,
       0 AS soft_deleted_active
FROM %s j
LEFT JOIN contacts c ON c.vcard_uid = j.%s AND c.user_id = j.user_id%s
GROUP BY j.user_id
HAVING missing > 0 OR soft_deleted > 0`, t.table, t.col, where)

	var rows []contactRefRow
	err := db.WithContext(ctx).Raw(q).Scan(&rows).Error
	return rows, err
}

func checkOrphanedContactRefs(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	targets := []contactRefTarget{
		{table: "circle_members", col: "member_vcard_uid", invariant: "INV-D3",
			missCheck: "circle_member.orphaned_contact", softCheck: "circle_member.soft_deleted_contact", repairable: true},
		{table: "household_members", col: "member_vcard_uid", invariant: "INV-D3",
			missCheck: "household_member.orphaned_contact", softCheck: "household_member.soft_deleted_contact", repairable: true},
		{table: "contact_tags", col: "contact_vcard_uid", invariant: "INV-D3",
			missCheck: "contact_tag.orphaned_contact", softCheck: "contact_tag.soft_deleted_contact", repairable: true},
		{table: "field_values", col: "entity_id", invariant: "INV-D3",
			missCheck: "field_value.orphaned_contact", softCheck: "field_value.soft_deleted_contact", repairable: true},
	}
	return scanContactRefTargets(ctx, db, targets)
}

func checkDanglingExternalRefs(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	targets := []contactRefTarget{
		{table: "external_identities", col: "entity_id", invariant: "INV-D4",
			missCheck: "external_identity.dangling_contact", softCheck: "external_identity.soft_deleted_contact", repairable: false},
		{table: "external_activities", col: "entity_id", invariant: "INV-D4",
			missCheck: "external_activity.dangling_contact", softCheck: "external_activity.soft_deleted_contact", repairable: false},
		{table: "import_source_links", col: "entity_uid", extraWhere: "j.entity_kind = 'contact'", invariant: "INV-D4",
			missCheck: "import_source_link.dangling_contact", softCheck: "import_source_link.soft_deleted_contact", repairable: false},
	}
	return scanContactRefTargets(ctx, db, targets)
}

func scanContactRefTargets(ctx context.Context, db *gorm.DB, targets []contactRefTarget) ([]IntegrityFinding, error) {
	var out []IntegrityFinding
	for _, t := range targets {
		rows, err := t.scan(ctx, db)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", t.table, t.col, err)
		}
		for _, r := range rows {
			if r.Missing > 0 {
				out = append(out, IntegrityFinding{
					Invariant: t.invariant, Check: t.missCheck,
					Severity: IntegritySeverityViolation, UserID: r.UserID, Count: r.Missing,
					Detail:     fmt.Sprintf("%d %s row(s) reference a contact that no longer exists", r.Missing, t.table),
					Repairable: t.repairable,
				})
			}
			if r.SoftDeleted > 0 {
				out = append(out, IntegrityFinding{
					Invariant: "INV-D7", Check: t.softCheck,
					Severity: IntegritySeverityViolation, UserID: r.UserID, Count: r.SoftDeleted,
					Detail:     fmt.Sprintf("%d %s row(s) reference a soft-deleted contact", r.SoftDeleted, t.table),
					Repairable: false,
				})
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// INV-D4 — field values reference a live definition
// ---------------------------------------------------------------------------

func checkDanglingFieldValues(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	// field_values.field_definition_id has an ON DELETE CASCADE foreign key,
	// so PRAGMA foreign_key_check (the storage pass) already catches this if
	// enforcement was ever bypassed. Re-checking it here is cheap
	// defence-in-depth and lets the data report name it in ADR terms.
	type row struct {
		UserID uint
		N      int
	}
	var rows []row
	const q = `
SELECT fv.user_id AS user_id, COUNT(*) AS n
FROM field_values fv
LEFT JOIN field_definitions fd ON fd.id = fv.field_definition_id
WHERE fd.id IS NULL
GROUP BY fv.user_id`
	if err := db.WithContext(ctx).Raw(q).Scan(&rows).Error; err != nil {
		return nil, err
	}
	var out []IntegrityFinding
	for _, r := range rows {
		out = append(out, IntegrityFinding{
			Invariant: "INV-D4", Check: "field_value.dangling_definition",
			Severity: IntegritySeverityViolation, UserID: r.UserID, Count: r.N,
			Detail:     fmt.Sprintf("%d field_values row(s) reference a field definition that no longer exists", r.N),
			Repairable: true,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// INV-D4 — attachment and photo rows resolve to a file on disk
// ---------------------------------------------------------------------------

func checkMissingFiles(ctx context.Context, db *gorm.DB, cfg config.Config) ([]IntegrityFinding, error) {
	var out []IntegrityFinding

	if cfg.AttachmentsDir != "" {
		type row struct {
			UserID     uint
			StoredName string
		}
		var rows []row
		if err := db.WithContext(ctx).
			Raw(`SELECT user_id, stored_name FROM attachments WHERE deleted_at IS NULL`).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		perUser := map[uint]int{}
		for _, r := range rows {
			p, err := attachments.StoredPath(cfg.AttachmentsDir, r.StoredName)
			if err != nil {
				perUser[r.UserID]++ // an unresolvable stored_name is itself a hole
				continue
			}
			if _, err := os.Stat(p); err != nil {
				perUser[r.UserID]++
			}
		}
		for uid, n := range perUser {
			out = append(out, IntegrityFinding{
				Invariant: "INV-D4", Check: "attachment.missing_file",
				Severity: IntegritySeverityViolation, UserID: uid, Count: n,
				Detail:     fmt.Sprintf("%d attachment row(s) have no file in the attachments directory", n),
				Repairable: false,
			})
		}
	}

	if cfg.ProfilePhotoDir != "" {
		type row struct {
			UserID uint
			Photo  string
		}
		var rows []row
		if err := db.WithContext(ctx).
			Raw(`SELECT user_id, photo FROM contacts WHERE deleted_at IS NULL AND photo <> ''`).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		perUser := map[uint]int{}
		for _, r := range rows {
			// contacts.photo holds a bare server-generated filename. Skip the
			// data-URI and remote-URL shapes the readers also tolerate.
			if strings.HasPrefix(r.Photo, "data:") || strings.Contains(r.Photo, "://") {
				continue
			}
			if r.Photo != filepath.Base(r.Photo) {
				perUser[r.UserID]++
				continue
			}
			if _, err := os.Stat(filepath.Join(cfg.ProfilePhotoDir, r.Photo)); err != nil {
				perUser[r.UserID]++
			}
		}
		for uid, n := range perUser {
			out = append(out, IntegrityFinding{
				Invariant: "INV-D4", Check: "contact.missing_photo_file",
				Severity: IntegritySeverityViolation, UserID: uid, Count: n,
				Detail:     fmt.Sprintf("%d contact(s) name a profile photo file that is not on disk", n),
				Repairable: false,
			})
		}
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// INV-D4 (info) — audit rows whose subject entity has entirely vanished
// ---------------------------------------------------------------------------

func checkVanishedAuditRefs(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	// The audit log is append-only and legitimately outlives a soft-deleted
	// contact, so this is INFO, not a violation: it does not flip Report.OK.
	// It is nonetheless worth surfacing — an audit "contact" row whose
	// entity_id matches no contact row at all means the contact was
	// hard-removed (a merge, a very old import) without the audit trail
	// recording a tombstone.
	type row struct {
		UserID uint
		N      int
	}
	var rows []row
	const q = `
SELECT ae.user_id AS user_id, COUNT(*) AS n
FROM audit_events ae
WHERE ae.entity_type = ?
  AND ae.entity_id <> ''
  AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.vcard_uid = ae.entity_id AND c.user_id = ae.user_id)
GROUP BY ae.user_id`
	if err := db.WithContext(ctx).Raw(q, models.AuditEntityContact).Scan(&rows).Error; err != nil {
		return nil, err
	}
	var out []IntegrityFinding
	for _, r := range rows {
		out = append(out, IntegrityFinding{
			Invariant: "INV-D4", Check: "audit_event.vanished_contact",
			Severity: IntegritySeverityInfo, UserID: r.UserID, Count: r.N,
			Detail:     fmt.Sprintf("%d audit row(s) describe a contact with no row (soft-deleted contacts are expected to keep theirs)", r.N),
			Repairable: false,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// INV-D5 / INV-D6 — identifiers present and well-formed, enum columns valid
// ---------------------------------------------------------------------------

func checkRequiredFields(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	var out []IntegrityFinding

	type row struct {
		UserID uint
		N      int
	}

	// RelationshipEdge enum / non-empty columns. The allowed sets mirror the
	// oneof validator tags on models.RelationshipEdge — kept in sync by hand
	// (CLAUDE.md frontend trap #4 is the same drift class).
	var edgeRows []row
	const edgeQ = `
SELECT user_id, COUNT(*) AS n FROM relationship_edges
WHERE status NOT IN ('confirmed','suggested')
   OR source NOT IN ('user-confirmed','household-inferred','imported','ai-suggested','graph-inferred')
   OR sensitivity NOT IN ('normal','private','secret')
   OR type IS NULL OR type = ''
GROUP BY user_id`
	if err := db.WithContext(ctx).Raw(edgeQ).Scan(&edgeRows).Error; err != nil {
		return nil, err
	}
	for _, r := range edgeRows {
		out = append(out, IntegrityFinding{
			Invariant: "INV-D6", Check: "relationship_edge.invalid_enum",
			Severity: IntegritySeverityViolation, UserID: r.UserID, Count: r.N,
			Detail:     fmt.Sprintf("%d relationship edge(s) have an out-of-range status/source/sensitivity or an empty type", r.N),
			Repairable: false,
		})
	}

	// Contact.VCardUID is generated once in BeforeCreate and is the stable
	// identifier every graph entity references (INV-D5). A blank one is both
	// an unstable-identifier and a required-field violation.
	var uidRows []row
	if err := db.WithContext(ctx).
		Raw(`SELECT user_id, COUNT(*) AS n FROM contacts WHERE deleted_at IS NULL AND (vcard_uid IS NULL OR vcard_uid = '') GROUP BY user_id`).
		Scan(&uidRows).Error; err != nil {
		return nil, err
	}
	for _, r := range uidRows {
		out = append(out, IntegrityFinding{
			Invariant: "INV-D5", Check: "contact.missing_vcard_uid",
			Severity: IntegritySeverityViolation, UserID: r.UserID, Count: r.N,
			Detail:     fmt.Sprintf("%d contact(s) have no vcard_uid — the stable identifier the graph references", r.N),
			Repairable: false,
		})
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// INV-D8 — canonical records are internally consistent
// ---------------------------------------------------------------------------

func checkCanonicalRecords(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	// Stream live contacts in pages so a large instance does not load every
	// Card blob at once.
	const page = 500
	type row struct {
		ID     uint
		UserID uint
		Card   string
		Photo  string
	}

	malformed := map[uint]int{}
	dupIDs := map[uint]int{}
	unresolvedRemotePhoto := map[uint]int{}
	var lastID uint

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var rows []row
		if err := db.WithContext(ctx).
			Raw(`SELECT id, user_id, card, photo FROM contacts WHERE deleted_at IS NULL AND id > ? ORDER BY id LIMIT ?`, lastID, page).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			lastID = r.ID
			card := strings.TrimSpace(r.Card)
			if card == "" || card == "{}" {
				continue
			}
			var top map[string]json.RawMessage
			if err := json.Unmarshal([]byte(card), &top); err != nil {
				malformed[r.UserID]++
				continue
			}
			if collectionHasDuplicateElementID(top) {
				dupIDs[r.UserID]++
			}
			if strings.TrimSpace(r.Photo) == "" && cardHasUnresolvedRemotePhoto(top) {
				unresolvedRemotePhoto[r.UserID]++
			}
		}
		if len(rows) < page {
			break
		}
	}

	var out []IntegrityFinding
	for uid, n := range malformed {
		out = append(out, IntegrityFinding{
			Invariant: "INV-D8", Check: "canonical_record.invalid_json",
			Severity: IntegritySeverityViolation, UserID: uid, Count: n,
			Detail:     fmt.Sprintf("%d contact(s) have a Card column that is not valid JSON", n),
			Repairable: false,
		})
	}
	for uid, n := range dupIDs {
		out = append(out, IntegrityFinding{
			Invariant: "INV-D8", Check: "canonical_record.duplicate_element_id",
			Severity: IntegritySeverityViolation, UserID: uid, Count: n,
			Detail:     fmt.Sprintf("%d contact(s) have a Card collection with two elements sharing one id (breaks the PROP-ID / JSContact-key round-trip)", n),
			Repairable: false,
		})
	}
	for uid, n := range unresolvedRemotePhoto {
		out = append(out, IntegrityFinding{
			Invariant: "INV-D8", Check: "canonical_record.unresolved_remote_photo",
			Severity: IntegritySeverityInfo, UserID: uid, Count: n,
			Detail:     fmt.Sprintf("%d contact(s) reference a remote photo URL in Card.Media that has not been downloaded to the photo store (contacts.photo empty); it is preserved across saves but not yet in durable local storage", n),
			Repairable: false,
		})
	}
	return out, nil
}

// cardHasUnresolvedRemotePhoto reports whether the Card's media array holds a
// {kind:"photo"} entry whose uri is a remote http(s) reference. Callers gate
// this on contacts.photo being empty: a photo entry that IS the flat
// projection (a data: URI) is expected, but a remote URL with no flat photo
// is a transient reference the ingesting caller has not resolved to durable
// storage yet (ADR 0012 INV-D8; mergeMedia preserves it across saves).
func cardHasUnresolvedRemotePhoto(top map[string]json.RawMessage) bool {
	raw, ok := top["media"]
	if !ok {
		return false
	}
	var media []struct {
		Kind string `json:"kind"`
		URI  string `json:"uri"`
	}
	if err := json.Unmarshal(raw, &media); err != nil {
		return false
	}
	for _, m := range media {
		if m.Kind != "photo" {
			continue
		}
		if _, _, remoteURL := photostore.DecodePhotoURI(m.URI, ""); remoteURL != "" {
			return true
		}
	}
	return false
}

// collectionHasDuplicateElementID reports whether any top-level Card value that
// is a JSON array of objects contains two elements with the same non-empty
// "id". Non-array values and arrays of scalars are skipped — this only asserts
// the JSContact-style ordered-collection round-trip invariant (ADR 0001).
func collectionHasDuplicateElementID(top map[string]json.RawMessage) bool {
	for _, raw := range top {
		var arr []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			continue // not an array of objects
		}
		seen := map[string]bool{}
		for _, el := range arr {
			idRaw, ok := el["id"]
			if !ok {
				continue
			}
			var id string
			if err := json.Unmarshal(idRaw, &id); err != nil || id == "" {
				continue
			}
			if seen[id] {
				return true
			}
			seen[id] = true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// INV-A5 — every derived index is regenerable from canonical data
// ---------------------------------------------------------------------------

func checkDerivedIndexes(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	// The cheap version: does each FTS virtual table hold exactly one row per
	// live base row? A mismatch means the triggers were bypassed and the
	// index needs a rebuild (services.RebuildSearchIndex). The deep per-row
	// content comparison is #462's scope.
	pairs := []struct {
		fts  string
		base string
	}{
		{"contacts_fts", "contacts"},
		{"notes_fts", "notes"},
		{"activities_fts", "activities"},
	}
	var out []IntegrityFinding
	for _, p := range pairs {
		var ftsN, baseN int64
		// #nosec G201 -- p.fts/p.base are constants from the slice literal above.
		if err := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", p.fts)).Scan(&ftsN).Error; err != nil {
			return nil, err
		}
		if err := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL", p.base)).Scan(&baseN).Error; err != nil {
			return nil, err
		}
		if ftsN != baseN {
			out = append(out, IntegrityFinding{
				Invariant: "INV-A5", Check: "derived_index.fts_row_count_divergent",
				Severity: IntegritySeverityViolation, Count: int(abs64(ftsN - baseN)),
				Detail:     fmt.Sprintf("%s has %d rows; %d live %s rows expected (rebuild with the search-index backfill)", p.fts, ftsN, baseN, p.base),
				Repairable: true,
			})
		}
	}
	return out, nil
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// ---------------------------------------------------------------------------
// INV-A5 (issue #497) — the flat contacts.* projection is a faithful,
// rebuildable derivation of the nested Card
// ---------------------------------------------------------------------------

// checkDerivedContactColumns is the flat-column-family analogue of
// checkDerivedIndexes: where that one asks "does the FTS index have the right
// row count", this asks "is every denormalized contact column still what a
// plain re-save would recompute". It streams live contacts in pages and, for
// each, re-derives the denormalized columns through the same
// models.Contact.RederiveDenormalized the write path and the rebuild use,
// then reports — per user, per column — how many rows drifted.
//
// A faithful database produces nothing here: the projection is a fixpoint of
// a re-save (ADR 0012 INV-A5 / INV-D8). A finding means a hook-bypassing
// write (a raw-SQL migration, a bulk import that INSERTed rows directly, a
// mid-write restore) left a column stale — repair is a full re-derivation,
// services.RebuildDerivedContactColumns (POST /api/v1/admin/contacts/
// rebuild-derived or cmd/backfill-derived-columns), never the orphan-row
// repair path, so these findings are Repairable:false.
func checkDerivedContactColumns(ctx context.Context, db *gorm.DB, _ config.Config) ([]IntegrityFinding, error) {
	const page = 500
	// perUserCols[userID][column] = number of live contacts of that user
	// whose stored `column` disagrees with the re-derived value.
	perUserCols := map[uint]map[string]int{}
	perUserRows := map[uint]int{}
	var lastID uint

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Page over ids by raw SQL, then load each contact through GORM so the
		// at-rest serializer decrypts card/crm/passthrough. A row whose card
		// is unreadable (corrupt JSON, or ciphertext the key can't open) is
		// already an INV-D8 canonical_record.invalid_json violation from
		// checkCanonicalRecords — a derived-column check on it is meaningless,
		// so skip it rather than failing the whole probe.
		var ids []uint
		if err := db.WithContext(ctx).Raw(
			`SELECT id FROM contacts WHERE deleted_at IS NULL AND id > ? ORDER BY id LIMIT ?`,
			lastID, page,
		).Scan(&ids).Error; err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		lastID = ids[len(ids)-1]

		var rows []models.Contact
		if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
			// One bad row fails the batch decode; fall back to per-row loads.
			rows = rows[:0]
			for _, id := range ids {
				var one models.Contact
				if err := db.WithContext(ctx).First(&one, id).Error; err != nil {
					continue
				}
				rows = append(rows, one)
			}
		}
		for i := range rows {
			c := &rows[i]
			fixes := c.RecomputeDerivedColumns()
			if len(fixes) == 0 {
				continue
			}
			if perUserCols[c.UserID] == nil {
				perUserCols[c.UserID] = map[string]int{}
			}
			for _, f := range fixes {
				perUserCols[c.UserID][f.Column]++
			}
			perUserRows[c.UserID]++
		}
		if len(ids) < page {
			break
		}
	}

	var out []IntegrityFinding
	for uid, cols := range perUserCols {
		names := make([]string, 0, len(cols))
		for col := range cols {
			names = append(names, col)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, col := range names {
			parts = append(parts, fmt.Sprintf("%s x%d", col, cols[col]))
		}
		out = append(out, IntegrityFinding{
			Invariant: "INV-A5", Check: "derived_contact_column.divergent",
			Severity: IntegritySeverityViolation, UserID: uid, Count: perUserRows[uid],
			Detail: fmt.Sprintf(
				"%d contact(s) have a denormalized column that disagrees with the value re-derived from Card (%s) — rebuild with cmd/backfill-derived-columns or POST /admin/contacts/rebuild-derived",
				perUserRows[uid], strings.Join(parts, ", ")),
			Repairable: false,
		})
	}
	return out, nil
}
