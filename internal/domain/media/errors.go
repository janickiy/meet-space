package media

import "errors"

// Code is the stable machine-readable media protocol error, never a human message.
type Code string

const (
	ErrUnavailable    Code = "media_unavailable"
	ErrInvalid        Code = "invalid_media_signal"
	ErrUnauthorized   Code = "media_unauthorized"
	ErrOwnership      Code = "media_ownership_lost"
	ErrLimit          Code = "media_limit_exceeded"
	ErrPeerNotFound   Code = "media_peer_not_found"
	ErrNegotiation    Code = "media_negotiation_conflict"
	ErrScreenConflict Code = "screen_sharing_conflict"
	ErrPolicy         Code = "media_policy_blocked"
)

// Error returns the error text required by the error interface.
//
// Returns:
//   - result 1: Error text exposed by this error implementation.
func (c Code) Error() string { return string(c) }

var protocolErrors = [...]Code{ErrUnavailable, ErrInvalid, ErrUnauthorized, ErrOwnership, ErrLimit, ErrPeerNotFound, ErrNegotiation, ErrScreenConflict, ErrPolicy}

// ErrorCode is shared by the internal HTTP and public WS error envelopes.
//
// Parameters:
//   - err: Error to classify, wrap or report safely.
//
// Returns:
//   - result 1: Bounded internal media failure code.
func ErrorCode(err error) string {
	for _, code := range protocolErrors {
		if errors.Is(err, code) {
			return string(code)
		}
	}
	return string(ErrUnavailable)
}

// ErrorFromCode accepts only protocol codes; unknown details never escape.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: The matching protocol error, or the safe unavailable error for an unknown code.
func ErrorFromCode(value string) error {
	for _, code := range protocolErrors {
		if value == string(code) {
			return code
		}
	}
	return ErrUnavailable
}
