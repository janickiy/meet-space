package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	domain "github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type Passwords interface {
	Hash(string) (string, error)
	Verify(string, string) (bool, error)
}
type Service struct {
	Repo      domain.Repository
	passwords Passwords
	dummy     string
	work      chan struct{}
}

func NewService(repo domain.Repository, passwords Passwords) (*Service, error) {
	dummy, err := passwords.Hash("unusable-dummy-" + uuid.NewString())
	if err != nil {
		return nil, err
	}
	return &Service{Repo: repo, passwords: passwords, dummy: dummy, work: make(chan struct{}, 2)}, nil
}
func (s *Service) passwordWork(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.work <- struct{}{}:
		defer func() { <-s.work }()
		return fn()
	default:
		return apperrors.New(apperrors.ErrRateLimited, "Слишком много попыток. Повторите позже")
	}
}
func HashToken(token string) string {
	v := sha256.Sum256([]byte(token))
	return hex.EncodeToString(v[:])
}
func CSRFToken(token string) string { return HashToken("meetspace-admin-csrf:" + token) }
func ValidToken(token string) bool {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(raw) == 32
}
func VerifyCSRF(token, csrf string) bool {
	return len(csrf) == 64 && subtle.ConstantTimeCompare([]byte(CSRFToken(token)), []byte(csrf)) == 1
}
func (s *Service) Authenticate(ctx context.Context, token string) (domain.Administrator, error) {
	if !ValidToken(token) {
		return domain.Administrator{}, apperrors.ErrUnauthorized
	}
	return s.Repo.Authenticate(ctx, HashToken(token))
}

type LoginResult struct {
	Administrator domain.Administrator `json:"administrator"`
	CSRFToken     string               `json:"csrfToken"`
	Token         string               `json:"-"`
}

