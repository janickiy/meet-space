package apperrors

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnavailable  = errors.New("unavailable")
	ErrRateLimited  = errors.New("rate limited")
	ErrTimeout      = errors.New("timeout")
	ErrInternal     = errors.New("internal")
)

// Error binds an application category to a safe client message. Kind is the category, Message is
// public text and Cause is the diagnostic error that Error() does not disclose.
type Error struct {
	Kind    error
	Message string
	Cause   error
}

// Error returns the error text required by the error interface.
//
// Returns:
//   - result 1: Error text exposed by this error implementation.
func (e *Error) Error() string { return e.Message }

// Unwrap returns the underlying cause so errors.Is and errors.As can inspect it.
//
// Returns:
//   - result 1: The wrapped diagnostic cause, retained for errors.Is and errors.As inspection.
func (e *Error) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Kind
}

// Is preserves the application category independently of the diagnostic cause.
//
// Parameters:
//   - target: Target state, account or decoded value, as defined by its type.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (e *Error) Is(target error) bool { return errors.Is(e.Kind, target) }

// Wrap retains a diagnostic cause without exposing it through Error().
//
// Parameters:
//   - kind: Application error category used for errors.Is matching and transport classification.
//   - cause: Underlying error that caused the operation to fail.
//   - message: Safe public error message that excludes the underlying diagnostic cause.
//
// Returns:
//   - result 1: The constructed application error with its safe public message and preserved
//     category or cause.
func Wrap(kind, cause error, message string) error {
	return &Error{Kind: kind, Message: message, Cause: cause}
}

// New creates an application error with its category and safe public message.
//
// Parameters:
//   - kind: Application error category used for errors.Is matching and transport classification.
//   - message: Safe public error message that excludes the underlying diagnostic cause.
//
// Returns:
//   - result 1: The constructed application error with its safe public message and preserved
//     category or cause.
func New(kind error, message string) error { return &Error{Kind: kind, Message: message} }

// Category is independent of transport status codes and human error messages.
type Category uint8

const (
	Internal Category = iota
	Validation
	Unauthenticated
	Forbidden
	NotFound
	Conflict
	RateLimited
	Unavailable
	Timeout
)

// Classify only promotes explicit application errors. A raw deadline/DB failure
// stays Internal until its owning operation deliberately classifies it.
//
// Parameters:
//   - err: Error to classify, wrap or report safely.
//
// Returns:
//   - result 1: The resulting category.
func Classify(err error) Category {
	var applicationError *Error
	if errors.As(err, &applicationError) {
		err = applicationError.Kind
	}
	switch {
	case errors.Is(err, ErrInvalidInput):
		return Validation
	case errors.Is(err, ErrUnauthorized):
		return Unauthenticated
	case errors.Is(err, ErrForbidden):
		return Forbidden
	case errors.Is(err, ErrNotFound):
		return NotFound
	case errors.Is(err, ErrConflict):
		return Conflict
	case errors.Is(err, ErrRateLimited):
		return RateLimited
	case errors.Is(err, ErrUnavailable):
		return Unavailable
	case errors.Is(err, ErrTimeout):
		return Timeout
	default:
		return Internal
	}
}
