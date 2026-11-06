package settings

import (
	"github.com/janickiy/meet-space/internal/domain/admin"
	domain "github.com/janickiy/meet-space/internal/domain/settings"
)

func describe(v domain.Values) admin.Settings {
	bounds := func(key, label, description, group string, value, min, max int64) admin.Setting {
		step := int64(1)
		return admin.Setting{Key: key, Label: label, Description: description, Group: group, Type: "number", Value: value, Editable: true, Configured: true, Min: &min, Max: &max, Step: &step}
	}
	setting := func(key, label, description, group, kind string, value any) admin.Setting {
		configured := true
		if text, ok := value.(string); ok {
			configured = text != ""
		}
		return admin.Setting{Key: key, Label: label, Description: description, Group: group, Type: kind, Value: value, Editable: true, Configured: configured}
	}
	items := []admin.Setting{
		bounds("attachmentMaxBytes", "Максимальный размер вложения", "Применяется к новым загрузкам; существующие файлы сохраняются.", "limits", v.AttachmentMaxBytes, 1024, 100*1024*1024),
		bounds("mediaMaxPeers", "Лимит участников медиакомнаты", "Новые подключения используют новый лимит. Действующие участники не отключаются.", "limits", int64(v.MediaMaxPeers), 1, 100),
		bounds("mediaMaxRooms", "Лимит медиакомнат", "Применяется к созданию новых медиакомнат.", "limits", int64(v.MediaMaxRooms), 1, 10000),
		bounds("recordingRetentionDays", "Срок хранения записей, дней", "Политика применяется при очередном проходе очистки записей.", "storage", int64(v.RecordingRetentionDays), 1, 3650),
		setting("smtpEnabled", "Доставка через SMTP", "Включайте после заполнения всех параметров SMTP.", "mail", "boolean", v.SMTPEnabled),
		setting("smtpHost", "SMTP-сервер", "Hostname или IP без схемы и порта.", "mail", "string", v.SMTPHost),
		bounds("smtpPort", "SMTP-порт", "Обычно 465 для TLS или 587 для STARTTLS.", "mail", int64(v.SMTPPort), 1, 65535),
		setting("smtpUsername", "Логин SMTP", "Учётная запись для аутентификации на SMTP-сервере.", "mail", "string", v.SMTPUsername),
		{Key: "smtpPassword", Label: "Пароль SMTP", Description: "Оставьте пустым, чтобы сохранить текущий пароль. Значение никогда не возвращается в браузер.", Group: "mail", Type: "string", Value: nil, Editable: true, Secret: true, Configured: v.SMTPPasswordConfigured},
		setting("smtpFrom", "Отправитель", "Один email; допускается отображаемое имя.", "mail", "string", v.SMTPFrom),
		{Key: "smtpTLSMode", Label: "Шифрование SMTP", Description: "Незашифрованная отправка не поддерживается.", Group: "mail", Type: "string", Value: v.SMTPTLSMode, Editable: true, Configured: true, Options: []string{"tls", "starttls"}},
		setting("emailVerificationRequired", "Подтверждение email", "Требует настроенной доставки SMTP.", "mail", "boolean", v.EmailVerificationRequired),
	}
	return admin.Settings{Revision: v.Revision, Items: items}
}
