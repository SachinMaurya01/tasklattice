// Package services holds use-case/business logic. Handlers stay thin and
// repositories stay persistence-only.
package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"net/mail"
	"strings"
	"time"
	"unicode"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/models"
	"tasklattice/internal/repository"

	"github.com/golang-jwt/jwt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
	queryTimeout     = 5 * time.Second
)

// TokenPair is issued on login and on refresh.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// AuthService implements registration, login, and refresh-session
// management.
type AuthService struct {
	users      repository.UserRepository
	sessions   repository.SessionRepository
	jwtSecret  string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewAuthService wires the auth use cases.
func NewAuthService(users repository.UserRepository, sessions repository.SessionRepository, jwtSecret string, accessTTL, refreshTTL time.Duration) *AuthService {
	return &AuthService{
		users:      users,
		sessions:   sessions,
		jwtSecret:  jwtSecret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// Register validates input, normalizes the email, hashes the password, and
// creates the user. Duplicate emails map to 409; hashes are never returned
// (models.User hides them) and passwords are never logged.
func (s *AuthService) Register(ctx context.Context, email, password string) (*models.User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperrors.Internal()
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	user, err := s.users.CreateUser(ctx, &models.User{Email: email, Password: string(hash)})
	if err != nil {
		var pgErr *pgconn.PgError
		if stderrors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.EmailTaken()
		}
		return nil, apperrors.Internal()
	}
	return user, nil
}

// Login verifies credentials (without distinguishing unknown user from wrong
// password) and issues an access + refresh token pair.
func (s *AuthService) Login(ctx context.Context, email, password string) (*models.User, *TokenPair, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, nil, apperrors.InvalidCredentials()
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	user, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, nil, apperrors.InvalidCredentials()
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, nil, apperrors.InvalidCredentials()
	}

	pair, err := s.issuePair(ctx, user)
	if err != nil {
		return nil, nil, apperrors.Internal()
	}
	return user, pair, nil
}

// Refresh validates a refresh token, rotates it (old session revoked, new
// session inserted atomically), and issues a new pair. Presenting a revoked
// token signals possible theft: all of the user's sessions are revoked.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return nil, err
	}
	if typ, _ := claims["type"].(string); typ != tokenTypeRefresh {
		return nil, apperrors.TokenInvalid()
	}
	userID, _ := claims["user_id"].(string)
	if userID == "" {
		return nil, apperrors.TokenInvalid()
	}

	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	session, err := s.sessions.GetByHash(ctx, hashToken(refreshToken))
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.TokenInvalid()
		}
		return nil, apperrors.Internal()
	}
	if session.UserID != userID {
		return nil, apperrors.TokenInvalid()
	}
	if session.Revoked() {
		_ = s.sessions.RevokeAllForUser(ctx, userID)
		return nil, apperrors.TokenReused()
	}
	if session.Expired(time.Now()) {
		return nil, apperrors.TokenExpired()
	}

	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		if stderrors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.TokenInvalid()
		}
		return nil, apperrors.Internal()
	}

	access, err := s.signAccessToken(user)
	if err != nil {
		return nil, apperrors.Internal()
	}
	next, err := s.newRefreshToken(user.ID)
	if err != nil {
		return nil, apperrors.Internal()
	}
	rotated := &models.RefreshSession{
		UserID:    user.ID,
		TokenHash: hashToken(next),
		ExpiresAt: time.Now().Add(s.refreshTTL),
	}
	if err := s.sessions.Rotate(ctx, session.ID, rotated); err != nil {
		return nil, apperrors.Internal()
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: next,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.accessTTL.Seconds()),
	}, nil
}

// Logout revokes the session behind refreshToken. Unknown tokens still
// return success so logout stays idempotent.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()

	session, err := s.sessions.GetByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return nil
	}
	_ = s.sessions.Revoke(ctx, session.ID)
	return nil
}

// LogoutAll revokes every live session of a user.
func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	ctx, cancel := withQueryTimeout(ctx)
	defer cancel()
	if err := s.sessions.RevokeAllForUser(ctx, userID); err != nil {
		return apperrors.Internal()
	}
	return nil
}

func (s *AuthService) issuePair(ctx context.Context, user *models.User) (*TokenPair, error) {
	access, err := s.signAccessToken(user)
	if err != nil {
		return nil, err
	}
	refresh, err := s.newRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}
	session := &models.RefreshSession{
		UserID:    user.ID,
		TokenHash: hashToken(refresh),
		ExpiresAt: time.Now().Add(s.refreshTTL),
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.accessTTL.Seconds()),
	}, nil
}

func (s *AuthService) signAccessToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"type":    tokenTypeAccess,
		"exp":     time.Now().Add(s.accessTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.jwtSecret))
}

func (s *AuthService) newRefreshToken(userID string) (string, error) {
	jti, err := randomJTI()
	if err != nil {
		return "", err
	}
	claims := jwt.MapClaims{
		"user_id": userID,
		"jti":     jti,
		"type":    tokenTypeRefresh,
		"exp":     time.Now().Add(s.refreshTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.jwtSecret))
}

func (s *AuthService) parseToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(strings.TrimSpace(tokenString), func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, apperrors.TokenInvalid()
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		var verr *jwt.ValidationError
		if stderrors.As(err, &verr) && verr.Errors&jwt.ValidationErrorExpired != 0 {
			return nil, apperrors.TokenExpired()
		}
		return nil, apperrors.TokenInvalid()
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, apperrors.TokenInvalid()
	}
	return claims, nil
}

// hashToken stores only the SHA-256 of a refresh token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomJTI() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// NormalizeEmail trims, lowercases, and validates an email address.
func NormalizeEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" || len(trimmed) > 254 {
		return "", apperrors.BadRequest("A valid email address is required")
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", apperrors.BadRequest("A valid email address is required")
	}
	normalized := strings.ToLower(strings.TrimSpace(parsed.Address))
	if normalized == "" {
		return "", apperrors.BadRequest("A valid email address is required")
	}
	return normalized, nil
}

// ValidatePassword enforces the Phase 1B password policy.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return apperrors.BadRequest("Password must be at least 8 characters long")
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return apperrors.BadRequest("Password must contain at least one letter and one number")
	}
	return nil
}

func withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, queryTimeout)
}
