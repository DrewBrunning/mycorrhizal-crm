package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// CSV restore of the entity sections (issue #2300, ADR 0043 decision 7).
//
// The flat CSV backup appends `=== ENTITIES ===`, `=== ENTITY LINKS ===` and
// `=== ENTITY FIELDS ===` after every older section (controllers/
// export_csv_entities.go). This file reads just those three sections back and
// lands them through the same importEntities pass the account bundle uses, so
// the two paths share one set of rules: ids are re-minted and the ledger makes
// a re-run idempotent, an id the account already owns is a reported conflict
// and never an overwrite, a link needs a live contact, and every row is
// validated like a REST write. The CONTACTS section and every other section
// are ignored here: contacts are restored by the existing contact import, and a
// link whose contact is not in this account is reported, not invented.

// System is the import-ledger classifier for a CSV backup restore.
const entityCSVSystem = "csv-backup"

// MaxEntityCSVRows bounds the entity rows one restore reads, per section, the
// same way MaxCSVRows bounds a contact import.
const MaxEntityCSVRows = MaxCSVRows

// EntityCSVRejection is a client-side rejection of the uploaded file (too many
// rows, unreadable CSV), as opposed to a database fault. The HTTP layer maps it
// to a 400; anything else is a server error.
type EntityCSVRejection struct{ msg string }

func (e *EntityCSVRejection) Error() string { return e.msg }

// IsEntityCSVRejection reports whether err is, or wraps, an EntityCSVRejection.
func IsEntityCSVRejection(err error) bool {
	var r *EntityCSVRejection
	return errors.As(err, &r)
}

// EntityCSVSections is the parsed content of the three entity sections. A CSV
// without the markers (any pre-1.7 export) parses to all-empty slices.
type EntityCSVSections struct {
	Entities []MappedEntity
	Links    []MappedEntityLink
	Values   []MappedEntityFieldValue
	// Kinds maps an entity id to its kind (needed to find a value's definition).
	Kinds map[string]string
	// ValueKeys maps a value's mapped FieldDefinitionID to its (kind, key).
	ValueKeys map[string][2]string
}

// EntityCSVReport is the outcome of RestoreEntityCSV.
type EntityCSVReport struct {
	EntitiesCreated    int
	LinksCreated       int
	FieldValuesCreated int
	Issues             []ImportIssue
}

const (
	csvEntitiesMarker      = "=== ENTITIES ==="
	csvEntityLinksMarker   = "=== ENTITY LINKS ==="
	csvEntityFieldsMarker  = "=== ENTITY FIELDS ==="
	csvFormulaLeadersChars = "=+-@\t\r"
)

var csvMarkerRE = regexp.MustCompile(`^=== [A-Z_ ]+ ===$`)

// unneutralizeCSV reverses the exporter's csvSafe on a plain-text cell: a
// leading apostrophe in front of a formula leader is the neutralization marker
// and is stripped. (A genuine value that itself begins with an apostrophe and
// a leader loses that apostrophe: the documented, accepted ambiguity of the
// text-marker convention.)
func unneutralizeCSV(s string) string {
	if len(s) >= 2 && s[0] == '\'' && strings.ContainsRune(csvFormulaLeadersChars, rune(s[1])) {
		return s[1:]
	}
	return s
}

// unneutralizeCSVJSON is the same for a column that holds JSON: JSON never
// starts with an apostrophe, so one there is always the marker.
func unneutralizeCSVJSON(s string) string {
	return strings.TrimPrefix(s, "'")
}

