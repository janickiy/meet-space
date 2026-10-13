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
	SessionAccount(context.Context, string) (users.View, error)
	DeleteAccount(context.Context, string, string) error
}

func accountCookieName(id string) string { return accountCookiePrefix + id }

// Cookie names are only selectors. Each secret is checked against its server-side owner.
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

// preserveSignIn keeps other identities in HttpOnly cookies, never in JavaScript storage.
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
	// Re-entering the same account replaces that browser's old sign-in. Other
	// saved accounts and sessions belonging to other devices are preserved.
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
	// Do not clear a different account's cookie selected concurrently by another tab.
	if currentError != nil || current.ID == id {
		h.setSessionCookie(c, "")
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
