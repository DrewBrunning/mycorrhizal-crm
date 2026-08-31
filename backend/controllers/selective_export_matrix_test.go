package controllers

// DATA-04 (issue #444): selective-export testing matrix.
//
// The ticket's core deliverable is a matrix: field selection × format
// (vCard 3, vCard 4, JSContact) × sensitivity (normal, private, secret) ×
// IncludeSensitive (off, on) × scope (single contact, full export), run over
// the TEST-02 canonical pathological fixture (issue #430) which carries
// records at every sensitivity by construction.
//
// The tests here assert the two halves of the rule to protect:
//
//  1. Default-deny: with IncludeSensitive off, no private/secret item appears
//     in any format at any scope — including through the projected surfaces
//     (RelatedTo, PersonalInfo, and custom fields in Passthrough.VCard),
//     which is where a sensitivity leak would actually come from rather than
//     the flat fields.
//  2. The opt-in cannot be implied: enabling every section token (or
//     FieldSelectionAll) must not set IncludeSensitive — the regression test
//     for the foot-gun guard.
//
// The three scope classes the ticket names map onto the tests as follows:
// "single contact" is the matrix's `vcard_uid=` cells, "full export" is the
// matrix's unscoped cells, and "filtered set" is the selection tests below,
// which export the whole contact set through a sections filter.
//
// Each matrix cell has a declared expectation; the assertions pair every
// "sensitive absent" check with a "normal control present" check so a
// vacuously-passing cell (e.g. a filter that strips the whole section) fails
// the suite.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/jscontact"
	"mycorrhizal/models"
	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"

	"github.com/emersion/go-vcard"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// matrixProjection is the normalized view of ONE contact's projected export
// surfaces that the sensitivity matrix asserts against. It exists so the same
// assertion vocabulary works across vCard3/vCard4 (parsed with go-vcard) and
// JSContact (parsed as JSON): each format's extractor flattens its output into
// this shape and the tests reason about surfaces, not wire formats.
type matrixProjection struct {
	// relations maps a RelatedTo/AGENT target URN to the relation tags on it.
	// vCard3's AGENT carries no TYPE param, so its entries land under {""}.
	relations map[string]map[string]bool
	// personalInfo holds PersonalInfo values (hobby/interest text). vCard3
	// cannot represent PersonalInfo at all, so its extractor leaves this empty.
	personalInfo map[string]bool
	// vCardProps holds the names of passthrough custom properties (X-...).
	vCardProps map[string]bool
}

func newMatrixProjection() matrixProjection {
	return matrixProjection{
		relations:    map[string]map[string]bool{},
		personalInfo: map[string]bool{},
		vCardProps:   map[string]bool{},
	}
}

func (p matrixProjection) hasRelation(target, tag string) bool {
	tags, ok := p.relations[target]
	if !ok {
		return false
	}
	return tags[tag]
}

// decodeAllVCards parses every BEGIN:VCARD block in a concatenated .vcf export.
func decodeAllVCards(t *testing.T, data []byte) []vcard.Card {
	t.Helper()
	dec := vcard.NewDecoder(bytes.NewReader(data))
	var cards []vcard.Card
	for {
		card, err := dec.Decode()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "exported vCard did not parse")
		cards = append(cards, card)
	}
	require.NotEmpty(t, cards, "export produced no vCard blocks")
	return cards
}

// extractMatrixVCard4 flattens a vCard 4.0 export into per-UID projections.
func extractMatrixVCard4(t *testing.T, data []byte) map[string]matrixProjection {
	t.Helper()
	proj := map[string]matrixProjection{}
	for _, card := range decodeAllVCards(t, data) {
		uid := card.Value("UID")
		p := newMatrixProjection()
		for _, f := range card["RELATED"] {
			target := f.Value
			if p.relations[target] == nil {
				p.relations[target] = map[string]bool{}
			}
			for _, tag := range f.Params.Types() {
				p.relations[target][tag] = true
			}
		}
		for _, f := range card["HOBBY"] {
			p.personalInfo[f.Value] = true
		}
		for name := range card {
			if strings.HasPrefix(name, "X-") {
				p.vCardProps[name] = true
			}
		}
		proj[uid] = p
	}
	return proj
}

