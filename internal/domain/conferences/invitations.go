package conferences

import (
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// ValidateInvitationEmail отклоняет адреса, требующие SMTPUTF8, до того, как они попадут в
// очередь.
//
// @Parameters:
//   - email: Адрес электронной почты для входа в аккаунт или доставки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ValidateInvitationEmail(email string) error {
	if err := users.ValidateEmail(email); err != nil {
		return err
	}
	for _, character := range email {
		if character > 127 {
			return apperrors.New(apperrors.ErrInvalidInput, "Укажите email латинскими буквами. Международные адреса email пока не поддерживаются.")
		}
	}
	return nil
}

// InvitationRequest содержит адреса или зарегистрированные аккаунты, выбранные организатором.
type InvitationRequest struct {
	Emails  []string `json:"emails,omitempty"`
	UserIDs []string `json:"userIds,omitempty"`
}

// InvitationUser предоставляет только ту информацию об учетной записи, которая необходима для
// выбора приглашенного участника.
type InvitationUser struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"displayName,omitempty"`
}

// Invitation — приглашение сохраняет запрос на надежную доставку отдельно от фактического присутствия в
// комнате.
type Invitation struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	ConferenceID string
	RequestedBy  string
	Email        string
	UserID       *string
	Status       string
	ErrorCode    string
	CreatedAt    time.Time
	SentAt       *time.Time
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Invitation) TableName() string { return "conference_invitations" }

// InvitationResult сообщает о принятии приглашения, не утверждая, что SMTP доставил сообщение.
type InvitationResult struct {
	ID     string  `json:"id"`
	Email  string  `json:"email"`
	UserID *string `json:"userId"`
	Status string  `json:"status"`
}
