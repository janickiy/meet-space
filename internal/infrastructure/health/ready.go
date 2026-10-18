// Package health checks trusted internal dependency readiness without exposing addresses.
package health

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// HTTPReady calls the configured internal readiness endpoint without proxies or redirects.
type HTTPReady struct {
	url    string
	client *http.Client
}

// NewHTTPReady constructs the HTTP ready with the dependencies and limits required for
// dependency readiness.
//
// Parameters:
//   - baseURL: Operator-configured base URL of the dependency.
//
// Returns:
//   - result 1: The constructed or loaded HTTP ready.
func NewHTTPReady(baseURL string) *HTTPReady {
	return &HTTPReady{url: strings.TrimRight(baseURL, "/") + "/health/ready", client: &http.Client{
		Timeout:       1500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second},
	}}
}

// Ready reports whether the component or configured dependency is currently usable.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (h *HTTPReady) Ready(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
	if err != nil {
		return false
	}
	response, err := h.client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// Close closes the component-owned resources and ends their lifecycle.
func (h *HTTPReady) Close() { h.client.CloseIdleConnections() }
