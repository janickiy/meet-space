package telemetry

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/admin"
)

// The caller registers Receive behind the administrative origin/session/CSRF
// guards. No public route may use this handler.
func NewAdminHandler(logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{enabled: true, admin: true, logger: logger, limiter: &adminReportLimiter{identities: map[string]reportWindow{}}}
}
func validAdminReport(report Report) bool {
	switch report.Route {
	case "/", "/users", "/administrators", "/conferences", "/chats", "/recordings", "/files", "/system", "/settings", "/audit", "/errors", "unknown":
	default:
		return false
	}
	report.Route = "unknown"
	return validReport(report)
}

type reportWindow struct {
	start time.Time
	count int
}
type adminReportLimiter struct {
	mu         sync.Mutex
	global     reportWindow
	identities map[string]reportWindow
}

func (l *adminReportLimiter) allow(c *gin.Context) bool {
	value, ok := c.Get("administrator")
	if !ok {
		return false
	}
	actor, ok := value.(admin.Administrator)
	if !ok || actor.ID == "" {
		return false
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.global.start) >= time.Minute {
		l.global = reportWindow{start: now}
	}
	if l.global.count >= 300 {
		return false
	}
	entry, found := l.identities[actor.ID]
	if !found || now.Sub(entry.start) >= time.Minute {
		for id, w := range l.identities {
			if now.Sub(w.start) >= time.Minute {
				delete(l.identities, id)
			}
		}
		if _, exists := l.identities[actor.ID]; !exists && len(l.identities) >= 256 {
			return false
		}
		entry = reportWindow{start: now}
	}
	if entry.count >= 30 {
		return false
	}
	entry.count++
	l.global.count++
	l.identities[actor.ID] = entry
	return true
}
