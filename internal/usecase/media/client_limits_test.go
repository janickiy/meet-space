package media

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/sdp/v3"
)

type limitsRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip implements the isolated test HTTP transport by invoking its configured request
// callback.
//
// Parameters:
//   - r: Incoming HTTP request.
//
// Returns:
//   - result 1: The constructed or loaded response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (f limitsRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// limitsCommand creates a media command fixture for encoded-SDP size-limit checks.
//
// Returns:
//   - result 1: The resulting command.
func limitsCommand() domain.Command {
	return domain.Command{
		RequestID: "11111111-1111-4111-8111-111111111111",
		Route: domain.Route{
			WorkerID: "media-worker-test", Endpoint: "http://media-worker:8090",
			LeaseID: "22222222-2222-4222-8222-222222222222",
		},
		MediaPeerID:   "33333333-3333-4333-8333-333333333333",
		NegotiationID: "44444444-4444-4444-8444-444444444444",
		Publications:  []domain.Publication{},
	}
}

// limitsResponse creates an isolated HTTP response fixture for body-boundary checks.
//
// Parameters:
//   - status: Lifecycle state or response status to record.
//   - body: Input content stream or serialized request body.
//
// Returns:
//   - result 1: The constructed or loaded response.
func limitsResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

// TestHTTPClientTransportsLargeEscapedSDP verifies HTTP client transports large escaped SDP to
// prevent regressions in authenticated media-room control.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestHTTPClientTransportsLargeEscapedSDP(t *testing.T) {
	// Extension attributes are legal SDP. Characters escaped by encoding/json
	// make the internal command larger than both historical request caps.
	offer := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n" +
		strings.Repeat("a=x-note:<>&\r\n", 6000) +
		"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\nc=IN IP4 0.0.0.0\r\na=mid:0\r\na=recvonly\r\na=rtpmap:111 opus/48000/2\r\n"
	var parsed sdp.SessionDescription
	if err := parsed.Unmarshal([]byte(offer)); err != nil {
		t.Fatalf("invalid SDP fixture: %v", err)
	}
	if len(offer) <= 49152 || len(offer) > domain.MaxSDPBytes {
		t.Fatalf("SDP fixture size %d does not exercise the new supported range", len(offer))
	}
	cmd := limitsCommand()
	cmd.SDP = offer
	called := false
	client := NewHTTPClient("test-internal-secret", time.Second)
	defer client.Close()
	client.client.Transport = limitsRoundTripper(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.Method != http.MethodPost || r.URL.String() != cmd.Route.Endpoint+"/internal/media/offer" {
			t.Fatalf("unexpected worker request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer test-internal-secret" || r.Header.Get("X-Request-ID") != cmd.RequestID {
			t.Fatal("internal authorization or request correlation was lost")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) <= 98304 || len(raw) > domain.MaxCommandBytes {
			t.Fatalf("serialized command size %d does not exercise the former limits", len(raw))
		}
		var received domain.Command
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Fatal(err)
		}
		if received.SDP != offer || received.NegotiationID != cmd.NegotiationID || received.Publications == nil {
			t.Fatal("worker received changed SDP, negotiation identity, or publication declaration")
		}
		return limitsResponse(http.StatusOK, `{"mediaPeerId":"`+cmd.MediaPeerID+`"}`), nil
	})
	result, err := client.Call(context.Background(), "offer", cmd)
	if err != nil || !called || result.MediaPeerID != cmd.MediaPeerID {
		t.Fatalf("large SDP was not delivered: called=%v, result=%+v, err=%v", called, result, err)
	}
}

// TestHTTPClientCommandBodyBoundary verifies HTTP client command body boundary to prevent
// regressions in authenticated media-room control.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestHTTPClientCommandBodyBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int
		wantErr error
	}{
		{"exactly_one_MiB", 1 << 20, nil},
		{"one_byte_over", (1 << 20) + 1, domain.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := limitsCommand()
			cmd.Ticket = "x"
			raw, err := json.Marshal(cmd)
			if err != nil {
				t.Fatal(err)
			}
			cmd.Ticket = strings.Repeat("x", tc.size-len(raw)+1)
			raw, err = json.Marshal(cmd)
			if err != nil || len(raw) != tc.size {
				t.Fatalf("wrong fixture size %d, expected %d: %v", len(raw), tc.size, err)
			}
			calls := 0
			client := NewHTTPClient("test-internal-secret", time.Second)
			defer client.Close()
			client.client.Transport = limitsRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				received, err := io.ReadAll(r.Body)
				if err != nil || len(received) != tc.size {
					t.Fatalf("transport received %d bytes, expected %d: %v", len(received), tc.size, err)
				}
				return limitsResponse(http.StatusOK, `{}`), nil
			})
			_, err = client.Call(context.Background(), "join", cmd)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			wantCalls := 1
			if tc.wantErr != nil {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("transport called %d times, expected %d", calls, wantCalls)
			}
		})
	}
}

// TestHTTPClientResponseBodyBoundary verifies HTTP client response body boundary to prevent
// regressions in authenticated media-room control.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestHTTPClientResponseBodyBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		size    int
		wantErr error
	}{
		{"exactly_one_MiB", http.StatusOK, 1 << 20, nil},
		{"one_byte_over", http.StatusOK, (1 << 20) + 1, domain.ErrUnavailable},
		{"oversized_error_body", http.StatusUnauthorized, (1 << 20) + 1, domain.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Even a valid JSON prefix followed only by whitespace must not hide
			// overflow, which would happen with a silently truncated LimitReader.
			prefix := `{"mediaPeerId":"33333333-3333-4333-8333-333333333333"}`
			body := prefix + strings.Repeat(" ", tc.size-len(prefix))
			client := NewHTTPClient("test-internal-secret", time.Second)
			defer client.Close()
			client.client.Transport = limitsRoundTripper(func(*http.Request) (*http.Response, error) {
				return limitsResponse(tc.status, body), nil
			})
			result, err := client.Call(context.Background(), "offer", limitsCommand())
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
			if err == nil && result.MediaPeerID != limitsCommand().MediaPeerID {
				t.Fatal("the complete response at the allowed boundary was not decoded")
			}
		})
	}
}
