package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type AdminConfig struct {
	Enabled        bool
	HTTPPort       int
	HTTPSPort      int
	PublicURL      string
	TrustedProxies []string
}

// LoadAdmin validates an independent HTTPS origin; request forwarding headers never determine it.
func LoadAdmin(publicURL string, trustedProxies []string) (AdminConfig, error) {
	c := AdminConfig{HTTPPort: 8082, HTTPSPort: 8081, TrustedProxies: trustedProxies}
	var err error
	if raw := os.Getenv("ADMIN_ENABLED"); raw != "" {
		c.Enabled, err = strconv.ParseBool(raw)
		if err != nil {
			return c, fmt.Errorf("ADMIN_ENABLED must be a boolean")
		}
	}
	if !c.Enabled {
		return c, nil
	}
	for key, target := range map[string]*int{"ADMIN_HTTP_PORT": &c.HTTPPort, "ADMIN_HTTPS_PORT": &c.HTTPSPort} {
		if raw := os.Getenv(key); raw != "" {
			*target, err = strconv.Atoi(raw)
			if err != nil || *target < 1 || *target > 65535 {
				return c, fmt.Errorf("%s must be a valid TCP port", key)
			}
		}
	}
	if c.HTTPSPort == 443 {
		return c, fmt.Errorf("ADMIN_HTTPS_PORT must differ from public HTTPS port 443")
	}
	configured := strings.TrimRight(os.Getenv("ADMIN_PUBLIC_URL"), "/")
	if configured == "" {
		base, parseErr := url.Parse(publicURL)
		if parseErr != nil || base.Hostname() == "" {
			return c, fmt.Errorf("ADMIN_PUBLIC_URL or PUBLIC_FRONTEND_URL is required when admin is enabled")
		}
		configured = "https://" + net.JoinHostPort(base.Hostname(), strconv.Itoa(c.HTTPSPort))
	}
	u, err := url.Parse(configured)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.Opaque != "" {
		return c, fmt.Errorf("ADMIN_PUBLIC_URL must be an HTTPS origin without credentials, path, query or fragment")
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	if port != strconv.Itoa(c.HTTPSPort) {
		return c, fmt.Errorf("ADMIN_PUBLIC_URL port must match ADMIN_HTTPS_PORT")
	}
	u.Host = strings.ToLower(u.Host)
	c.PublicURL = u.String()
	return c, nil
}
