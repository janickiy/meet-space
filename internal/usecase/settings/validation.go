package settings

import (
	"bytes"
	"encoding/json"
	"net"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
	domain "github.com/janickiy/meet-space/internal/domain/settings"
)

func invalid(message string) error { return apperrors.New(apperrors.ErrInvalidInput, message) }
func applyValue(v *domain.Values, key string, raw json.RawMessage) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return invalid("Значение настройки не может быть null")
	}
	var target any
	switch key {
	case "attachmentMaxBytes":
		target = &v.AttachmentMaxBytes
	case "mediaMaxPeers":
		target = &v.MediaMaxPeers
	case "mediaMaxRooms":
		target = &v.MediaMaxRooms
	case "recordingRetentionDays":
		target = &v.RecordingRetentionDays
	case "smtpEnabled":
		target = &v.SMTPEnabled
	case "smtpHost":
		target = &v.SMTPHost
	case "smtpPort":
		target = &v.SMTPPort
	case "smtpUsername":
		target = &v.SMTPUsername
	case "smtpFrom":
		target = &v.SMTPFrom
	case "smtpTLSMode":
		target = &v.SMTPTLSMode
	case "emailVerificationRequired":
		target = &v.EmailVerificationRequired
	case "ldapEnabled":
		target = &v.LDAPEnabled
	case "ldapMode":
		target = &v.LDAPMode
	case "ldapServerURL":
		target = &v.LDAPServerURL
	case "ldapBaseDN":
		target = &v.LDAPBaseDN
	case "ldapUsersDN":
		target = &v.LDAPUsersDN
	case "ldapUserFilter":
		target = &v.LDAPUserFilter
	case "ldapLoginAttribute":
		target = &v.LDAPLoginAttribute
	case "ldapIDAttribute":
		target = &v.LDAPIDAttribute
	case "ldapEmailAttribute":
		target = &v.LDAPEmailAttribute
	case "ldapNameAttribute":
		target = &v.LDAPNameAttribute
	case "ldapGroupDN":
		target = &v.LDAPGroupDN
	case "ldapGroupMembershipAttribute":
		target = &v.LDAPGroupMembershipAttribute
	case "ldapNestedGroups":
		target = &v.LDAPNestedGroups
	case "ldapBindDN":
		target = &v.LDAPBindDN
	case "ldapCA":
		target = &v.LDAPCA
	case "ldapSyncIntervalMinutes":
		target = &v.LDAPSyncIntervalMinutes
	case "ldapTimeoutSeconds":
		target = &v.LDAPTimeoutSeconds
	default:
		return invalid("Неизвестная или недоступная для изменения настройка")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return invalid("Неверный тип значения настройки")
	}
	return nil
}
func validPassword(v string) bool {
	return utf8.ValidString(v) && len(v) <= 4096 && !strings.ContainsRune(v, 0)
}
func validateBase(v domain.Values) error {
	if v.AttachmentMaxBytes < 1024 || v.AttachmentMaxBytes > 100*1024*1024 {
		return invalid("Размер вложения должен быть от 1024 до 104857600 байт")
	}
	if v.MediaMaxPeers < 1 || v.MediaMaxPeers > 100 {
		return invalid("Лимит медиаподключений должен быть от 1 до 100")
	}
	if v.MediaMaxRooms < 1 || v.MediaMaxRooms > 10000 {
		return invalid("Лимит медиакомнат должен быть от 1 до 10000")
	}
	if v.RecordingRetentionDays < 1 || v.RecordingRetentionDays > 3650 {
		return invalid("Срок хранения должен быть от 1 до 3650 дней")
	}
	if v.SMTPPort < 1 || v.SMTPPort > 65535 {
		return invalid("Порт SMTP должен быть от 1 до 65535")
	}
	if v.SMTPTLSMode != "tls" && v.SMTPTLSMode != "starttls" {
		return invalid("Режим SMTP должен быть tls или starttls")
	}
	if v.SMTPHost != "" && !validHost(v.SMTPHost) {
		return invalid("Укажите hostname или IP SMTP без схемы, порта и учётных данных")
	}
	if !utf8.ValidString(v.SMTPUsername) || len(v.SMTPUsername) > 512 || strings.ContainsAny(v.SMTPUsername, "\r\n\x00") {
		return invalid("Некорректный логин SMTP")
	}
	if v.SMTPFrom != "" && !validSender(v.SMTPFrom) {
		return invalid("Укажите один корректный email отправителя SMTP")
	}
	if err := directory.ValidateDraft(ldapConfig(v)); err != nil {
		return invalid("Некорректные настройки LDAP: " + err.Error())
	}
	return nil
}
func validate(v domain.Values) error {
	if err := validateBase(v); err != nil {
		return err
	}
	if v.SMTPEnabled && (v.SMTPHost == "" || strings.TrimSpace(v.SMTPUsername) == "" || v.SMTPFrom == "" || !v.SMTPPasswordConfigured) {
		return invalid("Для включения SMTP заполните сервер, порт, логин, пароль, отправителя и режим TLS")
	}
	if v.EmailVerificationRequired && !v.SMTPEnabled {
		return invalid("Для подтверждения email необходимо включить и настроить SMTP")
	}
	if v.LDAPEnabled {
		if !v.LDAPBindPasswordConfigured {
			return invalid("Для включения LDAP укажите пароль bind")
		}
		if err := directory.Validate(ldapConfig(v)); err != nil {
			return invalid("Для включения LDAP заполните корректные параметры каталога: " + err.Error())
		}
	}
	return nil
}

func ldapConfig(v domain.Values) directory.Config {
	c := directory.Config{
		Enabled: v.LDAPEnabled, Mode: v.LDAPMode, ServerURL: v.LDAPServerURL,
		BaseDN: v.LDAPBaseDN, UsersDN: v.LDAPUsersDN, UserFilter: v.LDAPUserFilter,
		LoginAttribute: v.LDAPLoginAttribute, IDAttribute: v.LDAPIDAttribute,
		EmailAttribute: v.LDAPEmailAttribute, NameAttribute: v.LDAPNameAttribute,
		GroupDN: v.LDAPGroupDN, GroupMembershipAttribute: v.LDAPGroupMembershipAttribute,
		NestedGroups: v.LDAPNestedGroups, BindDN: v.LDAPBindDN, CA: v.LDAPCA,
		SyncIntervalMinutes: v.LDAPSyncIntervalMinutes, TimeoutSeconds: v.LDAPTimeoutSeconds,
		Revision: v.Revision,
	}
	if v.LDAPBindPasswordConfigured {
		c.BindPassword = "configured"
	}
	return c
}

// smtpConfigured checks persisted prerequisites without opening a connection or
// requiring public consumers to decrypt credentials.
func smtpConfigured(v domain.Values) bool {
	return v.SMTPEnabled && validate(v) == nil
}

func validHost(host string) bool {
	if len(host) > 253 || strings.ContainsAny(host, "/\\\r\n\t @?#") || strings.TrimSpace(host) != host {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}
func validSender(from string) bool {
	if !utf8.ValidString(from) || len(from) > 512 || strings.ContainsAny(from, "\r\n\x00") {
		return false
	}
	address, err := mail.ParseAddress(from)
	if err != nil {
		return false
	}
	for _, char := range address.Address {
		if char < 33 || char > 126 {
			return false
		}
	}
	return strings.Contains(address.Address, "@")
}
