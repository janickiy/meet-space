package config

import "testing"

func TestAdminConfigurationOriginBoundary(t *testing.T) {
	t.Setenv("ADMIN_ENABLED", "true")
	t.Setenv("ADMIN_HTTP_PORT", "8082")
	t.Setenv("ADMIN_HTTPS_PORT", "8081")
	t.Setenv("ADMIN_PUBLIC_URL", "")
	cfg, err := LoadAdmin("https://meet.example", nil)
	if err != nil || cfg.PublicURL != "https://meet.example:8081" || cfg.HTTPPort != 8082 {
		t.Fatalf("configuration=%+v err=%v", cfg, err)
	}
	for _, bad := range []string{"http://meet.example:8081", "https://meet.example", "https://user:password@meet.example:8081", "https://meet.example:8081/path", "https://meet.example:8081?secret=1"} {
		t.Setenv("ADMIN_PUBLIC_URL", bad)
		if _, err := LoadAdmin("", nil); err == nil {
			t.Errorf("accepted unsafe origin %q", bad)
		}
	}
	t.Setenv("ADMIN_ENABLED", "false")
	cfg, err = LoadAdmin("", nil)
	if err != nil || cfg.Enabled {
		t.Fatalf("disabled=%+v %v", cfg, err)
	}
}
