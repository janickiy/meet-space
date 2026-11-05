package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
)

// AdminResources keeps privileged metadata separate from account authentication and public routes.
type AdminResources interface {
	Query(context.Context, string, domain.Query) (any, error)
	DeleteRecording(context.Context, string, domain.AuditInput) error
	UpdateSettings(context.Context, json.RawMessage, domain.AuditInput) (any, error)
}

type adminHandler struct {
	service       *adminusecase.Service
	cfg           config.AdminConfig
	resources     AdminResources
	onUserBlocked func(context.Context, string) error
}

// NewAdminRouter must only be served by the separate, unpublished internal administrative listener.
func NewAdminRouter(service *adminusecase.Service, cfg config.AdminConfig, resources AdminResources, onUserBlocked func(context.Context, string) error) (*gin.Engine, error) {
	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, err
	}
	h := &adminHandler{service: service, cfg: cfg, resources: resources, onUserBlocked: onUserBlocked}
	r.Use(func(c *gin.Context) {
		c.Next()
		h.auditFailure(c)
	}, gin.CustomRecovery(func(c *gin.Context, _ any) { adminFailure(c, apperrors.ErrInternal) }), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		if !cfg.Enabled {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		// A server WriteTimeout only bounds socket writes. Cancel database and
		// dependency work before that deadline even if the client stays connected.
		ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
		// All non-safe requests require a configured browser origin, including login (login CSRF).
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			if c.GetHeader("Origin") != cfg.PublicURL {
				adminFailure(c, apperrors.New(apperrors.ErrForbidden, "Недопустимый источник запроса"))
				return
			}
			mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
			if err != nil || mediaType != "application/json" {
				adminFailure(c, apperrors.New(apperrors.ErrInvalidInput, "Ожидается application/json"))
				return
			}
		}
		if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			adminFailure(c, apperrors.ErrForbidden)
			return
		}
		c.Next()
	})
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	group := r.Group(domain.CookiePath)
	group.POST("/auth/login", h.login)
	protected := group.Group("")
	protected.Use(h.authenticate)
	protected.GET("/auth/me", func(c *gin.Context) {
		a, _ := c.Get("administrator")
		token, _ := c.Cookie(domain.CookieName)
		c.JSON(200, gin.H{"administrator": a, "csrfToken": adminusecase.CSRFToken(token)})
	})
	protected.POST("/auth/logout", h.logout)
	protected.POST("/auth/password", h.password)
	protected.GET("/administrators", func(c *gin.Context) {
		v, err := service.Repo.List(c.Request.Context(), adminQuery(c))
		adminResult(c, v, err)
	})
	protected.POST("/administrators", h.create)
	protected.PATCH("/administrators/:id", h.update)
	protected.DELETE("/administrators/:id", h.remove)
	protected.POST("/administrators/:id/password", h.password)
	protected.GET("/users", func(c *gin.Context) {
		v, err := service.Repo.Users(c.Request.Context(), adminQuery(c))
		adminResult(c, v, err)
	})
	protected.PATCH("/users/:id", h.userStatus)
	protected.GET("/audit", func(c *gin.Context) {
		v, err := service.Repo.AuditLog(c.Request.Context(), adminQuery(c))
		adminResult(c, v, err)
	})
	for _, resource := range []string{"dashboard", "conferences", "chats", "recordings", "files", "system", "settings"} {
		resource := resource
		protected.GET("/"+resource, func(c *gin.Context) {
			if resources == nil {
				adminFailure(c, apperrors.ErrUnavailable)
				return
			}
			v, err := resources.Query(c.Request.Context(), resource, adminQuery(c))
			adminResult(c, v, err)
		})
	}
	protected.DELETE("/recordings/:id", func(c *gin.Context) {
		if resources == nil {
			adminFailure(c, apperrors.ErrUnavailable)
			return
		}
		err := resources.DeleteRecording(c.Request.Context(), c.Param("id"), h.audit(c))
		if err != nil {
			adminFailure(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	protected.PATCH("/settings", func(c *gin.Context) {
		if resources == nil {
			adminFailure(c, apperrors.ErrUnavailable)
			return
		}
		var payload json.RawMessage
		if !adminDecode(c, &payload) {
			return
		}
		v, err := resources.UpdateSettings(c.Request.Context(), payload, h.audit(c))
		adminResult(c, v, err)
	})
	return r, nil
}
func adminQuery(c *gin.Context) domain.Query {
	p, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("pageSize"))
	return (domain.Query{Page: p, PageSize: size, Q: c.Query("q"), Status: c.Query("status"), Sort: c.Query("sort"), Order: c.Query("order"), Action: c.Query("action"), ObjectType: c.Query("objectType")}).Normalized()
}
func adminDecode(c *gin.Context, v any) bool {
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		adminFailure(c, apperrors.New(apperrors.ErrInvalidInput, "Некорректные данные запроса"))
		return false
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		adminFailure(c, apperrors.New(apperrors.ErrInvalidInput, "Ожидается один JSON-объект"))
		return false
	}
	return true
}
func adminFailure(c *gin.Context, err error) {
	status := 500
	message := "Не удалось выполнить запрос"
	code := "internal_error"
	category := apperrors.Classify(err)
	var appErr *apperrors.Error
	explicitCategory := errors.As(err, &appErr)
	if category == apperrors.Internal && !explicitCategory && adminDependencyUnavailable(err) {
		category = apperrors.Unavailable
	}
	switch category {
	case apperrors.Validation:
		status = 400
		code = "invalid_input"
		message = "Некорректные данные"
	case apperrors.Unauthenticated:
		status = 401
		code = "unauthorized"
		message = "Необходим вход администратора"
	case apperrors.Forbidden:
		status = 403
		code = "forbidden"
		message = "Доступ запрещён"
	case apperrors.NotFound:
		status = 404
		code = "not_found"
		message = "Объект не найден"
	case apperrors.Conflict:
		status = 409
		code = "conflict"
		message = "Операция недоступна"
	case apperrors.RateLimited:
		status = 429
		code = "rate_limited"
		message = "Слишком много запросов"
		c.Header("Retry-After", "900")
	case apperrors.Unavailable:
		status = 503
		code = "unavailable"
		message = "Сервис временно недоступен"
	}
	if explicitCategory && status < 500 {
		message = appErr.Message
	}
	if status >= 500 {
		slog.Error("admin request failed", "route", c.FullPath(), "category", code)
	}
	c.AbortWithStatusJSON(status, gin.H{"error": code, "message": message})
}
func adminResult(c *gin.Context, v any, err error) {
	if err != nil {
		adminFailure(c, err)
		return
	}
	c.JSON(200, v)
}
func (h *adminHandler) audit(c *gin.Context) domain.AuditInput {
	input := domain.AuditInput{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
	if v, ok := c.Get("administrator"); ok {
		a := v.(domain.Administrator)
		input.AdministratorID = a.ID
		input.Username = a.Username
	}
	return input
}

// Rejected mutations must remain visible even when their transaction rolls back
// or their request is cancelled. Never copy payloads, query strings, errors, or
// credentials into this separate, bounded audit write.
func (h *adminHandler) auditFailure(c *gin.Context) {
	if c.Writer.Status() < 400 {
		return
	}
	if _, authenticated := c.Get("administrator"); !authenticated {
		return
	}
	var action, objectType string
	switch c.Request.Method + " " + c.FullPath() {
	case "POST " + domain.CookiePath + "/administrators":
		action, objectType = "administrator.create_failed", "administrator"
	case "PATCH " + domain.CookiePath + "/administrators/:id":
		action, objectType = "administrator.update_failed", "administrator"
	case "DELETE " + domain.CookiePath + "/administrators/:id":
		action, objectType = "administrator.delete_failed", "administrator"
	case "POST " + domain.CookiePath + "/administrators/:id/password", "POST " + domain.CookiePath + "/auth/password":
		action, objectType = "administrator.password_failed", "administrator"
	case "POST " + domain.CookiePath + "/auth/logout":
		action, objectType = "auth.logout_failed", "administrator"
	case "PATCH " + domain.CookiePath + "/users/:id":
		action, objectType = "user.status_failed", "user"
	case "DELETE " + domain.CookiePath + "/recordings/:id":
		action, objectType = "recording.delete_failed", "recording"
	case "PATCH " + domain.CookiePath + "/settings":
		action, objectType = "settings.update_failed", "settings"
	default:
		return
	}
	event := h.audit(c)
	event.Action, event.ObjectType = action, objectType
	if id, err := uuid.Parse(c.Param("id")); err == nil {
		event.ObjectID = id.String()
	} else if c.FullPath() == domain.CookiePath+"/auth/password" || c.FullPath() == domain.CookiePath+"/auth/logout" {
		event.ObjectID = event.AdministratorID
	}
	event.Metadata = map[string]any{"httpStatus": c.Writer.Status()}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), time.Second)
	defer cancel()
	if err := h.service.Repo.Audit(ctx, event); err != nil {
		slog.Error("admin failure audit unavailable", "action", action, "httpStatus", c.Writer.Status())
	}
}
func (h *adminHandler) cookie(c *gin.Context, token string) {
	maxAge := int(domain.SessionTTL.Seconds())
	if token == "" {
		maxAge = -1
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: domain.CookieName, Value: token, Path: domain.CookiePath, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (h *adminHandler) authenticate(c *gin.Context) {
	token, err := c.Cookie(domain.CookieName)
	if err != nil {
		adminFailure(c, apperrors.ErrUnauthorized)
		return
	}
	a, err := h.service.Authenticate(c.Request.Context(), token)
	if err != nil {
		adminFailure(c, err)
		return
	}
	c.Set("administrator", a)
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" && !adminusecase.VerifyCSRF(token, c.GetHeader("X-CSRF-Token")) {
		adminFailure(c, apperrors.New(apperrors.ErrForbidden, "Недействительный защитный токен. Обновите страницу"))
		return
	}
	c.Request = c.Request.WithContext(domain.WithAuthorization(c.Request.Context(), a.ID, adminusecase.HashToken(token)))
	c.Next()
}
func (h *adminHandler) login(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !adminDecode(c, &input) {
		return
	}
	result, err := h.service.Login(c.Request.Context(), input.Username, input.Password, h.audit(c))
	input.Password = ""
	if err != nil {
		adminFailure(c, err)
		return
	}
	h.cookie(c, result.Token)
	c.JSON(200, result)
}
func (h *adminHandler) logout(c *gin.Context) {
	token, _ := c.Cookie(domain.CookieName)
	if err := h.service.Logout(c.Request.Context(), token, h.audit(c)); err != nil {
		adminFailure(c, err)
		return
	}
	h.cookie(c, "")
	c.Status(http.StatusNoContent)
}
func (h *adminHandler) create(c *gin.Context) {
	var input domain.CreateRequest
	if !adminDecode(c, &input) {
		return
	}
	a, err := h.service.Create(c.Request.Context(), input, h.audit(c), false)
	input.Password = ""
	if err != nil {
		adminFailure(c, err)
		return
	}
	c.JSON(201, a)
}
func (h *adminHandler) update(c *gin.Context) {
	var input domain.UpdateRequest
	if !adminDecode(c, &input) {
		return
	}
	a, err := h.service.Update(c.Request.Context(), c.Param("id"), input, h.audit(c))
	adminResult(c, a, err)
}
func (h *adminHandler) remove(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id"), h.audit(c)); err != nil {
		adminFailure(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *adminHandler) password(c *gin.Context) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		Password        string `json:"password"`
	}
	if !adminDecode(c, &input) {
		return
	}
	audit := h.audit(c)
	id := c.Param("id")
	if id == "" {
		id = audit.AdministratorID
	} else if parsed, err := uuid.Parse(id); err == nil {
		id = parsed.String()
	}
	if err := h.service.Password(c.Request.Context(), id, input.CurrentPassword, input.Password, audit, false); err != nil {
		adminFailure(c, err)
		return
	}
	input.Password = ""
	input.CurrentPassword = ""
	if id == audit.AdministratorID {
		h.cookie(c, "")
	}
	c.Status(http.StatusNoContent)
}
func (h *adminHandler) userStatus(c *gin.Context) {
	var input struct {
		Status string `json:"status"`
	}
	if !adminDecode(c, &input) {
		return
	}
	id := c.Param("id")
	v, err := h.service.SetUserStatus(c.Request.Context(), id, strings.TrimSpace(input.Status), h.audit(c))
	if err != nil {
		adminFailure(c, err)
		return
	}
	if v.Status == "blocked" && h.onUserBlocked != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if err = h.onUserBlocked(ctx, id); err != nil {
			slog.Warn("blocked user immediate disconnect incomplete", "user_id", id)
		}
	}
	c.JSON(200, v)
}
