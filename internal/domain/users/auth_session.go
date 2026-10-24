package users

import "time"

// GuestSessionTTL limits an invitation-scoped guest session to three hours.
const GuestSessionTTL = 3 * time.Hour

// AuthSession retains an account sign-in until that browser explicitly logs out.
// Only a SHA-256 digest of the opaque cookie is stored; no bearer secret is persisted.
type AuthSession struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	UserID    string    `gorm:"type:uuid;not null"`
	TokenHash string    `gorm:"-"`
	CreatedAt time.Time `gorm:"not null"`
	RevokedAt *time.Time
}
