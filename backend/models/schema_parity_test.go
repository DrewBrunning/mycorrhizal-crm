package models

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Model <-> migration-SQL parity (CLAUDE.md backend trap #1).
//
// The migration SQL in database/migrations is the schema; the GORM struct tags
// are a second, independent description of it that GORM uses to build every
// INSERT/UPDATE/SELECT. When the two disagree the failure is silent: an
// AutoMigrate-built test DB derives its schema from the *tags*, so it agrees
// with itself and every test passes, while production writes to a column that
// does not exist (ContactSyncLink.ETag -> `e_tag` vs the real `etag`, which
// shipped broken) or GORM swaps a zero value for a tag default nobody meant
// (OccasionObligation.Active `default:true`, PR #1240).
//
// These tests parse every registered model with the connection's own naming
// strategy (the production gorm.Config) and check it against PRAGMA
// table_info of the real migrated schema (dbtest.New -> database.InitDB).

// registeredModels is the authoritative list of every GORM-persisted model.
// TestRegisteredModelsComplete fails if a table-mapped struct in this package
// is missing from it (or from nonTableStructs), and
// TestEveryMigratedTableHasAModel fails if a migrated table has no model here
// (or no entry in tablesWithoutModel).
var registeredModels = []any{
	&AlertState{},
	&ApiToken{},
	&Activity{},
	&Attachment{},
	&AuditEvent{},
	&CadencePolicy{},
	&CalendarEventLink{},
	&CalendarSubscription{},
	&CardDAVSync{},
	&Circle{},
	&CircleMember{},
	&Contact{},
	&ContactShare{},
	&ContactSubscription{},
	&ContactSyncConflict{},
	&ContactSyncLink{},
	&ContactTag{},
	&ConversationAgenda{},
	&DataDecayPolicy{},
	&DeviceGrant{},
	&DeviceRegistration{},
	&DismissedDuplicatePair{},
	&DismissedHouseholdSuggestion{},
	&ExternalActivity{},
	&ExternalIdentity{},
	&FieldDefinition{},
	&FieldValue{},
	&Gift{},
	&Household{},
	&HouseholdMember{},
	&IdempotencyKey{},
	&ImmichConfig{},
	&ImportRun{},
	&ImportSourceLink{},
	&JobExecution{},
	&JobRun{},
	&LifeEvent{},
	&LifeEventSuggestionResolution{},
	&LinkFieldType{},
	&Note{},
	&NotificationConfig{},
	&NotificationDelivery{},
	&OccasionEvent{},
	&OccasionEventAttendee{},
	&OccasionObligation{},
	&OperationalCheckResult{},
	&PaperlessConfig{},
	&Preference{},
	&PushSubscription{},
	&ReachOutCursor{},
	&ReachOutSuggestion{},
	&RecoveryCode{},
	&RelationshipEdge{},
	&Reminder{},
	&ReminderCompletion{},
	&SeafileConfig{},
	&ServerSetting{},
	&Session{},
	&StorageSample{},
	&SystemEvent{},
	&Tag{},
	&User{},
	&WebDAVConfig{},
	&Webhook{},
	&WebhookDelivery{},
}

// nonTableStructs are structs in this package that look table-mapped to the
// AST heuristic in TestRegisteredModelsComplete (they embed gorm.Model, carry
// a primaryKey tag, or declare TableName) but are not persisted models in
// their own right. Each entry needs a reason.
var nonTableStructs = map[string]string{}

// tablesWithoutModel are migrated tables that deliberately have no GORM model
// (raw-SQL-only tables, FTS virtual tables and their shadow tables, the
// migrator's own bookkeeping). Each entry needs a reason.
//
// FTS5 shadow tables (<vtab>_config/_content/_data/_docsize/_idx) are
// recognised automatically from their parent virtual table, and GORM
// many2many join tables are covered by the models that declare them.
var tablesWithoutModel = map[string]string{
	"schema_migrations":    "golang-migrate's own version bookkeeping; never touched through GORM",
	"contacts_fts":         "FTS5 virtual table maintained by migration triggers (ADR 0012 INV-D9); queried with raw SQL only",
	"notes_fts":            "FTS5 virtual table maintained by migration triggers; queried with raw SQL only",
	"activities_fts":       "FTS5 virtual table maintained by migration triggers; queried with raw SQL only",
	"data_backfills":       "startup-backfill bookkeeping written with raw SQL by atrest/ and services/unicode_nfc_backfill.go",
	"data_encryption_keys": "at-rest DEK envelope store, raw SQL only in atrest/ (kept out of GORM on purpose)",
}

// tagDefaultAllowed are non-pointer bool/numeric fields whose `default:` tag
// differs from the Go zero value, reviewed as safe. GORM omits such a field
// from INSERT whenever it holds the zero value, so an explicit false/0 is
// silently replaced by the default (the PR #1240 bug). An entry is only safe
// when the zero value is never a legitimate value to persist. Key is
// "Model.Field".
var tagDefaultAllowed = map[string]string{
	"Activity.Revision":           "revision token (ADR 0006): `<-:create`, starts at 1 by construction; 0 is never a value to persist",
	"Contact.Revision":            "revision token (ADR 0006): `<-:create`, starts at 1 by construction; 0 is never a value to persist",
	"LifeEvent.Revision":          "revision token (ADR 0006): `<-:create`, starts at 1 by construction; 0 is never a value to persist",
	"Note.Revision":               "revision token (ADR 0006): `<-:create`, starts at 1 by construction; 0 is never a value to persist",
	"Reminder.Revision":           "revision token (ADR 0006): `<-:create`, starts at 1 by construction; 0 is never a value to persist",
	"RelationshipEdge.Confidence": "server-derived, never client-supplied; every creation site sets a positive confidence (user edges 1.0, suggestions 0.4-0.9)",
	"WebhookDelivery.Attempts":    "1-based attempt counter; saveDelivery is only ever called with attempt >= 1",
}

func parseModel(t *testing.T, db *gorm.DB, m any) *schema.Schema {
	t.Helper()
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(m), "parse %T", m)
	return stmt.Schema
}