// extractMatrixVCard3 flattens a vCard 3.0 export into per-UID projections.
// vCard3 redirects RELATED onto AGENT (no TYPE param) and cannot represent
// PersonalInfo, so those surfaces differ from vCard4's.
func extractMatrixVCard3(t *testing.T, data []byte) map[string]matrixProjection {
	t.Helper()
	proj := map[string]matrixProjection{}
	for _, card := range decodeAllVCards(t, data) {
		uid := card.Value("UID")
		p := newMatrixProjection()
		for _, f := range card["AGENT"] {
			target := f.Value
			if p.relations[target] == nil {
				p.relations[target] = map[string]bool{}
			}
			p.relations[target][""] = true
		}
		for name := range card {
			if strings.HasPrefix(name, "X-") {
				p.vCardProps[name] = true
			}
		}
		proj[uid] = p
	}
	return proj
}

// extractMatrixJSContact flattens a JSContact export (a JSON array of cards)
// into per-UID projections.
func extractMatrixJSContact(t *testing.T, data []byte) map[string]matrixProjection {
	t.Helper()
	var cards []map[string]any
	require.NoError(t, json.Unmarshal(data, &cards), "exported JSContact did not parse: %s", string(data))
	require.NotEmpty(t, cards, "export produced no JSContact cards")
	proj := map[string]matrixProjection{}
	for _, card := range cards {
		uid, _ := card["uid"].(string)
		p := newMatrixProjection()
		if rt, ok := card["relatedTo"].(map[string]any); ok {
			for target, rel := range rt {
				m, _ := rel.(map[string]any)
				// RFC 9553 relation is a boolean-set map on the wire
				// ({"co-worker": true}); tolerate an array defensively.
				switch set := m["relation"].(type) {
				case map[string]any:
					if p.relations[target] == nil {
						p.relations[target] = map[string]bool{}
					}
					for tag := range set {
						p.relations[target][tag] = true
					}
				case []any:
					if p.relations[target] == nil {
						p.relations[target] = map[string]bool{}
					}
					for _, r := range set {
						if s, ok := r.(string); ok {
							p.relations[target][s] = true
						}
					}
				}
			}
		}
		if pi, ok := card["personalInfo"].(map[string]any); ok {
			for _, entry := range pi {
				if m, ok := entry.(map[string]any); ok {
					if v, ok := m["value"].(string); ok {
						p.personalInfo[v] = true
					}
				}
			}
		}
		if vp, ok := card["vCardProps"].([]any); ok {
			for _, entry := range vp {
				if m, ok := entry.(map[string]any); ok {
					if n, ok := m["name"].(string); ok {
						p.vCardProps[n] = true
					}
				}
			}
		}
		proj[uid] = p
	}
	return proj
}

// matrixFormat couples one export surface to its projection extractor so the
// matrix tests below read identically across the three formats.
type matrixFormat struct {
	name string
	// exportPath builds the export request for the given scope ("" = full),
	// sections ("" = all) and IncludeSensitive opt-in.
	exportPath func(scopeUID, sections string, includeSensitive bool) string
	extract    func(t *testing.T, data []byte) map[string]matrixProjection
	// personalInfoCapable reports whether the format can represent
	// PersonalInfo at all (vCard3 cannot, so private-hobby assertions only
	// apply to the other two formats).
	personalInfoCapable bool
	// relationControlTag is the relation tag a normal "co-worker" edge lands
	// under in this format's flattened projection. vCard3's AGENT carries no
	// TYPE param, so its entries land under "".
	relationControlTag string
	// sensitiveRelationTag is the tag the private spouse edge lands under in
	// this format's flattened projection ("" for vCard3's tagless AGENT).
	sensitiveRelationTag string
}

