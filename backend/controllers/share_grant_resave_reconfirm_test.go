package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2578, ADR 0042 decision 5: only a normal -> private/secret move marks
// a section. Re-saving an item that is already private (after the owner
// re-confirmed) must not withhold its section again, for every writer: the
// prior sensitivity has to survive from BeforeSave to AfterSave.

type resaveCase struct {
	name    string
	section string
	// create persists a normal item for the granted contact.
	create func(e *sgEnv, c models.Contact) any
	// touch re-saves the item without changing its sensitivity.
	touch func(e *sgEnv, item any) error
	// setSens moves the item to the given sensitivity.
	setSens func(e *sgEnv, item any, s string) error
}

func resaveCases() []resaveCase {
	return []resaveCase{
		{
			name: "relationship edge", section: "related_to",
			create: func(e *sgEnv, c models.Contact) any {
				tgt := e.contact(e.A, "T")
				edge := &models.RelationshipEdge{UserID: e.A.ID, SourceID: c.VCardUID, TargetID: tgt.VCardUID, Type: "parent_of",
					Source: models.RelationshipSourceUserConfirmed, Confidence: 1, Status: models.RelationshipStatusConfirmed, Sensitivity: "normal"}
				require.NoError(e.t, e.db.Create(edge).Error)
				return edge
			},
			touch: func(e *sgEnv, item any) error {
				return e.db.Model(item).Update("confidence", 0.9).Error
			},
			setSens: func(e *sgEnv, item any, s string) error {
				return e.db.Model(item).Update("sensitivity", s).Error
			},
		},
		{
			name: "preference", section: "personal_info",
			create: func(e *sgEnv, c models.Contact) any {
				p := &models.Preference{UserID: e.A.ID, EntityID: c.VCardUID, Category: models.PreferenceCategoryHobby, Value: "chess", Sensitivity: "normal"}
				require.NoError(e.t, e.db.Create(p).Error)
				return p
			},
			touch: func(e *sgEnv, item any) error {
				return e.db.Model(item).Update("value", "go").Error
			},
			setSens: func(e *sgEnv, item any, s string) error {
				return e.db.Model(item).Update("sensitivity", s).Error
			},
		},
		{
			name: "field definition", section: "custom_fields",
			create: func(e *sgEnv, _ models.Contact) any {
				d := &models.FieldDefinition{UserID: e.A.ID, Label: "F", Key: "f", Target: models.FieldDefinitionTargetContact,
					Type: models.FieldTypeString, Projection: "vcard:X-F", Sensitivity: "normal"}
				require.NoError(e.t, e.db.Create(d).Error)
				return d
			},
			touch: func(e *sgEnv, item any) error {
				return e.db.Model(item).Update("label", "G").Error
			},
			setSens: func(e *sgEnv, item any, s string) error {
				return e.db.Model(item).Update("sensitivity", s).Error
			},
		},
	}
}

func TestReconfirm_ResavingAnAlreadyPrivateItemDoesNotRemark(t *testing.T) {
	for _, tc := range resaveCases() {
		t.Run(tc.name, func(t *testing.T) {
			e := newSGEnv(t)
			id, c := e.activeContactGrant([]string{"related_to", "personal_info", "custom_fields"})
			read := func() []string {
				var g models.ShareGrant
				require.NoError(t, e.db.First(&g, "id = ?", id).Error)
				return g.ReconfirmSections
			}
			item := tc.create(e, c)
			require.Empty(t, read(), "a normal item never sets a marker")

			// Raising normal -> private still marks.
			require.NoError(t, tc.setSens(e, item, "private"))
			require.Equal(t, []string{tc.section}, read())

			// Owner re-confirms; the item stays private.
			require.Equal(t, http.StatusOK, e.do(e.A, http.MethodPost, "/share-grants/"+id+"/reconfirm", nil).Code)
			require.Empty(t, read())

			// An unrelated re-save and a private -> secret move do not re-mark.
			require.NoError(t, tc.touch(e, item))
			assert.Empty(t, read(), "re-saving an already-private item must not re-mark")
			require.NoError(t, tc.setSens(e, item, "secret"))
			assert.Empty(t, read(), "private -> secret is not a normal -> non-normal move")

			// Back to normal and up again marks once more.
			require.NoError(t, tc.setSens(e, item, "normal"))
			require.Empty(t, read())
			require.NoError(t, tc.setSens(e, item, "private"))
			assert.Equal(t, []string{tc.section}, read())
		})
	}
}
