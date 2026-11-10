package services

// Issue #2300 (ADR 0043 decisions 3, 4 and 7): no import or sync path creates,
// matches or rewrites an entity. Inbound ORG/ADR (CardDAV PUT, vCard/JSContact
// import, CSV column mapping) stay contact values; Monica/Meerkat map no
// entity; MCP exposes none. The only writers are the entity REST surface, the
// account-bundle / CSV-backup restore (the owner's own full backup), the
// delete/purge cascades and the contact-link service.
//
// The test pins that structurally: every non-test Go file outside package
// models that mentions an entity model, the link writers or the entity ledger
// kinds must be in the allow-list below, and the allow-list never contains an
// import-from-other-system, CardDAV/CalDAV/WebDAV or MCP file. A new file that
// touches entities fails here until its author decides, in review, that it is a
// deliberate entity surface.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var entityTouchRE = regexp.MustCompile(`models\.(Entity|ContactEntityLink|DismissedEntityPair|EntityDeletionSnapshot)\b|\b(Create|Save)ContactEntityLink\(|\bimportEntities\(|ImportSourceLinkKindEntity|\bMapped(Entity|EntityLink|EntityFieldValue)\b|mapBundleEntities\(`)

// entitySurfaceFiles are the deliberate entity surfaces (paths relative to
// backend/). Each is either the entity API, a cascade, or the owner's own
// backup/restore.
var entitySurfaceFiles = map[string]string{
	"controllers/contact_entity_link_controller.go": "link REST API (#2297)",
	"controllers/entity_controller.go":              "entity REST API (#2296)",
	"controllers/export_csv_entities.go":            "CSV backup entity sections (#2300), read-only",
	"controllers/user_delete_cascade.go":            "account deletion cascade",
	"services/account_bundle_import.go":             "maps the owner's own bundle into the import plan (#2300)",
	"services/import_source.go":                     "generic engine: dispatches importEntities; only the bundle/CSV mappers ever fill plan entities",
	"services/account_bundle.go":                    "account bundle export (#2300), read-only",
	"services/contact_delete.go":                    "contact delete cascade registry",
	"services/contact_entity_link.go":               "link writes (#2297)",
	"services/entity_csv.go":                        "CSV backup restore (#2300)",
	"services/entity_delete.go":                     "entity delete/restore cascade",
	"services/entity_merge.go":                      "entity merge (#2299)",
	"services/entity_suggestions.go":                "entity suggestions (#2299), read-only",
	"services/import_source_entities.go":            "account-bundle/CSV-backup entity restore (#2300)",
	"services/purge_service.go":                     "retention purge",
}

// neverEntityPrefixes are paths that must not mention an entity at all.
var neverEntityPrefixes = []string{
	"carddav/", "caldav/", "vcard3/", "vcard4/", "jscontact/", "monica/", "meerkat/", "meerkatapi/",
	"integrations/", "contactmodel/", "roundtrip/", "differential/",
	"services/monica_import", "services/meerkat_import", "services/import_service.go",
	"services/import_groupings_service.go", "services/import_custom_field_mapping.go",
	"services/source_import_session.go", "services/carddav", "services/mycorrhizal_import_session.go",
	"controllers/import_controller.go", "controllers/monica_import", "controllers/meerkat_import",
	"controllers/carddav", "controllers/webdav_controller.go", "controllers/mcp_controller.go",
}

func TestNoImportOrSyncPathTouchesEntities(t *testing.T) {
	root, err := filepath.Abs("..")
	require.NoError(t, err)

	touching := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch rel {
			case "models", "node_modules", "static", ".gremlins", "cmd":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if entityTouchRE.Match(src) {
			touching[rel] = true
		}
		return nil
	})
	require.NoError(t, err)

	var unexpected []string
	for rel := range touching {
		if _, ok := entitySurfaceFiles[rel]; !ok {
			unexpected = append(unexpected, rel)
		}
	}
	sort.Strings(unexpected)
	assert.Empty(t, unexpected, "these files touch entities but are not a declared entity surface (ADR 0043 decisions 3/7: imports and sync never create, match or rewrite an entity); add them to entitySurfaceFiles only if they are deliberate")

	// Anti-rot: the allow-list names real touching files, and never an
	// import/sync/MCP path.
	for rel := range entitySurfaceFiles {
		assert.True(t, touching[rel], "%s is allow-listed but no longer touches entities; remove it", rel)
		for _, p := range neverEntityPrefixes {
			assert.False(t, strings.HasPrefix(rel, p), "%s is an import/sync/MCP path and must never be an entity surface", rel)
		}
	}
}