type pragmaColumn struct {
	CID       int
	Name      string
	Type      string
	NotNull   int
	DfltValue *string
	PK        int
}

func tableColumns(t *testing.T, db *gorm.DB, table string) map[string]pragmaColumn {
	t.Helper()
	var cols []pragmaColumn
	// PRAGMA table_info cannot take a bound parameter; table names come from
	// the parsed model schema, not input.
	require.NoError(t, db.Raw(fmt.Sprintf("SELECT cid, name, type, `notnull` AS not_null, dflt_value, pk FROM pragma_table_info(%s)", strconv.Quote(table))).Scan(&cols).Error)
	out := make(map[string]pragmaColumn, len(cols))
	for _, c := range cols {
		out[c.Name] = c
	}
	return out
}

// persistedFields is every field GORM will read or write as a column of the
// model's own table.
func persistedFields(s *schema.Schema) []*schema.Field {
	var out []*schema.Field
	for _, f := range s.Fields {
		if f.DBName == "" || f.IgnoreMigration {
			continue
		}
		if !f.Creatable && !f.Updatable && !f.Readable {
			continue
		}
		out = append(out, f)
	}
	return out
}

// TestModelColumnsExistInMigratedSchema: every column GORM derives for a
// registered model is a real column of the real migrated table.
func TestModelColumnsExistInMigratedSchema(t *testing.T) {
	db := dbtest.New(t)
	for _, m := range registeredModels {
		s := parseModel(t, db, m)
		cols := tableColumns(t, db, s.Table)
		if len(cols) == 0 {
			t.Errorf("%s: table %q does not exist in the migrated schema", s.Name, s.Table)
			continue
		}
		for _, f := range persistedFields(s) {
			if _, ok := cols[f.DBName]; !ok {
				t.Errorf("%s.%s: GORM maps it to column %s.%s, which the migrations do not create (add an explicit gorm:\"column:...\" tag, or a migration)",
					s.Name, f.Name, s.Table, f.DBName)
			}
		}
		for _, rel := range s.Relationships.Relations {
			if rel.JoinTable == nil {
				continue
			}
			jcols := tableColumns(t, db, rel.JoinTable.Table)
			if len(jcols) == 0 {
				t.Errorf("%s.%s: many2many join table %q does not exist in the migrated schema", s.Name, rel.Name, rel.JoinTable.Table)
				continue
			}
			for _, f := range rel.JoinTable.Fields {
				if f.DBName == "" {
					continue
				}
				if _, ok := jcols[f.DBName]; !ok {
					t.Errorf("%s.%s: many2many join column %s.%s does not exist in the migrated schema", s.Name, rel.Name, rel.JoinTable.Table, f.DBName)
				}
			}
		}
	}
}

