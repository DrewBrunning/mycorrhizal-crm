package controllers

import (
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerGiftShoppingRoute(router *gin.Engine) {
	router.GET("/occasion-obligations/gift-shopping-list", GetGiftShoppingList)
}

func doGiftShoppingGET(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGetGiftShoppingListRejectsInvalidDays(t *testing.T) {
	_, router := setupRouter(t)
	registerGiftShoppingRoute(router)

	w := doGiftShoppingGET(router, "/occasion-obligations/gift-shopping-list?days=45")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetGiftShoppingListEmptyResultIsArrayNotNull mirrors
// TestGetUpcomingOccasionsEmptyResultIsArrayNotNull (CLAUDE.md frontend trap
// #8): an empty gift shopping list must serialize as [], not be absent.
func TestGetGiftShoppingListEmptyResultIsArrayNotNull(t *testing.T) {
	_, router := setupRouter(t)
	registerGiftShoppingRoute(router)

	w := doGiftShoppingGET(router, "/occasion-obligations/gift-shopping-list?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	listRaw, present := raw["gift_shopping_list"]
	require.True(t, present, "the gift_shopping_list key must be present even when empty")
	assert.Equal(t, "[]", string(listRaw), "an empty result must serialize as [], not be absent or null")
}

func TestGetGiftShoppingListReportsNeededWithNoMatchingGift(t *testing.T) {
	db, router := setupRouter(t)
	registerGiftShoppingRoute(router)

	var user models.User
	db.First(&user)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)
	contact := models.Contact{UserID: user.ID, Firstname: "Nia"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "gift", Label: "Birthday gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	w := doGiftShoppingGET(router, "/occasion-obligations/gift-shopping-list?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		GiftShoppingList []models.GiftShoppingItem `json:"gift_shopping_list"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.GiftShoppingList, 1)
	assert.Equal(t, "needed", resp.GiftShoppingList[0].Status)
	assert.Empty(t, resp.GiftShoppingList[0].LinkedGiftID)
}

func TestGetGiftShoppingListReportsMatchedGiftStatus(t *testing.T) {
	db, router := setupRouter(t)
	registerGiftShoppingRoute(router)

	var user models.User
	db.First(&user)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)
	contact := models.Contact{UserID: user.ID, Firstname: "Otis"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "gift", Label: "Birthday gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	giftDate := now.AddDate(0, 0, -10)
	gift := models.Gift{
		UserID: user.ID, EntityID: contact.VCardUID, Status: models.GiftStatusPurchased,
		Description: "Board game", Date: &giftDate,
	}
	require.NoError(t, db.Create(&gift).Error)

	w := doGiftShoppingGET(router, "/occasion-obligations/gift-shopping-list?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		GiftShoppingList []models.GiftShoppingItem `json:"gift_shopping_list"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.GiftShoppingList, 1)
	assert.Equal(t, models.GiftStatusPurchased, resp.GiftShoppingList[0].Status)
	assert.Equal(t, gift.ID, resp.GiftShoppingList[0].LinkedGiftID)
}

func TestGetGiftShoppingListIgnoresStaleGiftOutsideWindow(t *testing.T) {
	db, router := setupRouter(t)
	registerGiftShoppingRoute(router)

	var user models.User
	db.First(&user)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)
	contact := models.Contact{UserID: user.ID, Firstname: "Priya"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "gift", Label: "Birthday gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	// A gift from well over a year ago — last cycle's, not this one's.
	staleDate := now.AddDate(-1, -6, 0)
	stale := models.Gift{
		UserID: user.ID, EntityID: contact.VCardUID, Status: models.GiftStatusGiven,
		Description: "Old gift", Date: &staleDate,
	}
	require.NoError(t, db.Create(&stale).Error)

	w := doGiftShoppingGET(router, "/occasion-obligations/gift-shopping-list?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		GiftShoppingList []models.GiftShoppingItem `json:"gift_shopping_list"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.GiftShoppingList, 1)
	assert.Equal(t, "needed", resp.GiftShoppingList[0].Status, "a gift from well over a year ago must not count as this cycle's")
}

func TestGetGiftShoppingListExcludesNonGiftKind(t *testing.T) {
	db, router := setupRouter(t)
	registerGiftShoppingRoute(router)

	var user models.User
	db.First(&user)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)
	contact := models.Contact{UserID: user.ID, Firstname: "Card Only"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	w := doGiftShoppingGET(router, "/occasion-obligations/gift-shopping-list?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		GiftShoppingList []models.GiftShoppingItem `json:"gift_shopping_list"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.GiftShoppingList, "a card-kind obligation must not appear in the gift shopping list")
}

// TestGetGiftShoppingList_RealMigratedSchema is the real-DB check (CLAUDE.md
// backend trap #1) for issue #387, ticket #1226.
func TestGetGiftShoppingList_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "giftshopping-realdb", Password: "password123!A", Email: "giftshopping-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)
	contact := models.Contact{UserID: user.ID, Firstname: "Reva"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "gift", Label: "Birthday gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	registerGiftShoppingRoute(router)

	w := doGiftShoppingGET(router, "/occasion-obligations/gift-shopping-list?days=30")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		GiftShoppingList []models.GiftShoppingItem `json:"gift_shopping_list"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.GiftShoppingList, 1)
	assert.Equal(t, "Reva", resp.GiftShoppingList[0].ContactName)
}
