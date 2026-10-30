package config

import (
	"strings"
	"testing"
)

func TestRecordWorkerDefaultsAndExplicitIdentity(t *testing.T) {
	// Load only synthetic environment values, never an operator's local .env.
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "local")
	t.Setenv("WORKER_ID", "")
	t.Setenv("WORKER_PORT", "")
	for _, key := range []string{"EMAIL_PROVIDER_MODE", "PUSH_PROVIDER_MODE", "CALENDAR_PROVIDER_MODE"} {
		t.Setenv(key, "noop")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerPort != 8090 || cfg.WorkerID != "record-worker-local" {
		t.Fatalf("unexpected record-worker defaults: port=%d ID=%q", cfg.WorkerPort, cfg.WorkerID)
	}
	t.Setenv("WORKER_ID", "custom-recorder")
	t.Setenv("WORKER_PORT", "8095")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerID != "custom-recorder" || cfg.WorkerPort != 8095 {
		t.Fatalf("explicit recorder identity was not preserved: port=%d ID=%q", cfg.WorkerPort, cfg.WorkerID)
	}
}

// TestRemovedRecorderSettingsAreIgnored allows existing environment files to be
// reused without opening obsolete WebRTC listeners or requiring an unused key.
func TestRemovedRecorderSettingsAreIgnored(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "local")
	for _, key := range []string{"WORKER_INTERNAL_URL", "WORKER_INTERNAL_SECRET", "WEBRTC_UDP_PORT", "WEBRTC_TCP_PORT", "WEBRTC_NAT_IPS"} {
		t.Setenv(key, "obsolete-and-unused")
	}
	for _, key := range []string{"EMAIL_PROVIDER_MODE", "PUSH_PROVIDER_MODE", "CALENDAR_PROVIDER_MODE"} {
		t.Setenv(key, "noop")
	}
	if _, err := Load(); err != nil {
		t.Fatalf("retired recorder settings should not prevent startup: %v", err)
	}
}

// TestProductionOperationsNeedOnlyMetricsSecret checks that removing the old
// recording API does not weaken authentication for health administration.
func TestProductionOperationsNeedOnlyMetricsSecret(t *testing.T) {
	t.Setenv("WORKER_INTERNAL_SECRET", "")
	t.Setenv("METRICS_SECRET", strings.Repeat("m", 32))
	if _, err := loadOperations(false); err != nil {
		t.Fatalf("production operations should not require a retired recorder key: %v", err)
	}
	t.Setenv("METRICS_SECRET", "")
	if _, err := loadOperations(false); err == nil {
		t.Fatal("production operations must still require a metrics secret")
	}
}

// TestProductionIgnoresRetiredRecorderSecret preserves upgrade compatibility
// while retaining distinct credentials for current production services.
func TestProductionIgnoresRetiredRecorderSecret(t *testing.T) {
	for i, key := range []string{"JWT_SECRET", "MEDIA_TICKET_SECRET", "MEDIA_INTERNAL_SECRET", "METRICS_SECRET", "TURN_SHARED_SECRET"} {
		t.Setenv(key, strings.Repeat(string(rune('a'+i)), 32))
	}
	t.Setenv("TURN_URLS", "turn:meet.example:3478?transport=udp")
	t.Setenv("GIN_MODE", "release")
	t.Setenv("WS_ALLOWED_ORIGINS", "https://meet.example")
	for _, key := range []string{"POSTGRES_USER", "POSTGRES_DB", "MINIO_ROOT_USER", "RABBIT_MQ_USER"} {
		t.Setenv(key, "meetspace")
	}
	t.Setenv("MEDIA_NAT_IPS", "192.0.2.10")
	for _, key := range []string{"POSTGRES_PASSWORD", "RABBIT_MQ_PASSWORD", "MINIO_ROOT_PASSWORD", "REDIS_PASSWORD"} {
		t.Setenv(key, strings.Repeat("synthetic", 4))
	}
	for _, retiredKey := range []string{"", "old-key"} {
		t.Setenv("WORKER_INTERNAL_SECRET", retiredKey)
		if err := validateProduction(Config{AppEnv: "production", RateLimitEnabled: true, MinIOPublicOrigin: "https://meet.example"}); err != nil {
			t.Fatalf("production validation must ignore an obsolete recorder key: %v", err)
		}
	}
}
