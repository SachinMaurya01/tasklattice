package handlers

import (
	"net/http"

	apperrors "Todo-App/internal/errors"
	"Todo-App/internal/services"

	"github.com/gin-gonic/gin"
)

// AuthHandler exposes the /api/v1/auth endpoints. It only binds input,
// delegates to the service, and renders output.
type AuthHandler struct {
	auth *services.AuthService
}

// NewAuthHandler wires the auth endpoints to the auth service.
func NewAuthHandler(auth *services.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

type registerRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Register handles POST /api/v1/auth/register.
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	user, err := h.auth.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, user)
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	user, pair, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user":          user,
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"token_type":    pair.TokenType,
		"expires_in":    pair.ExpiresIn,
	})
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	pair, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, pair)
}

// Logout handles POST /api/v1/auth/logout.
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperrors.Respond(c, apperrors.BadRequest("Invalid request body"))
		return
	}
	_ = h.auth.Logout(c.Request.Context(), req.RefreshToken)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
}

// LogoutAll handles POST /api/v1/auth/logout-all (authenticated).
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	rawID, exists := c.Get("user_id")
	if !exists {
		apperrors.Respond(c, apperrors.Unauthorized("Authentication required"))
		return
	}
	userID, ok := rawID.(string)
	if !ok || userID == "" {
		apperrors.Respond(c, apperrors.Unauthorized("Authentication required"))
		return
	}
	if err := h.auth.LogoutAll(c.Request.Context(), userID); err != nil {
		apperrors.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "All sessions revoked"})
}
