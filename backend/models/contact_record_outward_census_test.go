package models

// TestOutwardProjectionCensus is the mechanical guard behind issue #1935: a
// NEW outward copy of a contact must not be able to build its Card through the
// unfiltered projection and ship private/secret data silently.
//
// The sensitivity rule (backend/AGENTS.md, "Sensitivity") is enforced in one
// place: models/contact_record.go's RecordForContactFiltered (and the
// FieldSelection address filter it calls). The per-surface tests pin the
// surfaces that exist today, but nothing failed when a *new* surface was
// added that called RecordForContact/RecordFromContact directly. This is the
// same "an undeclared member fails in both directions" shape as
// routes/authorization_matrix_test.go and services.ContactCascadeRegistry.
//
// The scanner walks every non-test .go file under backend/ and records:
//
//   - every call to a Record-building function (RecordForContact,
//     RecordForContactFiltered, RecordForContactForSync, RecordFromContact,
//     NewContactRecordResponse, NewContactRecordResponseFiltered), and
//   - every one-argument .Export(...) call in a file that imports a neutral
//     Card adapter (contactmodel/jscontact/vcard3/vcard4) — the exporter
//     sink a Card must pass through to leave the instance.
//
// Every enclosing function must then have a row in outwardProjectionSites
// below. A new call site with no row ("missing") and a row with no call site
// ("stale") both fail. A row classified outwardSiteFiltered may not also call
// an unfiltered builder, and a row classified outwardSiteInternal may not
// emit through an exporter. That forces a human to decide, at the moment the
// new surface is added, whether it is filtered, a deliberate owner/backup
// exception, or not an outward copy at all — with the reason recorded here.
//
// The build list is deliberately conservative: it catches direct record
// builders and format-adapter export sinks. A surface that re-implements the
// Card projection from scratch without either is still invisible to this
// gate; the per-surface sensitivity tests remain the behavioural backstop.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const outwardProjectionRoot = ".."

// outwardBuilderFuncs are the record-building functions whose call sites must
// be classified. NewContactRecordResponse* are included because a REST
// response DTO is a Card builder too; the owner-facing rows below document why
// those callers are not outward copies.
var outwardBuilderFuncs = map[string]bool{
	"RecordForContact":                 true,
	"RecordForContactFiltered":         true,
	"RecordForContactForSync":          true,
	"RecordFromContact":                true,
	"NewContactRecordResponse":         true,
	"NewContactRecordResponseFiltered": true,
}

// outwardAdapterImports marks a file as an exporter sink candidate. Every
// neutral-Card adapter package is listed; the census only counts a one-argument
// .Export(...) call, so an incidental method with the same name does not match.
var outwardAdapterImports = map[string]bool{
	"mycorrhizal/contactmodel": true,
	"mycorrhizal/jscontact":    true,
	"mycorrhizal/vcard3":       true,
	"mycorrhizal/vcard4":       true,
}

// outwardSiteClass is the reviewed classification of one call site.
type outwardSiteClass int

const (
	// outwardSiteFiltered goes through the sensitivity-filtered projection
	// (RecordForContactFiltered/ForSync or a FieldSelection-bearing wrapper).
	outwardSiteFiltered outwardSiteClass = iota
	// outwardSiteOwner is a deliberate full-fidelity copy to the owner
	// (REST read, CSV/account-bundle backup) — not an outward copy.
	outwardSiteOwner
	// outwardSiteInternal is a read-modify-write/backfill/restore path that
	// never sends the record anywhere.
	outwardSiteInternal
)

func (c outwardSiteClass) String() string {
	switch c {
	case outwardSiteFiltered:
		return "filtered"
	case outwardSiteOwner:
		return "owner"
	case outwardSiteInternal:
		return "internal"
	default:
		return "unknown"
	}
}

type outwardSiteRow struct {
	file   string // path relative to backend/, slash-separated
	fn     string // receiver-qualified top-level function name
	class  outwardSiteClass
	reason string
}

