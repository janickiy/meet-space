package config

import (
	"crypto/hmac"
	"crypto/sha1" // Coturn REST compatibility; this is not used for password hashing.
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

// TURNConfig contains the shared server secret, TURN endpoints and temporary credential
// lifetime. ForceRelay affects only new browser connections. Secret is never serialized into
// HTTP responses or logs.
type TURNConfig struct {
	URLs       []string
	Secret     string `json:"-"`
	TTL        time.Duration
	ForceRelay bool
}

// LoadTURN loads and validates the TURN endpoints, shared secret and temporary credential
// lifetime.
//
// Returns:
//   - result 1: The resulting turn config.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// ClientICE builds browser ICE settings, including temporary TURN credentials when TURN is
// enabled.
//
// Parameters:
//   - base: Base configuration or value from which the result is derived.
//   - now: Reference time used for expiration or duration calculations.
//
// Returns:
//   - result 1: The resulting ICE config.
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
