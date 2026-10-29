package config

import "testing"

func TestRemovedSpeechConfigurationDoesNotAffectIntegrations(t *testing.T) {
	for _, key := range []string{"STT_PROVIDER_MODE", "AI_PROVIDER_MODE", "AI_PROVIDER_MODEL", "TRANSCRIPTION_ENABLED", "AI_SUMMARY_ENABLED", "LIVE_STT_ENABLED", "LIVE_STT_PROVIDER_MODE", "EMBEDDINGS_ENABLED", "MEETING_ANALYTICS_ENABLED", "TRANSCRIPTION_MAX_DURATION_SEC", "AI_SUMMARY_TIMEOUT", "PRODUCT_TEMP_PATH", "PRODUCT_MAX_REPROCESS"} {
		t.Setenv(key, "removed-invalid-value")
	}
	for _, key := range []string{"EMAIL_PROVIDER_MODE", "PUSH_PROVIDER_MODE", "CALENDAR_PROVIDER_MODE"} {
		t.Setenv(key, "noop")
	}
	cfg, err := LoadStageSeven(true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email.Mode != "noop" || cfg.Push.Mode != "noop" || cfg.Calendar.Mode != "noop" || cfg.MaxAttempts < 1 {
		t.Fatalf("ordinary integrations changed: %+v", cfg)
	}
}
