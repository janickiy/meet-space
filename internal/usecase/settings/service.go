package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
	domain "github.com/janickiy/meet-space/internal/domain/settings"
)

const smtpPasswordBinding = "meetspace:platform-settings:smtp-password:v1"
const ldapBindPasswordBinding = "meetspace:platform-settings:ldap-bind-password:v1"

type Cipher interface {
	Encrypt(value, binding string) (string, error)
	Decrypt(value, binding string) (string, error)
}

type Service struct {
	repository          domain.Repository
	defaults            domain.Values
	defaultSMTPPassword string
	cipher              Cipher
	publicURL           string
}

func NewService(repo domain.Repository, defaults domain.Values, defaultSMTPPassword string, cipher Cipher, publicURL ...string) *Service {
	if defaults.LDAPMode == "" {
		defaults.LDAPMode = directory.ModeAD
	}
	if defaults.LDAPUserFilter == "" {
		defaults.LDAPUserFilter = "(&(objectClass=user)(!(objectClass=computer)))"
	}
	if defaults.LDAPLoginAttribute == "" {
		defaults.LDAPLoginAttribute = "sAMAccountName"
	}
	if defaults.LDAPIDAttribute == "" {
		defaults.LDAPIDAttribute = "objectGUID"
	}
	if defaults.LDAPEmailAttribute == "" {
		defaults.LDAPEmailAttribute = "mail"
	}
	if defaults.LDAPNameAttribute == "" {
		defaults.LDAPNameAttribute = "displayName"
	}
	if defaults.LDAPGroupMembershipAttribute == "" {
		defaults.LDAPGroupMembershipAttribute = "memberOf"
	}
	if defaults.LDAPSyncIntervalMinutes == 0 {
		defaults.LDAPSyncIntervalMinutes = 60
	}
	if defaults.LDAPTimeoutSeconds == 0 {
		defaults.LDAPTimeoutSeconds = 10
	}
	s := &Service{repository: repo, defaults: defaults, defaultSMTPPassword: defaultSMTPPassword, cipher: cipher}
	if len(publicURL) > 0 {
		s.publicURL = publicURL[0]
	}
	return s
}

// Snapshot does one indexed singleton read. Database errors are never replaced
// by defaults: callers must not silently reopen a disabled/limited operation.
func (s *Service) Snapshot(ctx context.Context) (domain.Values, error) {
	state, err := s.repository.Load(ctx)
	if err != nil {
		return domain.Values{}, err
	}
	return s.merge(state)
}

// LDAPEnabled читает только публичный флаг; отсутствие ключа шифрования не ломает выбор входа.
func (s *Service) LDAPEnabled(ctx context.Context) (bool, error) {
	values, err := s.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	return values.LDAPEnabled, nil
}
func (s *Service) merge(state domain.State) (domain.Values, error) {
	values := s.defaults
	var overrides map[string]json.RawMessage
	if err := json.Unmarshal(state.Overrides, &overrides); err != nil || overrides == nil {
		return domain.Values{}, apperrors.New(apperrors.ErrInternal, "Не удалось прочитать настройки платформы")
	}
	for key, raw := range overrides {
		if key == "smtpPassword" || key == "ldapBindPassword" || applyValue(&values, key, raw) != nil {
			return domain.Values{}, apperrors.New(apperrors.ErrInternal, "Некорректные сохранённые настройки платформы")
		}
	}
	values.Revision = state.Revision
	values.SMTPPasswordConfigured = state.SMTPPasswordCiphertext != "" || s.defaultSMTPPassword != ""
	values.LDAPBindPasswordConfigured = state.LDAPBindPasswordCiphertext != ""
	if err := validateBase(values); err != nil {
		return domain.Values{}, apperrors.Wrap(apperrors.ErrInternal, err, "Некорректные настройки платформы")
	}
	return values, nil
}
func (s *Service) SMTP(ctx context.Context) (domain.SMTPConfig, error) {
	state, err := s.repository.Load(ctx)
	if err != nil {
		return domain.SMTPConfig{}, err
	}
	values, err := s.merge(state)
	if err != nil {
		return domain.SMTPConfig{}, err
	}
	if err := validate(values); err != nil {
		return domain.SMTPConfig{}, apperrors.Wrap(apperrors.ErrUnavailable, err, "SMTP настроен не полностью")
	}
	out := domain.SMTPConfig{Enabled: values.SMTPEnabled, Host: values.SMTPHost, Port: values.SMTPPort, Username: values.SMTPUsername, From: values.SMTPFrom, TLSMode: values.SMTPTLSMode}
	if !out.Enabled {
		return out, nil
	}
	out.Password = s.defaultSMTPPassword
	if state.SMTPPasswordCiphertext != "" {
		if s.cipher == nil {
			return domain.SMTPConfig{}, apperrors.New(apperrors.ErrUnavailable, "Для SMTP требуется ключ шифрования PROVIDER_TOKEN_ENCRYPTION_KEY на сервере.")
		}
		out.Password, err = s.cipher.Decrypt(state.SMTPPasswordCiphertext, smtpPasswordBinding)
		if err != nil {
			return domain.SMTPConfig{}, apperrors.New(apperrors.ErrUnavailable, "Не удалось расшифровать пароль SMTP. Проверьте ключ шифрования на сервере.")
		}
	}
	return out, nil
}