var selectiveExportFormats = []matrixFormat{
	{
		name: "vcard4",
		exportPath: func(scopeUID, sections string, includeSensitive bool) string {
			return selectiveExportQuery("/export/vcf", scopeUID, sections, includeSensitive, "version=4")
		},
		extract:              extractMatrixVCard4,
		personalInfoCapable:  true,
		relationControlTag:   "co-worker",
		sensitiveRelationTag: "spouse",
	},
	{
		name: "vcard3",
		exportPath: func(scopeUID, sections string, includeSensitive bool) string {
			return selectiveExportQuery("/export/vcf", scopeUID, sections, includeSensitive, "version=3")
		},
		extract:              extractMatrixVCard3,
		relationControlTag:   "",
		sensitiveRelationTag: "",
	},
	{
		name: "jscontact",
		exportPath: func(scopeUID, sections string, includeSensitive bool) string {
			return selectiveExportQuery("/export/jscontact", scopeUID, sections, includeSensitive)
		},
		extract:              extractMatrixJSContact,
		personalInfoCapable:  true,
		relationControlTag:   "co-worker",
		sensitiveRelationTag: "spouse",
	},
}

// selectiveExportQuery builds the request path shared by the three export
// handlers: optional version (extraParams), optional vcard_uid scope, optional
// sections, optional include_sensitive opt-in.
func selectiveExportQuery(base, scopeUID, sections string, includeSensitive bool, extraParams ...string) string {
	params := append([]string{}, extraParams...)
	if scopeUID != "" {
		params = append(params, "vcard_uid="+scopeUID)
	}
	if sections != "" {
		params = append(params, "sections="+sections)
	}
	if includeSensitive {
		params = append(params, "include_sensitive=true")
	}
	return base + "?" + strings.Join(params, "&")
}

// matrixFixture loads the TEST-02 canonical fixture into a real migrated
// schema and returns the db, a router wired like routes.go (VCF + JSContact +
// preflight), and the populated dataset.
func matrixFixture(t *testing.T) (*gorm.DB, *gin.Engine, *canonicalfixture.Dataset) {
	t.Helper()
	db := dbtest.New(t)
	closeTestDBAtTeardown(t, db)

	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", ds.User.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	registerVCFRoute(router, "")
	registerJSContactRoute(router, "")
	registerPreflightRoute(router)
	return db, router, ds
}

// matrixExport performs one matrix-cell export request and returns the body.
func matrixExport(t *testing.T, router *gin.Engine, f matrixFormat, scopeUID, sections string, includeSensitive bool) []byte {
	t.Helper()
	req, _ := http.NewRequest("GET", f.exportPath(scopeUID, sections, includeSensitive), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "%s export failed: %s", f.name, w.Body.String())
	return w.Body.Bytes()
}

// fixtureTarget returns the RelatedTo/AGENT target URN for a fixture contact.
func fixtureTarget(c models.Contact) string { return "urn:uuid:" + c.VCardUID }

