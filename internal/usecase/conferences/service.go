// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package conferences

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// repository — репозиторий определяет заменяемый контракт зависимостей для управления жизненным циклом
// конференции и членством участников.
type repository interface {
	// Create создает и сохраняет новую конференцию для действующей учетной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Учетная запись, участник или работник, которому принадлежит ресурс.
	//
	// @returns:
	//   - result1: Результирующая конференция.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Create(context.Context, domain.Conference, domain.Participant) (domain.Conference, error)
	// Get загружает запрошенную конференцию в рамках полномочий вызывающей стороны.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Результирующая конференция.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Get(context.Context, string) (domain.Conference, error)
	// GetByInvite загружает конференцию, связанную с действительным кодом приглашения.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ограниченный публичный статус или код ошибки.
	//
	// @returns:
	//   - result1: Результирующая конференция.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetByInvite(context.Context, string) (domain.Conference, error)
	// ListForUser возвращает только те конференции, которые доступны для просмотра в рамках
	// членства указанной учетной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Максимальное количество результатов или операций в этом пакете.
	//   - arg4: Количество результатов, пропущенных до перехода на запрошенную страницу.
	//
	// @returns:
	//   - result1: Собранные данные о конференции.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ListForUser(context.Context, string, int, int) ([]domain.Conference, error)
	// Membership загружает сохраненное членство учетной записи в запрошенной конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: Полученный участник.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Membership(context.Context, string, string) (domain.Participant, error)
	// Participants возвращает участников конференции, доступных для просмотра действующей
	// учетной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Максимальное количество результатов или операций в этом пакете.
	//   - arg4: Количество результатов, пропущенных до перехода на запрошенную страницу.
	//
	// @returns:
	//   - result1: Собранные данные об участниках.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Participants(context.Context, string, int, int) ([]domain.Participant, error)
	// Transition применяет допустимый переход между этапами жизненного цикла конференции без
	// перезаписи более нового состояния.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg4: Целевое состояние, учетная запись или декодированное значение в
	//   соответствии с типом.
	//
	// @returns:
	//   - result1: Результирующая конференция.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Transition(context.Context, string, string, domain.Status) (domain.Conference, error)
	// Join создает или возобновляет авторизованное членство или подключение к медиакомнате,
	// сохраняя текущие идентификационные данные.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg4: Подтвержденный код приглашения на конференцию.
	//
	// @returns:
	//   - result1: Полученный участник.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Join(context.Context, string, users.User, string) (domain.Participant, error)
	// Leave отменяет активное членство или подключение к мультимедиа, сохраняя при этом
	// сохранённую историю разговоров или конференций.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Полученный участник.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Leave(context.Context, string, string) (domain.Participant, error)
}

// userRepository определяет заменяемый контракт зависимостей для жизненного цикла конференции и
// членства участников.
type userRepository interface {
	// GetByID загружает конференцию с помощью ID.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Полученный пользователь.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetByID(context.Context, string) (users.User, error)
}

// Service — сервис координирует жизненный цикл конференции и управление членством участников с помощью
// настроенных зависимостей для сохранения данных и передачи данных.
type Service struct {
	repository repository
	users      userRepository
	invite     func() (string, error)
	observer   interface {
		// ConferenceChanged уведомляет подключенные сеансы или наблюдателей об изменении
		// состояния конференции.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
		ConferenceChanged(context.Context, string)
	}
}

// NewService создает сервис с зависимостями и ограничениями, необходимыми для управления
// жизненным циклом конференции и членством участников.
//
// @Parameters:
//   - repository: Зависимость персистентности, используемая операцией приложения.
//   - users: Идентификаторы учетных записей или репозиторий учетных записей в соответствии с их
//     типом.
//   - invite: Функция, генерирующая криптографически случайный код приглашения.
//
// @returns:
//   - result1: Созданный или загруженный сервис.
func NewService(repository repository, users userRepository, invite func() (string, error)) *Service {
	return &Service{repository: repository, users: users, invite: invite}
}

