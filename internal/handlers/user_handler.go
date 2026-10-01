package handlers

// Registration and login now live in AuthHandler (auth_handler.go), backed
// by services.AuthService (service layer + session auth).
// TestProtectedHandler remains for manual middleware verification.

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TestProtectedHandler echoes the authenticated user id.
func TestProtectedHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")

		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user_id not found in context"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Protected route accessed successfully!",
			"user_id": userID,
		})
	}
}
