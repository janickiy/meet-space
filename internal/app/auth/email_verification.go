package authapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"net/http"
)

type emailVerificationService interface {
	VerifyEmail(context.Context, string) error
	ResendVerification(context.Context, string) error
}

func (h *Handler) VerifyEmail(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var input struct {
		Token string `json:"token"`
	}
	if !httpresponse.BindJSON(c, &input, false) {
		return
	}
	service, ok := h.service.(emailVerificationService)
	if !ok {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	if err := service.VerifyEmail(c.Request.Context(), input.Token); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
func (h *Handler) ResendVerification(c *gin.Context) {
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
	service, ok := h.service.(emailVerificationService)
	if !ok {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	if err := service.ResendVerification(c.Request.Context(), input.Email); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Если аккаунт ожидает подтверждения, письмо будет отправлено."})
}
