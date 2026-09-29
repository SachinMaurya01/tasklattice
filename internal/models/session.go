package models

import "time"

// RefreshSession is one persisted refresh-token session. Only the SHA-256
// hash is stored; the raw token is never persisted.
type RefreshSession struct {
	ID        string     `json:"id" db:"id"`
	UserID    string     `json:"-" db:"user_id"`
	TokenHash string     `json:"-" db:"token_hash"`
	ExpiresAt time.Time  `json:"expires_at" db:"expires_at"`
	RevokedAt *time.Time `json:"-" db:"revoked_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// Revoked reports whether the session was revoked (logout or rotation).
func (s *RefreshSession) Revoked() bool { return s.RevokedAt != nil }

// Expired reports whether the session passed its expiry.
func (s *RefreshSession) Expired(now time.Time) bool { return !s.ExpiresAt.After(now) }
