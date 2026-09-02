package controllers

// DB-03 (issue #494), action 4 — the two operations whose faithful run needs
// the unexported deleteContactAssociations: contact merge (#433) and the
// delete cascade. (Import / migration / restore are in
// services/data_integrity_operations_test.go.)
//
// The cascade half also covers action 5's spirit from the checker's side:
// TestDeleteCascadeCoverage (#611) proves the enumeration matches the schema;
// this proves the data-integrity checker *catches* a contact left behind when
// a cascade line is missing, so "a forgotten entry is silent" (CLAUDE.md trap
// #6) becomes a failing check.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func runIntegrityChecks(t *testing.T, db *gorm.DB) services.DataIntegrityReport {
	t.Helper()
	r, err := services.RunDataIntegrityChecks(context.Background(), db, config.Config{})
	require.NoError(t, err, "no probe should error")
	return r
}

// ---------------------------------------------------------------------------
// merge (#433) — a repoint that dropped an association, inverted an edge, or
// left the loser as a live target would show up as an INV-D1/D3/D7 violation
// ---------------------------------------------------------------------------

func TestDataIntegrity_HoldsAfterContactMerge(t *testing.T) {
	sc := setupRichPairScenario(t)

	resp := doGoldenMerge(t, sc.router, "/contacts/merge",
		models.ContactMergeRequest{KeepID: sc.ada.ID, MergeID: sc.bob.ID, Resolutions: sc.resolutions})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	r := runIntegrityChecks(t, sc.db)
	assert.True(t, r.OK, "merge must leave every invariant intact; got: %+v", r.Findings)
}

// ---------------------------------------------------------------------------
// delete cascade — invariants hold after a full cascade, and the checker
// names every association left stranded when the cascade is skipped
// ---------------------------------------------------------------------------

func newCascadeDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	db.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	return db
}

// seedContactWithAssociations creates one contact and one row in each of the
// association tables the data-integrity checker probes for a contact
// reference: a confirmed edge, a circle / household / tag membership, a field
// value, an external identity.
func seedContactWithAssociations(t *testing.T, db *gorm.DB, userID uint) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: "Cascade", Lastname: "Target"}
	require.NoError(t, db.Create(&contact).Error)
	uid := contact.VCardUID
	cc := seedContactContainers(t, db, userID, contact)

	other := models.Contact{UserID: userID, Firstname: "Other"}
	require.NoError(t, db.Create(&other).Error)

	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: userID, SourceID: other.VCardUID, TargetID: uid, Type: "friend_of",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1,
		Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: cc.circleID, UserID: userID, MemberVCardUID: uid}).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: cc.householdID, UserID: userID, MemberVCardUID: uid, Role: "adult"}).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: cc.tagID, UserID: userID, ContactVCardUID: uid}).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: cc.fieldDef.ID, UserID: userID, EntityID: uid, Value: json.RawMessage(`"v"`)}).Error)
	require.NoError(t, db.Create(&models.ExternalIdentity{UserID: userID, EntityID: uid, System: "immich", ExternalID: "p-1"}).Error)

	return contact
}

func TestDataIntegrity_HoldsAfterDeleteCascade(t *testing.T) {
	db := newCascadeDB(t)
	user := seedCascadeUser(t, db, "di-cascade-holds")
	contact := seedContactWithAssociations(t, db, user.ID)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return deleteContactAssociations(tx, contact, user.ID)
	}))
	require.NoError(t, db.Delete(&contact).Error)

	r := runIntegrityChecks(t, db)
	assert.True(t, r.OK, "a full delete cascade must leave no orphan or stale target; got: %+v", r.Findings)
}

func TestDataIntegrity_DetectsSkippedDeleteCascade(t *testing.T) {
	db := newCascadeDB(t)
	user := seedCascadeUser(t, db, "di-cascade-skipped")
	contact := seedContactWithAssociations(t, db, user.ID)

	// Soft-delete the contact the way DeleteContact does, but run NONE of the
	// cascade — the worst case of a forgotten enumeration entry. Every
	// association table must be named by the checker, asserted with Unscoped
	// (CLAUDE.md trap #6: a plain Count cannot tell "gone" from "tombstoned").
	require.NoError(t, db.Delete(&contact).Error)
	var check models.Contact
	require.NoError(t, db.Unscoped().First(&check, "id = ?", contact.ID).Error)
	require.NotNil(t, check.DeletedAt)

	r := runIntegrityChecks(t, db)
	assert.False(t, r.OK)

	want := []string{
		"relationship_edge.endpoint_soft_deleted",
		"circle_member.soft_deleted_contact",
		"household_member.soft_deleted_contact",
		"contact_tag.soft_deleted_contact",
		"external_identity.soft_deleted_contact",
	}
	got := map[string]bool{}
	for _, f := range r.Findings {
		got[f.Check] = true
	}
	for _, w := range want {
		assert.True(t, got[w], "checker must name the stranded %s; got findings: %+v", w, r.Findings)
	}

	// field_values has an ON DELETE CASCADE to contacts, but the parent is
	// only soft-deleted so nothing fired — the checker reports it too.
	assert.True(t, got["field_value.soft_deleted_contact"],
		"field_value referencing a soft-deleted contact must be reported; got: %+v", r.Findings)

}