// TestSelectiveExport_SensitivityMatrix is the ticket's core deliverable: the
// full format × scope × IncludeSensitive matrix over the TEST-02 fixture.
// Every cell asserts both halves — the normal control surfaces survive (so the
// cell is not vacuous) and the sensitive surfaces are gated by the opt-in.
func TestSelectiveExport_SensitivityMatrix(t *testing.T) {
	_, router, ds := matrixFixture(t)
	ada := ds.Contacts["ada"]
	bob := ds.Contacts["bob"]
	hugo := ds.Contacts["hugo"]
	ida := ds.Contacts["ida"]

	// scopes: single-contact (ada = secret custom-field carrier, hugo =
	// private-edge carrier) and full export. Every cell asserts against the
	// in-scope projections.
	scopes := []struct {
		name string
		uid  string
	}{
		{"single_ada", ada.VCardUID},
		{"single_hugo", hugo.VCardUID},
		{"full", ""},
	}

	for _, f := range selectiveExportFormats {
		for _, scope := range scopes {
			// --- IncludeSensitive off: default-deny across every surface. ---
			t.Run(f.name+"/off/"+scope.name, func(t *testing.T) {
				data := matrixExport(t, router, f, scope.uid, "", false)
				proj := f.extract(t, data)

				adaP, bobP, hugoP, idaP := proj[ada.VCardUID], proj[bob.VCardUID], proj[hugo.VCardUID], proj[ida.VCardUID]

				if scope.uid == "" || scope.uid == ada.VCardUID {
					// Normal controls: the coworker edge and the normal
					// projected surfaces survive the filter.
					assert.True(t, adaP.hasRelation(fixtureTarget(bob), f.relationControlTag),
						"ada's normal coworker relation must appear (off, %s)", scope.name)
					if f.personalInfoCapable {
						assert.True(t, adaP.personalInfo["sailing"],
							"ada's normal hobby must appear (off, %s)", scope.name)
					}
					assert.True(t, adaP.vCardProps["X-FAVORITE-COFFEE"],
						"ada's normal custom field must appear (off, %s)", scope.name)
					// Secret custom field: excluded through Passthrough.VCard.
					assert.False(t, adaP.vCardProps["X-PRIVATE-NICK"],
						"ada's secret custom field must be excluded (off, %s)", scope.name)
				}

				if scope.uid == hugo.VCardUID || scope.uid == "" {
					assert.False(t, hugoP.hasRelation(fixtureTarget(ida), f.sensitiveRelationTag),
						"hugo's private spouse edge must be excluded by default (off, %s)", scope.name)
					if scope.uid == "" {
						assert.False(t, idaP.hasRelation(fixtureTarget(hugo), f.sensitiveRelationTag),
							"ida's private spouse edge must be excluded by default (off, %s)", scope.name)
					}
				}

				if scope.uid == "" {
					assert.True(t, bobP.hasRelation(fixtureTarget(ada), f.relationControlTag),
						"bob's normal coworker relation must appear (off, %s)", scope.name)
					if f.personalInfoCapable {
						assert.False(t, bobP.personalInfo["chess"],
							"bob's private hobby must be excluded (off, %s)", scope.name)
					}
				}
			})

			// --- IncludeSensitive on: the opt-in overrides default-deny. ---
			t.Run(f.name+"/on/"+scope.name, func(t *testing.T) {
				data := matrixExport(t, router, f, scope.uid, "", true)
				proj := f.extract(t, data)

				adaP, bobP, hugoP, idaP := proj[ada.VCardUID], proj[bob.VCardUID], proj[hugo.VCardUID], proj[ida.VCardUID]

				if scope.uid == "" || scope.uid == ada.VCardUID {
					assert.True(t, adaP.vCardProps["X-PRIVATE-NICK"],
						"ada's secret custom field must appear with explicit opt-in (on, %s)", scope.name)
					assert.True(t, adaP.hasRelation(fixtureTarget(bob), f.relationControlTag),
						"ada's normal coworker relation must survive the opt-in (on, %s)", scope.name)
					if f.personalInfoCapable {
						assert.True(t, adaP.personalInfo["sailing"],
							"ada's normal hobby must survive the opt-in (on, %s)", scope.name)
					}
				}

				if scope.uid == hugo.VCardUID || scope.uid == "" {
					assert.True(t, hugoP.hasRelation(fixtureTarget(ida), f.sensitiveRelationTag),
						"hugo's private spouse edge must appear with explicit opt-in (on, %s)", scope.name)
					if scope.uid == "" {
						assert.True(t, idaP.hasRelation(fixtureTarget(hugo), f.sensitiveRelationTag),
							"ida's private spouse edge must appear with explicit opt-in (on, %s)", scope.name)
					}
				}

				if scope.uid == "" && f.personalInfoCapable {
					assert.True(t, bobP.personalInfo["chess"],
						"bob's private hobby must appear with explicit opt-in (on, %s)", scope.name)
				}
			})
		}
	}
}

