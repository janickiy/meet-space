package providers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// OAuthConfig contains only trusted server endpoints and the calendar adapter's minimal scopes.
type OAuthConfig struct {
	AuthorizationURL, TokenURL, RevokeURL, ClientID, ClientSecret, RedirectURL string
	Scopes                                                                     []string
	Timeout                                                                    time.Duration
	AllowHTTP                                                                  bool
}

// OAuth implements server-side S256 PKCE code exchange, refresh and revocation without a
// provider SDK.
type OAuth struct {
	cfg    OAuthConfig
	client *http.Client
}

// NewOAuth constructs the o auth with the dependencies and limits required for bounded external
// integrations.
//
// Parameters:
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The constructed or loaded o auth.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewOAuth(cfg OAuthConfig) (*OAuth, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || len(cfg.Scopes) == 0 {
		return nil, errors.New("calendar OAuth client credentials and minimal scopes are required")
	}
	for _, value := range []string{cfg.AuthorizationURL, cfg.TokenURL, cfg.RevokeURL, cfg.RedirectURL} {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(cfg.AllowHTTP && u.Scheme == "http")) {
			return nil, errors.New("calendar OAuth endpoints must be trusted HTTPS URLs")
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout > 5*time.Minute {
		return nil, errors.New("OAuth timeout exceeds five minutes")
	}
	return &OAuth{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// AuthorizeURL builds the provider authorization redirect with one-time state and S256 PKCE
// challenge, without exposing a client secret.
//
// Parameters:
//   - state: Current state or validated state update.
//   - challenge: S256 PKCE challenge sent to the authorization provider.
//
// Returns:
//   - result 1: Provider authorization URL without a client secret.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (o *OAuth) AuthorizeURL(state, challenge string) (string, error) {
	u, err := url.Parse(o.cfg.AuthorizationURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", o.cfg.ClientID)
	q.Set("redirect_uri", o.cfg.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(o.cfg.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// tokenResponse restricts parsed OAuth JSON to the credential fields required by the calendar.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// request calls the fixed OAuth endpoint without exposing provider response bodies in errors.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - endpoint: Operator-configured service endpoint.
//   - form: Server-constructed OAuth form fields.
//   - response: Application response whose session or public metadata is being processed.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (o *OAuth) request(ctx context.Context, endpoint string, form url.Values, response *tokenResponse) error {
	form.Set("client_id", o.cfg.ClientID)
	form.Set("client_secret", o.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return jobs.Error{Code: "oauth_request"}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := o.client.Do(req)
	if err != nil {
		return jobs.Error{Code: "oauth_unavailable", Retryable: true}
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if err != nil {
		return jobs.Error{Code: "oauth_response", Retryable: true}
	}
	if len(b) > 64*1024 {
		return jobs.Error{Code: "oauth_response_size"}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		code := "oauth_http"
		if res.StatusCode == 429 {
			code = "oauth_rate_limited"
		}
		return jobs.Error{Code: code, Retryable: res.StatusCode == 429 || res.StatusCode >= 500}
	}
	if response != nil && (json.Unmarshal(b, response) != nil || response.AccessToken == "" || !strings.EqualFold(response.TokenType, "bearer") || response.ExpiresIn < 0 || response.ExpiresIn > 366*24*3600) {
		return jobs.Error{Code: "oauth_invalid_token"}
	}
	return nil
}

// credentials decodes and validates provider credentials without returning them in public
// responses.
//
// Parameters:
//   - r: Untrusted provider token response to validate and convert to private credentials.
//
// Returns:
//   - result 1: The resulting calendar credentials.
func credentials(r tokenResponse) d.CalendarCredentials {
	var at *time.Time
	if r.ExpiresIn > 0 {
		v := time.Now().UTC().Add(time.Duration(r.ExpiresIn) * time.Second)
		at = &v
	}
	return d.CalendarCredentials{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, ExpiresAt: at, Scopes: r.Scope}
}

// Exchange exchanges the one-time authorization code and PKCE verifier for provider credentials
// on the server.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - code: Bounded public status or failure code.
//   - verifier: Token verifier or private PKCE verifier, as defined by its type.
//
// Returns:
//   - result 1: The resulting calendar credentials.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (o *OAuth) Exchange(ctx context.Context, code, verifier string) (d.CalendarCredentials, error) {
	var r tokenResponse
	err := o.request(ctx, o.cfg.TokenURL, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {o.cfg.RedirectURL}}, &r)
	return credentials(r), err
}

// Refresh renews the short-lived access credential using the current provider or account refresh
// session.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - token: Signed credential or operation-ownership token to verify.
//
// Returns:
//   - result 1: The resulting calendar credentials.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (o *OAuth) Refresh(ctx context.Context, token string) (d.CalendarCredentials, error) {
	var r tokenResponse
	err := o.request(ctx, o.cfg.TokenURL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}}, &r)
	if r.RefreshToken == "" {
		r.RefreshToken = token
	}
	return credentials(r), err
}

// Revoke revokes the stored provider authorization without changing the account's local delivery
// preferences.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - token: Signed credential or operation-ownership token to verify.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (o *OAuth) Revoke(ctx context.Context, token string) error {
	return o.request(ctx, o.cfg.RevokeURL, url.Values{"token": {token}}, nil)
}