func (s *Service) Login(ctx context.Context, username, password string, audit domain.AuditInput) (LoginResult, error) {
	var out LoginResult
	username = strings.ToLower(strings.TrimSpace(username))
	allowed, err := s.Repo.LoginAllowed(ctx, audit.IP, username)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, apperrors.New(apperrors.ErrRateLimited, "Слишком много попыток входа. Повторите через 15 минут")
	}
	if len(password) > 512 || len(username) > 100 {
		return out, apperrors.ErrUnauthorized
	}
	a, lookupErr := s.Repo.ByUsername(ctx, username)
	if lookupErr != nil && !errors.Is(lookupErr, apperrors.ErrNotFound) {
		return out, lookupErr
	}
	hash := s.dummy
	if lookupErr == nil {
		hash = a.PasswordHash
	}
	valid := false
	err = s.passwordWork(ctx, func() error {
		var verifyErr error
		valid, verifyErr = s.passwords.Verify(password, hash)
		return verifyErr
	})
	if err != nil {
		return out, err
	}
	if !valid || lookupErr != nil || a.Status != "active" {
		audit.AdministratorID = ""
		audit.Username = ""
		audit.Action = "auth.login_failed"
		audit.ObjectType = "administrator"
		audit.ObjectID = ""
		if err := s.Repo.Audit(ctx, audit); err != nil {
			return out, err
		}
		return out, apperrors.New(apperrors.ErrUnauthorized, "Неверный логин или пароль")
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return out, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	session := domain.Session{ID: uuid.NewString(), AdministratorID: a.ID, TokenHash: HashToken(token), CreatedAt: now, ExpiresAt: now.Add(domain.SessionTTL)}
	if err = s.Repo.CreateSession(ctx, session, a.PasswordHash, audit); err != nil {
		return out, err
	}
	a.LastLoginAt = &now
	return LoginResult{Administrator: a, CSRFToken: CSRFToken(token), Token: token}, nil
}
func (s *Service) Logout(ctx context.Context, token string, audit domain.AuditInput) error {
	return s.Repo.RevokeSession(ctx, HashToken(token), audit)
}
func (s *Service) Create(ctx context.Context, input domain.CreateRequest, audit domain.AuditInput, bootstrap bool) (domain.Administrator, error) {
	var out domain.Administrator
	username, err := domain.NormalizeUsername(input.Username)
	if err != nil {
		return out, err
	}
	if err = domain.ValidatePassword(input.Password); err != nil {
		return out, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = username
	}
	if err = domain.ValidateName(name); err != nil {
		return out, err
	}
	email, err := domain.NormalizeEmail(input.Email)
	if err != nil {
		return out, err
	}
	var hash string
	if err = s.passwordWork(ctx, func() error { var e error; hash, e = s.passwords.Hash(input.Password); return e }); err != nil {
		return out, err
	}
	out = domain.Administrator{ID: uuid.NewString(), Username: username, PasswordHash: hash, Name: name, Email: email, Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if bootstrap {
		return s.Repo.Bootstrap(ctx, out)
	}
	return s.Repo.Create(ctx, out, audit)
}
func (s *Service) Update(ctx context.Context, id string, input domain.UpdateRequest, audit domain.AuditInput) (domain.Administrator, error) {
	canonical, err := canonicalID(id)
	if err != nil {
		return domain.Administrator{}, err
	}
	id = canonical
	if input.Username != nil {
		v, err := domain.NormalizeUsername(*input.Username)
		if err != nil {
			return domain.Administrator{}, err
		}
		input.Username = &v
	}
	if input.Name != nil {
		v := strings.TrimSpace(*input.Name)
		if err := domain.ValidateName(v); err != nil {
			return domain.Administrator{}, err
		}
		input.Name = &v
	}
	if input.Email != nil {
		v, err := domain.NormalizeEmail(input.Email)
		if err != nil {
			return domain.Administrator{}, err
		}
		if v == nil {
			empty := ""
			v = &empty
		}
		input.Email = v
	}
	if input.Status != nil && *input.Status != "active" && *input.Status != "blocked" {
		return domain.Administrator{}, apperrors.New(apperrors.ErrInvalidInput, "Некорректный статус")
	}
	if input.Username == nil && input.Name == nil && input.Email == nil && input.Status == nil {
		return domain.Administrator{}, apperrors.New(apperrors.ErrInvalidInput, "Нет изменяемых полей")
	}
	return s.Repo.Update(ctx, id, input, audit)
}
func canonicalID(id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "Некорректный идентификатор")
	}
	return parsed.String(), nil
}
func (s *Service) Delete(ctx context.Context, id string, audit domain.AuditInput) error {
	canonical, err := canonicalID(id)
	if err != nil {
		return err
	}
	id = canonical
	return s.Repo.Delete(ctx, id, audit)
}
func (s *Service) Password(ctx context.Context, id, current, password string, audit domain.AuditInput, recovery bool) error {
	canonical, err := canonicalID(id)
	if err != nil {
		return err
	}
	id = canonical
	if err := domain.ValidatePassword(password); err != nil {
		return err
	}
	a, err := s.Repo.ByID(ctx, id)
	if err != nil {
		return err
	}
	expected := ""
	if !recovery && id == audit.AdministratorID {
		if len(current) > 512 {
			return apperrors.ErrUnauthorized
		}
		var valid bool
		err = s.passwordWork(ctx, func() error { var e error; valid, e = s.passwords.Verify(current, a.PasswordHash); return e })
		if err != nil {
			return err
		}
		if !valid {
			return apperrors.New(apperrors.ErrUnauthorized, "Текущий пароль неверен")
		}
		expected = a.PasswordHash
	}
	var hash string
	if err = s.passwordWork(ctx, func() error { var e error; hash, e = s.passwords.Hash(password); return e }); err != nil {
		return err
	}
	if recovery {
		audit.Action = "administrator.recover"
		audit.Metadata = map[string]any{"source": "server-cli"}
	}
	return s.Repo.ChangePassword(ctx, id, hash, expected, audit)
}
func (s *Service) SetUserStatus(ctx context.Context, id, status string, audit domain.AuditInput) (domain.UserView, error) {
	canonical, err := canonicalID(id)
	if err != nil {
		return domain.UserView{}, err
	}
	id = canonical
	if status != "active" && status != "blocked" {
		return domain.UserView{}, apperrors.New(apperrors.ErrInvalidInput, "Некорректный статус")
	}
	return s.Repo.SetUserStatus(ctx, id, status, audit)
}
