package authapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type passwordRecoveryService interface {
	PasswordRecovery(context.Context, string) error
}

func (h *Handler) PasswordRecovery(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var input struct {
		Email string `json:"email"`
	}
	if !httpresponse.BindJSON(c, &input, false) {
		return
	}
	service, ok := h.service.(passwordRecoveryService)
	if !ok {
		httpresponse.Fail(c, apperrors.New(apperrors.ErrUnavailable, "password recovery unavailable"))
		return
	}
	// The response does not disclose whether an active account exists.
	if err := service.PasswordRecovery(c.Request.Context(), input.Email); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Если аккаунт существует, ссылка для восстановления пароля будет отправлена."})
}

type passwordResetService interface {
	ResetPassword(context.Context, string, string) error
}

func (h *Handler) ResetPassword(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !httpresponse.BindJSON(c, &input, false) {
		return
	}
	service, ok := h.service.(passwordResetService)
	if !ok {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	if err := service.ResetPassword(c.Request.Context(), input.Token, input.Password); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
