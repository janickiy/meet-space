// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package chat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	chatdomain "github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type ActionRepository interface {
	// ChatInfo возвращает информацию о конференц-чате для аутентифицированного участника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Полученная информация.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ChatInfo(context.Context, string, string) (chatdomain.Info, error)
	// ChatMembers возвращает страницу участников, ограниченную курсором, которые видны в
	// авторизованном конференц-чате.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Непрозрачная граница продолжения с предыдущей страницы.
	//   - arg5: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Собранные значения представления участника.
	//   - result2: Курсор для перехода к следующей странице авторизованных участников.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ChatMembers(context.Context, string, string, string, int) ([]chatdomain.MemberView, string, error)
	// UpdateChatInfo обновляет авторизованные метаданные конференц-чата и возвращает их
	// представление для конкретного участника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Полученная информация.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	UpdateChatInfo(context.Context, string, string, chatdomain.UpdateInfoRequest) (chatdomain.Info, error)
	// ChatPreferences считывает сохраненные настройки уведомлений конференц-чата действующего
	// участника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Результирующие настройки.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ChatPreferences(context.Context, string, string) (chatdomain.Preferences, error)
	// SetChatPreferences сохраняет только настройки уведомлений о конференциях и чатах
	// действующего участника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Включает или отключает запрошенную функцию или настройку.
	//
	// @returns:
	//   - result1: Результирующие настройки.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SetChatPreferences(context.Context, string, string, bool) (chatdomain.Preferences, error)
	// LeaveChat фиксирует выход аккаунта из чата конференции в соответствии с правилами
	// членства.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	LeaveChat(context.Context, string, string) error
	// SearchMessages возвращает ограниченную страницу с видимыми сообщениями, соответствующими
	// подтвержденному поисковому запросу.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции или диалога, выбранный в зависимости от режима
	//   репозитория.
	//   - arg4: Текст для поиска или валидированные фильтры запросов.
	//   - arg5: Непрозрачная граница продолжения с предыдущей страницы.
	//   - arg6: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Результирующая страница с сообщениями.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SearchMessages(context.Context, string, string, string, string, int) (chatdomain.MessagePage, error)
	// ChatMessageContext возвращает видимую историю сообщений вокруг запрошенного сообщения в
	// пределах авторизованной области.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции или диалога, выбранный в зависимости от режима
	//   репозитория.
	//   - arg4: Идентификатор сообщения в рамках авторизованного диалога.
	//
	// @returns:
	//   - result1: Полученная страница.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ChatMessageContext(context.Context, string, string, string) (chatdomain.Page, error)
	// ChatMaterials возвращает ограниченную страницу с видимыми вложениями и ссылками в
	// сообщениях.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Подтвержденное событие, задание, категория медиаданных или ресурсов.
	//   - arg5: Непрозрачная граница продолжения с предыдущей страницы.
	//   - arg6: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Результирующая страница с материалами.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ChatMaterials(context.Context, string, string, string, string, int) (chatdomain.MaterialPage, error)
	// ChatPins возвращает видимые закреплённые сообщения в чате конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Непрозрачная граница продолжения с предыдущей страницы.
	//   - arg5: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Результирующая страница с сообщениями.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ChatPins(context.Context, string, string, string, int) (chatdomain.MessagePage, error)
	// SetChatPin изменяет статус закрепления видимого сообщения в чате конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор сообщения в рамках авторизованного диалога.
	//   - arg5: Нужно ли закрепить сообщение или пометить его как важное.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SetChatPin(context.Context, string, string, string, bool) error
}

type actionObserver interface {
	// ConferenceChanged уведомляет подключенные сеансы или наблюдателей об изменении состояния
	// конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	ConferenceChanged(context.Context, string)
}

type AccountPresence interface {
	// Online возвращает текущее состояние присутствия для запрошенных идентификаторов учетных
	// записей.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификаторы учетных записей или репозиторий учетных записей в
	//   соответствии с их типом.
	//
	// @returns:
	//   - result1: Результирующая карта [строка]bool.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Online(context.Context, []string) (map[string]bool, error)
}

