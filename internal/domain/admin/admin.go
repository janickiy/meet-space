// Package admin defines a separate administrative identity boundary.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const CookieName = "__Secure-meetspace_admin_session"
const CookiePath = "/api/v1/admin"
const SessionTTL = 12 * time.Hour

// Administrator must never expose its password hash through JSON.
type Administrator struct {
	ID           string     `json:"id" gorm:"type:uuid;primaryKey"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Name         string     `json:"name"`
	Email        *string    `json:"email"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastLoginAt  *time.Time `json:"lastLoginAt"`
}

func (Administrator) TableName() string { return "administrators" }

type Session struct {
	ID              string    `json:"-" gorm:"type:uuid;primaryKey"`
	AdministratorID string    `json:"-"`
	TokenHash       string    `json:"-"`
	CreatedAt       time.Time `json:"-"`
	ExpiresAt       time.Time `json:"-"`
}

func (Session) TableName() string { return "administrator_sessions" }

type AuditInput struct {
	AdministratorID string
	Username        string
	Action          string
	ObjectType      string
	ObjectID        string
	IP              string
	UserAgent       string
	Metadata        map[string]any
}

type AuditEvent struct {
	ID              string          `json:"id"`
	AdministratorID *string         `json:"administratorId"`
	Username        string          `json:"administratorUsername"`
	Action          string          `json:"action"`
	ObjectType      string          `json:"objectType"`
	ObjectID        string          `json:"objectId"`
	IP              string          `json:"ip"`
	UserAgent       string          `json:"userAgent"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       time.Time       `json:"createdAt"`
}

func (AuditEvent) TableName() string { return "administrator_audit_log" }

type UserView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     *string   `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type Query struct {
	Page, PageSize                             int
	Q, Status, Sort, Order, Action, ObjectType string
}

func (q Query) Normalized() Query {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Page > 100000 {
		q.Page = 100000
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	q.Q = strings.ToValidUTF8(strings.TrimSpace(q.Q), "")
	if runes := []rune(q.Q); len(runes) > 200 {
		q.Q = string(runes[:200])
	}
	if q.Order != "asc" {
		q.Order = "desc"
	}
	return q
}
func (q Query) Offset() int { q = q.Normalized(); return (q.Page - 1) * q.PageSize }

type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

func NewPage[T any](q Query) Page[T] {
	q = q.Normalized()
	return Page[T]{Items: []T{}, Page: q.Page, PageSize: q.PageSize}
}

type CreateRequest struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Name     string  `json:"name"`
	Email    *string `json:"email"`
}
type UpdateRequest struct {
	Username *string `json:"username"`
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Status   *string `json:"status"`
}

// UnmarshalJSON preserves the distinction between omitted email and explicit null.
func (r *UpdateRequest) UnmarshalJSON(data []byte) error {
	type plain UpdateRequest
	var next plain
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&next); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw, exists := fields["email"]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		empty := ""
		next.Email = &empty
	}
	*r = UpdateRequest(next)
	return nil
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,39}$`)

func NormalizeUsername(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if !usernamePattern.MatchString(v) {
		return "", apperrors.New(apperrors.ErrInvalidInput, "Логин должен содержать 3–40 латинских букв, цифр, точек, дефисов или подчёркиваний")
	}
	return v, nil
}
func ValidatePassword(v string) error {
	n := utf8.RuneCountInString(v)
	if !utf8.ValidString(v) || n < 8 || n > 128 || len(v) > 512 {
		return apperrors.New(apperrors.ErrInvalidInput, "Пароль должен содержать от 8 до 128 символов")
	}
	return nil
}
func ValidateName(v string) error {
	if utf8.RuneCountInString(strings.TrimSpace(v)) < 1 || utf8.RuneCountInString(v) > 100 {
		return apperrors.New(apperrors.ErrInvalidInput, "Укажите имя длиной до 100 символов")
	}
	return nil
}
func NormalizeEmail(v *string) (*string, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	s := strings.ToLower(strings.TrimSpace(*v))
	a, e := mail.ParseAddress(s)
	if e != nil || a.Address != s || len(s) > 254 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "Некорректный email")
	}
	return &s, nil
}

type Repository interface {
	ByUsername(context.Context, string) (Administrator, error)
	ByID(context.Context, string) (Administrator, error)
	Status(context.Context) (*Administrator, error)
	Bootstrap(context.Context, Administrator) (Administrator, error)
	Create(context.Context, Administrator, AuditInput) (Administrator, error)
	Update(context.Context, string, UpdateRequest, AuditInput) (Administrator, error)
	Delete(context.Context, string, AuditInput) error
	ChangePassword(context.Context, string, string, string, AuditInput) error
	CreateSession(context.Context, Session, string, AuditInput) error
	Authenticate(context.Context, string) (Administrator, error)
	RevokeSession(context.Context, string, AuditInput) error
	Audit(context.Context, AuditInput) error
	LoginAllowed(context.Context, string, string) (bool, error)
	List(context.Context, Query) (Page[Administrator], error)
	Users(context.Context, Query) (Page[UserView], error)
	SetUserStatus(context.Context, string, string, AuditInput) (UserView, error)
	AuditLog(context.Context, Query) (Page[AuditEvent], error)
}
