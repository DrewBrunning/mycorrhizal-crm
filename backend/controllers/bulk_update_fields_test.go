package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1996: bulk "update_fields" — allowlisted core fields (set map +
// clear list) and custom-field values across a selection.

func doBulkRaw(t *testing.T, router http.Handler, body string) (int, models.BulkOperationResult, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", "/contacts/bulk", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var result models.BulkOperationResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w.Code, result, w.Body.String()
}

func reloadBulkContact(t *testing.T, db *gorm.DB, id uint) models.Contact {
	t.Helper()
	var c models.Contact
	require.NoError(t, db.First(&c, id).Error)
	return c
}

func TestBulkUpdateFieldsSetsAndClearsAllowlistedFields(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice", JobTitle: "Old title", Department: "Keep me"}
	require.NoError(t, db.Create(&alice).Error) // BeforeSave derives the Card from the flat fields
	bob := createBulkContact(t, db, user.ID, "Bob")

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "update_fields",
		VCardUIDs: []string{alice.VCardUID, bob.VCardUID},
		Set: map[string]string{
			"organization": "Acme", "role": "Lead", "gender": "f",
			"how_we_met": "Conf", "work_information": "W", "contact_information": "C",
		},
		Clear: []string{"job_title"},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 2, result.Succeeded)
	assert.Equal(t, 0, result.Failed)

	for _, id := range []uint{alice.ID, bob.ID} {
		c := reloadBulkContact(t, db, id)
		assert.Equal(t, "Acme", c.Organization)
		assert.Equal(t, "Lead", c.Role)
		assert.Equal(t, "f", c.Gender)
		assert.Equal(t, "Conf", c.HowWeMet)
		assert.Equal(t, "W", c.WorkInformation)
		assert.Equal(t, "C", c.ContactInformation)
		assert.Empty(t, c.JobTitle, "clear list empties the field")
	}
	// Omitted field untouched.
	assert.Equal(t, "Keep me", reloadBulkContact(t, db, alice.ID).Department)
	// Names and other identity data survive the Card round trip.
	assert.Equal(t, "Alice", reloadBulkContact(t, db, alice.ID).Firstname)
}

func TestBulkUpdateFieldsClearGender(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice", Gender: "f"}
	require.NoError(t, db.Create(&alice).Error)

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID}, Clear: []string{"gender"},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)
	assert.Empty(t, reloadBulkContact(t, db, alice.ID).Gender)
}