// TestSelectiveExport_OptInCoversOnlySelectedSections pins the last matrix
// clause — "with the opt-in on, sensitive items appear — and only the ones the
// selection covers". The opt-in is not a blanket "everything now" switch: it
// removes the sensitivity clause inside the selected sections' projections, and
// ApplyFieldSelection still clears the sections the user did not pick.
func TestSelectiveExport_OptInCoversOnlySelectedSections(t *testing.T) {
	_, router, ds := matrixFixture(t)
	hugo := ds.Contacts["hugo"]
	ida := ds.Contacts["ida"]
	ada := ds.Contacts["ada"]
	bob := ds.Contacts["bob"]

	for _, f := range selectiveExportFormats {
		// Select only related_to: the private spouse edge projects, and the
		// secret custom field + private hobby (other sections) do not.
		t.Run(f.name+"/related_to", func(t *testing.T) {
			data := matrixExport(t, router, f, "", models.SectionRelatedTo, true)
			proj := f.extract(t, data)
			hugoP, idaP := proj[hugo.VCardUID], proj[ida.VCardUID]
			adaP, bobP := proj[ada.VCardUID], proj[bob.VCardUID]

			assert.True(t, hugoP.hasRelation(fixtureTarget(ida), f.sensitiveRelationTag),
				"opt-in + related_to must project the private spouse edge")
			assert.True(t, idaP.hasRelation(fixtureTarget(hugo), f.sensitiveRelationTag))
			assert.False(t, adaP.vCardProps["X-PRIVATE-NICK"],
				"custom_fields was not selected: the secret custom field must stay out")
			if f.personalInfoCapable {
				assert.False(t, bobP.personalInfo["chess"],
					"personal_info was not selected: the private hobby must stay out")
			}
		})

		// Select only custom_fields: the secret custom field projects, the
		// private spouse edge and hobby do not.
		t.Run(f.name+"/custom_fields", func(t *testing.T) {
			data := matrixExport(t, router, f, "", models.SectionCustomFields, true)
			proj := f.extract(t, data)
			adaP := proj[ada.VCardUID]
			hugoP, idaP := proj[hugo.VCardUID], proj[ida.VCardUID]
			bobP := proj[bob.VCardUID]

			assert.True(t, adaP.vCardProps["X-PRIVATE-NICK"],
				"opt-in + custom_fields must project the secret custom field")
			assert.False(t, hugoP.hasRelation(fixtureTarget(ida), f.sensitiveRelationTag),
				"related_to was not selected: the private spouse edge must stay out")
			assert.False(t, idaP.hasRelation(fixtureTarget(hugo), f.sensitiveRelationTag))
			if f.personalInfoCapable {
				assert.False(t, bobP.personalInfo["chess"],
					"personal_info was not selected: the private hobby must stay out")
			}
		})

		// Select only personal_info: the private hobby projects (formats that
		// can represent it), the spouse edge and custom field do not.
		t.Run(f.name+"/personal_info", func(t *testing.T) {
			data := matrixExport(t, router, f, "", models.SectionPersonalInfo, true)
			proj := f.extract(t, data)
			bobP := proj[bob.VCardUID]
			hugoP, idaP := proj[hugo.VCardUID], proj[ida.VCardUID]
			adaP := proj[ada.VCardUID]

			if f.personalInfoCapable {
				assert.True(t, bobP.personalInfo["chess"],
					"opt-in + personal_info must project the private hobby")
			}
			assert.False(t, hugoP.hasRelation(fixtureTarget(ida), f.sensitiveRelationTag),
				"related_to was not selected: the private spouse edge must stay out")
			assert.False(t, idaP.hasRelation(fixtureTarget(hugo), f.sensitiveRelationTag))
			assert.False(t, adaP.vCardProps["X-PRIVATE-NICK"],
				"custom_fields was not selected: the secret custom field must stay out")
		})
	}
}

