// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Проверка параметров каталога до подключения и до сохранения настроек.
package directory

import (
	"crypto/x509"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	ldap "github.com/go-ldap/ldap/v3"
)

var ldapAttribute = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,63}$`)

// Validate проверяет готовую конфигурацию. LDAP без TLS допускается только через StartTLS.
func Validate(c Config) error { return validate(c, true) }

// ValidateDraft проверяет заполненные поля, пока LDAP ещё выключен.
func ValidateDraft(c Config) error { return validate(c, false) }

func validate(c Config, complete bool) error {
	bad := func(field string) error { return fmt.Errorf("%w: %s", ErrInvalidConfig, field) }
	if c.Mode != ModeAD && c.Mode != ModeGeneric {
		return bad("mode")
	}
	if c.SyncIntervalMinutes < 5 || c.SyncIntervalMinutes > 1440 || c.TimeoutSeconds < 2 || c.TimeoutSeconds > 60 {
		return bad("interval or timeout")
	}
	if c.Mode != ModeAD && c.NestedGroups {
		return bad("nested groups require AD")
	}
	if complete || c.ServerURL != "" {
		if len(c.ServerURL) > 2048 || strings.TrimSpace(c.ServerURL) != c.ServerURL || strings.ContainsAny(c.ServerURL, "\r\n\x00") {
			return bad("server URL")
		}
		u, err := url.Parse(c.ServerURL)
		if err != nil || (u.Scheme != "ldap" && u.Scheme != "ldaps") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Opaque != "" {
			return bad("server URL")
		}
		if u.Port() != "" {
			port, err := strconv.Atoi(u.Port())
			if err != nil || port < 1 || port > 65535 {
				return bad("server URL port")
			}
		}
	}
	distinguishedNames := make(map[string]*ldap.DN, 4)
	for _, item := range []struct{ name, value string }{{"base DN", c.BaseDN}, {"users DN", c.UsersDN}, {"bind DN", c.BindDN}, {"group DN", c.GroupDN}} {
		if !complete && item.value == "" {
			continue
		}
		if len(item.value) > 2048 || !utf8.ValidString(item.value) {
			return bad(item.name)
		}
		parsed, err := ldap.ParseDN(item.value)
		if err != nil || parsed == nil || len(parsed.RDNs) == 0 {
			return bad(item.name)
		}
		distinguishedNames[item.name] = parsed
	}
	// Области поиска должны принадлежать выбранному корню; bind может находиться в другой ветке.
	if base := distinguishedNames["base DN"]; base != nil {
		for _, name := range []string{"users DN", "group DN"} {
			if dn := distinguishedNames[name]; dn != nil && !base.EqualFold(dn) && !base.AncestorOfFold(dn) {
				return bad(name + " outside base DN")
			}
		}
	}
	for _, item := range []struct{ name, value string }{{"login attribute", c.LoginAttribute}, {"ID attribute", c.IDAttribute}, {"email attribute", c.EmailAttribute}, {"name attribute", c.NameAttribute}, {"group membership attribute", c.GroupMembershipAttribute}} {
		if !complete && item.value == "" {
			continue
		}
		if !ldapAttribute.MatchString(item.value) {
			return bad(item.name)
		}
	}
	if complete || c.UserFilter != "" {
		if len(c.UserFilter) > 4096 || !utf8.ValidString(c.UserFilter) {
			return bad("user filter")
		}
		if _, err := ldap.CompileFilter(c.UserFilter); err != nil {
			return bad("user filter")
		}
	}
	if c.CA != "" {
		if len(c.CA) > 65536 || !utf8.ValidString(c.CA) {
			return bad("CA")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(c.CA)) {
			return bad("CA")
		}
	}
	if complete && (c.BindPassword == "" || len(c.BindPassword) > 4096 || !utf8.ValidString(c.BindPassword) || strings.ContainsRune(c.BindPassword, 0)) {
		return bad("bind password")
	}
	return nil
}
