package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestThinContactPetKind_RealMigratedSchema is the T37 (docs/fork-plan/
// tickets/46-T37-pet-relationship-kind-default.md) core round-trip against
// the real migrated schema: a thin contact created as the pet side of an
// owned_by/owns edge must persist with Contact.CRM.Kind == "animal", so the
// household-suggestion engine (household_service.go's classifyMember) treats
// it as a pet instead of a human adult.
//
// Uses database.InitDB rather than setupRouter's AutoMigrate (CLAUDE.md
// backend trap 1): the animal kind lives only in the crm JSON column, whose
// existence/derivation is part of the hand-written migration SQL, and the
// whole point of T37 is that BeforeSave must NOT re-derive and discard it —
// exactly the class of bug AutoMigrate-based tests cannot see.
func TestThinContactPetKind_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pet-kind-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "petkind", Password: "password123!A", Email: "petkind@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/relationship-edges", middleware.ValidateJSONMiddleware(&models.RelationshipEdgeInput{}), CreateRelationshipEdge)

	doCreate := func(t *testing.T, payload models.RelationshipEdgeInput) *httptest.ResponseRecorder {
		t.Helper()
		jsonValue, err := json.Marshal(payload)
		require.NoError(t, err)
		req, _ := http.NewRequest("POST", "/relationship-edges", bytes.NewBuffer(jsonValue))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Reload the persisted contact by vcard_uid and assert its kind — reads
	// the real crm JSON column through GORM, so an empty kind here means the
	// kind was either never set or silently dropped by BeforeSave.
	kindFor := func(t *testing.T, vcardUID string) string {
		t.Helper()
		var contact models.Contact
		require.NoError(t, db.Where("vcard_uid = ? AND user_id = ?", vcardUID, user.ID).First(&contact).Error)
		return contact.CRM.Kind
	}

	owner := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&owner).Error)

	// Case 1: thin SOURCE of an owned_by edge is the pet -> animal kind.
	// This is the frontend's "from a contact's page, add a pet by name"
	// flow: target_id = viewed contact, source_thin = the new pet.
	w := doCreate(t, models.RelationshipEdgeInput{
		TargetID:   owner.VCardUID,
		SourceThin: &models.ThinContactInput{Name: "Fluffy"},
		Type:       "owned_by",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var petSource struct {
		Edge models.RelationshipEdge `json:"relationship_edge"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &petSource))
	assert.Equal(t, "animal", kindFor(t, petSource.Edge.SourceID), "thin source of an owned_by edge must be an animal")

	// Case 2: thin TARGET of an owns edge is the pet -> animal kind (the
	// mirror-image flow: from a pet's page, linking it to a new owner picks
	// the "owns" token, making the pet the target).
	w = doCreate(t, models.RelationshipEdgeInput{
		SourceID:   owner.VCardUID,
		TargetThin: &models.ThinContactInput{Name: "Rex"},
		Type:       "owns",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var ownsEdge struct {
		Edge models.RelationshipEdge `json:"relationship_edge"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ownsEdge))
	assert.Equal(t, "animal", kindFor(t, ownsEdge.Edge.TargetID), "thin target of an owns edge must be an animal")

	// Case 3: the OWNER side must stay human — thin TARGET of an owned_by
	// edge (creating the owner inline while the pet exists as the source).
	w = doCreate(t, models.RelationshipEdgeInput{
		SourceID:   petSource.Edge.SourceID,
		TargetThin: &models.ThinContactInput{Name: "Bob"},
		Type:       "owned_by",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var ownerThin struct {
		Edge models.RelationshipEdge `json:"relationship_edge"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ownerThin))
	assert.Empty(t, kindFor(t, ownerThin.Edge.TargetID), "the owner side of an owned_by edge must NOT be an animal")

	// Case 4: thin SOURCE of an owns edge is the owner -> stays human.
	w = doCreate(t, models.RelationshipEdgeInput{
		TargetID:   ownsEdge.Edge.TargetID,
		SourceThin: &models.ThinContactInput{Name: "Carol"},
		Type:       "owns",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var ownerSource struct {
		Edge models.RelationshipEdge `json:"relationship_edge"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ownerSource))
	assert.Empty(t, kindFor(t, ownerSource.Edge.SourceID), "the owner side of an owns edge must NOT be an animal")

	// Case 5: a NON-pet relationship type must never get the animal kind —
	// a thin target created via parent_of stays a plain human contact.
	w = doCreate(t, models.RelationshipEdgeInput{
		SourceID:   owner.VCardUID,
		TargetThin: &models.ThinContactInput{Name: "Dana"},
		Type:       "parent_of",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var parentEdge struct {
		Edge models.RelationshipEdge `json:"relationship_edge"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parentEdge))
	assert.Empty(t, kindFor(t, parentEdge.Edge.TargetID), "a thin contact on a parent_of edge must NOT be an animal")
}