type ConferencePresence interface {
	// GetActiveSessions возвращает текущие физические сеансы, используемые для авторизации
	// медиакоманд.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Собранные значения сеанса.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetActiveSessions(context.Context, string) ([]realtime.Session, error)
}

type ActionService struct {
	repo     ActionRepository
	observer actionObserver
	presence AccountPresence
	guests   ConferencePresence
}

// NewActionService создает сервис действий с зависимостями и ограничениями, необходимыми для
// сохранения сообщений в чате и личных вложений.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - observer: Наблюдатель получает уведомление после изменения жизненного цикла.
//
// @returns:
//   - result1: Созданный или загруженный сервис действий.
func NewActionService(repo ActionRepository, observer actionObserver) *ActionService {
	return &ActionService{repo: repo, observer: observer}
}

// WithPresence привязывает присутствие к компоненту до того, как будут выполнены его операции.
//
// @Parameters:
//   - accounts: Зависимость от присутствия учетной записи или ее валидации.
//   - guests: Зависимость присутствия в конференц-сессиях, используемая для гостевых аккаунтов.
//
// @returns:
//   - result1: Созданный или загруженный сервис действий.
func (s *ActionService) WithPresence(accounts AccountPresence, guests ConferencePresence) *ActionService {
	s.presence, s.guests = accounts, guests
	return s
}

// Info возвращает информацию о чате конкретного участника, доступную для текущего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Полученная информация.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Info(ctx context.Context, user, conference string) (chatdomain.Info, error) {
	return s.repo.ChatInfo(ctx, user, conference)
}

// Members возвращает текущие идентификаторы участников или их представления для авторизованного
// диалога.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Собранные значения представления участника.
//   - result2: Курсор для перехода к следующей странице авторизованных участников.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Members(ctx context.Context, user, conference, cursor string, limit int) ([]chatdomain.MemberView, string, error) {
	if limit < 1 || limit > 100 {
		return nil, "", apperrors.ErrInvalidInput
	}
	// Авторизация и ограничения страниц предшествуют доступу к Redis: это не пользовательский
	// каталог.
	items, next, err := s.repo.ChatMembers(ctx, user, conference, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if len(items) == 0 {
		return items, next, nil
	}
	ids, hasGuests := []string{}, false
	for _, item := range items {
		if item.UserID == nil {
			continue
		}
		if item.IsGuest {
			hasGuests = true
		} else {
			ids = append(ids, *item.UserID)
		}
	}
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	statuses := map[string]bool{}
	accountAvailable := s.presence != nil
	if len(ids) > 0 && accountAvailable {
		statuses, err = s.presence.Online(lookup, ids)
		accountAvailable = err == nil
	}
	guestStatuses := map[string]bool{}
	guestAvailable := s.guests != nil
	if hasGuests && guestAvailable {
		sessions, err := s.guests.GetActiveSessions(lookup, conference)
		guestAvailable = err == nil
		if guestAvailable {
			for _, session := range sessions {
				if session.ConferenceID == conference {
					guestStatuses[session.UserID] = true
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	for i := range items {
		// Из-за сбоя в системе присутствия действительный участник PG не должен переходить в
		// статус false «не в сети».
		items[i].Online = nil
		if items[i].UserID == nil {
			continue
		}
		if items[i].IsGuest {
			if guestAvailable {
				online := guestStatuses[*items[i].UserID]
				items[i].Online = &online
			}
		} else if accountAvailable {
			online := statuses[*items[i].UserID]
			items[i].Online = &online
		}
	}
	return items, next, nil
}

// UpdateInfo обновляет валидированный заголовок или описание чата после проверки прав
// администратора.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученная информация.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) UpdateInfo(ctx context.Context, user, conference string, request chatdomain.UpdateInfoRequest) (chatdomain.Info, error) {
	if request.Title == nil && request.Description == nil {
		return chatdomain.Info{}, apperrors.ErrInvalidInput
	}
	if request.Title != nil {
		title, err := conferences.NormalizeTitle(*request.Title)
		if err != nil {
			return chatdomain.Info{}, err
		}
		request.Title = &title
	}
	if request.Description != nil {
		value := strings.TrimSpace(*request.Description)
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 1000 {
			return chatdomain.Info{}, apperrors.New(apperrors.ErrInvalidInput, "description must be at most 1000 characters")
		}
		request.Description = &value
	}
	info, err := s.repo.UpdateChatInfo(ctx, user, conference, request)
	if err == nil && s.observer != nil {
		s.observer.ConferenceChanged(ctx, conference)
	}
	return info, err
}

// Preferences считывает сохраненные настройки доставки или переписки действующего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Результирующие настройки.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Preferences(ctx context.Context, user, conference string) (chatdomain.Preferences, error) {
	return s.repo.ChatPreferences(ctx, user, conference)
}

// SetPreferences сохраняет валидированные настройки диалога действующего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - enabled: Включает или отключает запрошенную функцию или настройку.
//
// @returns:
//   - result1: Результирующие настройки.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) SetPreferences(ctx context.Context, user, conference string, enabled bool) (chatdomain.Preferences, error) {
	return s.repo.SetChatPreferences(ctx, user, conference, enabled)
}

// Leave отменяет активное членство или подключение к мультимедиа, сохраняя при этом сохранённую
// историю разговоров или конференций.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Leave(ctx context.Context, user, conference string) error {
	err := s.repo.LeaveChat(ctx, user, conference)
	if err == nil && s.observer != nil {
		s.observer.ConferenceChanged(ctx, conference)
	}
	return err
}

