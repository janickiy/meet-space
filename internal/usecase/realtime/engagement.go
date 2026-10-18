package realtime

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

// Engagement проверяет, допущен ли участник, перед публикацией временных реакций в виде эмодзи.
type Engagement struct {
	repo Repository
	hub  *Hub
}

// NewEngagement формирует взаимодействие с учетом зависимостей и ограничений, необходимых для
// физических сеансов и доставки событий в режиме реального времени.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - hub: Центр управления в режиме реального времени, координирующий авторизованную доставку
//     данных через сокет.
//
// @returns:
//   - result1: Сформированное или загруженное взаимодействие.
func NewEngagement(repo Repository, hub *Hub) *Engagement {
	return &Engagement{repo: repo, hub: hub}
}

// Authorize проверяет текущие идентификационные данные и сохраненный статус доступа перед
// физическими сеансами и доставкой событий в режиме реального времени.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Engagement) Authorize(ctx context.Context, conferenceID, userID string) error {
	_, err := s.authorized(ctx, conferenceID, userID)
	return err
}

// authorized — авторизованный пользователь повторно проверяет сохраненное членство в активной конференции,
// чтобы отозванный доступ не использовался повторно.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Engagement) authorized(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	actor, err := s.repo.Authorize(ctx, conferenceID, userID)
	if err != nil {
		return actor, err
	}
	status, roster, err := s.repo.Roster(ctx, conferenceID)
	if err != nil {
		return actor, err
	}
	if status != conferences.Active {
		return actor, apperrors.New(apperrors.ErrConflict, "conference is not active")
	}
	for _, p := range roster {
		if p.ID == actor.ID && p.CanParticipate() {
			return p, nil
		}
	}
	return actor, apperrors.ErrForbidden
}

// Reaction — реакция публикует поддерживаемую временную реакцию от имени участника конференции, прошедшего
// проверку.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - emoji: Одна из временных реакций, поддерживаемых протоколом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Engagement) Reaction(ctx context.Context, conferenceID, userID, emoji string) error {
	if !domain.AllowedReaction(emoji) {
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported reaction")
	}
	actor, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return err
	}
	return s.hub.Broadcast(ctx, domain.Event("reaction.created", conferenceID, map[string]string{"participantId": actor.ID, "emoji": emoji}))
}