// outwardProjectionSites is the reviewed exception list. Every current call
// site of a record builder or Card exporter appears exactly once.
var outwardProjectionSites = []outwardSiteRow{
	// ── outward copies: they use the filtered projection ──────────────────
	{
		file: "carddav/backend.go", fn: "Backend.contactToAddressObject",
		class: outwardSiteFiltered,
		reason: "CardDAV is an outward copy and serves the sync view " +
			"(RecordForContactForSync), which withholds above-normal " +
			"addresses/personalInfo/edges/preferences/custom fields. Paired " +
			"with PreserveSensitiveAddresses on the write path.",
	},
	{
		file: "controllers/export_loss.go", fn: "renderContactExport",
		class: outwardSiteFiltered,
		reason: "The single shared computation behind the vCard3/vCard4/" +
			"JSContact exports and the preflight endpoint. It passes the " +
			"caller's FieldSelection to RecordForContactFiltered, so the " +
			"sensitivity default-deny runs before the adapter.",
	},
	{
		file: "controllers/contact_share_controller.go", fn: "CreateContactShare",
		class: outwardSiteFiltered,
		reason: "A contact share is a copy to another user. It builds " +
			"JSContact through RecordForContactFiltered with the request's " +
			"section and include_sensitive selection.",
	},

	// ── deliberate full-fidelity owner copies ─────────────────────────────
	{
		file: "services/account_bundle.go", fn: "loadBundleContacts",
		class: outwardSiteOwner,
		reason: "GET /export/account is the versioned full-fidelity export " +
			"used to move the user's own data between their own instances. " +
			"It deliberately keeps every sensitivity level; withholding " +
			"would be silent data loss (backend/AGENTS.md sensitivity " +
			"exceptions; ADR 0028 Decision 3).",
	},
	{
		file: "models/contact_summary.go", fn: "NewContactRecordResponseFiltered",
		class: outwardSiteOwner,
		reason: "Owner-facing REST contact response. The includeSensitive=" +
			"false path is the owner's own read (REST detail deliberately " +
			"keeps the whole card so a full-overwrite save cannot drop " +
			"data); the true path is an explicit opt-in. Not an outward " +
			"copy — outward surfaces must build their own filtered Record.",
	},
	{
		file: "models/contact_summary.go", fn: "NewContactRecordResponse",
		class: outwardSiteOwner,
		reason: "Owner-facing REST response helper; delegates to " +
			"NewContactRecordResponseFiltered with includeSensitive=false, " +
			"the nil-selection owner read above.",
	},
	{
		file: "controllers/contact_controller.go", fn: "CreateContact",
		class: outwardSiteOwner,
		reason: "REST create response to the authenticated owner; " +
			"sensitivity is not an access-control tier against the owner " +
			"(backend/AGENTS.md).",
	},
	{
		file: "controllers/contact_controller.go", fn: "GetContact",
		class: outwardSiteOwner,
		reason: "REST read response to the authenticated owner; same " +
			"owner-read rule as CreateContact.",
	},
	{
		file: "controllers/contact_controller.go", fn: "UpdateContact",
		class: outwardSiteOwner,
		reason: "REST update response to the authenticated owner; same " +
			"owner-read rule as CreateContact.",
	},
	{
		file: "controllers/contact_merge_controller.go", fn: "CommitContactMerge",
		class: outwardSiteOwner,
		reason: "REST merge response to the authenticated owner; same " +
			"owner-read rule as CreateContact.",
	},
	{
		file: "controllers/contact_detail_controller.go", fn: "buildContactDetailScoped",
		class: outwardSiteOwner,
		reason: "REST owner detail view; includeSensitive is an explicit " +
			"query opt-in. MCP get_contact reuses it with include_sensitive " +
			"and applies its own address filter; any other outward caller " +
			"must filter the returned Card before sending it out.",
	},

	// ── not outward copies: read-modify-write / migrations / restore ──────
	{
		file: "services/wedding_sync.go", fn: "SyncWeddingFromLifeEvent",
		class: outwardSiteInternal,
		reason: "Read-modify-write of a contact's own card to mirror a " +
			"LifeEvent date; the record is saved back, never sent outward.",
	},
	{
		file: "services/hobby_preference_backfill.go", fn: "MigrateHobbyPreferencesToPersonalInfo",
		class: outwardSiteInternal,
		reason: "One-shot data migration: reads the canonical record to " +
			"merge deprecated hobby Preference rows into it, then saves it " +
			"back. No outward copy.",
	},
	{
		file: "services/unicode_nfc_backfill.go", fn: "normalizeContactRecordToNFC",
		class: outwardSiteInternal,
		reason: "One-shot NFC-normalization migration; rewrites the stored " +
			"record and saves it. No outward copy.",
	},
	{
		file: "controllers/relationship_edge_controller.go", fn: "resolveRelationshipEndpoint",
		class: outwardSiteInternal,
		reason: "Thin-contact creation path; builds a fresh record only to " +
			"set CRM.Kind through ApplyRecordToContact, then saves it. No " +
			"outward copy.",
	},
	{
		file: "models/contact.go", fn: "Contact.deriveDenormalized",
		class: outwardSiteInternal,
		reason: "Save-path projection from the flat fields into the " +
			"canonical record; never an outward copy.",
	},
	{
		file: "models/contact_audit_snapshot.go", fn: "Contact.RestoreFullStateFrom",
		class: outwardSiteInternal,
		reason: "Audit undo/restore path; rebuilds photo media on the " +
			"persisted card. No outward copy.",
	},
}

