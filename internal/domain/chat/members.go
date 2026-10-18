package chat

import "github.com/janickiy/meet-space/internal/domain/conferences"

// MemberView отделяет членство в конференции от подтверждённого присутствия в реальном времени.
type MemberView struct {
	conferences.ParticipantView
	Online  *bool `json:"online" gorm:"-"`
	IsGuest bool  `json:"isGuest"`
}
