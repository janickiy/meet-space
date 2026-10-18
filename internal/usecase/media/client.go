package media

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/media"
)

// HTTPClient performs internal media calls, never forwards credentials through redirects and
// never exposes SDP-bearing response bodies in errors.
type HTTPClient struct {
	secret string
	client *http.Client
}

// NewHTTPClient constructs the HTTP client with the dependencies and limits required for
// authenticated media-room control.
//
// Parameters:
//   - secret: Private key material or raw secret; it must not be logged or exposed.
//   - timeout: Maximum time allowed for this operation.
//
// Returns:
//   - result 1: The constructed or loaded HTTP client.
func NewHTTPClient(secret string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{secret: secret, client: &http.Client{Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, MaxIdleConns: 64, MaxIdleConnsPerHost: 32, IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: timeout},
	}}
}

// Call performs the authenticated internal HTTP operation against the selected media worker.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - action: Requested lifecycle or moderation action.
//   - command: Validated internal recording or media command.
//
// Returns:
//   - result 1: The resulting result.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *HTTPClient) Call(ctx context.Context, action string, command domain.Command) (domain.Result, error) {
	if config.ValidateMediaEndpoint(command.Route.Endpoint) != nil {
		return domain.Result{}, domain.ErrInvalid
	}
	switch action {
	case "join", "offer", "ready", "ice", "leave", "unpublish", "policy", "close":
	default:
		return domain.Result{}, domain.ErrInvalid
	}
	raw, err := json.Marshal(command)
	if err != nil || len(raw) > domain.MaxCommandBytes {
		return domain.Result{}, domain.ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(command.Route.Endpoint, "/")+"/internal/media/"+action, bytes.NewReader(raw))
	if err != nil {
		return domain.Result{}, domain.ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("X-Request-ID", command.RequestID)
	resp, err := c.client.Do(req)
	if err != nil {
		return domain.Result{}, apperrors.Wrap(domain.ErrUnavailable, err, domain.ErrorCode(domain.ErrUnavailable))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, domain.MaxCommandBytes+1))
	if err != nil {
		return domain.Result{}, apperrors.Wrap(domain.ErrUnavailable, err, domain.ErrorCode(domain.ErrUnavailable))
	}
	if len(body) > domain.MaxCommandBytes {
		return domain.Result{}, domain.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		code := failure.Code
		// Older workers used the error field for the same machine code.
		if code == "" {
			code = failure.Error
		}
		return domain.Result{}, domain.ErrorFromCode(code)
	}
	var result domain.Result
	if len(body) > 0 && json.Unmarshal(body, &result) != nil {
		return domain.Result{}, domain.ErrUnavailable
	}
	return result, nil
}

// Close closes the component-owned resources and ends their lifecycle.
func (c *HTTPClient) Close() { c.client.CloseIdleConnections() }