// ParseEntityCSVSections reads the entity sections out of a full CSV backup.
// Unknown sections and the CONTACTS section are skipped; malformed rows are
// returned as issues rather than failing the whole parse.
func ParseEntityCSVSections(r io.Reader) (*EntityCSVSections, []ImportIssue, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	out := &EntityCSVSections{Kinds: map[string]string{}, ValueKeys: map[string][2]string{}}
	var issues []ImportIssue

	section := ""
	headerSeen := false
	rows := map[string]int{}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, &EntityCSVRejection{msg: fmt.Sprintf("invalid CSV format: %v", err)} // # pragma: no cover — defensive: LazyQuotes and a variable field count leave the reader nothing to reject
		}
		if len(rec) == 1 && csvMarkerRE.MatchString(rec[0]) {
			section, headerSeen = rec[0], false
			continue
		}
		if section != csvEntitiesMarker && section != csvEntityLinksMarker && section != csvEntityFieldsMarker {
			continue
		}
		if !headerSeen {
			headerSeen = true
			continue
		}
		rows[section]++
		if rows[section] > MaxEntityCSVRows {
			return nil, nil, &EntityCSVRejection{msg: fmt.Sprintf("too many rows in %s: maximum is %d rows", section, MaxEntityCSVRows)}
		}
		switch section {
		case csvEntitiesMarker:
			if len(rec) < 5 {
				issues = append(issues, ImportIssue{Record: section, Field: "row", Category: ImportIssueCategoryInvalid, Message: "an entity row has too few columns; it was not imported"})
				continue
			}
			e := MappedEntity{
				Ref: SourceRef{System: entityCSVSystem, ExternalID: sourceKindEntity + "/" + rec[0]},
				ID:  rec[0], Kind: rec[1], Name: unneutralizeCSV(rec[2]), Sensitivity: rec[3],
				Core: json.RawMessage(unneutralizeCSVJSON(rec[4])),
			}
			if len(rec) > 6 {
				e.CreatedAt, _ = time.Parse(time.RFC3339, rec[6])
			}
			if len(rec) > 7 {
				e.UpdatedAt, _ = time.Parse(time.RFC3339, rec[7])
			}
			out.Entities = append(out.Entities, e)
			out.Kinds[e.ID] = e.Kind
		case csvEntityLinksMarker:
			if len(rec) < 13 {
				issues = append(issues, ImportIssue{Record: section, Field: "row", Category: ImportIssueCategoryInvalid, Message: "an entity link row has too few columns (a link needs its Contact UID); it was not imported"})
				continue
			}
			start, ok1 := parseCSVPartialDate(rec[9])
			end, ok2 := parseCSVPartialDate(rec[10])
			if !ok1 || !ok2 {
				issues = append(issues, ImportIssue{Record: section + " " + rec[0], Field: "entity_link.period", Category: ImportIssueCategoryInvalid, Message: "unreadable start or end date; the link was not imported"})
				continue
			}
			out.Links = append(out.Links, MappedEntityLink{
				Ref: SourceRef{System: entityCSVSystem, ExternalID: sourceKindEntityLink + "/" + rec[0]},
				ID:  rec[0], Contact: SourceRef{System: entityCSVSystem, ExternalID: "contact/" + rec[12]},
				EntityID: rec[3], Role: rec[6], RoleLabel: unneutralizeCSV(rec[7]), IsPrimary: rec[8] == "true",
				Start: start, End: end, Sensitivity: rec[11],
			})
		case csvEntityFieldsMarker:
			if len(rec) < 3 {
				issues = append(issues, ImportIssue{Record: section, Field: "row", Category: ImportIssueCategoryInvalid, Message: "an entity field row has too few columns; it was not imported"})
				continue
			}
			kind := out.Kinds[rec[0]]
			defRef := kind + "/" + rec[1]
			out.ValueKeys[defRef] = [2]string{kind, rec[1]}
			out.Values = append(out.Values, MappedEntityFieldValue{
				Ref:               SourceRef{System: entityCSVSystem, ExternalID: "entity_field_value/" + defRef + "/" + rec[0]},
				EntityID:          rec[0],
				FieldDefinitionID: defRef,
				Value:             json.RawMessage(unneutralizeCSVJSON(rec[2])),
			})
		}
	}
	return out, issues, nil
}

var csvPartialDateRE = regexp.MustCompile(`^(?:(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?|--(\d{2})(?:-(\d{2}))?|---(\d{2}))$`)

// parseCSVPartialDate reads what the exporter's formatExportPartialDate
// writes. Empty is (nil, true); an unreadable value is (nil, false).
func parseCSVPartialDate(s string) (*contactmodel.PartialDate, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	m := csvPartialDateRE.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	num := func(v string) *int {
		if v == "" {
			return nil
		}
		n, _ := strconv.Atoi(v)
		return &n
	}
	p := &contactmodel.PartialDate{}
	switch {
	case m[1] != "":
		p.Year, p.Month, p.Day = num(m[1]), num(m[2]), num(m[3])
	case m[4] != "":
		p.Month, p.Day = num(m[4]), num(m[5])
	default:
		p.Day = num(m[6])
	}
	return p, true
}

