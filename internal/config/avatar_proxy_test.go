package config

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestAvatarProxyPolicy prevents a smaller proxy limit or CSP from silently
// breaking authenticated avatar uploads/previews. It reads templates, not .env.
func TestAvatarProxyPolicy(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "../..")
	for _, name := range []string{"frontend/nginx.conf.template", "dockers/production/routes.conf", "dockers/production/nginx.conf", "dockers/https/nginx.conf"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			start := strings.Index(text, "location ~ ^/api/v1/(conversations/[^/]+/avatar|auth/me/avatar)$ {")
			if start < 0 {
				t.Fatal("avatar-specific upload location missing")
			}
			end := strings.Index(text[start:], "}")
			if end < 0 || !strings.Contains(text[start:start+end], "client_max_body_size 3m;") {
				t.Fatal("proxy must admit the API's 2 MiB body without changing unrelated routes")
			}
			if name == "frontend/nginx.conf.template" {
				if !regexp.MustCompile(`img-src[^;]*\bblob:`).MatchString(text) {
					t.Fatal("authenticated raster object URLs blocked by CSP")
				}
				if !strings.Contains(text, "script-src 'self';") {
					t.Fatal("avatar support must not weaken script policy")
				}
			}
		})
	}
}
