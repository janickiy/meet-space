// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Package directory описывает безопасный контракт синхронизации и входа через LDAP.
package directory

import (
	"context"
	"errors"
	"fmt"
)

const (
	ModeAD      = "ad"
	ModeGeneric = "generic"
	// MaxUsers ограничивает полный снимок одинаково при чтении каталога и записи в БД.
	MaxUsers = 10000
)

var (
	ErrInvalidCredentials = errors.New("directory: invalid credentials")
	ErrNotAllowed         = errors.New("directory: user not allowed")
	ErrUnavailable        = errors.New("directory: unavailable")
	ErrInvalidConfig      = errors.New("directory: invalid configuration")
	ErrIncompleteScan     = errors.New("directory: incomplete scan")
)

// Config доступен только серверному коду. Пароль bind никогда не сериализуется в JSON.
type Config struct {
	Enabled                  bool   `json:"enabled"`
	ServerURL                string `json:"serverURL"`
	BaseDN                   string `json:"baseDN"`
	UsersDN                  string `json:"usersDN"`
	UserFilter               string `json:"userFilter"`
	LoginAttribute           string `json:"loginAttribute"`
	IDAttribute              string `json:"idAttribute"`
	EmailAttribute           string `json:"emailAttribute"`
	NameAttribute            string `json:"nameAttribute"`
	GroupDN                  string `json:"groupDN"`
	GroupMembershipAttribute string `json:"groupMembershipAttribute"`
	Mode                     string `json:"mode"`
	NestedGroups             bool   `json:"nestedGroups"`
	BindDN                   string `json:"bindDN"`
	BindPassword             string `json:"-"`
	CA                       string `json:"ca"`
	SyncIntervalMinutes      int    `json:"syncIntervalMinutes"`
	TimeoutSeconds           int    `json:"timeoutSeconds"`
	Revision                 int64  `json:"revision"`
}

// String и GoString скрывают bind пароль при стандартном логировании структуры.
func (c Config) String() string {
	return fmt.Sprintf("directory.Config{Enabled:%t,Mode:%q,Revision:%d,BindPassword:<redacted>}", c.Enabled, c.Mode, c.Revision)
}
func (c Config) GoString() string { return c.String() }

// Entry связывает запись каталога с неизменяемым внешним ID.
type Entry struct {
	ExternalID  string
	Login       string
	Email       string
	DisplayName string
	Disabled    bool
}

// TestResult не раскрывает учётные данные и содержимое каталога.
type TestResult struct {
	Reachable    bool   `json:"reachable"`
	MatchedUsers int    `json:"matchedUsers"`
	Message      string `json:"message"`
}

// LDAPApplyResult описывает применение полного снимка каталога к локальным учётным записям.
type LDAPApplyResult struct {
	Created         int      `json:"created"`
	Updated         int      `json:"updated"`
	Disabled        int      `json:"disabled"`
	DisabledUserIDs []string `json:"-"`
}

type Client interface {
	Authenticate(context.Context, Config, string, string) (Entry, error)
	Scan(context.Context, Config) ([]Entry, error)
	Test(context.Context, Config) (TestResult, error)
}
