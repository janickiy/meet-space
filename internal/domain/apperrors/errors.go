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

// Error — ошибка привязывает категорию приложения к безопасному клиентскому сообщению. Kind — это
// категория, Message — общедоступный текст, а Cause — диагностическая ошибка, которую Error()
// не раскрывает.
type Error struct {
	Kind    error
	Message string
	Cause   error
}

// Error возвращает текст ошибки, требуемый интерфейсом обработки ошибок.
//
// @returns:
//   - result1: Текст ошибки, предоставляемый данной реализацией обработки ошибок.
func (e *Error) Error() string { return e.Message }

// Unwrap возвращает основную причину, чтобы errors.Is и errors.As могли ее проверить.
//
// @returns:
//   - result1: Обернутая диагностическая причина, сохраненная для проверки errors.Is и
//     errors.As.
func (e *Error) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Kind
}

// Is — сохраняет категорию приложения независимо от диагностической причины.
//
// @Parameters:
//   - target: Целевое состояние, учетная запись или декодированное значение в соответствии с
//     типом.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (e *Error) Is(target error) bool { return errors.Is(e.Kind, target) }

// Wrap — обертка сохраняет диагностическую причину, не раскрывая ее через Error().
//
// @Parameters:
//   - kind: Категория ошибки приложения, используемая для сопоставления errors.Is и
//     классификации при передаче.
//   - cause: Основная ошибка, из-за которой операция завершилась неудачно.
//   - message: Безопасное общедоступное сообщение об ошибке, в котором не указывается основная
//     диагностическая причина.
//
// @returns:
//   - result1: Сформированная ошибка приложения с безопасным общедоступным сообщением и
//     сохраненной категорией или причиной.
func Wrap(kind, cause error, message string) error {
	return &Error{Kind: kind, Message: message, Cause: cause}
}

// New создает ошибку приложения с категорией и безопасным общедоступным сообщением.
//
// @Parameters:
//   - kind: Категория ошибки приложения, используемая для сопоставления errors.Is и
//     классификации при передаче.
//   - message: Безопасное общедоступное сообщение об ошибке, в котором не указывается основная
//     диагностическая причина.
//
// @returns:
//   - result1: Сформированная ошибка приложения с безопасным общедоступным сообщением и
//     сохраненной категорией или причиной.
func New(kind error, message string) error { return &Error{Kind: kind, Message: message} }

// Category не зависит от кодов состояния при передаче и сообщений об ошибках, связанных с
// человеческим фактором.
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

// Classify — классификация только способствует выявлению явных ошибок в работе приложения. Необработанная
// ошибка, связанная с истечением срока действия/DB, остается в категории «Внутренняя» до тех
// пор, пока операция, в рамках которой она возникла, не классифицирует ее.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: Результирующая категория.
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
