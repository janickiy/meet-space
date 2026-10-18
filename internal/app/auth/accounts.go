package authapp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

const accountCookiePrefix = "meetrix_account_"
const maxBrowserAccounts = 5

type accountService interface {
	// SessionAccount возвращает общедоступное представление учетной записи для действительного постоянного сеанса.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: сериализованное значение, не требующее доверия, для декодирования или проверки.
	//
	// @returns:
	//   - result1: результирующее представление.
	//   - result2: nil в случае успеха; в противном случае — ошибка при проверке, доступе, передаче или сохранении. 	//
	SessionAccount(context.Context, string) (users.View, error)
	// DeleteAccount проверяет право собственности на учетную запись и удаляет ее вместе с областью действия
	// состояние сеанса.
	//
	// @Parameters:
	// - arg1: Контекст, управляющий отменой, крайними сроками и сроком действия операции.
	// - arg2: Идентификатор ресурса, к которому обращается эта операция.
	// - arg3: Открытый текстовый пароль принимается только для проверки или хеширования.
	//
	// @returns:
	// - result1: ноль в случае успеха; в противном случае проверка, доступ, транспортировка или сохранение
	// сбой.
	DeleteAccount(context.Context, string, string) error
}

// accountCookieName формирует имя cookie-файла для обновления данных для каждого аккаунта на основе идентификатора аккаунта.
//
// @Parameters:
//   - id: идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: имя cookie-файла для обновления данных для каждого аккаунта с атрибутом HttpOnly.
func accountCookieName(id string) string { return accountCookiePrefix + id }

// browserAccounts считывает действительные постоянные сеансы учетных записей браузера, не возвращая их
// секреты.
//
// Имена файлов cookie используются только в качестве селекторов. Каждый секрет проверяется на соответствие владельцу на стороне сервера.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// @returns:
//   - result1: собранные значения представления.
//   - result2: nil в случае успешного выполнения; в противном случае — ошибка проверки, доступа, передачи или сохранения
//     данных.
func (h *Handler) browserAccounts(c *gin.Context) ([]users.View, error) {
	service, ok := h.service.(accountService)
	if !ok || !h.persistentSessions {
		return nil, apperrors.ErrUnavailable
	}
	items := []users.View{}
	seen := map[string]bool{}
	for _, cookie := range c.Request.Cookies() {
		if !strings.HasPrefix(cookie.Name, accountCookiePrefix) {
			continue
		}
		id := strings.TrimPrefix(cookie.Name, accountCookiePrefix)
		if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
			continue
		}
		user, err := service.SessionAccount(c.Request.Context(), cookie.Value)
		if errors.Is(err, apperrors.ErrUnauthorized) || errors.Is(err, apperrors.ErrForbidden) {
			h.setNamedSessionCookie(c, cookie.Name, "")
			continue
		}
		if err != nil {
			return nil, err
		}
		if user.ID != id {
			h.setNamedSessionCookie(c, cookie.Name, "")
			continue
		}
		if !seen[id] {
			items = append(items, user)
			seen[id] = true
		}
		if len(items) > maxBrowserAccounts {
			return nil, apperrors.New(apperrors.ErrConflict, "browser account limit reached")
		}
	}
	return items, nil
}

// preserveSignIn сохраняет другие идентификационные данные в файлах cookie с атрибутом HttpOnly, а не в хранилище JavaScript.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированный идентификатор.
//   - response: ответ приложения, в котором обрабатываются метаданные сеанса или общедоступные метаданные.
//
// Возвращаемое значение:
//   - result1: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения.
func (h *Handler) preserveSignIn(c *gin.Context, response users.LoginResponse) error {
	service, ok := h.service.(accountService)
	if !ok {
		return apperrors.ErrUnavailable
	}
	items, err := h.browserAccounts(c)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.ID] = true
	}
	previous := h.sessionCookie(c)
	if previous != "" {
		user, err := service.SessionAccount(c.Request.Context(), previous)
		if err != nil && !errors.Is(err, apperrors.ErrUnauthorized) && !errors.Is(err, apperrors.ErrForbidden) {
			return err
		}
		if err == nil {
			seen[user.ID] = true
			if user.ID != response.User.ID {
				h.setNamedSessionCookie(c, accountCookieName(user.ID), previous)
			}
		}
	}
	if !seen[response.User.ID] && len(seen) >= maxBrowserAccounts {
		return apperrors.New(apperrors.ErrConflict, "browser account limit reached")
	}
	// При повторном входе в тот же аккаунт старый вход в браузере будет отменен. Другие
	// сохраненные аккаунты и сеансы, принадлежащие другим устройствам, будут сохранены.
	if sessions, ok := h.service.(persistentService); ok {
		saved, _ := c.Cookie(accountCookieName(response.User.ID))
		for _, raw := range []string{saved, previous} {
			if raw == "" || raw == response.SessionToken {
				continue
			}
			user, err := service.SessionAccount(c.Request.Context(), raw)
			if errors.Is(err, apperrors.ErrUnauthorized) || errors.Is(err, apperrors.ErrForbidden) {
				continue
			}
			if err != nil {
				return err
			}
			if user.ID == response.User.ID {
				if err := sessions.Logout(c.Request.Context(), raw, ""); err != nil {
					return err
				}
			}
		}
	}
	h.setNamedSessionCookie(c, accountCookieName(response.User.ID), response.SessionToken)
	h.setSessionCookie(c, response.SessionToken)
	return nil
}

// Accounts выводит список учетных записей браузера, прошедших аутентификацию, без возврата секретов сеанса.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Accounts(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	items, err := h.browserAccounts(c)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

// SwitchAccount выбирает учетную запись браузера только после проверки ее постоянного сеанса на сервере.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) SwitchAccount(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if parsed, err := uuid.Parse(request.UserID); err != nil || parsed.String() != request.UserID {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	sessions, ok := h.service.(persistentService)
	if !ok || !h.persistentSessions {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	raw, _ := c.Cookie(accountCookieName(request.UserID))
	response, err := sessions.Refresh(c.Request.Context(), raw)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if response.User.ID != request.UserID {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return
	}
	h.setSessionCookie(c, response.SessionToken)
	c.JSON(http.StatusOK, response)
}

// DeleteAccount проверяет право собственности на учетную запись и удаляет ее вместе с состоянием сеанса в области видимости.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) DeleteAccount(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request struct {
		Password string `json:"password"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	service, ok := h.service.(accountService)
	if !ok {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	id := httpmiddleware.UserID(c)
	current, currentError := service.SessionAccount(c.Request.Context(), h.sessionCookie(c))
	if currentError != nil && !errors.Is(currentError, apperrors.ErrUnauthorized) && !errors.Is(currentError, apperrors.ErrForbidden) {
		httpresponse.Fail(c, currentError)
		return
	}
	if err := service.DeleteAccount(c.Request.Context(), id, request.Password); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.setNamedSessionCookie(c, accountCookieName(id), "")
	// Не удаляйте файлы cookie другой учетной записи, выбранные одновременно на другой вкладке.
	if currentError != nil || current.ID == id {
		h.setSessionCookie(c, "")
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
