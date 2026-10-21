// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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
