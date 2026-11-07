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
		setting("emailVerificationRequired", "Подтверждение email", "Новые пользователи получают ссылку на email и могут войти после подтверждения адреса.", "general", "boolean", v.EmailVerificationRequired),
		setting("ldapEnabled", "LDAP вход", "Включает вход и синхронизацию после сохранения всех параметров.", "ldap", "boolean", v.LDAPEnabled),
		{Key: "ldapMode", Label: "Тип каталога", Description: "AD использует objectGUID и проверку статуса учётной записи; generic подходит для LLDAP.", Group: "ldap", Type: "string", Value: v.LDAPMode, Editable: true, Configured: true, Options: []string{"ad", "generic"}},
		setting("ldapServerURL", "Адрес LDAP", "ldaps:// или ldap:// с обязательным StartTLS.", "ldap", "string", v.LDAPServerURL),
		setting("ldapBaseDN", "Базовый DN", "Корень каталога.", "ldap", "string", v.LDAPBaseDN),
		setting("ldapUsersDN", "DN пользователей", "Область поиска пользователей.", "ldap", "string", v.LDAPUsersDN),
		setting("ldapUserFilter", "Фильтр пользователей", "Дополнительный LDAP фильтр для поиска и синхронизации.", "ldap", "string", v.LDAPUserFilter),
		setting("ldapLoginAttribute", "Атрибут логина", "AD: sAMAccountName; LLDAP: uid.", "ldap", "string", v.LDAPLoginAttribute),
		setting("ldapIDAttribute", "Неизменяемый ID", "AD: objectGUID; LLDAP: entryUUID. Не меняйте после первой синхронизации.", "ldap", "string", v.LDAPIDAttribute),
		setting("ldapEmailAttribute", "Атрибут email", "Обычно mail.", "ldap", "string", v.LDAPEmailAttribute),
		setting("ldapNameAttribute", "Атрибут имени", "AD: displayName; LLDAP: cn.", "ldap", "string", v.LDAPNameAttribute),
		setting("ldapGroupDN", "Группа доступа", "Только участники этой группы могут войти; пустое значение при включении запрещено.", "ldap", "string", v.LDAPGroupDN),
		setting("ldapGroupMembershipAttribute", "Атрибут членства", "Обычно memberOf.", "ldap", "string", v.LDAPGroupMembershipAttribute),
		setting("ldapNestedGroups", "Вложенные группы AD", "Использует правило Active Directory для рекурсивного членства.", "ldap", "boolean", v.LDAPNestedGroups),
		setting("ldapBindDN", "Bind DN", "Учётная запись с правом чтения каталога.", "ldap", "string", v.LDAPBindDN),
		{Key: "ldapBindPassword", Label: "Пароль bind", Description: "Оставьте пустым, чтобы сохранить текущий пароль. Значение никогда не возвращается в браузер.", Group: "ldap", Type: "string", Value: nil, Editable: true, Secret: true, Configured: v.LDAPBindPasswordConfigured},
		setting("ldapCA", "CA сертификат PEM", "Доверенный CA сервера LDAP; при пустом значении используются системные корни.", "ldap", "string", v.LDAPCA),
		bounds("ldapSyncIntervalMinutes", "Период синхронизации, минут", "От 5 до 1440 минут.", "ldap", int64(v.LDAPSyncIntervalMinutes), 5, 1440),
		bounds("ldapTimeoutSeconds", "Тайм-аут LDAP, секунд", "От 2 до 60 секунд.", "ldap", int64(v.LDAPTimeoutSeconds), 2, 60),
	}
	return admin.Settings{Revision: v.Revision, Items: items}
}
