package app

import (
	"context"
	"encoding/json"
	"time"

	content "github.com/janickiy/meet-space/internal/domain/content"
	integrations "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/operations"
)

// measuredEmail counts delivery attempts around a replaceable adapter without copying email
// content.
type measuredEmail struct{ integrations.EmailProvider }

// Send доставляет ограниченное электронное письмо со стабильным идентификатором сообщения; настроенный транспорт // определяет возможность повторной дедупликации.
//
// Параметры:
//   - ctx: контекст, управляющий отменой, сроками и временем жизни операции.
//   - message: ограниченное содержимое электронного письма, получатель и стабильный идентификатор доставки.
//
// Возвращаемое значение:
//   - err: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения.
func (p measuredEmail) Send(ctx context.Context, message integrations.EmailMessage) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("email", time.Since(started), err) }()
	return p.EmailProvider.Send(ctx, message)
}

// measuredPush measures platform delivery calls without placing device tokens in metrics.
type measuredPush struct{ integrations.PushProvider }

// Send отправляет минимальное уведомление на выбранное зарегистрированное устройство без расшифровки текста
// или секретов провайдера.
//
// Параметры:
//   - ctx: контекст, управляющий отменой, сроками и временем выполнения операции.
//   - message: минимальное push-уведомление и зарегистрированное устройство, которому оно предназначено.
//
// Возвращаемое значение:
//   - err: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения данных.
func (p measuredPush) Send(ctx context.Context, message integrations.PushMessage) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("push", time.Since(started), err) }()
	return p.PushProvider.Send(ctx, message)
}

// measuredCalendar preserves the provider-independent contract and a fixed set of technical
// metric labels.
type measuredCalendar struct{ integrations.CalendarProvider }

// CreateEvent создает идемпотентное календарное событие с помощью настроенного провайдера.
//
// Параметры:
//   - ctx: контекст, управляющий отменой, сроками и временем жизни операции.
//   - credentials: учетные данные провайдера только для сервера; никогда не раскрывайте их в логах или общедоступных
//     ответах.
//   - event: доверенное событие для публикации или обработки.
//
// Возвращаемые значения:
//   - out: результирующее календарное событие.
//   - err: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.
func (p measuredCalendar) CreateEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.CreateEvent(ctx, credentials, event)
}

// UpdateEvent обновляет сопоставленное внешнее календарное событие для текущего расписания конференции.
//
// Параметры:
//   - ctx: контекст, управляющий отменой, сроками и временем жизни операции.
//   - credentials: учетные данные провайдера, доступные только на сервере; никогда не раскрывайте их в логах или общедоступных
//     ответах.
//   - event: доверенное событие для публикации или обработки.
//
// Возвращаемые значения:
//   - out: результирующее календарное событие.
//   - err: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.
func (p measuredCalendar) UpdateEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.UpdateEvent(ctx, credentials, event)
}

// CancelEvent cancels the external calendar event associated with the saved mapping.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - credentials: Server-only provider credentials; never expose them in logs or public
//     responses.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredCalendar) CancelEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.CancelEvent(ctx, credentials, event)
}

// GetEvent считывает сопоставленное внешнее календарное событие для сверки.
//
// Параметры:
//   - ctx: контекст, управляющий отменой, сроками и временем жизни операции.
//   - credentials: учетные данные провайдера, доступные только на сервере; никогда не раскрывайте их в логах или общедоступных
//     ответах.
//   - event: доверенное событие для публикации или обработки.
//
// Возвращаемые значения:
//   - out: результирующее календарное событие.
//   - err: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.
func (p measuredCalendar) GetEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.GetEvent(ctx, credentials, event)
}

// measuredSTT measures the audio request while preserving streaming and the adapter's provider
// name.
type measuredSTT struct{ content.TranscriptionProvider }

// Transcribe отправляет ограниченную аудиозапись в настроенный сервис расшифровки.
//
// Параметры:
//   - ctx: контекст, управляющий отменой, сроками и временем выполнения операции.
//   - request: проверенный запрос приложения или входящие параметры операции.
//
// Возвращает:
//   - out: результат расшифровки.
//   - err: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения данных.
func (p measuredSTT) Transcribe(ctx context.Context, request content.TranscriptionRequest) (out content.TranscriptionResult, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("stt", time.Since(started), err) }()
	return p.TranscriptionProvider.Transcribe(ctx, request)
}

// measuredAI measures each chunk or merge without changing model instructions or schema
// validation.
type measuredAI struct{ content.AIProvider }

// Summarize создает сводную информацию с ограничениями схемы на основе переданных входных данных.
//
// Параметры:
//   - ctx: контекст, управляющий отменой, сроками и временем жизни операции.
//   - request: валидированный запрос приложения или входящие параметры операции.
//
// Возвращаемые значения:
//   - out: результирующее необработанное сообщение.
//   - err: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения.
func (p measuredAI) Summarize(ctx context.Context, request content.AIRequest) (out json.RawMessage, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("ai", time.Since(started), err) }()
	return p.AIProvider.Summarize(ctx, request)
}
