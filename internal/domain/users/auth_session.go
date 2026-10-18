package users

import "time"

// AuthSession сохраняет данные для входа в учетную запись до тех пор, пока пользователь явно не
// выйдет из системы в браузере.  Сохраняется только дайджест SHA-256 непрозрачного
// cookie-файла; секретный ключ не сохраняется.
type AuthSession struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	UserID    string    `gorm:"type:uuid;not null"`
	TokenHash string    `gorm:"-"`
	CreatedAt time.Time `gorm:"not null"`
	RevokedAt *time.Time
}
