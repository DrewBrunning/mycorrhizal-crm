package services

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2290, docs/security/standing-shares-review.md section 4 and 7.2/7.3:
// the field census and the single-chokepoint source scan.

type fieldClass struct{ exposed bool }

func jsonTagOrName(f reflect.StructField) string {
	if tag := f.Tag.Get("json"); tag != "" && tag != "-" {
		return strings.Split(tag, ",")[0]
	}
	return f.Name
}

func fieldNames(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

// TestShareGrantViews_FieldCensus classes every exported field of the source
// models as exposed or withheld with a reason. A new column on Note, Activity
// or Circle fails this test until a person classifies it, so it cannot reach a
// recipient by accident (ADR 0042 decision 4).
func TestShareGrantViews_FieldCensus(t *testing.T) {
	type census struct {
		model    reflect.Type
		exposed  map[string]string // field -> view field that carries it
		withheld map[string]string // field -> reason
		view     reflect.Type
	}
	cases := map[string]census{
		"note": {
			model: reflect.TypeOf(models.Note{}), view: reflect.TypeOf(ShareNoteView{}),
			exposed: map[string]string{"Content": "Content", "Date": "Date"},
			withheld: map[string]string{
				"ID": "integer pk", "CreatedAt": "internal", "UpdatedAt": "internal", "DeletedAt": "internal",
				"UserID": "owner identity", "ContactID": "the note's contact link is not granted", "Contact": "contact link",
				"UUID": "internal identity", "Revision": "internal", "ETag": "internal", "Deleted": "tombstone marker",
				"Model": "embedded gorm.Model: ids and timestamps are internal",
			},
		},
		"activity": {
			model: reflect.TypeOf(models.Activity{}), view: reflect.TypeOf(ShareActivityView{}),
			exposed: map[string]string{"Title": "Title", "Description": "Description", "Location": "Location",
				"Date": "Date", "EndDate": "EndDate", "Type": "Type"},
			withheld: map[string]string{
				"ID": "integer pk", "CreatedAt": "internal", "UpdatedAt": "internal", "DeletedAt": "internal",
				"UserID": "owner identity", "UUID": "internal identity", "Contacts": "participants are not granted",
				"ExternalRef": "external ref / GeoPulse data is not granted", "Revision": "internal", "ETag": "internal",
				"Deleted": "tombstone marker", "Model": "embedded gorm.Model: ids and timestamps are internal",
			},
		},
		"circle": {
			model: reflect.TypeOf(models.Circle{}), view: reflect.TypeOf(ShareCircleView{}),
			exposed: map[string]string{"Name": "Name", "Description": "Description", "Color": "Color"},
			withheld: map[string]string{
				"ID": "internal identity", "CreatedAt": "internal", "UpdatedAt": "internal", "UserID": "owner identity",
			},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			viewFields := map[string]bool{}
			for _, n := range fieldNames(c.view) {
				viewFields[n] = true
			}
			for _, f := range fieldNames(c.model) {
				_, isExposed := c.exposed[f]
				_, isWithheld := c.withheld[f]
				assert.True(t, isExposed != isWithheld, "%s.%s must be classified exactly once (exposed or withheld with a reason)", name, f)
				if isExposed {
					assert.True(t, viewFields[c.exposed[f]], "%s.%s is classed exposed but the view has no %s", name, f, c.exposed[f])
				}
			}
			// Every view field is accounted for: carried from the model or a
			// derived field listed here.
			derived := map[string]bool{"MemberCount": true, "Members": true, "Truncated": true}
			carried := map[string]bool{}
			for _, v := range c.exposed {
				carried[v] = true
			}
			for f := range viewFields {
				assert.True(t, carried[f] || derived[f], "%s view field %s is not in the census", name, f)
			}
		})
	}
}

// TestShareGrantViews_RecipientViewsCarryNoEntityID: the recipient list view
// never exposes the entity id or the owner's configuration.
func TestShareGrantViews_RecipientViewsCarryNoEntityID(t *testing.T) {
	for _, f := range fieldNames(reflect.TypeOf(ShareGrantRecipientView{})) {
		assert.NotContains(t, strings.ToLower(f), "entityid")
		assert.NotContains(t, strings.ToLower(f), "reconfirm")
		assert.NotContains(t, strings.ToLower(f), "touser")
	}
}

// TestShareGrantReadsOnlyThroughService: no non-test source file outside the
// reviewed set reads or writes share_grants / models.ShareGrant. The service
// (share_grant.go) is the chokepoint; the others are the delete-cascade
// revoke helper and the reconfirm persistence hooks, plus callers of the
// revoke helper. A new reader fails here until reviewed.
func TestShareGrantReadsOnlyThroughService(t *testing.T) {
	allowed := map[string]string{
		"controllers/share_grant_controller.go": "thin handlers; only names the model type for DTO binding/service calls",
		"controllers/note_controller.go":        "calls services.RevokeShareGrantsForEntity on delete",
		"controllers/circle_controller.go":      "calls services.RevokeShareGrantsForEntity on delete",
		"controllers/activity_controller.go":    "calls services.RevokeShareGrantsForEntity on delete",
		"controllers/user_delete_cascade.go":    "hard-deletes a deleted user's grants and blocks",
		"services/image_sweep_service.go":       "calls RevokeShareGrantsForEntity when sweeping deleted notes",
		"services/share_grant.go":               "the resolver / chokepoint",
		"services/contact_delete.go":            "contact cascade registry step",
		"services/share_grant_revoke.go":        "the revoke-on-delete helper",
		"models/share_grant.go":                 "the model",
		"models/share_grant_reconfirm.go":       "reconfirm marker hooks",
		"models/audit.go":                       "audit entity token",
		"models/share_grant_dto.go":             "request DTOs (create/patch/decline bodies)",
		"errors/reasons.go":                     "reason code names (share_grant_*)",
		"routes/routes.go":                      "route registration",
	}
	root := ".."
	var offenders []string
	require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "testdata", "node_modules", ".git", "cmd", "internal":
				if p != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		src := string(b)
		if !strings.Contains(src, "share_grants") && !strings.Contains(src, "ShareGrant") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if _, ok := allowed[rel]; !ok {
			offenders = append(offenders, rel)
		}
		return nil
	}))
	assert.Empty(t, offenders, "a new file reads or writes share grants outside the reviewed chokepoint")

	// The share-grant controller must not query anything itself: every
	// lookup goes through services (which scope by the caller's role column).
	b, err := os.ReadFile(filepath.Join(root, "controllers/share_grant_controller.go"))
	require.NoError(t, err)
	// Cursor paging may add its own predicate; the role scope may not live here.
	assert.NotContains(t, string(b), "from_user_id", "role scoping belongs in services")
	assert.NotContains(t, string(b), "to_user_id", "role scoping belongs in services")
}
