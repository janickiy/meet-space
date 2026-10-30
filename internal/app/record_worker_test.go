package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/operations"
)

// TestRecordWorkerHasNoDirectRecordingRoutes verifies removed endpoints stay
// unavailable even when the caller supplies the operational bearer secret.
func TestRecordWorkerHasNoDirectRecordingRoutes(t *testing.T) {
	secret := strings.Repeat("x", 32)
	ops := operations.New("record-worker", "route-test", config.OperationsConfig{MetricsSecret: secret}, nil)
	handler := newRecordWorkerHandler(ops)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, suffix := range []string{"start", "stop", "webrtc/offer", "files"} {
			path := "/records/a2401653-24bc-447e-a663-8b525d799088/" + suffix
			request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer "+secret)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s %s returned %d, want 404", method, path, response.Code)
			}
		}
	}
	for _, path := range []string{"/health", "/health/live", "/version"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("operational route %s returned %d", path, response.Code)
		}
	}
}
