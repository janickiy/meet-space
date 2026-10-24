// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package config

import (
	"crypto/hmac"
	"crypto/sha1" // совместимость с Coturn REST; не используется для хеширования паролей.
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/pion/stun/v3"
)

// TURNConfig содержит общий секретный ключ сервера, конечные точки TURN и время действия
// временных учетных данных. ForceRelay влияет только на новые подключения через браузер.
// Секретный ключ никогда не сериализуется в ответах HTTP или в логах.
type TURNConfig struct {
	URLs       []string
	Secret     string `json:"-"`
	TTL        time.Duration
	ForceRelay bool
}

// LoadTURN загружает и проверяет конечные точки TURN, общий секретный ключ и временные учетные данные
// на время жизни.
//
// @returns:
//   - result1: результирующая конфигурация turn.
//   - result2: nil в случае успешного выполнения; в противном случае — ошибка при проверке, доступе, передаче или сохранении
//     данных.
func LoadTURN() (TURNConfig, error) {
	if v := strings.ToLower(os.Getenv("TURN_FORCE_RELAY")); v != "" && !strings.Contains("|true|false|1|0|yes|no|on|off|", "|"+v+"|") {
		return TURNConfig{}, fmt.Errorf("TURN_FORCE_RELAY must be boolean")
	}
	c := TURNConfig{URLs: envList("TURN_URLS"), Secret: os.Getenv("TURN_SHARED_SECRET"), TTL: envDuration("TURN_CREDENTIAL_TTL", 10*time.Minute), ForceRelay: envBool("TURN_FORCE_RELAY", false)}
	if len(c.URLs) == 0 {
		if c.ForceRelay {
			return c, fmt.Errorf("TURN_FORCE_RELAY requires TURN_URLS")
		}
		return c, nil
	}
	if len(c.URLs) > 8 || len(c.Secret) < 32 || c.TTL < time.Minute || c.TTL > time.Hour {
		return c, fmt.Errorf("invalid TURN URLs, shared secret or credential TTL")
	}
	for _, address := range c.URLs {
		if parsed, err := stun.ParseURI(address); err != nil || parsed.Port < 1 || parsed.Port > 65535 || parsed.Host == "" {
			return c, fmt.Errorf("invalid TURN address")
		}
		u, err := url.Parse(address)
		if err != nil || (u.Scheme != "turn" && u.Scheme != "turns") || u.Opaque == "" || len(address) > 512 || strings.ContainsAny(address, "\r\n\t @/#") {
			return c, fmt.Errorf("TURN_URLS must contain bounded turn/turns addresses without credentials")
		}
		q, e := url.ParseQuery(u.RawQuery)
		if e != nil || len(q) > 1 || (len(q) > 0 && (len(q["transport"]) != 1 || (q.Get("transport") != "udp" && q.Get("transport") != "tcp"))) {
			return c, fmt.Errorf("TURN transport must be udp or tcp")
		}
	}
	return c, nil
}

// ClientICE формирует настройки ICE для браузера, включая временные учетные данные TURN, если функция TURN включена.
//
// @Parameters:
//   - base: базовая конфигурация или значение, на основе которого формируется результат.
//   - now: эталонное время, используемое для расчета срока действия или продолжительности.
//
// Возвращаемое значение:
//   - result1: результирующая конфигурация ICE.
func (c TURNConfig) ClientICE(base realtime.ICEConfig, now time.Time) realtime.ICEConfig {
	result := realtime.ICEConfig{ICEServers: append([]realtime.ICEServer{}, base.ICEServers...), ICETransportPolicy: "all"}
	for i := range result.ICEServers {
		result.ICEServers[i].URLs = append([]string{}, result.ICEServers[i].URLs...)
	}
	if len(c.URLs) == 0 {
		return result
	}
	expires := now.Add(c.TTL).Unix()
	username := strconv.FormatInt(expires, 10) + ":" + uuid.NewString()
	mac := hmac.New(sha1.New, []byte(c.Secret))
	_, _ = mac.Write([]byte(username))
	result.ICEServers = append(result.ICEServers, realtime.ICEServer{URLs: append([]string{}, c.URLs...), Username: username, Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil))})
	result.ExpiresAt = expires
	if c.ForceRelay {
		result.ICETransportPolicy = "relay"
	}
	return result
}