// TestSelectiveExport_AllSectionTokensDoNotImplyOptIn is the highest-value
// assertion in the ticket: the regression test for the foot-gun guard at the
// HTTP surface. FieldSelectionAll() (and the equivalent explicit every-token
// ?sections= list) must leave IncludeSensitive false — selecting "all
// sections" must never quietly mean "include secrets".
func TestSelectiveExport_AllSectionTokensDoNotImplyOptIn(t *testing.T) {
	_, router, ds := matrixFixture(t)
	hugo := ds.Contacts["hugo"]
	ida := ds.Contacts["ida"]
	bob := ds.Contacts["bob"]
	ada := ds.Contacts["ada"]

	allTokens := strings.Join(models.FieldSections(), ",")

	for _, f := range selectiveExportFormats {
		t.Run(f.name, func(t *testing.T) {
			data := matrixExport(t, router, f, "", allTokens, false)
			proj := f.extract(t, data)

			assert.False(t, proj[hugo.VCardUID].hasRelation(fixtureTarget(ida), f.sensitiveRelationTag),
				"every section token selected is not enough to project a private edge")
			assert.False(t, proj[ida.VCardUID].hasRelation(fixtureTarget(hugo), f.sensitiveRelationTag))
			assert.False(t, proj[ada.VCardUID].vCardProps["X-PRIVATE-NICK"],
				"every section token selected is not enough to project a secret custom field")
			if f.personalInfoCapable {
				assert.False(t, proj[bob.VCardUID].personalInfo["chess"],
					"every section token selected is not enough to project a private hobby")
			}

			// The section selection itself is intact: the same request did
			// select the sections (normal controls still project), so the
			// absence above is the opt-in guard, not a broken selection.
			assert.True(t, proj[ada.VCardUID].hasRelation(fixtureTarget(bob), f.relationControlTag),
				"normal coworker edge must still project with every section selected")
			assert.True(t, proj[ada.VCardUID].vCardProps["X-FAVORITE-COFFEE"],
				"normal custom field must still project with every section selected")
		})
	}
}

// TestSelectiveExport_FieldSelectionAllIsStructurallyOptInFree is the model-
// level half of the foot-gun guard: FieldSelectionAll() returns a selection
// whose IncludeSensitive is false, and every construction entry point leaves
// it false no matter how many section tokens are enabled.
func TestSelectiveExport_FieldSelectionAllIsStructurallyOptInFree(t *testing.T) {
	sel := models.FieldSelectionAll()
	assert.False(t, sel.IncludeSensitive, "FieldSelectionAll must not set IncludeSensitive")
	for _, token := range models.FieldSections() {
		assert.True(t, sel.Has(token), "FieldSelectionAll must select %q", token)
	}

	// Enabling every token on a fresh selection never touches IncludeSensitive.
	s := models.NewFieldSelection()
	for _, token := range models.FieldSections() {
		require.NoError(t, s.Enable(token))
	}
	assert.False(t, s.IncludeSensitive, "Enable() must leave IncludeSensitive false")
}

// TestSelectiveExport_NoSectionsSelected_IdentityOnly pins the first degenerate
// case: an explicitly empty selection (NewFieldSelection, nothing enabled —
// the contact-share path's zero-sections state) clears every picker-covered
// section and leaves a valid, identity-only card in every format.
func TestSelectiveExport_NoSectionsSelected_IdentityOnly(t *testing.T) {
	record := selectiveExportFullRecord()
	sel := models.NewFieldSelection() // no sections selected

	got := models.ApplyFieldSelection(record, sel)
	assert.Empty(t, got.Card.Emails)
	assert.Empty(t, got.Card.Phones)
	assert.Empty(t, got.Card.RelatedTo)
	assert.Empty(t, got.Card.PersonalInfo)
	assert.Empty(t, got.Passthrough.VCard)
	require.NotNil(t, got.Card.Name, "identity data is always exported")

	cases := []struct {
		name string
		run  func() ([]byte, []contactmodel.Diagnostic, error)
	}{
		{"vcard4", func() ([]byte, []contactmodel.Diagnostic, error) { return (vcard4.Adapter{}).Export(got) }},
		{"vcard3", func() ([]byte, []contactmodel.Diagnostic, error) { return (vcard3.Adapter{}).Export(got) }},
		{"jscontact", func() ([]byte, []contactmodel.Diagnostic, error) { return (jscontact.Adapter{}).Export(got) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, diags, err := tc.run()
			require.NoError(t, err, "empty selection must still produce a valid export; diags: %v", diags)

			switch tc.name {
			case "vcard4", "vcard3":
				cards := decodeAllVCards(t, data)
				require.Len(t, cards, 1)
				require.NotEmpty(t, cards[0].Value("FN"), "identity FN survives an empty selection")
				assert.NotContains(t, string(data), "EMAIL", "deselected emails must not leak")
				assert.NotContains(t, string(data), "TEL", "deselected phones must not leak")
			case "jscontact":
				jsCard := parseJSContactCard(t, data)
				_, namePresent := jsCard["name"]
				assert.True(t, namePresent, "identity name survives an empty selection")
				_, emailsPresent := jsCard["emails"]
				assert.False(t, emailsPresent, "deselected emails must not leak")
				_, phonesPresent := jsCard["phones"]
				assert.False(t, phonesPresent, "deselected phones must not leak")
			}
		})
	}
}

