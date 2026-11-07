// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Package ldap реализует подключение к AD и совместимому LDAP каталогу.
package ldap

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/directory"
)

const pageSize = 500

type Client struct{}

func NewClient() *Client { return &Client{} }

var _ directory.Client = (*Client)(nil)

// Authenticate проверяет пароль только после нахождения ровно одной разрешённой записи.
func (c *Client) Authenticate(ctx context.Context, cfg directory.Config, login, password string) (directory.Entry, error) {
	return c.authenticate(ctx, cfg, cfg.LoginAttribute, login, password)
}

// AuthenticateEmail ищет единственную запись по почте без изменения атрибута логина синхронизации.
func (c *Client) AuthenticateEmail(ctx context.Context, cfg directory.Config, email, password string) (directory.Entry, error) {
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || strings.ContainsAny(email, " \t\r\n") {
		return directory.Entry{}, directory.ErrInvalidCredentials
	}
	return c.authenticate(ctx, cfg, cfg.EmailAttribute, email, password)
}

func (c *Client) authenticate(ctx context.Context, cfg directory.Config, attribute, login, password string) (directory.Entry, error) {
	if err := directory.Validate(cfg); err != nil {
		return directory.Entry{}, err
	}
	if login == "" || password == "" || len(login) > 256 || len(password) > 4096 || !utf8.ValidString(login) || !utf8.ValidString(password) || strings.ContainsAny(login, "\r\n\x00") || strings.ContainsRune(password, 0) {
		return directory.Entry{}, directory.ErrInvalidCredentials
	}
	conn, done, err := open(ctx, cfg)
	if err != nil {
		return directory.Entry{}, err
	}
	defer done()
	return authenticateConnection(conn, cfg, attribute, login, password)
}

type authenticationConnection interface {
	Bind(string, string) error
	Search(*ldap.SearchRequest) (*ldap.SearchResult, error)
}

func authenticateConnection(conn authenticationConnection, cfg directory.Config, attribute, login, password string) (directory.Entry, error) {
	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		return directory.Entry{}, unavailable(err)
	}
	filter := identityFilter(cfg, attribute, login, false)
	request := ldap.NewSearchRequest(cfg.UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, cfg.TimeoutSeconds, false, filter, attributes(cfg), nil)
	result, err := conn.Search(request)
	if err != nil {
		return directory.Entry{}, unavailable(err)
	}
	if result == nil || result.ResultCode != 0 || len(result.Referrals) != 0 || len(result.Entries) > 1 {
		return directory.Entry{}, directory.ErrIncompleteScan
	}
	if len(result.Entries) == 0 {
		return directory.Entry{}, directory.ErrInvalidCredentials
	}
	user := result.Entries[0]
	entry, err := parseEntry(cfg, user)
	if err != nil {
		return directory.Entry{}, err
	}
	if entry.Disabled {
		return directory.Entry{}, directory.ErrNotAllowed
	}
	member := ldap.NewSearchRequest(cfg.UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, cfg.TimeoutSeconds, false, identityFilter(cfg, attribute, login, true), []string{"1.1"}, nil)
	allowed, err := conn.Search(member)
	if err != nil {
		return directory.Entry{}, unavailable(err)
	}
	if allowed == nil || allowed.ResultCode != 0 || len(allowed.Referrals) != 0 || len(allowed.Entries) > 1 {
		return directory.Entry{}, directory.ErrIncompleteScan
	}
	if len(allowed.Entries) != 1 || !sameDN(allowed.Entries[0].DN, user.DN) {
		return directory.Entry{}, directory.ErrNotAllowed
	}
	if err := conn.Bind(user.DN, password); err != nil {
		var ldapErr *ldap.Error
		if errors.As(err, &ldapErr) && ldapErr.ResultCode == ldap.LDAPResultInvalidCredentials {
			return directory.Entry{}, directory.ErrInvalidCredentials
		}
		return directory.Entry{}, unavailable(err)
	}
	return entry, nil
}

