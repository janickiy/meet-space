package operations

import (
	"log/slog"
	"os"
	"strings"
)

// LogWriter adapts legacy log.Logger output to the service's JSON format. It stores no messages
// and redacts secret environment values before output.
type LogWriter struct{}

// Write accepts output bytes through the io.Writer contract while enforcing the component's
// buffering rules.
//
// Parameters:
//   - p: Input bytes consumed by this operation.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (LogWriter) Write(p []byte) (int, error) {
	message := string(p)
	for _, key := range []string{"JWT_SECRET", "MEDIA_TICKET_SECRET", "MEDIA_INTERNAL_SECRET", "WORKER_INTERNAL_SECRET", "METRICS_SECRET", "TURN_SHARED_SECRET", "POSTGRES_PASSWORD", "REDIS_PASSWORD", "RABBIT_MQ_PASSWORD", "MINIO_ROOT_PASSWORD", "PROVIDER_TOKEN_ENCRYPTION_KEY", "EMAIL_PROVIDER_TOKEN", "PUSH_PROVIDER_TOKEN", "CALENDAR_PROVIDER_TOKEN", "STT_PROVIDER_TOKEN", "AI_PROVIDER_TOKEN", "CALENDAR_OAUTH_CLIENT_SECRET"} {
		if secret := os.Getenv(key); len(secret) >= 8 {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	if len(message) > 4096 {
		message = message[:4096] + " [truncated]"
	}
	slog.Info(strings.TrimSpace(message), "event_type", "component.log")
	return len(p), nil
}
