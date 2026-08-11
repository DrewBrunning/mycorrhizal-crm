package controllers

import (
	"errors"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// notificationConfigResponse is the Settings card's full view of a user's
// notification setup: the channel config (Gotify token only as a has/hasn't),
// the per-user toggles, and the server's VAPID public key so the browser can
// subscribe to push.
type notificationConfigResponse struct {
	NtfyURL        string `json:"ntfy_url"`
	NtfyTopic      string `json:"ntfy_topic"`
	GotifyURL      string `json:"gotify_url"`
	GotifyHasToken bool   `json:"gotify_has_token"`
	NotifyNtfy     bool   `json:"notify_ntfy"`
	NotifyGotify   bool   `json:"notify_gotify"`
	NotifyPush     bool   `json:"notify_push"`
	VAPIDPublicKey string `json:"vapid_public_key"`
}

func buildNotificationConfigResponse(nc *models.NotificationConfig, user models.User, vapidPublic string) notificationConfigResponse {
	resp := notificationConfigResponse{
		NotifyNtfy:     user.NotifyNtfy,
		NotifyGotify:   user.NotifyGotify,
		NotifyPush:     user.NotifyPush,
		VAPIDPublicKey: vapidPublic,
	}
	if nc != nil {
		resp.NtfyURL = nc.NtfyURL
		resp.NtfyTopic = nc.NtfyTopic
		resp.GotifyURL = nc.GotifyURL
		resp.GotifyHasToken = nc.HasGotifyToken()
	}
	return resp
}

// GetNotificationConfig returns the current user's notification channel setup
// (empty defaults when nothing is configured yet) plus the VAPID public key
// the browser needs to register a push subscription.
func GetNotificationConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	nc, err := services.GetNotificationConfigForUser(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve notification config").WithError(err))
		return
	}

	user, ok := loadNotificationUser(c, db, userID)
	if !ok {
		return
	}

	vapidPublic, _, err := services.GetVAPIDKeys(db)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to prepare push notifications").WithError(err))
		return
	}

	c.JSON(http.StatusOK, buildNotificationConfigResponse(nc, user, vapidPublic))
}

// SaveNotificationConfig upserts the user's channel config and per-user
// toggles from the Settings form.
func SaveNotificationConfig(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	input, appErr := middleware.GetValidated[models.NotificationConfigInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	cfg := currentConfig(c)
	nc, err := services.SaveNotificationConfig(db, cfg.JWTSecretKey, userID, *input)
	if err != nil {
		// A malformed URL is the user's own input, not a database failure —
		// reject it as a 400 with the specific reason immediately at save time,
		// rather than only on first use.
		if errors.Is(err, services.ErrInvalidNotificationURL) {
			apperrors.AbortWithError(c, apperrors.ErrValidation(err.Error()))
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to save notification config").WithError(err))
		return
	}

	user, ok := loadNotificationUser(c, db, userID)
	if !ok {
		return
	}

	vapidPublic, _, err := services.GetVAPIDKeys(db)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to prepare push notifications").WithError(err))
		return
	}

	c.JSON(http.StatusOK, buildNotificationConfigResponse(nc, user, vapidPublic))
}

// TestNotificationChannel sends a test notification through the requested
// channel using the user's saved config. A diagnosed failure (unconfigured,
// unreachable, private address blocked) is still a 200 with ok:false — the
// Settings card renders the reason — mirroring TestImmichConnection.
func TestNotificationChannel(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var input struct {
		Channel string `json:"channel" validate:"required,oneof=ntfy gotify push"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("request body", err.Error()))
		return
	}
	if verrs := middleware.ValidateStruct(&input); len(verrs) > 0 {
		appErr := apperrors.ErrValidation("Request validation failed")
		for _, ve := range verrs {
			appErr.WithDetails(ve.Field, ve.Message)
		}
		apperrors.AbortWithError(c, appErr)
		return
	}

	user, ok := loadNotificationUser(c, db, userID)
	if !ok {
		return
	}

	cfg := currentConfig(c)
	if err := services.TestNotificationChannel(db, cfg, user, models.NotificationChannel(input.Channel)); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListPushSubscriptions returns the current user's registered push devices.
func ListPushSubscriptions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	subs, err := services.ListPushSubscriptions(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to load push subscriptions").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscriptions": subs})
}

// CreatePushSubscription registers the browser's push subscription for the
// current user (called after the Push API subscribe succeeds).
func CreatePushSubscription(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	input, appErr := middleware.GetValidated[models.PushSubscriptionInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	sub, err := services.CreatePushSubscription(db, userID, *input)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to register push subscription").WithError(err))
		return
	}
	c.JSON(http.StatusCreated, sub)
}

// DeletePushSubscription removes one of the current user's push devices.
func DeletePushSubscription(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "must be a positive integer"))
		return
	}

	if err := services.DeletePushSubscription(db, userID, uint(id)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Push subscription"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete push subscription").WithError(err))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Push subscription deleted"})
}

// loadNotificationUser fetches the current user for the notification handlers.
func loadNotificationUser(c *gin.Context, db *gorm.DB, userID uint) (models.User, bool) {
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("User"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve user").WithError(err))
		}
		return models.User{}, false
	}
	return user, true
}

// ListDeviceRegistrations returns the current user's registered mobile push
// devices (M2). Platform-agnostic: each row carries its client (fcm, apns) so
// an iOS client later appears in the same list.
func ListDeviceRegistrations(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	devices, err := services.ListDeviceRegistrations(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to load device registrations").WithError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"devices": devices})
}

// CreateDeviceRegistration registers a mobile device push token for the
// current user (called by the native app after the platform push service hands
// out a token). Re-registering the same (user, client, token) updates the
// existing row instead of duplicating it.
func CreateDeviceRegistration(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	input, appErr := middleware.GetValidated[models.DeviceRegistrationInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	device, err := services.CreateDeviceRegistration(db, userID, *input)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to register device").WithError(err))
		return
	}
	c.JSON(http.StatusCreated, device)
}

// DeleteDeviceRegistration removes one of the current user's mobile push
// devices.
func DeleteDeviceRegistration(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, strconv.IntSize)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "must be a positive integer"))
		return
	}

	if err := services.DeleteDeviceRegistration(db, userID, uint(id)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Device registration"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to delete device registration").WithError(err))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Device registration deleted"})
}