// Scan возвращает только полный проверенный снимок. Частичный ответ не применяется.
func (c *Client) Scan(ctx context.Context, cfg directory.Config) ([]directory.Entry, error) {
	if err := directory.Validate(cfg); err != nil {
		return nil, err
	}
	conn, done, err := open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer done()
	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		return nil, unavailable(err)
	}
	// До потенциально пустого снимка проверяем обе области поиска.
	// BaseDN задаёт границу DN, но naming context не обязан быть отдельной записью.
	for _, dn := range []string{cfg.UsersDN, cfg.GroupDN} {
		if err := requireObject(conn, cfg, dn); err != nil {
			return nil, err
		}
	}
	request := ldap.NewSearchRequest(cfg.UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, cfg.TimeoutSeconds, false, "(&"+cfg.UserFilter+memberFilter(cfg)+")", attributes(cfg), nil)
	paging := ldap.NewControlPaging(pageSize)
	request.Controls = []ldap.Control{paging}
	out := make([]directory.Entry, 0, pageSize)
	ids, logins, emails := make(map[string]struct{}), make(map[string]struct{}), make(map[string]struct{})
	seenCookies := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, unavailable(err)
		}
		result, err := conn.Search(request)
		if err != nil {
			return nil, incomplete(err)
		}
		if result == nil || result.ResultCode != 0 || len(result.Referrals) != 0 {
			return nil, directory.ErrIncompleteScan
		}
		control, ok := ldap.FindControl(result.Controls, ldap.ControlTypePaging).(*ldap.ControlPaging)
		if !ok {
			return nil, directory.ErrIncompleteScan
		}
		if len(out)+len(result.Entries) > directory.MaxUsers {
			return nil, directory.ErrIncompleteScan
		}
		for _, raw := range result.Entries {
			entry, err := parseEntry(cfg, raw)
			if err != nil {
				return nil, err
			}
			id, login, email := entry.ExternalID, strings.ToLower(entry.Login), strings.ToLower(entry.Email)
			if _, ok := ids[id]; ok {
				return nil, directory.ErrIncompleteScan
			}
			if _, ok := logins[login]; ok {
				return nil, directory.ErrIncompleteScan
			}
			if _, ok := emails[email]; ok {
				return nil, directory.ErrIncompleteScan
			}
			ids[id], logins[login], emails[email] = struct{}{}, struct{}{}, struct{}{}
			out = append(out, entry)
		}
		if len(control.Cookie) == 0 {
			break
		}
		cookie := string(control.Cookie)
		if _, ok := seenCookies[cookie]; ok || len(result.Entries) == 0 {
			return nil, directory.ErrIncompleteScan
		}
		seenCookies[cookie] = struct{}{}
		paging.SetCookie(control.Cookie)
	}
	return out, nil
}

// Test проверяет первый блок записей без затрат полного снимка большого каталога.
func (c *Client) Test(ctx context.Context, cfg directory.Config) (directory.TestResult, error) {
	if err := directory.Validate(cfg); err != nil {
		return directory.TestResult{}, err
	}
	conn, done, err := open(ctx, cfg)
	if err != nil {
		return directory.TestResult{}, err
	}
	defer done()
	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		return directory.TestResult{}, unavailable(err)
	}
	for _, dn := range []string{cfg.UsersDN, cfg.GroupDN} {
		if err := requireObject(conn, cfg, dn); err != nil {
			return directory.TestResult{}, err
		}
	}
	request := ldap.NewSearchRequest(cfg.UsersDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, cfg.TimeoutSeconds, false, "(&"+cfg.UserFilter+memberFilter(cfg)+")", attributes(cfg), []ldap.Control{ldap.NewControlPaging(pageSize)})
	result, err := conn.Search(request)
	if err != nil {
		return directory.TestResult{}, incomplete(err)
	}
	if result == nil || result.ResultCode != 0 || len(result.Referrals) != 0 || len(result.Entries) > pageSize {
		return directory.TestResult{}, directory.ErrIncompleteScan
	}
	control, ok := ldap.FindControl(result.Controls, ldap.ControlTypePaging).(*ldap.ControlPaging)
	if !ok {
		return directory.TestResult{}, directory.ErrIncompleteScan
	}
	ids, logins, emails := make(map[string]struct{}), make(map[string]struct{}), make(map[string]struct{})
	for _, raw := range result.Entries {
		entry, err := parseEntry(cfg, raw)
		if err != nil {
			return directory.TestResult{}, err
		}
		id, login, email := entry.ExternalID, strings.ToLower(entry.Login), strings.ToLower(entry.Email)
		if _, ok := ids[id]; ok {
			return directory.TestResult{}, directory.ErrIncompleteScan
		}
		if _, ok := logins[login]; ok {
			return directory.TestResult{}, directory.ErrIncompleteScan
		}
		if _, ok := emails[email]; ok {
			return directory.TestResult{}, directory.ErrIncompleteScan
		}
		ids[id], logins[login], emails[email] = struct{}{}, struct{}{}, struct{}{}
	}
	message := "Проверено пользователей: " + strconv.Itoa(len(result.Entries))
	if len(control.Cookie) != 0 {
		message = "Проверены первые " + strconv.Itoa(len(result.Entries)) + " записей; всего может быть больше"
	}
	if len(result.Entries) == 0 {
		message = "LDAP доступен, но фильтр не вернул пользователей. Синхронизация отключит ранее связанные учётные записи."
	}
	return directory.TestResult{Reachable: true, MatchedUsers: len(result.Entries), Message: message}, nil
}