func TestBulkUpdateFieldsRejectsInvalidRequestsWithoutWriting(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	uid := alice.VCardUID
	long := string(bytes.Repeat([]byte("x"), 201))

	cases := map[string]string{
		"unknown set key":      `{"action":"update_fields","vcard_uids":["` + uid + `"],"set":{"organization":"Acme","firstname":"Evil"}}`,
		"unknown clear key":    `{"action":"update_fields","vcard_uids":["` + uid + `"],"clear":["user_id"]}`,
		"city deferred":        `{"action":"update_fields","vcard_uids":["` + uid + `"],"set":{"city":"Paris"}}`,
		"set and clear same":   `{"action":"update_fields","vcard_uids":["` + uid + `"],"set":{"organization":"A"},"clear":["organization"]}`,
		"empty set value":      `{"action":"update_fields","vcard_uids":["` + uid + `"],"set":{"organization":""}}`,
		"too long":             `{"action":"update_fields","vcard_uids":["` + uid + `"],"set":{"organization":"` + long + `"}}`,
		"nothing to do":        `{"action":"update_fields","vcard_uids":["` + uid + `"]}`,
		"custom set + clear":   `{"action":"update_fields","vcard_uids":["` + uid + `"],"custom_set":[{"field_definition_id":"00000000-0000-4000-8000-000000000001","value":"x"}],"custom_clear":["00000000-0000-4000-8000-000000000001"]}`,
		"unknown custom id":    `{"action":"update_fields","vcard_uids":["` + uid + `"],"set":{"organization":"A"},"custom_set":[{"field_definition_id":"00000000-0000-4000-8000-000000000001","value":"x"}]}`,
		"unknown custom clear": `{"action":"update_fields","vcard_uids":["` + uid + `"],"set":{"organization":"A"},"custom_clear":["00000000-0000-4000-8000-000000000001"]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			code, _, raw := doBulkRaw(t, router, body)
			assert.Equal(t, http.StatusBadRequest, code, raw)
			assert.Empty(t, reloadBulkContact(t, db, alice.ID).Organization, "no write on a rejected request")
		})
	}
}

func TestBulkUpdateFieldsCustomFields(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	def := models.FieldDefinition{UserID: user.ID, Label: "Pronouns", Key: "pronouns", Target: "contact", Type: "string"}
	require.NoError(t, db.Create(&def).Error)
	gone := models.FieldDefinition{UserID: user.ID, Label: "Gone", Key: "gone", Target: "contact", Type: "string"}
	require.NoError(t, db.Create(&gone).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: gone.ID, UserID: user.ID, EntityID: alice.VCardUID, Value: json.RawMessage(`"bye"`)}).Error)

	// Set (insert), then set again (update), then clear.
	for _, v := range []string{`"she/her"`, `"they/them"`} {
		code, result := doBulk(t, router, models.BulkContactOperationInput{
			Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
			CustomSet:   []models.FieldValueInput{{FieldDefinitionID: def.ID, Value: json.RawMessage(v)}},
			CustomClear: []string{gone.ID},
		})
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, 1, result.Succeeded)
		var vals []models.FieldValue
		require.NoError(t, db.Where("entity_id = ?", alice.VCardUID).Find(&vals).Error)
		require.Len(t, vals, 1, "set definition present, cleared one gone")
		assert.Equal(t, def.ID, vals[0].FieldDefinitionID)
		assert.JSONEq(t, v, string(vals[0].Value))
	}
}

func TestBulkUpdateFieldsRejectsForeignAndInvalidCustomFields(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	other := createBulkForeignUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	foreignDef := models.FieldDefinition{UserID: other.ID, Label: "F", Key: "f", Target: "contact", Type: "string"}
	require.NoError(t, db.Create(&foreignDef).Error)
	numDef := models.FieldDefinition{UserID: user.ID, Label: "N", Key: "n", Target: "contact", Type: "number"}
	require.NoError(t, db.Create(&numDef).Error)

	code, _ := doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
		Set:       map[string]string{"organization": "Acme"},
		CustomSet: []models.FieldValueInput{{FieldDefinitionID: foreignDef.ID, Value: json.RawMessage(`"x"`)}},
	})
	assert.Equal(t, http.StatusBadRequest, code, "foreign definition id")

	code, _ = doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
		Set:       map[string]string{"organization": "Acme"},
		CustomSet: []models.FieldValueInput{{FieldDefinitionID: numDef.ID, Value: json.RawMessage(`"not a number"`)}},
	})
	assert.Equal(t, http.StatusBadRequest, code, "type mismatch")

	code, _ = doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
		CustomClear: []string{foreignDef.ID},
	})
	assert.Equal(t, http.StatusBadRequest, code, "foreign definition id in clear")

	assert.Empty(t, reloadBulkContact(t, db, alice.ID).Organization)
	var n int64
	db.Model(&models.FieldValue{}).Count(&n)
	assert.EqualValues(t, 0, n)
}

func TestBulkUpdateFieldsCrossUserAndPartialSuccess(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	other := createBulkForeignUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	foreign := createBulkContact(t, db, other.ID, "Mallory")
	missing := "00000000-0000-4000-8000-0000000000aa"

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "update_fields",
		VCardUIDs: []string{alice.VCardUID, foreign.VCardUID, missing},
		Set:       map[string]string{"organization": "Acme"},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, 2, result.Failed)
	failed := map[string]bool{}
	for _, f := range result.Failures {
		failed[f.VCardUID] = true
	}
	assert.True(t, failed[foreign.VCardUID], "foreign contact is a reported failure")
	assert.True(t, failed[missing])

	assert.Equal(t, "Acme", reloadBulkContact(t, db, alice.ID).Organization)
	assert.Empty(t, reloadBulkContact(t, db, foreign.ID).Organization, "foreign contact untouched")
}

func TestBulkUpdateFieldsNoPartialWriteWithinContact(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	def := models.FieldDefinition{UserID: user.ID, Label: "P", Key: "p", Target: "contact", Type: "string"}
	require.NoError(t, db.Create(&def).Error)
	// The custom-value write fails after the scalar write would have landed.
	require.NoError(t, db.Exec(`CREATE TRIGGER bulk_fv_fail BEFORE INSERT ON field_values BEGIN SELECT RAISE(ABORT, 'forced failure'); END`).Error)

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
		Set:       map[string]string{"organization": "Acme"},
		CustomSet: []models.FieldValueInput{{FieldDefinitionID: def.ID, Value: json.RawMessage(`"x"`)}},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Failed)
	assert.Equal(t, 0, result.Succeeded)
	assert.Empty(t, reloadBulkContact(t, db, alice.ID).Organization, "scalar write rolled back with the failed custom write")
}

func TestBulkUpdateFieldsOrganizationAndTitleEdits(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	full := models.Contact{UserID: user.ID, Firstname: "Full", Organization: "Acme", Department: "Sales", JobTitle: "Eng", Role: "Lead"}
	require.NoError(t, db.Create(&full).Error)
	bare := createBulkContact(t, db, user.ID, "Bare")

	// Overwrite existing department/title; add a role where none existed.
	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{full.VCardUID, bare.VCardUID},
		Set: map[string]string{"department": "Ops", "job_title": "Staff"},
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, result.Succeeded)
	got := reloadBulkContact(t, db, full.ID)
	assert.Equal(t, "Ops", got.Department)
	assert.Equal(t, "Staff", got.JobTitle)
	assert.Equal(t, "Acme", got.Organization, "untouched")
	assert.Equal(t, "Lead", got.Role, "untouched")
	bareGot := reloadBulkContact(t, db, bare.ID)
	assert.Equal(t, "Ops", bareGot.Department, "department creates the organization entry")
	assert.Equal(t, "Staff", bareGot.JobTitle)

	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{bare.VCardUID},
		Set: map[string]string{"role": "Mentor"},
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 1, result.Succeeded)
	assert.Equal(t, "Mentor", reloadBulkContact(t, db, bare.ID).Role, "role appended where none existed")

	// Clear every organizational field, and the free-text envelope fields.
	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{full.VCardUID},
		Clear: []string{"organization", "department", "job_title", "role", "how_we_met", "work_information", "contact_information"},
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 1, result.Succeeded)
	got = reloadBulkContact(t, db, full.ID)
	assert.Empty(t, got.Organization)
	assert.Empty(t, got.Department)
	assert.Empty(t, got.JobTitle)
	assert.Empty(t, got.Role)
	assert.Empty(t, got.Card.Organizations, "emptied organization entry is dropped from the Card")
	assert.Empty(t, got.Card.Titles)
	assert.Equal(t, "Full", got.Firstname)
}

func TestBulkUpdateFieldsRejectsOverlappingCustomFields(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	def := models.FieldDefinition{UserID: user.ID, Label: "P", Key: "p", Target: "contact", Type: "string"}
	require.NoError(t, db.Create(&def).Error)
	val := models.FieldValueInput{FieldDefinitionID: def.ID, Value: json.RawMessage(`"x"`)}

	code, _ := doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
		CustomSet: []models.FieldValueInput{val, val},
	})
	assert.Equal(t, http.StatusBadRequest, code, "duplicate definition in custom_set")

	code, _ = doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
		CustomSet: []models.FieldValueInput{val}, CustomClear: []string{def.ID},
	})
	assert.Equal(t, http.StatusBadRequest, code, "same definition in custom_set and custom_clear")
}

func TestBulkUpdateFieldsPerContactDatabaseFailuresAreReported(t *testing.T) {
	for name, trigger := range map[string]string{
		"contact save":  `CREATE TRIGGER bulk_fail BEFORE UPDATE ON contacts BEGIN SELECT RAISE(ABORT, 'forced failure'); END`,
		"custom clear":  `CREATE TRIGGER bulk_fail BEFORE DELETE ON field_values BEGIN SELECT RAISE(ABORT, 'forced failure'); END`,
		"custom update": `CREATE TRIGGER bulk_fail BEFORE UPDATE ON field_values BEGIN SELECT RAISE(ABORT, 'forced failure'); END`,
	} {
		t.Run(name, func(t *testing.T) {
			db, router := bulkTestRouter(t)
			user := createBulkTestUser(t, db)
			alice := createBulkContact(t, db, user.ID, "Alice")
			def := models.FieldDefinition{UserID: user.ID, Label: "P", Key: "p", Target: "contact", Type: "string"}
			require.NoError(t, db.Create(&def).Error)
			require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: def.ID, UserID: user.ID, EntityID: alice.VCardUID, Value: json.RawMessage(`"old"`)}).Error)
			require.NoError(t, db.Exec(trigger).Error)
			in := models.BulkContactOperationInput{
				Action: "update_fields", VCardUIDs: []string{alice.VCardUID},
				Set: map[string]string{"organization": "Acme"},
			}
			switch name {
			case "custom clear":
				in.CustomClear = []string{def.ID}
			default:
				in.CustomSet = []models.FieldValueInput{{FieldDefinitionID: def.ID, Value: json.RawMessage(`"new"`)}}
			}
			code, result := doBulk(t, router, in)
			require.Equal(t, http.StatusOK, code)
			assert.Equal(t, 1, result.Failed, "per-contact failure is reported, not a 500")
			assert.Equal(t, 0, result.Succeeded)
		})
	}
}

// ADR 0038: a user-facing bulk validation error carries a reason code and its
// params so clients can localize it; message keeps the rendered English.
func TestBulkUpdateFieldsRejectionCarriesReasonCode(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	bob := createBulkContact(t, db, user.ID, "Bob")

	code, _, raw := doBulkRaw(t, router, `{"action":"update_fields","vcard_uids":["`+bob.VCardUID+`"],"set":{"notes":"x"}}`)
	require.Equal(t, http.StatusBadRequest, code, raw)

	var env struct {
		Error struct {
			Message      string         `json:"message"`
			ReasonCode   string         `json:"reason_code"`
			ReasonParams map[string]any `json:"reason_params"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &env), raw)
	assert.Equal(t, "contact_bulk_field_not_editable", env.Error.ReasonCode)
	assert.Equal(t, map[string]any{"field": "notes"}, env.Error.ReasonParams)
	assert.Equal(t, "notes is not bulk-editable", env.Error.Message)
}
