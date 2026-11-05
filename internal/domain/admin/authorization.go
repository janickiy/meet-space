package admin

import "context"

// Authorization is a server-created capability. Only the digest is retained;
// the opaque cookie and CSRF token never enter repository/audit values.
type Authorization struct {
	AdministratorID string
	SessionHash     string
}
type authorizationKey struct{}

// WithAuthorization is called after the HTTP session and CSRF checks succeed.
func WithAuthorization(ctx context.Context, administratorID, sessionHash string) context.Context {
	return context.WithValue(ctx, authorizationKey{}, Authorization{AdministratorID: administratorID, SessionHash: sessionHash})
}
func AuthorizationFromContext(ctx context.Context) (Authorization, bool) {
	value, ok := ctx.Value(authorizationKey{}).(Authorization)
	return value, ok
}
