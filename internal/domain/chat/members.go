// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package chat

import "github.com/janickiy/meet-space/internal/domain/conferences"

// MemberView отделяет членство в конференции от подтверждённого присутствия в реальном времени.
type MemberView struct {
	conferences.ParticipantView
	Online  *bool `json:"online" gorm:"-"`
	IsGuest bool  `json:"isGuest"`
}