// LDAP возвращает конфигурацию только серверному коду; bind пароль расшифровывается с отдельным AAD.
func (s *Service) LDAP(ctx context.Context) (directory.Config, error) {
	state, err := s.repository.Load(ctx)
	if err != nil {
		return directory.Config{}, err
	}
	values, err := s.merge(state)
	if err != nil {
		return directory.Config{}, err
	}
	config := ldapConfig(values)
	if state.LDAPBindPasswordCiphertext != "" {
		if s.cipher == nil {
			return directory.Config{}, apperrors.New(apperrors.ErrUnavailable, "Для LDAP требуется ключ шифрования PROVIDER_TOKEN_ENCRYPTION_KEY на сервере.")
		}
		config.BindPassword, err = s.cipher.Decrypt(state.LDAPBindPasswordCiphertext, ldapBindPasswordBinding)
		if err != nil {
			return directory.Config{}, apperrors.New(apperrors.ErrUnavailable, "Не удалось расшифровать пароль LDAP bind. Проверьте ключ шифрования на сервере.")
		}
	}
	if config.Enabled {
		if err := directory.Validate(config); err != nil {
			return directory.Config{}, apperrors.Wrap(apperrors.ErrUnavailable, err, "LDAP настроен не полностью")
		}
	}
	return config, nil
}
func (s *Service) Admin(ctx context.Context) (admin.Settings, error) {
	values, err := s.Snapshot(ctx)
	if err != nil {
		return admin.Settings{}, err
	}
	return describe(values), nil
}

