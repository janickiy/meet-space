// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package personalapp

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
)

type Repository interface {
	GroupRepository
	// Account проверяет, является ли действующая учетная запись зарегистрированной, а не гостевой.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - arg2: идентификатор действующей учетной записи, используемый для ограничения доступа.
	//
	// @returns:
	//   - result1: nil в случае успешного выполнения; в противном случае — ошибка проверки, доступа, передачи или сохранения.
	//
	Account(context.Context, string) error
	// GetOrCreate возвращает существующий прямой диалог или создает его уникальную пару учетных записей
	// диалог.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: действующий идентификатор учетной записи, используемый для ограничения доступа.
	//   - arg3: целевое состояние, учетная запись или декодированное значение в зависимости от его типа.
	//
	// @returns:
	//   - result1: результирующий диалог.
	//   - result2: указывает, был ли создан новый диалог, а не повторно использован идемпотентный результат.
	//   - result3: nil в случае успеха; в противном случае — ошибка валидации, доступа, транспорта или сохранения
	//     данных.
	GetOrCreate(context.Context, string, string) (personal.Conversation, bool, error)
	// Get загружает запрошенный диалог в пределах разрешенной области действия вызывающего абонента.
	//
	// @Parameters:
	// - arg1: Контекст, управляющий отменой, крайними сроками и сроком действия операции.
	// - arg2: Действующий идентификатор учетной записи, используемый для доступа к области действия.
	// - arg3: Идентификатор ресурса, к которому обращается эта операция.
	//
	// @returns:
	// - result1: Итоговый диалог.
	// - result2: нулевой результат при успешном выполнении; в противном случае проверка, доступ, транспортировка или сохранение
	// сбой.
	Get(context.Context, string, string) (personal.Conversation, error)
	// List возвращает ограниченную страницу записей о беседах с использованием заданных параметров разбивки на страницы и фильтров.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: действующий идентификатор аккаунта, используемый для ограничения доступа.
	//   - arg3: непрозрачная граница продолжения предыдущей страницы.
	//   - arg4: максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: результирующая страница.
	//   - result2: nil в случае успеха; в противном случае — ошибка валидации, доступа, транспорта или сохранения
	//     данных.
	List(context.Context, string, string, int) (personal.Page, error)
	// ListFiltered возвращает страницу беседы с привязкой к члену группы, используя проверенные фильтры личных сообщений, групповых сообщений или непрочитанных сообщений.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: действующий идентификатор аккаунта, используемый для ограничения доступа.
	//   - arg3: непрозрачная граница продолжения предыдущей страницы.
	//   - arg4: максимальное количество результатов или операций в этом пакете.
	//   - arg5: проверенные фильтры, ограничивающие набор результатов.
	//
	// @returns:
	//   - result1: результирующая страница.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения.  //
	ListFiltered(context.Context, string, string, int, personal.ListFilter) (personal.Page, error)
	// Search поиск личных и групповых чатов с использованием ограниченного ввода и фильтров видимости вызывающей стороны.
	//
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - arg2: действующий идентификатор аккаунта, используемый для ограничения доступа.
	//   - arg3: текст для поиска или проверенные фильтры запроса.
	//
	// @returns:
	//   - result1: собранные значения peer.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения.
	//
	Search(context.Context, string, string) ([]personal.Peer, error)
	// SetConversationPreferences обновляет настройку уведомлений о переписке
	// только для текущего участника.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: идентификатор действующей учетной записи, используемый для ограничения доступа.
	//   - arg3: идентификатор ресурса, к которому обращается эта операция.
	//   - arg4: указание на то, следует ли включить запрашиваемую функцию или настройку.
	//
	// @returns:
	//   - result1: полученная переписка.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка проверки, доступа, транспорта или сохранения
	//     данных.
	SetConversationPreferences(context.Context, string, string, bool) (personal.Conversation, error)
	// ClearDirectHistory перемещает только границу истории прямого чата действующего участника, не
	// удаляя историю собеседника.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: идентификатор действующего аккаунта, используемый для ограничения доступа.
	//   - arg3: идентификатор ресурса, к которому обращается эта операция.
	//
	// @returns:
	//   - result1: итоговый диалог.
	//   - result2: nil в случае успеха; в противном случае — ошибка валидации, доступа, транспорта или сохранения
	//     данных.
	ClearDirectHistory(context.Context, string, string) (personal.Conversation, error)
	// HideDirectConversation скрывает прямой диалог для действующего аккаунта и сохраняет сохраненную историю сообщений собеседника.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: идентификатор действующего аккаунта, используемый для ограничения доступа.
	//   - arg3: идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: итоговое количество, размер или числовое значение.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка проверки, доступа, передачи или сохранения.  //
	HideDirectConversation(context.Context, string, string) (int64, error)
	// ConversationPins возвращает страницу закрепленных сообщений авторизованного диалога.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: действующий идентификатор учетной записи, используемый для ограничения доступа.
	//   - arg3: идентификатор ресурса, к которому обращается данная операция.
	//   - arg4: непрозрачная граница продолжения предыдущей страницы.
	//   - arg5: максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: страница с результатами.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения. 	//
	ConversationPins(context.Context, string, string, string, int) (chat.MessagePage, error)
	// SetConversationPin закрепляет или открепляет видимое сообщение и возвращает область беседы, на которую повлияло изменение.
	//
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: действующий идентификатор аккаунта, используемый для ограничения доступа.
	//   - arg3: идентификатор ресурса, к которому применяется эта операция.
	//   - arg4: идентификатор сообщения в рамках авторизованной беседы.
	//   - arg5: нужно ли закрепить выбранное сообщение.
	//
	// @returns:
	//   - result1: идентификатор беседы, состояние закрепления которой изменилось.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения.
	//
	SetConversationPin(context.Context, string, string, string, bool) (string, error)
}
type Handler struct {
	Repo     Repository
	Events   *personalusecase.Events
	Presence personalusecase.PresenceReader
}