func open(ctx context.Context, cfg directory.Config) (*ldap.Conn, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, unavailable(err)
	}
	u, _ := url.Parse(cfg.ServerURL) // Validate уже проверил URL.
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if cfg.CA != "" && !pool.AppendCertsFromPEM([]byte(cfg.CA)) {
		return nil, nil, directory.ErrInvalidConfig
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), RootCAs: pool}
	setupCtx, cancelSetup := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancelSetup()
	port := u.Port()
	if port == "" {
		if u.Scheme == "ldaps" {
			port = "636"
		} else {
			port = "389"
		}
	}
	raw, err := (&net.Dialer{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}).DialContext(setupCtx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return nil, nil, unavailable(err)
	}
	if u.Scheme == "ldaps" {
		secured := tls.Client(raw, tlsConfig)
		if err := secured.HandshakeContext(setupCtx); err != nil {
			raw.Close()
			return nil, nil, unavailable(err)
		}
		raw = secured
	}
	conn := ldap.NewConn(raw, u.Scheme == "ldaps")
	conn.Start()
	conn.SetTimeout(time.Duration(cfg.TimeoutSeconds) * time.Second)
	if u.Scheme == "ldap" {
		// StartTLS может зависнуть на ответе сервера или TLS handshake. Закрываем
		// именно transport: Conn.Close может ждать mutex самого StartTLS.
		interrupted := make(chan struct{})
		stopInterrupt := context.AfterFunc(setupCtx, func() {
			_ = raw.Close()
			close(interrupted)
		})
		err := conn.StartTLS(tlsConfig)
		if !stopInterrupt() {
			// Отмена уже запустила callback; ждём закрытия transport и не
			// возвращаем соединение, которое callback может закрыть позже.
			<-interrupted
			conn.Close()
			return nil, nil, unavailable(setupCtx.Err())
		}
		if err != nil {
			conn.Close()
			return nil, nil, unavailable(err)
		}
	}
	if err := setupCtx.Err(); err != nil {
		conn.Close()
		return nil, nil, unavailable(err)
	}
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stopped:
		}
	}()
	return conn, func() { close(stopped); conn.Close() }, nil
}

func requireObject(conn *ldap.Conn, cfg directory.Config, dn string) error {
	request := ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, cfg.TimeoutSeconds, false, "(objectClass=*)", []string{"1.1"}, nil)
	result, err := conn.Search(request)
	if err != nil || result == nil || result.ResultCode != 0 || len(result.Referrals) != 0 || len(result.Entries) != 1 || !sameDN(result.Entries[0].DN, dn) {
		return directory.ErrIncompleteScan
	}
	return nil
}

func attributes(cfg directory.Config) []string {
	attrs := []string{cfg.IDAttribute, cfg.LoginAttribute, cfg.EmailAttribute, cfg.NameAttribute}
	if cfg.Mode == directory.ModeAD {
		attrs = append(attrs, "userAccountControl", "accountExpires")
	}
	return attrs
}

