// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package operations

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// productKind сопоставляет тип задания продукта с ограниченной меткой, используемой для
// операционных показателей.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func productKind(kind string) bool {
	switch kind {
	case "integrations.conference", "integrations.event", "integrations.delivery", "integrations.invitation", "integrations.invitation_reminder", "integrations.calendar", "content.transcribe", "content.summarize", "content.embed", "analytics.aggregate":
		return true
	}
	return false
}

// Product записывает ограниченный результат категории фоновых заданий продукта.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - outcome: Фиксированная метка операционного результата.
//   - duration: Продолжительность или период действия, используемый в ходе операции.
func Product(kind, outcome string, duration time.Duration) {
	if !productKind(kind) {
		return
	}
	switch outcome {
	case "done", "failed", "skipped", "queued":
	default:
		return
	}
	if r := current.Load(); r != nil {
		r.productJobs.WithLabelValues(kind, outcome).Inc()
		r.productDuration.WithLabelValues(kind).Observe(duration.Seconds())
	}
}

// ProductQueue обновляет метрики глубины очереди для фиксированной категории заданий продукта.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - state: Текущее состояние или обновлённое валидированное состояние.
//   - count: Ожидаемое или максимальное количество элементов.
func ProductQueue(kind, state string, count int64) {
	if !productKind(kind) || (state != "queued" && state != "processing" && state != "failed") {
		return
	}
	if r := current.Load(); r != nil {
		r.productQueue.WithLabelValues(kind, state).Set(float64(count))
	}
}

// ProviderCall записывает результат вызова провайдера, используя фиксированные технические
// метки, а не приватные данные запроса.
//
// @Parameters:
//   - provider: Идентификатор провайдера, выбранный оператором, или фиксированная техническая
//     метка провайдера.
//   - duration: Продолжительность или период действия, используемый в ходе операции.
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
func ProviderCall(provider string, duration time.Duration, err error) {
	switch provider {
	case "email", "push", "calendar", "stt", "ai", "embedding", "live_stt":
	default:
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "retryable"
		var value jobs.Error
		var pointer *jobs.Error
		if errors.As(err, &pointer) && pointer != nil {
			value = *pointer
		} else {
			_ = errors.As(err, &value)
		}
		switch {
		case errors.Is(err, jobs.ErrSkip):
			outcome = "skipped"
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			outcome = "timeout"
		case value.Code == "provider_rate_limited":
			outcome = "rate_limited"
		case value.Code != "" && !value.Retryable:
			outcome = "permanent"
		}
	}
	if r := current.Load(); r != nil {
		r.providerCalls.WithLabelValues(provider, outcome).Inc()
		r.providerTime.WithLabelValues(provider).Observe(duration.Seconds())
	}
}

// Search записывает задержку при поиске и ограниченную метку успеха или неудачи без сохранения
// текста запроса.
//
// @Parameters:
//   - duration: Продолжительность или период действия, используемый в ходе операции.
//   - failed: Указывает, завершилась ли ограниченная операция с ошибкой.
func Search(duration time.Duration, failed bool) {
	outcome := "success"
	if failed {
		outcome = "failed"
	}
	if r := current.Load(); r != nil {
		r.searchRequests.WithLabelValues(outcome).Inc()
		r.searchTime.Observe(duration.Seconds())
	}
}