// normalizeDefault folds the spellings GORM tags and SQLite DDL use for the
// same default into one form: quotes stripped, booleans as 0/1, NULL upper.
func normalizeDefault(v string) string {
	v = strings.TrimSpace(v)
	for len(v) >= 2 && v[0] == '(' && v[len(v)-1] == ')' {
		v = strings.TrimSpace(v[1 : len(v)-1])
	}
	if len(v) >= 2 && (v[0] == '\'' && v[len(v)-1] == '\'' || v[0] == '"' && v[len(v)-1] == '"') {
		return "s:" + v[1:len(v)-1]
	}
	switch strings.ToLower(v) {
	case "true":
		return "1"
	case "false":
		return "0"
	case "null":
		return "NULL"
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	// An unquoted word in a GORM tag (default:pending) is a string literal.
	return "s:" + v
}

func isZeroDefault(k reflect.Kind, norm string) bool {
	switch k {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return norm == "0"
	}
	return true
}

// TestModelTagDefaultsMatchMigrations covers the PR #1240 class from two
// sides:
//
//  1. A `default:` tag on a non-pointer bool/numeric field whose value is not
//     the Go zero value is a data-loss trap: GORM leaves a zero-valued field out
//     of the INSERT so the column default wins, i.e. `Active: false` persists
//     as true. Such a tag must be dropped (let GORM write the zero value; the
//     SQL DEFAULT still serves raw SQL) or make the field a pointer. Strings are
//     exempt: every non-empty string tag default in this package is an
//     enum/token column whose empty value means "unset", so collapsing "" to
//     the default is the intended behavior, not a lost write.
//  2. Any `default:` tag must agree with the migration's column DEFAULT. A
//     disagreeing (or absent) SQL default means AutoMigrate-built test DBs and
//     production DBs behave differently for the same Create, and the tag is
//     lying about the schema.
func TestModelTagDefaultsMatchMigrations(t *testing.T) {
	db := dbtest.New(t)
	usedAllow := map[string]bool{}
	for _, m := range registeredModels {
		s := parseModel(t, db, m)
		cols := tableColumns(t, db, s.Table)
		for _, f := range persistedFields(s) {
			if !f.HasDefaultValue || f.DefaultValue == "" || f.AutoIncrement {
				continue
			}
			key := s.Name + "." + f.Name
			tagNorm := normalizeDefault(f.DefaultValue)

			if f.FieldType.Kind() != reflect.Pointer && !isZeroDefault(f.FieldType.Kind(), tagNorm) {
				usedAllow[key] = true
				if _, ok := tagDefaultAllowed[key]; !ok {
					t.Errorf("%s: `default:%s` on a non-pointer %s: GORM omits the zero value from INSERT, so an explicit %v silently persists as %s (the PR #1240 bug). Drop the tag default or make the field a pointer.",
						key, f.DefaultValue, f.FieldType, reflect.Zero(f.FieldType).Interface(), f.DefaultValue)
				}
			}

			col, ok := cols[f.DBName]
			if !ok {
				continue // reported by TestModelColumnsExistInMigratedSchema
			}
			if tagNorm == "NULL" && col.DfltValue == nil {
				continue
			}
			if col.DfltValue == nil {
				t.Errorf("%s: tag says `default:%s` but migrated column %s.%s has no DEFAULT", key, f.DefaultValue, s.Table, f.DBName)
				continue
			}
			sqlNorm := normalizeDefault(*col.DfltValue)
			// A numeric SQL default is written unquoted; an unquoted numeric tag
			// normalizes the same way. A string-kind field with a numeric-looking
			// default compares on the raw text.
			if f.FieldType.Kind() == reflect.String {
				tagNorm = strings.TrimPrefix(tagNorm, "s:")
				sqlNorm = strings.TrimPrefix(sqlNorm, "s:")
			}
			if tagNorm != sqlNorm {
				t.Errorf("%s: tag `default:%s` disagrees with migrated column %s.%s DEFAULT %s", key, f.DefaultValue, s.Table, f.DBName, *col.DfltValue)
			}
		}
	}
	for key := range tagDefaultAllowed {
		if !usedAllow[key] {
			t.Errorf("tagDefaultAllowed entry %q no longer matches a non-zero tag default; remove it", key)
		}
	}
}

// TestRegisteredModelsComplete scans this package's non-test source for
// struct types that look table-mapped (embed gorm.Model, carry a primaryKey
// tag, or declare a TableName method) and requires each to be in
// registeredModels or nonTableStructs, so a new model cannot silently skip
// the parity checks above.
func TestRegisteredModelsComplete(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)

	candidates := map[string]bool{}
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Name.Name == "TableName" && d.Recv != nil && len(d.Recv.List) == 1 {
						typ := d.Recv.List[0].Type
						if st, ok := typ.(*ast.StarExpr); ok {
							typ = st.X
						}
						if id, ok := typ.(*ast.Ident); ok {
							candidates[id.Name] = true
						}
					}
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						st, ok := ts.Type.(*ast.StructType)
						if !ok {
							continue
						}
						for _, fld := range st.Fields.List {
							if sel, ok := fld.Type.(*ast.SelectorExpr); ok && len(fld.Names) == 0 && sel.Sel.Name == "Model" {
								if x, ok := sel.X.(*ast.Ident); ok && x.Name == "gorm" {
									candidates[ts.Name.Name] = true
								}
							}
							if fld.Tag != nil {
								tag, _ := strconv.Unquote(fld.Tag.Value)
								if strings.Contains(strings.ToLower(reflect.StructTag(tag).Get("gorm")), "primarykey") {
									candidates[ts.Name.Name] = true
								}
							}
						}
					}
				}
			}
		}
	}

	registered := map[string]bool{}
	for _, m := range registeredModels {
		registered[reflect.TypeOf(m).Elem().Name()] = true
	}
	var missing []string
	for name := range candidates {
		if !registered[name] {
			if _, ok := nonTableStructs[name]; !ok {
				missing = append(missing, name)
			}
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing, "table-mapped structs missing from registeredModels (or nonTableStructs with a reason) in schema_parity_test.go")

	for name := range nonTableStructs {
		require.True(t, candidates[name], "nonTableStructs entry %q no longer matches a candidate struct; remove it", name)
	}
}