// SetObserver обновляет данные наблюдателя для управления жизненным циклом конференции и
// членством участников.
//
// @Parameters:
//   - observer: Наблюдатель получает уведомление после изменения жизненного цикла.
func (s *Service) SetObserver(observer interface {
	// ConferenceChanged уведомляет подключенные сеансы или наблюдателей об изменении состояния
	// конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	ConferenceChanged(context.Context, string)
}) {
	s.observer = observer
}

// changed уведомляет наблюдателя компонента об изменениях в локальном или сохранённом
// состоянии.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
func (s *Service) changed(ctx context.Context, id string) {
	if s.observer != nil {
		s.observer.ConferenceChanged(ctx, id)
	}
}

// Create создает и сохраняет новую конференцию для действующей учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующее представление.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Create(ctx context.Context, userID string, request domain.CreateRequest) (domain.View, error) {
	if err := domain.ValidateSchedule(request.ScheduledAt, request.PlannedDurationMin, time.Now()); err != nil {
		return domain.View{}, err
	}
	title, err := domain.NormalizeTitle(request.Title)
	if err != nil {
		return domain.View{}, err
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.View{}, err
	}
	for range 5 {
		code, err := s.invite()
		if err != nil {
			return domain.View{}, err
		}
		if !validInvite(code) {
			return domain.View{}, fmt.Errorf("invalid generated invite code")
		}
		conference := domain.Conference{ID: uuid.NewString(), OwnerID: user.ID, Title: title, InviteCode: code, Status: domain.Created,
			WaitingRoomEnabled: request.WaitingRoomEnabled, ScheduledAt: request.ScheduledAt, PlannedDurationMin: request.PlannedDurationMin}
		if conference.ScheduledAt != nil {
			at := conference.ScheduledAt.UTC()
			conference.ScheduledAt = &at
			conference.Status = domain.Scheduled
		}
		ownerID := user.ID
		owner := domain.Participant{ID: uuid.NewString(), ConferenceID: conference.ID, UserID: &ownerID,
			DisplayName: user.ParticipantName(), Role: domain.Owner, Status: domain.Left, AdmissionState: domain.AdmissionAdmitted}
		created, err := s.repository.Create(ctx, conference, owner)
		if errors.Is(err, domain.ErrInviteCollision) {
			continue
		}
		if err != nil {
			return domain.View{}, err
		}
		return created.View(), nil
	}
	return domain.View{}, fmt.Errorf("unable to generate a unique invite code")
}

// List возвращает страницу с ограниченным количеством записей о конференциях с использованием
// заданных параметров разбивки на страницы и фильтров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - offset: Количество результатов, пропущенных до перехода на запрошенную страницу.
//
// @returns:
//   - result1: Собранные значения представлений.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]domain.View, error) {
	items, err := s.repository.ListForUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]domain.View, 0, len(items))
	for _, item := range items {
		view := item.View()
		view.InviteCode, view.InviteURL = "", ""
		views = append(views, view)
	}
	return views, nil
}

// Read считывает состояние конференции или поток данных, необходимые вызывающей стороне.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующее представление.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Read(ctx context.Context, userID, id string) (domain.View, error) {
	conference, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.View{}, err
	}
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return domain.View{}, err
	}
	view := conference.View()
	if !p.CanReadHistory() {
		view.InviteCode, view.InviteURL = "", ""
	}
	return view, nil
}

// Transition применяет допустимый переход между этапами жизненного цикла конференции без
// перезаписи более нового состояния.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - target: Целевое состояние, учетная запись или декодированное значение в соответствии с
//     типом.
//
// @returns:
//   - result1: Результирующее представление.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Transition(ctx context.Context, userID, id string, target domain.Status) (domain.View, error) {
	conference, err := s.repository.Transition(ctx, id, userID, target)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return conference.View(), nil
}

