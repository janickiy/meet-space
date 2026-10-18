package personalapp

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
)

type GroupRepository interface {
	// CreateGroup создает групповой чат с проверенными зарегистрированными участниками и владельцем.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: идентификатор учетной записи, выполняющей операцию.
	//   - аргумент 3: проверенный запрос приложения или входящие параметры операции.
	//
	// Возвращаемые значения:
	//   - результат 1: созданный чат.
	//   - результат 2: указывает, был ли создан новый объект, а не повторно использован идемпотентный результат.
	//   - результат 3: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения
	//     данных.
	CreateGroup(context.Context, string, personal.CreateGroupRequest) (personal.Conversation, bool, error)
	// UpdateGroup обновляет валидированные метаданные группы после проверки прав доступа к управлению.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: идентификатор учетной записи, выполняющей операцию.
	//   - аргумент 3: идентификатор ресурса, к которому применяется эта операция.
	//   - аргумент 4: валидированный запрос приложения или входящие параметры операции.
	//
	// Возвращаемые значения:
	//   - результат 1: результирующий диалог.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения
	//     данных.
	UpdateGroup(context.Context, string, string, personal.UpdateGroupRequest) (personal.Conversation, error)
	// GroupMembers возвращает список участников группы, видимых для действующего аккаунта.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: идентификатор аккаунта, выполняющего операцию.
	//   - аргумент 3: идентификатор ресурса, к которому обращается эта операция.
	//
	// Возвращает:
	//   - результат 1: собранные значения участников.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	GroupMembers(context.Context, string, string) ([]personal.Member, error)
	// AddGroupMembers добавляет проверенные зарегистрированные аккаунты после проверки членства в группе и соблюдения правил управления.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: идентификатор аккаунта, выполняющего операцию.
	//   - аргумент 3: идентификатор ресурса, к которому применяется эта операция.
	//   - аргумент 4: идентификаторы ненадёжных участников для проверки и дедупликации.
	//
	// Возвращаемые значения:
	//   - результат 1: результирующий диалог.
	//   - результат 2: собранные значения участников.
	//   - результат 3: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.  //
	AddGroupMembers(context.Context, string, string, []string) (personal.Conversation, []personal.Member, error)
	// RemoveGroupMember удаляет участника группы, соблюдая ограничения на владение и управление
	// ресурсами.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: идентификатор аккаунта, выполняющего операцию.
	//   - аргумент 3: идентификатор ресурса, к которому применяется эта операция.
	//   - аргумент 4: целевое состояние, аккаунт или декодированное значение в соответствии с его типом.
	//
	// Возвращаемые значения:
	//   - результат 1: результирующий диалог.
	//   - результат 2: выполняется ли задокументированное условие операции или изменение условного состояния
	//     прошло успешно.
	//   - результат 3: nil в случае успеха; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	RemoveGroupMember(context.Context, string, string, string) (personal.Conversation, bool, error)
	// ChangeGroupRole изменяет роль участника группы в рамках полномочий действующего менеджера.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - аргумент 2: идентификатор аккаунта, выполняющего операцию.
	//   - аргумент 3: идентификатор ресурса, к которому применяется эта операция.
	//   - аргумент 4: целевое состояние, аккаунт или декодированное значение в соответствии с его типом.
	//   - аргумент 5: подтвержденная роль, которую нужно назначить целевому участнику группы.
	//
	// Возвращаемые значения:
	//   - результат 1: результирующий диалог.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	ChangeGroupRole(context.Context, string, string, string, personal.Role) (personal.Conversation, error)
	// TransferGroupOwnership атомарно передает право владения группой соответствующему текущему участнику.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - аргумент 2: идентификатор учетной записи, выполняющей операцию.
	//   - аргумент 3: идентификатор ресурса, к которому применяется эта операция.
	//   - аргумент 4: целевое состояние, учетная запись или декодированное значение в соответствии с его типом.
	//
	// Возвращаемые значения:
	//   - результат 1: результирующий диалог.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	TransferGroupOwnership(context.Context, string, string, string) (personal.Conversation, error)
	// LeaveGroup покидает группу только в том случае, если правила владения позволяют действующему участнику выйти из нее.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - аргумент 2: идентификатор учетной записи, выполняющей операцию.
	//   - аргумент 3: идентификатор ресурса, к которому обращается эта операция.
	//
	// Возвращаемые значения:
	//   - результат 1: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.
	//
	LeaveGroup(context.Context, string, string) error
	// DeleteGroup удаляет группу для ее авторизованного владельца и возвращает затронутых получателей для
	// доставки событий.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: идентификатор учетной записи, выполняющей операцию.
	//   - аргумент 3: идентификатор ресурса, к которому применяется эта операция.
	//
	// Возвращает:
	//   - результат 1: собранные строковые значения.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	DeleteGroup(context.Context, string, string) ([]string, error)
}

