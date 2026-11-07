// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package directory

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		Mode: ModeGeneric, ServerURL: "ldaps://ldap.example.test:636",
		BaseDN: "dc=example,dc=test", UsersDN: "ou=people,dc=example,dc=test",
		UserFilter: "(objectClass=person)", LoginAttribute: "uid", IDAttribute: "entryUUID",
		EmailAttribute: "mail", NameAttribute: "cn", GroupDN: "cn=members,ou=groups,dc=example,dc=test",
		GroupMembershipAttribute: "memberOf", BindDN: "uid=reader,ou=people,dc=example,dc=test",
		BindPassword: "secret", SyncIntervalMinutes: 60, TimeoutSeconds: 10,
	}
}

func TestValidateRequiresEncryptedTransportAndSafeFilter(t *testing.T) {
	good := validConfig()
	if err := Validate(good); err != nil {
		t.Fatal(err)
	}
	good.ServerURL = "ldap://ldap.example.test:389"
	if err := Validate(good); err != nil {
		t.Fatal("StartTLS URL rejected", err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.ServerURL = "http://ldap.example.test" },
		func(c *Config) { c.ServerURL = "ldaps://reader:secret@ldap.example.test" },
		func(c *Config) { c.ServerURL = "ldaps://ldap.example.test/search" },
		func(c *Config) { c.LoginAttribute = "uid)(|(uid=*" },
		func(c *Config) { c.UserFilter = "(uid=*" },
		func(c *Config) { c.GroupDN = "not a DN" },
		func(c *Config) { c.BindPassword = "" },
		func(c *Config) { c.NestedGroups = true },
		func(c *Config) { c.CA = "not a PEM certificate" },
	} {
		c := validConfig()
		mutate(&c)
		if err := Validate(c); err == nil {
			t.Fatalf("unsafe config accepted: %+v", c)
		}
	}
}

func TestConfigDoesNotSerializeOrFormatBindPassword(t *testing.T) {
	cfg := validConfig()
	for _, formatted := range []string{fmt.Sprintf("%v", cfg), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg)} {
		if strings.Contains(formatted, cfg.BindPassword) {
			t.Fatal("bind password leaked through formatting")
		}
	}
	encoded, err := json.Marshal(cfg)
	if err != nil || strings.Contains(string(encoded), cfg.BindPassword) {
		t.Fatal("bind password leaked through JSON", err)
	}
}

// Базовый DN ограничивает области пользователей и группы по разобранной структуре, а не строковому суффиксу.
func TestValidateDirectorySearchScope(t *testing.T) {
	for _, tc := range []struct {
		name, base, users, group string
		valid                    bool
	}{
		{"descendants", "dc=example,dc=test", "ou=people,dc=example,dc=test", "cn=members,ou=groups,dc=example,dc=test", true},
		{"same base", "dc=example,dc=test", "DC=EXAMPLE,DC=TEST", "cn=members,ou=groups,dc=example,dc=test", true},
		{"case insensitive", "DC=Example,DC=Test", "OU=People,dc=EXAMPLE,dc=TEST", "CN=Members,OU=Groups,DC=Example,DC=Test", true},
		{"escaped comma", "ou=Sales\\, EMEA,dc=example,dc=test", "ou=people,ou=Sales\\, EMEA,dc=example,dc=test", "cn=members,ou=Sales\\, EMEA,dc=example,dc=test", true},
		{"users in another directory", "dc=example,dc=test", "ou=people,dc=other,dc=test", "cn=members,ou=groups,dc=example,dc=test", false},
		{"group in another directory", "dc=example,dc=test", "ou=people,dc=example,dc=test", "cn=members,ou=groups,dc=other,dc=test", false},
		{"users above base", "ou=people,dc=example,dc=test", "dc=example,dc=test", "cn=members,ou=people,dc=example,dc=test", false},
		{"escaped suffix is not ancestry", "dc=example,dc=test", "cn=people\\,dc=example\\,dc=test", "cn=members,dc=example,dc=test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.BaseDN, cfg.UsersDN, cfg.GroupDN = tc.base, tc.users, tc.group
			for _, validate := range []func(Config) error{Validate, ValidateDraft} {
				err := validate(cfg)
				if (err == nil) != tc.valid {
					t.Fatalf("scope valid=%v, error=%v", tc.valid, err)
				}
			}
		})
	}
}