// uuidParameter нормализует UUID маршрута и записывает существующий ответ об ошибке
// в случае сбоя валидации, прежде чем можно будет выполнить какую-либо операцию с репозиторием.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - name: имя настраиваемого параметра, cookie, ресурса или провайдера.
//
// @returns:
//   - result1: валидированный ненулевой UUID ресурса.
//   - result2: выполняется ли задокументированное условие операции или ее условное состояние
//     изменилось успешно.
func uuidParameter(c *gin.Context, name string) (string, bool) {
	id, err := chat.UUID(c.Param(name))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// AccountOnly отклоняет идентификационные данные гостей перед обработкой диалогов только для зарегистрированных аккаунтов или
// операций с папками.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
func (h *Handler) AccountOnly(c *gin.Context) {
	if err := h.Repo.Account(c.Request.Context(), middleware.UserID(c)); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Next()
}

// Create проверяет запрос и возвращает вновь созданный диалог для действующего
// аккаунта.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Create(c *gin.Context) {
	var body struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	id, err := chat.UUID(body.UserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	item, created, err := h.Repo.GetOrCreate(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if created && h.Events != nil {
		_ = h.Events.PublishConversation(c.Request.Context(), item.ID, "conversation.updated", gin.H{"conversationId": item.ID, "type": "direct"})
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// List возвращает ограниченную страницу записей бесед с использованием заданных параметров
// разбивки на страницы и фильтров.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
func (h *Handler) List(c *gin.Context) {
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = n
	}
	filter := personal.ListFilter{Type: c.Query("type"), Search: c.Query("search")}
	if raw, present := c.GetQuery("unreadOnly"); present {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		filter.UnreadOnly = value
	}
	page, err := h.Repo.ListFiltered(c.Request.Context(), middleware.UserID(c), c.Query("before"), limit, filter)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, page)
}

// Get предоставляет запрошенный диалог в рамках полномочий вызывающей стороны.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Get(c *gin.Context) {
	id, err := chat.UUID(c.Param("id"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	item, err := h.Repo.Get(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// Search поиск личных и групповых чатов с использованием ограниченного ввода и фильтров видимости вызывающей стороны.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Search(c *gin.Context) {
	items, err := h.Repo.Search(c.Request.Context(), middleware.UserID(c), c.Query("search"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}