// groupResult записывает общедоступный ответ о групповом разговоре после успешного выполнения операции.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - item: состояние диалога, обработанное в ходе этой операции.
func groupResult(c *gin.Context, item personal.Conversation) {
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// groupEvent публикует зафиксированные метаданные группы или изменения в членстве в группе вместе с ограниченным событием
// полезная нагрузка.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - id: идентификатор ресурса, к которому применяется эта операция.
//   - kind: валидированное событие, задание, медиафайл или категория ресурса.
//   - user: действующий идентификатор учетной записи, используемый для ограничения доступа.
//   - role: запрошенная роль участника или члена группы.
//   - extra: дополнительные поля ограниченного события или обратные вызовы.
func (h *Handler) groupEvent(c *gin.Context, id, kind, user string, role personal.Role, extra ...string) {
	if h.Events == nil {
		return
	}
	data := gin.H{"conversationId": id, "type": "group"}
	if user != "" {
		data["userId"] = user
	}
	if role != "" {
		data["role"] = role
	}
	_ = h.Events.PublishConversationTo(c.Request.Context(), id, kind, data, extra...)
}

// CreateGroup создает групповой чат с проверенными зарегистрированными участниками и владельцем.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) CreateGroup(c *gin.Context) {
	var body personal.CreateGroupRequest
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	actor := middleware.UserID(c)
	item, created, err := h.Repo.CreateGroup(c.Request.Context(), actor, body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if created {
		h.groupEvent(c, item.ID, "conversation.member.added", actor, personal.Owner)
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// UpdateGroup обновляет проверенные метаданные группы после проверки прав администратора.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) UpdateGroup(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body personal.UpdateGroupRequest
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.UpdateGroup(c.Request.Context(), middleware.UserID(c), id, body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.updated", "", "")
	groupResult(c, item)
}

// GroupMembers возвращает список участников группы, видимых для действующего аккаунта.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) GroupMembers(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	items, err := personalusecase.ReadGroupMembers(c.Request.Context(), h.Repo, h.Presence, middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}

// AddGroupMembers добавляет проверенные зарегистрированные учетные записи после проверки членства в группе и соблюдения правил управления.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) AddGroupMembers(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, added, err := h.Repo.AddGroupMembers(c.Request.Context(), middleware.UserID(c), id, body.UserIDs)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	for _, member := range added {
		h.groupEvent(c, id, "conversation.member.added", member.ID, member.Role)
	}
	groupResult(c, item)
}

// RemoveGroupMember удаляет участника группы, соблюдая ограничения на владение и управление.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) RemoveGroupMember(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	target, ok := uuidParameter(c, "userId")
	if !ok {
		return
	}
	item, changed, err := h.Repo.RemoveGroupMember(c.Request.Context(), middleware.UserID(c), id, target)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if changed {
		h.groupEvent(c, id, "conversation.member.removed", target, "", target)
	}
	groupResult(c, item)
}

// ChangeGroupRole изменяет роль участника группы в рамках полномочий действующего руководителя.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) ChangeGroupRole(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	target, ok := uuidParameter(c, "userId")
	if !ok {
		return
	}
	var body struct {
		Role personal.Role `json:"role"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.ChangeGroupRole(c.Request.Context(), middleware.UserID(c), id, target, body.Role)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.updated", target, body.Role)
	groupResult(c, item)
}

// TransferGroupOwnership атомарно передает право собственности на группу соответствующему текущему участнику.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) TransferGroupOwnership(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	target, err := chat.UUID(body.UserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	actor := middleware.UserID(c)
	item, err := h.Repo.TransferGroupOwnership(c.Request.Context(), actor, id, target)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.updated", actor, personal.Admin)
	h.groupEvent(c, id, "conversation.member.updated", target, personal.Owner)
	groupResult(c, item)
}

// LeaveGroup покидает группу только в том случае, если правила владения группой позволяют действующему участнику выйти из нее.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) LeaveGroup(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	var body struct{}
	if !httpresponse.BindJSON(c, &body, true) {
		return
	}
	actor := middleware.UserID(c)
	if err := h.Repo.LeaveGroup(c.Request.Context(), actor, id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.groupEvent(c, id, "conversation.member.removed", actor, "", actor)
	c.JSON(200, gin.H{"status": "success"})
}

// DeleteGroup удаляет группу для ее авторизованного владельца и возвращает затронутых получателей для
// доставки событий.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) DeleteGroup(c *gin.Context) {
	id, ok := uuidParameter(c, "id")
	if !ok {
		return
	}
	former, err := h.Repo.DeleteGroup(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	for _, user := range former {
		h.groupEvent(c, id, "conversation.member.removed", user, "", user)
	}
	c.JSON(200, gin.H{"status": "success"})
}