type outwardProjectionSite struct {
	file          string
	fn            string
	line          int
	builders      map[string]bool
	exporterSinks int
}

func (s outwardProjectionSite) builderList() string {
	var names []string
	for name := range s.builders {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func (s outwardProjectionSite) callList() string {
	var names []string
	for name := range s.builders {
		names = append(names, name)
	}
	if s.exporterSinks > 0 {
		names = append(names, ".Export")
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// TestOutwardProjectionCensus_EverySiteClassified is the gate. Adding a new
// record builder or exporter call site without a row here fails; so does a
// row whose reason is empty or whose class contradicts its callees.
func TestOutwardProjectionCensus_EverySiteClassified(t *testing.T) {
	sites := scanOutwardProjections(t, outwardProjectionRoot)

	declared := map[string]outwardSiteRow{}
	for _, row := range outwardProjectionSites {
		key := row.file + ":" + row.fn
		require.NotEmptyf(t, row.reason, "%s: every classification needs a reason", key)
		_, dup := declared[key]
		require.Falsef(t, dup, "%s is declared twice", key)
		// Fail closed on a typo'd class rather than silently treating it as
		// filtered (the zero value).
		require.Containsf(t,
			[]outwardSiteClass{outwardSiteFiltered, outwardSiteOwner, outwardSiteInternal},
			row.class, "%s: unknown class %d", key, row.class)
		declared[key] = row
	}

	var missing, stale []string
	for key, site := range sites {
		row, ok := declared[key]
		if !ok {
			missing = append(missing, fmt.Sprintf(
				"%s (line %d: calls %s)", key, site.line, site.callList()))
			continue
		}
		switch row.class {
		case outwardSiteFiltered:
			for _, unfiltered := range []string{"RecordForContact", "RecordFromContact", "NewContactRecordResponse"} {
				require.Falsef(t, site.builders[unfiltered],
					"%s is declared filtered but calls the unfiltered %s; "+
						"route it through RecordForContactFiltered/ForSync or reclassify it",
					key, unfiltered)
			}
			require.Truef(t,
				site.builders["RecordForContactFiltered"] ||
					site.builders["RecordForContactForSync"] ||
					site.builders["NewContactRecordResponseFiltered"],
				"%s is declared filtered but calls no filtered projection; "+
					"route it through RecordForContactFiltered/ForSync or reclassify it", key)
		case outwardSiteInternal:
			require.Zerof(t, site.exporterSinks,
				"%s emits through a Card exporter but is classified internal; "+
					"an exporter sink must be filtered or a reviewed owner exception", key)
		}
	}
	for key := range declared {
		if _, ok := sites[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)

	require.Emptyf(t, missing,
		"record builder/exporter call sites with no classification in outwardProjectionSites. "+
			"Decide whether each is an outward copy (use the filtered projection), a deliberate "+
			"owner/full-fidelity exception, or not outward at all, and add a row with a reason "+
			"(issue #1935): %v", missing)
	require.Emptyf(t, stale,
		"outwardProjectionSites lists call sites that no longer exist (dead entries): %v", stale)
}

// TestOutwardProjectionCensus_ScannerFindsBuildersAndSinks proves the scanner
// itself is not silently passing: a synthetic new function that builds an
// unfiltered record or emits through an adapter must be returned as an
// undeclared site.
func TestOutwardProjectionCensus_ScannerFindsBuildersAndSinks(t *testing.T) {
	const src = `package sample

import (
	"mycorrhizal/contactmodel"
	"mycorrhizal/vcard4"
)

func Build(c *contactmodel.Record) *contactmodel.Record {
	return models.RecordForContact(nil, "", nil)
}

func Emit(r *contactmodel.Record) {
	vcard4.Adapter{}.Export(r)
}

func EmitWithExtraArg(r *contactmodel.Record, extra int) {
	vcard4.Adapter{}.Export(r, extra)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "sample.go", src, 0)
	require.NoError(t, err)

	sites := collectOutwardProjectionSites(fset, f, "sample.go", true)

	build, ok := sites["sample.go:Build"]
	require.True(t, ok, "Build calls RecordForContact and must be discovered")
	require.True(t, build.builders["RecordForContact"])

	emit, ok := sites["sample.go:Emit"]
	require.True(t, ok, "Emit calls the adapter's one-argument Export and must be discovered")
	require.Equal(t, 1, emit.exporterSinks)

	_, ok = sites["sample.go:EmitWithExtraArg"]
	require.False(t, ok, "a multi-argument Export is not a neutral-Card exporter sink")
}

// scanOutwardProjections walks backend/ and collects every record builder and
// exporter call site, keyed by "<relative file>:<receiver-qualified function>".
func scanOutwardProjections(t *testing.T, root string) map[string]outwardProjectionSite {
	t.Helper()

	fset := token.NewFileSet()
	sites := map[string]outwardProjectionSite{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		// The projection definitions themselves are not call sites; scanning
		// them would only create self-referential rows.
		if rel == "models/contact_record.go" {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		importsAdapter := false
		for _, imp := range f.Imports {
			if outwardAdapterImports[strings.Trim(imp.Path.Value, `"`)] {
				importsAdapter = true
				break
			}
		}

		for key, site := range collectOutwardProjectionSites(fset, f, rel, importsAdapter) {
			if prev, dup := sites[key]; dup {
				return fmt.Errorf("duplicate outward call-site key %q (lines %d and %d)", key, prev.line, site.line)
			}
			sites[key] = site
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, sites, "the outward projection scanner found zero call sites — it is broken")
	return sites
}

// collectOutwardProjectionSites inspects one parsed file. A call site is
// attributed to its enclosing top-level function (a closure is attributed to
// the function that contains it), so the registry identifies stable code
// locations rather than line numbers.
func collectOutwardProjectionSites(fset *token.FileSet, file *ast.File, rel string, importsAdapter bool) map[string]outwardProjectionSite {
	out := map[string]outwardProjectionSite{}

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		key := rel + ":" + receiverQualifiedName(fd)
		site := outwardProjectionSite{
			file:     rel,
			fn:       receiverQualifiedName(fd),
			line:     fset.Position(fd.Pos()).Line,
			builders: map[string]bool{},
		}

		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name := calledFunctionName(call.Fun); outwardBuilderFuncs[name] {
				site.builders[name] = true
			}
			if importsAdapter && len(call.Args) == 1 {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Export" {
					site.exporterSinks++
				}
			}
			return true
		})

		if len(site.builders) > 0 || site.exporterSinks > 0 {
			out[key] = site
		}
	}

	return out
}

// calledFunctionName returns the unqualified name of a call's callee:
// RecordForContact, models.RecordForContact, and (pkg.X).RecordForContact all
// yield "RecordForContact".
func calledFunctionName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.ParenExpr:
		return calledFunctionName(e.X)
	case *ast.IndexExpr:
		return calledFunctionName(e.X)
	case *ast.IndexListExpr:
		return calledFunctionName(e.X)
	default:
		return ""
	}
}

// receiverQualifiedName returns "Receiver.Method" for methods and "Func" for
// plain functions, matching the keys in outwardProjectionSites.
func receiverQualifiedName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	return exprTypeName(fd.Recv.List[0].Type) + "." + fd.Name.Name
}

func exprTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return exprTypeName(e.X)
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		return exprTypeName(e.X)
	case *ast.IndexListExpr:
		return exprTypeName(e.X)
	default:
		return ""
	}
}