// Search — поиск выполняет поиск в постоянных сообщениях чата и личных вложениях с использованием
// ограниченных входных данных и фильтров видимости, заданных вызывающей стороной.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - query: Текст для поиска или валидированные фильтры запросов.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Результирующая страница с сообщениями.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Search(ctx context.Context, user, conference, query, cursor string, limit int) (chatdomain.MessagePage, error) {
	query = strings.TrimSpace(query)
	queryLength := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || queryLength < 2 || queryLength > 200 {
		return chatdomain.MessagePage{}, apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 200 characters")
	}
	if limit < 1 || limit > 50 {
		return chatdomain.MessagePage{}, apperrors.ErrInvalidInput
	}
	return s.repo.SearchMessages(ctx, user, conference, query, cursor, limit)
}

// Context возвращает видимые сообщения, окружающие выбранное сообщение, чтобы пользователь мог
// перейти к результату поиска.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - messageID: Идентификатор сообщения в рамках авторизованного диалога.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Context(ctx context.Context, user, conference, messageID string) (chatdomain.Page, error) {
	id, err := chatdomain.UUID(messageID)
	if err != nil {
		return chatdomain.Page{}, err
	}
	return s.repo.ChatMessageContext(ctx, user, conference, id)
}

// Materials возвращает страницу с ограниченным количеством видимых вложений и общих ссылок в
// переписке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Результирующая страница с материалами.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Materials(ctx context.Context, user, conference, kind, cursor string, limit int) (chatdomain.MaterialPage, error) {
	if kind != "image" && kind != "file" && kind != "link" {
		return chatdomain.MaterialPage{}, apperrors.New(apperrors.ErrInvalidInput, "kind must be image, file or link")
	}
	if limit < 1 || limit > 50 {
		return chatdomain.MaterialPage{}, apperrors.ErrInvalidInput
	}
	return s.repo.ChatMaterials(ctx, user, conference, kind, cursor, limit)
}

// Pins возвращает страницу с ограниченным количеством закреплённых сообщений в переписке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Результирующая страница с сообщениями.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) Pins(ctx context.Context, user, conference, cursor string, limit int) (chatdomain.MessagePage, error) {
	if limit < 1 || limit > 50 {
		return chatdomain.MessagePage{}, apperrors.ErrInvalidInput
	}
	return s.repo.ChatPins(ctx, user, conference, cursor, limit)
}

// SetPin обновляет закреплённые сообщения в постоянных сообщениях чата и личных вложениях.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - messageID: Идентификатор сообщения в рамках авторизованного диалога.
//   - important: Нужно ли закрепить сообщение или пометить его как важное.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ActionService) SetPin(ctx context.Context, user, conference, messageID string, important bool) error {
	id, err := chatdomain.UUID(messageID)
	if err != nil {
		return err
	}
	return s.repo.SetChatPin(ctx, user, conference, id, important)
}
