// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package authapp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type ldapLoginService interface {
	LDAPLogin(context.Context, users.LDAPLoginRequest) (users.LoginResponse, error)
}

type ldapAvailability interface {
	Enabled(context.Context) (bool, error)
}

func (h *Handler) WithLDAPProviders(reader ldapAvailability) *Handler {
	h.ldap = reader
	return h
}

func (h *Handler) Providers(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	enabled := false
	if h.ldap != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var err error
		enabled, err = h.ldap.Enabled(ctx)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrUnavailable)
			return
		}
	}
	mode := "local"
	if enabled {
		mode = "ldap"
	}
	c.JSON(http.StatusOK, gin.H{"mode": mode, "registrationEnabled": !enabled, "passwordRecoveryEnabled": !enabled, "ldap": gin.H{"enabled": enabled, "label": "Корпоративный вход"}})
}

func (h *Handler) LDAPLogin(c *gin.Context) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	service, ok := h.service.(ldapLoginService)
	if !ok {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	// Запас на экранирование JSON; длина логина и пароля проверяется после разбора.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	var request users.LDAPLoginRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	response, err := service.LDAPLogin(ctx, request)
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
