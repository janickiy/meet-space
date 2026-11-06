package authapp

import (
	"context"

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
	// There is no recovery-delivery implementation yet, so this method always
	// reports either account ineligibility or unavailable recovery.
	if err := service.PasswordRecovery(c.Request.Context(), input.Email); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	httpresponse.Fail(c, apperrors.New(apperrors.ErrUnavailable, "password recovery unavailable"))
}
