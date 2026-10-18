package mediaworker

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/media"
)

// bodyLimitsCommand creates the media command fixture used to exercise protocol body boundaries.
//
// Returns:
//   - result 1: The resulting command.
func bodyLimitsCommand() media.Command {
	return media.Command{
		RequestID: "11111111-1111-4111-8111-111111111111",
		Binding: media.Binding{
			ConferenceID:  "22222222-2222-4222-8222-222222222222",
			ParticipantID: "33333333-3333-4333-8333-333333333333",
			SessionID:     "44444444-4444-4444-8444-444444444444",
			ConnectionID:  "55555555-5555-4555-8555-555555555555",
			UserID:        "66666666-6666-4666-8666-666666666666",
		},
		Route: media.Route{
			WorkerID: "another-worker", Endpoint: "http://another-worker:8090",
			LeaseID: "77777777-7777-4777-8777-777777777777",
		},
		Publications: []media.Publication{},
	}
}

// bodyLimitsHandler creates the isolated media-worker handler used by request-boundary tests.
//
// Returns:
//   - result 1: The resulting handler.
func bodyLimitsHandler() http.Handler {
	// A command addressed to another worker must reach the ownership guard
	// after JSON decoding, without requiring a real engine, Redis or ticket.
	return NewHandler(config.MediaConfig{
		WorkerID: "this-worker", WorkerInternalURL: "http://this-worker:8090",
		InternalSecret: "test-internal-secret",
	}, nil, nil, nil, nil, nil, nil).Routes()
}

// TestWorkerCommandBodyBoundary verifies worker command body boundary to prevent regressions in
// mediaworker.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestWorkerCommandBodyBoundary(t *testing.T) {
	cmd := bodyLimitsCommand()
	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		size   int
		status int
		code   media.Code
	}{
		{"above_former_64_KiB", (64 << 10) + 1, http.StatusConflict, media.ErrOwnership},
		{"one_byte_below_MiB", (1 << 20) - 1, http.StatusConflict, media.ErrOwnership},
		{"exactly_one_MiB", 1 << 20, http.StatusConflict, media.ErrOwnership},
		{"one_byte_over", (1 << 20) + 1, http.StatusBadRequest, media.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := append(append([]byte(nil), raw...), bytes.Repeat([]byte(" "), tc.size-len(raw))...)
			r := httptest.NewRequest(http.MethodPost, "/internal/media/join", bytes.NewReader(body))
			r.Header.Set("Authorization", "Bearer test-internal-secret")
			r.Header.Set("X-Request-ID", cmd.RequestID)
			w := httptest.NewRecorder()
			bodyLimitsHandler().ServeHTTP(w, r)
			var failure struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
				t.Fatalf("invalid worker error response: %v", err)
			}
			if w.Code != tc.status || failure.Code != string(tc.code) {
				t.Fatalf("%d-byte request returned HTTP %d / %q; expected HTTP %d / %q", tc.size, w.Code, failure.Code, tc.status, tc.code)
			}
			if tc.status == http.StatusConflict && w.Header().Get("X-Request-ID") != cmd.RequestID {
				t.Fatal("request was not decoded and correlated before the ownership guard")
			}
		})
	}
}

type bodyReadCounter struct {
	io.Reader
	reads int
}

// Read copies at most the destination buffer size from the owned stream while preserving its
// read and cancellation limits.
//
// Parameters:
//   - p: Destination byte buffer filled from the stream.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *bodyReadCounter) Read(p []byte) (int, error) { r.reads++; return r.Reader.Read(p) }

// Close closes the component-owned resources and ends their lifecycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (*bodyReadCounter) Close() error { return nil }

// TestWorkerOversizedBodyDoesNotBypassAuthentication verifies worker oversized body does not
// bypass authentication to prevent regressions in mediaworker.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestWorkerOversizedBodyDoesNotBypassAuthentication(t *testing.T) {
	for _, authorization := range []string{"", "Bearer wrong-secret", "test-internal-secret"} {
		t.Run(authorization, func(t *testing.T) {
			body := &bodyReadCounter{Reader: strings.NewReader(strings.Repeat(" ", (1<<20)+1))}
			r := httptest.NewRequest(http.MethodPost, "/internal/media/join", nil)
			r.Body = body
			r.ContentLength = (1 << 20) + 1
			r.Header.Set("Authorization", authorization)
			w := httptest.NewRecorder()
			bodyLimitsHandler().ServeHTTP(w, r)
			var failure struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusUnauthorized || failure.Code != string(media.ErrUnauthorized) {
				t.Fatalf("oversized unauthenticated request returned HTTP %d / %q", w.Code, failure.Code)
			}
			if body.reads != 0 {
				t.Fatalf("unauthenticated request body was read %d times", body.reads)
			}
		})
	}
}