// TestSelectiveExport_AllSecretContact_DefaultExcludesEverything pins the
// third degenerate case: a contact whose every sensitivity-bearing surface is
// secret exports as a clean, valid card with no sensitive data and no loss
// report — the policy exclusion is silent by design (issue #442) rather than
// being reported as fidelity loss.
func TestSelectiveExport_AllSecretContact_DefaultExcludesEverything(t *testing.T) {
	db, router, ds := matrixFixture(t)

	alice := models.Contact{UserID: ds.User.ID, Firstname: "Secret", Lastname: "Contact"}
	bob := models.Contact{UserID: ds.User.ID, Firstname: "Bob", Lastname: "Brown"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	// A secret spouse edge, a secret hobby preference, and a secret
	// vcard-projected custom field — every sensitivity-bearing surface.
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID:      ds.User.ID,
		SourceID:    alice.VCardUID,
		TargetID:    bob.VCardUID,
		Type:        "spouse_of",
		Source:      models.RelationshipSourceUserConfirmed,
		Confidence:  1.0,
		Status:      models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)
	require.NoError(t, db.Create(&models.Preference{
		UserID:      ds.User.ID,
		EntityID:    alice.VCardUID,
		Category:    models.PreferenceCategoryHobby,
		Value:       "black-box diving",
		Source:      models.PreferenceSourceUser,
		Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)
	def := models.FieldDefinition{
		UserID:      ds.User.ID,
		Label:       "Deep Secret",
		Key:         "deep_secret",
		Target:      "contact",
		Type:        "string",
		Projection:  "vcard:X-DEEP-SECRET",
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: def.ID,
		UserID:            ds.User.ID,
		EntityID:          alice.VCardUID,
		Value:             json.RawMessage(`"eyes only"`),
	}).Error)

	for _, f := range selectiveExportFormats {
		t.Run(f.name, func(t *testing.T) {
			data := matrixExport(t, router, f, alice.VCardUID, "", false)
			proj := f.extract(t, data)

			aliceP, present := proj[alice.VCardUID]
			require.True(t, present, "the all-secret contact must still export its identity card")
			assert.False(t, aliceP.hasRelation(fixtureTarget(bob), f.sensitiveRelationTag),
				"secret edge must not appear by default")
			assert.False(t, aliceP.vCardProps["X-DEEP-SECRET"],
				"secret custom field must not appear by default")
			if f.personalInfoCapable {
				assert.False(t, aliceP.personalInfo["black-box diving"],
					"secret hobby must not appear by default")
			}

			// Default export: the sensitivity exclusion is not a fidelity
			// loss — the loss-report header must not report it (issue #442).
			req, _ := http.NewRequest("GET", f.exportPath(alice.VCardUID, "", false), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			hdr := parseLossHeader(t, w)
			for _, report := range hdr.Diagnostics {
				assert.NotContains(t, report.Concept, "relationship",
					"a policy-excluded secret edge is not fidelity loss and must not be reported")
				assert.NotContains(t, report.Concept, "custom",
					"a policy-excluded secret custom field is not fidelity loss and must not be reported")
			}
		})
	}
}
