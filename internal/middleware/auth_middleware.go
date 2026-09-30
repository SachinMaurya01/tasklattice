package middleware

import (
	"strings"

	"tasklattice/internal/config"
	apperrors "tasklattice/internal/errors"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

// AuthMiddleware verifies the Bearer access token and stores user_id
// (and user_email when present) in the gin context. Failures use the
// standard error envelope and never echo the token back.
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			apperrors.AbortWith(c, apperrors.Unauthorized("Authorization header required"))
			return
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			apperrors.AbortWith(c, apperrors.Unauthorized("Invalid authorization header format"))
			return
		}
		tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if tokenString == "" {
			apperrors.AbortWith(c, apperrors.Unauthorized("Invalid authorization header format"))
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, apperrors.Unauthorized("Unexpected signing method")
			}
			return []byte(cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			apperrors.AbortWith(c, apperrors.Unauthorized("Invalid or expired token"))
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			apperrors.AbortWith(c, apperrors.Unauthorized("Invalid token claims"))
			return
		}

		userID, ok := claims["user_id"].(string)
		if !ok || userID == "" {
			apperrors.AbortWith(c, apperrors.Unauthorized("Invalid token claims"))
			return
		}

		c.Set("user_id", userID)
		if email, ok := claims["email"].(string); ok {
			c.Set("user_email", email)
		}
		c.Next()
	}
}