func memberFilter(cfg directory.Config) string {
	if cfg.Mode == directory.ModeAD && cfg.NestedGroups {
		return "(" + cfg.GroupMembershipAttribute + ":1.2.840.113556.1.4.1941:=" + ldap.EscapeFilter(cfg.GroupDN) + ")"
	}
	return "(" + cfg.GroupMembershipAttribute + "=" + ldap.EscapeFilter(cfg.GroupDN) + ")"
}

func loginFilter(cfg directory.Config, login string) string {
	return identityFilter(cfg, cfg.LoginAttribute, login, false)
}

func identityFilter(cfg directory.Config, attribute, identifier string, requireMembership bool) string {
	membership := ""
	if requireMembership {
		membership = memberFilter(cfg)
	}
	return "(&" + cfg.UserFilter + "(" + attribute + "=" + ldap.EscapeFilter(identifier) + ")" + membership + ")"
}

func sameDN(a, b string) bool {
	first, err := ldap.ParseDN(a)
	if err != nil {
		return false
	}
	second, err := ldap.ParseDN(b)
	return err == nil && first.EqualFold(second)
}

func parseEntry(cfg directory.Config, raw *ldap.Entry) (directory.Entry, error) {
	if raw == nil || raw.DN == "" {
		return directory.Entry{}, directory.ErrIncompleteScan
	}
	var id string
	if cfg.Mode == directory.ModeAD && strings.EqualFold(cfg.IDAttribute, "objectGUID") {
		value := raw.GetEqualFoldRawAttributeValue(cfg.IDAttribute)
		if len(value) != 16 {
			return directory.Entry{}, directory.ErrIncompleteScan
		}
		id = fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%x", binary.LittleEndian.Uint32(value[0:4]), binary.LittleEndian.Uint16(value[4:6]), binary.LittleEndian.Uint16(value[6:8]), value[8], value[9], value[10:16])
	} else {
		id = strings.TrimSpace(raw.GetEqualFoldAttributeValue(cfg.IDAttribute))
		if strings.EqualFold(cfg.IDAttribute, "entryUUID") {
			parsed, err := uuid.Parse(id)
			if err != nil {
				return directory.Entry{}, directory.ErrIncompleteScan
			}
			id = parsed.String()
		}
	}
	entry := directory.Entry{ExternalID: id, Login: strings.TrimSpace(raw.GetEqualFoldAttributeValue(cfg.LoginAttribute)), Email: strings.TrimSpace(raw.GetEqualFoldAttributeValue(cfg.EmailAttribute)), DisplayName: strings.TrimSpace(raw.GetEqualFoldAttributeValue(cfg.NameAttribute))}
	if !validEntry(entry) {
		return directory.Entry{}, directory.ErrIncompleteScan
	}
	if cfg.Mode == directory.ModeAD {
		uac := raw.GetEqualFoldAttributeValue("userAccountControl")
		flags, err := strconv.ParseUint(uac, 10, 32)
		if err != nil {
			return directory.Entry{}, directory.ErrIncompleteScan
		}
		entry.Disabled = flags&0x2 != 0
		expires := raw.GetEqualFoldAttributeValue("accountExpires")
		if expires == "" {
			return directory.Entry{}, directory.ErrIncompleteScan
		}
		if expires != "0" && expires != "9223372036854775807" {
			filetime, err := strconv.ParseInt(expires, 10, 64)
			if err != nil {
				return directory.Entry{}, directory.ErrIncompleteScan
			}
			if filetime <= time.Now().UTC().UnixNano()/100+116444736000000000 {
				entry.Disabled = true
			}
		}
	}
	return entry, nil
}

func validEntry(e directory.Entry) bool {
	for _, value := range []string{e.ExternalID, e.Login, e.Email, e.DisplayName} {
		if value == "" || len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n\x00") {
			return false
		}
	}
	addr, err := mail.ParseAddress(e.Email)
	return err == nil && addr.Address == e.Email && !strings.ContainsAny(e.Email, " \t") && !bytes.ContainsRune([]byte(e.ExternalID), 0)
}

func unavailable(_ error) error { return directory.ErrUnavailable }
func incomplete(_ error) error  { return directory.ErrIncompleteScan }
