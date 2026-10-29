package config

import "testing"

func TestRecordWorkerDefaultsAndExplicitURL(t *testing.T) {
	// Load only synthetic environment values, never an operator's local .env.
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "local")
	t.Setenv("WORKER_INTERNAL_URL", "")
	t.Setenv("WORKER_ID", "")
	for _, key := range []string{"EMAIL_PROVIDER_MODE", "PUSH_PROVIDER_MODE", "CALENDAR_PROVIDER_MODE"} {
		t.Setenv(key, "noop")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerInternalURL != "http://record-worker:8090" || cfg.WorkerID != "record-worker-local" {
		t.Fatalf("unexpected record-worker defaults: URL=%q ID=%q", cfg.WorkerInternalURL, cfg.WorkerID)
	}
	t.Setenv("WORKER_INTERNAL_URL", "http://custom-recorder:8090")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerInternalURL != "http://custom-recorder:8090" {
		t.Fatalf("explicit WORKER_INTERNAL_URL was not preserved: %q", cfg.WorkerInternalURL)
	}
}