// RestoreEntityCSV restores the entity sections of a CSV backup into the
// user's account in one transaction. Nothing is overwritten; see the file
// comment for the shared rules.
func RestoreEntityCSV(ctx context.Context, db *gorm.DB, userID uint, r io.Reader) (*EntityCSVReport, error) {
	sections, parseIssues, err := ParseEntityCSVSections(r)
	if err != nil {
		return nil, err
	}
	report := &ImportReport{}
	for _, i := range parseIssues {
		report.appendIssue(i)
	}
	plan := &ImportSourcePlan{System: entityCSVSystem, Entities: sections.Entities, EntityLinks: sections.Links, EntityFieldValues: sections.Values}

	var result EntityCSVReport
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		imported := loadSourceLinks(tx, userID, plan.System)
		ids := importIDMap{}

		// Contacts are resolved by vCard UID against the account's own live
		// contacts; the CSV does not carry them.
		var uids []string
		if err := tx.Model(&models.Contact{}).Where("user_id = ?", userID).Pluck("vcard_uid", &uids).Error; err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		}
		liveContact := make(map[string]bool, len(uids))
		for _, u := range uids {
			liveContact[u] = true
		}
		uidOf := func(record string, ref SourceRef) (string, bool) {
			uid := strings.TrimPrefix(ref.ExternalID, "contact/")
			if !liveContact[uid] {
				report.appendIssue(ImportIssue{Record: record, Field: "entity_link.contact", Category: ImportIssueCategoryUnsupported, Message: "references a contact that is not in this account"})
				return "", false
			}
			return uid, true
		}
		skipImported := func(record string, ref SourceRef) bool {
			if imported[ref.ExternalID] {
				report.appendIssue(ImportIssue{Record: record, Field: ref.String(), Category: ImportIssueCategorySkipped, Message: "already imported in an earlier run"})
				return true
			}
			return false
		}

		// The CSV carries no definitions: a value finds its definition by
		// (entity kind, key) among the account's own, or is reported.
		for ref, kk := range sections.ValueKeys {
			var def models.FieldDefinition
			if tx.Where("user_id = ? AND target = ? AND key = ?", userID, kk[0], kk[1]).First(&def).Error == nil {
				ids[sourceKindFieldDefinition+"/"+ref] = def.ID
			}
		}

		before, err := countEntityRows(tx, userID)
		if err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		}
		if err := importEntities(tx, userID, plan, imported, uidOf, ids, skipImported, report); err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err // # pragma: no cover — defensive: importEntities only returns database faults
		}
		after, err := countEntityRows(tx, userID)
		if err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return err // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		}
		result = EntityCSVReport{
			EntitiesCreated:    after[0] - before[0],
			LinksCreated:       after[1] - before[1],
			FieldValuesCreated: after[2] - before[2],
			Issues:             report.Issues,
		}
		return nil
	})
	if err != nil {
		return nil, err // # pragma: no cover — defensive: the transaction only fails on a database fault
	}
	return &result, nil
}

// countEntityRows returns the live entity, link and entity-owned value counts.
func countEntityRows(tx *gorm.DB, userID uint) ([3]int, error) {
	var n [3]int64
	entityDefs := tx.Model(&models.FieldDefinition{}).Select("id").Where("user_id = ? AND target <> ?", userID, models.FieldDefinitionTargetContact)
	for i, q := range []*gorm.DB{
		tx.Model(&models.Entity{}).Where("user_id = ?", userID),
		tx.Model(&models.ContactEntityLink{}).Where("user_id = ?", userID),
		tx.Model(&models.FieldValue{}).Where("user_id = ? AND field_definition_id IN (?)", userID, entityDefs),
	} {
		if err := q.Count(&n[i]).Error; err != nil { // # pragma: no cover — defensive error handling, unreachable in a healthy migrated schema
			return [3]int{}, err // # pragma: no cover — defensive: a healthy migrated schema does not fail this query
		}
	}
	return [3]int{int(n[0]), int(n[1]), int(n[2])}, nil
}