// Participants возвращает участников конференции, доступных для просмотра действующей учетной
// записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - offset: Количество результатов, пропущенных до перехода на запрошенную страницу.
//
// @returns:
//   - result1: Собранные значения представлений участников.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Participants(ctx context.Context, userID, id string, limit, offset int) ([]domain.ParticipantView, error) {
	if _, err := s.repository.Get(ctx, id); err != nil {
		return nil, err
	}
	actor, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if !actor.CanReadHistory() {
		return nil, apperrors.ErrForbidden
	}
	var items []domain.Participant
	if repo, ok := s.repository.(interface {
		// ParticipantsVisible возвращает только информацию об участниках, доступную для
		// действующего аккаунта.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
		//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
		//   доступа.
		//   - arg4: Максимальное количество результатов или операций в этом пакете.
		//   - arg5: Количество результатов, пропущенных до перехода на запрошенную
		//   страницу.
		//
		// @returns:
		//   - result1: Собранные данные об участниках.
		//   - result2: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		ParticipantsVisible(context.Context, string, string, int, int) ([]domain.Participant, error)
	}); ok {
		items, err = repo.ParticipantsVisible(ctx, id, userID, limit, offset)
	} else {
		items, err = s.repository.Participants(ctx, id, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	views := make([]domain.ParticipantView, 0, len(items))
	for _, item := range items {
		if !item.IsAdmitted() && actor.Role != domain.Owner && actor.Role != domain.CoHost {
			continue
		}
		views = append(views, item.View())
	}
	return views, nil
}

// Join создает или возобновляет авторизованное членство или подключение к медиакомнате,
// сохраняя текущие идентификационные данные.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученное представление участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Join(ctx context.Context, userID, id string, request domain.JoinRequest) (domain.ParticipantView, error) {
	if request.InviteCode != "" && !validInvite(request.InviteCode) {
		return domain.ParticipantView{}, apperrors.New(apperrors.ErrInvalidInput, "invalid inviteCode")
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	participant, err := s.repository.Join(ctx, id, user, request.InviteCode)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	if participant.Status == domain.Waiting {
		s.event(ctx, "participant.waiting", id, participant)
	}
	return participant.View(), nil
}

// Leave отменяет активное членство или подключение к мультимедиа, сохраняя при этом сохранённую
// историю разговоров или конференций.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученное представление участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Leave(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	participant, err := s.repository.Leave(ctx, id, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	return participant.View(), nil
}

// LookupInvite возвращает ограниченную информацию о конференции, связанную с действительным
// кодом приглашения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: Результирующее представление приглашения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) LookupInvite(ctx context.Context, code string) (domain.InviteView, error) {
	if !validInvite(code) {
		return domain.InviteView{}, apperrors.ErrNotFound
	}
	conference, err := s.repository.GetByInvite(ctx, code)
	if err != nil {
		return domain.InviteView{}, err
	}
	return domain.InviteView{ID: conference.ID, Title: conference.Title, Status: conference.Status, WaitingRoomEnabled: conference.WaitingRoomEnabled, ScheduledAt: conference.ScheduledAt}, nil
}

// JoinInvite присоединяется к аутентифицированной учетной записи по действительному
// приглашению, сохраняя при этом существующее членство в конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: Полученное представление участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) JoinInvite(ctx context.Context, userID, code string) (domain.ParticipantView, error) {
	invite, err := s.LookupInvite(ctx, code)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	return s.Join(ctx, userID, invite.ID, domain.JoinRequest{InviteCode: code})
}

// currentUser загружает действующую учетную запись и подготавливает общедоступную информацию об
// участниках.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученный пользователь.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) currentUser(ctx context.Context, id string) (users.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.User{}, apperrors.ErrUnauthorized
	}
	return user, err
}

// event создает доверенное событие, представляющее зафиксированное изменение членства в
// конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - participant: Членство в конференции или публичное состояние участника.
func (s *Service) event(ctx context.Context, kind, id string, participant domain.Participant) {
	if events, ok := s.observer.(interface {
		// Функция Broadcast публикует доверенное событие только для тех участников конференции
		// или беседы, которые в данный момент имеют к ней доступ.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Доверенное событие для публикации или обработки.
		//
		// @returns:
		//   - result1: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		Broadcast(context.Context, realtime.Envelope) error
	}); ok {
		_ = events.Broadcast(ctx, realtime.Event(kind, id, map[string]any{"participant": participant.View()}))
	}
}

// validInvite проверяет правильность кода приглашения перед обращением к хранилищу данных.
//
// @Parameters:
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func validInvite(code string) bool {
	if len(code) != 32 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(code)
	return err == nil && len(decoded) == 24
}
