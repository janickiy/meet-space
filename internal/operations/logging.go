package operations

import (
	"log/slog"
	"os"
	"strings"
)

// LogWriter адаптирует устаревшие выходные данные log.Logger под формат JSON, используемый
// сервисом.  Он не сохраняет сообщения и удаляет из них секретные значения окружения перед
// выводом.
type LogWriter struct{}

// Функция Write принимает выходные байты через контракт io.Writer, соблюдая правила буферизации
// компонента.
//
// @Parameters:
//   - p: Входные байты, потребляемые этой операцией.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
