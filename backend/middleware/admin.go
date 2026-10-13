package middleware

import (
	"errors"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminMiddleware checks if the authenticated user has admin privileges.
// Must be used AFTER AuthMiddleware which sets "userID" in context.
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDValue, exists := c.Get("userID")
		if !exists {
			apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Authentication required"))
			return
		}

		userID, ok := userIDValue.(uint)
		if !ok {
			apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Invalid user ID"))
			return
		}

		if isAPIToken, _ := c.Get("isAPIToken"); isAPIToken == true {
			apperrors.AbortWithError(c, apperrors.ErrForbidden("API tokens cannot access admin endpoints"))
			return
		}

		db := c.MustGet("db").(*gorm.DB)

		var user models.User
		if err := db.Select("is_admin").First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				apperrors.AbortWithError(c, apperrors.ErrUnauthorized("User not found"))
			} else {
				// A lookup failure is not "no such user": surface it as the
				// server fault it is rather than masking an outage as a 401.
				apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to verify admin access").WithError(err))
			}
			return
		}

		if !user.IsAdmin {
			apperrors.AbortWithError(c, apperrors.ErrForbidden("Admin access required"))
			return
		}

		c.Next()
	}
}
