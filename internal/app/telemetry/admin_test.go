package telemetry

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
	"github.com/janickiy/meet-space/internal/operations"
)

type eventProbe struct{ events []errorlogs.Event }

func (p *eventProbe) Record(e errorlogs.Event) bool { p.events = append(p.events, e); return true }
func TestAdminTelemetrySanitizedCaptureAndBoundedRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	probe := &eventProbe{}
	restore := operations.SetErrorRecorder(probe)
	defer restore()
	var out bytes.Buffer
	h := NewAdminHandler(slog.New(operations.NewErrorHandler(slog.NewJSONHandler(&out, nil))))
	actor := admin.Administrator{ID: uuid.NewString()}
	body := Report{Version: "test-release", Route: "/errors", Code: "react_render_error", Browser: "chromium", Stack: []Frame{{File: "index-abcdefgh.js", Line: 12, Column: 3}}}
	receive := func(payload []byte) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/v1/admin/errors/client", bytes.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("administrator", actor)
		h.Receive(c)
		c.Writer.WriteHeaderNow()
		return w.Code
	}
	raw, _ := json.Marshal(body)
	for i := 0; i < 30; i++ {
		if status := receive(raw); status != 202 {
			t.Fatalf("report %d status %d", i, status)
		}
	}
	if receive(raw) != 429 {
		t.Fatal("per-administrator rate limit not enforced")
	}
	if len(probe.events) != 30 {
		t.Fatal("unexpected persisted event count")
	}
	event := probe.events[0]
	if event.Service != "admin-frontend" || event.Code != "client.react_render_error" || event.Route != "/errors" || event.Release != "test-release" || len(event.Stack) != 1 {
		t.Fatalf("unexpected safe event: %+v", event)
	}
	if strings.Contains(out.String(), actor.ID) {
		t.Fatal("administrator identity entered log")
	}
	if receive([]byte(`{"version":"test","route":"/errors","code":"uncaught_error","browser":"other","stack":[],"message":"private-password-token"}`)) != 400 {
		t.Fatal("unknown raw error field accepted")
	}
	if len(probe.events) != 30 {
		t.Fatal("rejected event entered log")
	}
}
func TestAdminTelemetryRejectsConcretePathsAndRawStack(t *testing.T) {
	valid := Report{Version: "test-release", Route: "/settings", Code: "uncaught_error", Browser: "safari", Stack: []Frame{}}
	if !validAdminReport(valid) {
		t.Fatal("valid safe report rejected")
	}
	for _, route := range []string{"/errors?password=secret", "/users/secret-person", "https://private.invalid/", "/i/token"} {
		p := valid
		p.Route = route
		if validAdminReport(p) {
			t.Fatal("concrete path accepted")
		}
	}
	valid.Stack = []Frame{{File: "https://private.invalid/secret.js", Line: 1, Column: 1}}
	if validAdminReport(valid) {
		t.Fatal("raw stack asset accepted")
	}
}
func TestAdminReportLimiterHasFixedMemoryBound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &adminReportLimiter{identities: map[string]reportWindow{}}
	for i := 0; i < 256; i++ {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("administrator", admin.Administrator{ID: uuid.NewString()})
		if !limiter.allow(c) {
			t.Fatal("bounded identity rejected prematurely")
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("administrator", admin.Administrator{ID: uuid.NewString()})
	if limiter.allow(c) || len(limiter.identities) != 256 {
		t.Fatal("identity map exceeded bound")
	}
}