// TestEveryMigratedTableHasAModel is the table-side completeness check: every
// real table the migrations create is mapped by a registered model, or is
// listed in tablesWithoutModel with a reason.
func TestEveryMigratedTableHasAModel(t *testing.T) {
	db := dbtest.New(t)
	var tables []string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`).Scan(&tables).Error)
	var vtabs []string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type = 'table' AND sql LIKE 'CREATE VIRTUAL TABLE%'`).Scan(&vtabs).Error)

	mapped := map[string]bool{}
	for _, vt := range vtabs {
		for _, suffix := range []string{"_config", "_content", "_data", "_docsize", "_idx"} {
			mapped[vt+suffix] = true // FTS5 shadow table, owned by its virtual table
		}
	}
	for _, m := range registeredModels {
		s := parseModel(t, db, m)
		mapped[s.Table] = true
		for _, rel := range s.Relationships.Relations {
			if rel.JoinTable != nil {
				mapped[rel.JoinTable.Table] = true
			}
		}
	}
	isTable := map[string]bool{}
	var unmapped []string
	for _, tbl := range tables {
		isTable[tbl] = true
		if !mapped[tbl] {
			if _, ok := tablesWithoutModel[tbl]; !ok {
				unmapped = append(unmapped, tbl)
			}
		}
	}
	require.Empty(t, unmapped, "migrated tables with no registered model (add the model to registeredModels, or the table to tablesWithoutModel with a reason)")
	for tbl := range tablesWithoutModel {
		require.True(t, isTable[tbl], "tablesWithoutModel entry %q is not a migrated table; remove it", tbl)
	}
}
