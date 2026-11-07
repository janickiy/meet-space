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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type providerModeProbe struct {
	enabled bool
	err     error
}

func (p providerModeProbe) Enabled(context.Context) (bool, error) { return p.enabled, p.err }

func TestProviderMetadataUsesExclusiveAuthenticationMode(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		handler := NewHandler(nil).WithLDAPProviders(providerModeProbe{enabled: enabled})
		writer := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(writer)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
		handler.Providers(ctx)
		var body struct {
			Mode                                         string
			RegistrationEnabled, PasswordRecoveryEnabled bool
			LDAP                                         struct {
				Enabled bool
				Label   string
			}
		}
		if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		mode := "local"
		if enabled {
			mode = "ldap"
		}
		if writer.Code != 200 || body.Mode != mode || body.RegistrationEnabled == enabled || body.PasswordRecoveryEnabled == enabled || body.LDAP.Enabled != enabled || body.LDAP.Label == "" {
			t.Fatalf("inconsistent providers: %s", writer.Body.String())
		}
		if writer.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("mode may be cached across a policy change")
		}
	}
}

func TestProviderMetadataCannotDefaultToLocalWhenSettingsFail(t *testing.T) {
	handler := NewHandler(nil).WithLDAPProviders(providerModeProbe{err: apperrors.ErrUnavailable})
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	handler.Providers(ctx)
	if writer.Code != http.StatusServiceUnavailable {
		t.Fatalf("settings failure allowed local mode: %d", writer.Code)
	}
}