func DecodePatch(data json.RawMessage) (domain.Patch, error) {
	var input struct {
		Revision *int64                     `json:"revision"`
		Changes  map[string]json.RawMessage `json:"changes"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.Revision == nil || *input.Revision < 0 || len(input.Changes) == 0 {
		return domain.Patch{}, apperrors.New(apperrors.ErrInvalidInput, "Укажите текущую версию и изменяемые настройки")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.Patch{}, apperrors.ErrInvalidInput
	}
	return domain.Patch{Revision: *input.Revision, Changes: input.Changes}, nil
}
func encryptionUnavailable() error {
	return apperrors.New(apperrors.ErrInvalidInput, "Для изменения SMTP настройте PROVIDER_TOKEN_ENCRYPTION_KEY на сервере.")
}
func ldapEncryptionUnavailable() error {
	return apperrors.New(apperrors.ErrInvalidInput, "Для изменения пароля LDAP bind настройте PROVIDER_TOKEN_ENCRYPTION_KEY на сервере.")
}
func (s *Service) Update(ctx context.Context, input domain.Patch, audit admin.AuditInput) (admin.Settings, error) {
	if input.Revision < 0 || len(input.Changes) == 0 {
		return admin.Settings{}, apperrors.ErrInvalidInput
	}
	state, err := s.repository.Load(ctx)
	if err != nil {
		return admin.Settings{}, err
	}
	if state.Revision != input.Revision {
		return admin.Settings{}, apperrors.New(apperrors.ErrConflict, "Настройки уже изменены. Обновите страницу и повторите изменения.")
	}
	values, err := s.merge(state)
	if err != nil {
		return admin.Settings{}, err
	}
	_, verificationChanged := input.Changes["emailVerificationRequired"]
	if verificationChanged {
		// Mail must already be saved before registration policy can be edited;
		// SMTP fields supplied in this same patch are not a saved prerequisite.
		if !smtpConfigured(values) {
			return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Для изменения подтверждения email сначала настройте и сохраните SMTP во вкладке «Почта».")
		}
		if state.SMTPPasswordCiphertext != "" {
			if s.cipher == nil {
				return admin.Settings{}, encryptionUnavailable()
			}
			if _, err = s.cipher.Decrypt(state.SMTPPasswordCiphertext, smtpPasswordBinding); err != nil {
				return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Не удалось расшифровать пароль SMTP. Укажите пароль заново или восстановите ключ шифрования.")
			}
		}
	}
	var overrides map[string]json.RawMessage
	if err = json.Unmarshal(state.Overrides, &overrides); err != nil {
		return admin.Settings{}, err
	}
	fields := make([]string, 0, len(input.Changes))
	smtpTouched := false
	ldapTouched := false
	for key, raw := range input.Changes {
		if strings.HasPrefix(key, "smtp") {
			smtpTouched = true
		}
		if strings.HasPrefix(key, "ldap") {
			ldapTouched = true
		}
		if key == "smtpPassword" {
			var password string
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &password) != nil || !validPassword(password) {
				return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Некорректный пароль SMTP: максимум 4096 байт, без нулевого символа")
			}
			if password == "" {
				continue
			}
			if s.cipher == nil {
				return admin.Settings{}, encryptionUnavailable()
			}
			state.SMTPPasswordCiphertext, err = s.cipher.Encrypt(password, smtpPasswordBinding)
			if err != nil {
				return admin.Settings{}, encryptionUnavailable()
			}
			values.SMTPPasswordConfigured = true
		} else if key == "ldapBindPassword" {
			var password string
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &password) != nil || !validPassword(password) {
				return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Некорректный пароль LDAP bind: максимум 4096 байт, без нулевого символа")
			}
			if password == "" {
				continue
			}
			if s.cipher == nil {
				return admin.Settings{}, ldapEncryptionUnavailable()
			}
			state.LDAPBindPasswordCiphertext, err = s.cipher.Encrypt(password, ldapBindPasswordBinding)
			if err != nil {
				return admin.Settings{}, ldapEncryptionUnavailable()
			}
			values.LDAPBindPasswordConfigured = true
		} else {
			if err = applyValue(&values, key, raw); err != nil {
				return admin.Settings{}, err
			}
			if strings.HasPrefix(key, "smtp") && s.cipher == nil {
				return admin.Settings{}, encryptionUnavailable()
			}
			overrides[key] = append(json.RawMessage(nil), raw...)
		}
		fields = append(fields, key)
	}
	if err = validate(values); err != nil {
		return admin.Settings{}, err
	}
	if values.SMTPEnabled {
		parsed, e := url.Parse(s.publicURL)
		if e != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" || parsed.Scheme != "https" && parsed.Scheme != "http" {
			return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Для отправки писем задайте PUBLIC_FRONTEND_URL с адресом платформы без пути и параметров.")
		}
	}
	if len(fields) == 0 {
		return describe(values), nil
	}
	// Enabling SMTP must prove that the configured credential is usable with the
	// current encryption key, not merely that some ciphertext exists in storage.
	if verificationChanged && values.EmailVerificationRequired && s.cipher == nil {
		return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Для подтверждения email настройте PROVIDER_TOKEN_ENCRYPTION_KEY на сервере.")
	}
	if values.SMTPEnabled && (smtpTouched || (verificationChanged && values.EmailVerificationRequired)) && state.SMTPPasswordCiphertext != "" {
		if s.cipher == nil {
			return admin.Settings{}, encryptionUnavailable()
		}
		if _, err = s.cipher.Decrypt(state.SMTPPasswordCiphertext, smtpPasswordBinding); err != nil {
			return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Не удалось расшифровать пароль SMTP. Укажите пароль заново или восстановите ключ шифрования.")
		}
	}
	if values.LDAPEnabled && ldapTouched {
		if s.cipher == nil {
			return admin.Settings{}, ldapEncryptionUnavailable()
		}
		if state.LDAPBindPasswordCiphertext != "" {
			if _, err = s.cipher.Decrypt(state.LDAPBindPasswordCiphertext, ldapBindPasswordBinding); err != nil {
				return admin.Settings{}, apperrors.New(apperrors.ErrInvalidInput, "Не удалось расшифровать пароль LDAP bind. Укажите пароль заново или восстановите ключ шифрования.")
			}
		}
	}
	encoded, err := json.Marshal(overrides)
	if err != nil {
		return admin.Settings{}, err
	}
	sort.Strings(fields)
	saved, err := s.repository.Save(ctx, domain.SaveInput{ExpectedRevision: input.Revision, Overrides: encoded, SMTPPasswordCiphertext: state.SMTPPasswordCiphertext, LDAPBindPasswordCiphertext: state.LDAPBindPasswordCiphertext, Fields: fields}, audit)
	if err != nil {
		return admin.Settings{}, err
	}
	values.Revision = saved.Revision
	return describe(values), nil
}
