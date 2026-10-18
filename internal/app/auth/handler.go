package authapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// Сервис определяет заменяемый контракт зависимостей для аутентификации учетных записей и постоянных сеансов.
type Service interface {
	// Register проверяет поля регистрации, хеширует пароль и создает новую учетную запись, не возвращая
	// конфиденциальные данные учетных данных.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: проверенный запрос приложения или входящие параметры операции.
	//
	// Возвращает:
	//   - результат 1: результирующее представление.
	//   - результат 2: nil в случае успеха; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	Register(context.Context, users.RegisterRequest) (users.View, error)
	// Функция Login проверяет введенные учетные данные и возвращает безопасное представление учетной записи и токен доступа.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: проверенный запрос приложения или входящие параметры операции.
	//
	// Возвращаемые значения:
	//   - результат 1: ответ на запрос входа.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения. 	//
	Login(context.Context, users.LoginRequest) (users.LoginResponse, error)
	// Me возвращает общедоступное представление аккаунта, для которого выполнена аутентификация.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: действующий идентификатор аккаунта, используемый для ограничения доступа.
	//
	// Возвращает:
	//   - результат 1: результирующее представление.
	//   - результат 2: nil в случае успеха; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	Me(context.Context, string) (users.View, error)
	// UpdateProfile обновляет только проверенные общедоступные поля профиля аутентифицированной учетной записи.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: действующий идентификатор учетной записи, используемый для ограничения доступа.
	//   - аргумент 3: проверенный запрос приложения или входящие параметры операции.
	//
	// Возвращаемые значения:
	//   - результат 1: результирующее представление.
	//   - результат 2: nil в случае успеха; в противном случае — ошибка валидации, доступа, транспорта или сохранения. 	//
	UpdateProfile(context.Context, string, users.UpdateProfileRequest) (users.View, error)
}

// Обработчик адаптирует HTTP-запросы для аутентификации аккаунта и постоянных сеансов, проверяет
// входные данные и формирует ответ.
type Handler struct {
	service            Service
	publicOrigin       string
	secureCookie       bool
	persistentSessions bool
}

// NewHandler создает обработчик с зависимостями и ограничениями, необходимыми для аутентификации учетной записи
// и постоянных сеансов.
//
// Параметры:
//   - service: служба приложения, вызываемая этим адаптером.
//
// Возвращает:
//   - result 1: созданный или загруженный обработчик.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// Register validates registration fields, hashes the password and creates a new account without
// returning private credential data.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Register(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request users.RegisterRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	user, err := h.service.Register(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "user": user})
}

// Login проверяет введенные учетные данные и возвращает безопасное представление учетной записи и токен доступа.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Login(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request users.LoginRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	response, err := h.service.Login(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if h.persistentSessions {
		if err := h.preserveSignIn(c, response); err != nil {
			if sessions, ok := h.service.(persistentService); ok {
				_ = sessions.Logout(c.Request.Context(), response.SessionToken, "")
			}
			httpresponse.Fail(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, response)
}

// Me возвращает общедоступный вид аккаунта, для которого выполнена аутентификация.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая данные аутентифицированного пользователя.
func (h *Handler) Me(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user, err := h.service.Me(c.Request.Context(), httpmiddleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

// UpdateProfile обновляет только проверенные общедоступные поля профиля аутентифицированной учетной записи.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) UpdateProfile(c *gin.Context) {
	var request users.UpdateProfileRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	user, err := h.service.UpdateProfile(c.Request.Context(), httpmiddleware.UserID(c), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

// Logout revokes the selected persistent session and invalidates the supplied access-session
// binding.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Logout(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	if h.persistentSessions {
		sessions, ok := h.service.(persistentService)
		if !ok {
			httpresponse.Fail(c, apperrors.ErrUnavailable)
			return
		}
		raw := h.sessionCookie(c)
		id := request.UserID
		if id != "" {
			raw, _ = c.Cookie(accountCookieName(id))
			if raw == "" {
				if service, ok := h.service.(accountService); ok {
					if current, err := service.SessionAccount(c.Request.Context(), h.sessionCookie(c)); err == nil && current.ID == id {
						raw = h.sessionCookie(c)
					}
				}
			}
		}
		if service, ok := h.service.(accountService); ok {
			if user, err := service.SessionAccount(c.Request.Context(), raw); err == nil {
				id = user.ID
			}
		}
		if err := sessions.Logout(c.Request.Context(), raw, bearerToken(c)); err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if id != "" {
			h.setNamedSessionCookie(c, accountCookieName(id), "")
		}
		if request.UserID == "" || raw == h.sessionCookie(c) {
			h.setSessionCookie(c, "")
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
