// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Проверка работоспособности пакета проверяет готовность доверенных внутренних зависимостей без
// раскрытия адресов.
package health

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// HTTPReady вызывает настроенную внутреннюю конечную точку проверки готовности без
// использования прокси-серверов или редиректов.
type HTTPReady struct {
	url    string
	client *http.Client
}

// NewHTTPReady создает объект HTTP с учетом зависимостей и ограничений, необходимых для
// проверки готовности зависимостей.
//
// @Parameters:
//   - baseURL: Настроенная оператором база URL зависимости.
//
// @returns:
//   - result1: Созданная или загруженная база HTTP готова.
func NewHTTPReady(baseURL string) *HTTPReady {
	return &HTTPReady{url: strings.TrimRight(baseURL, "/") + "/health/ready", client: &http.Client{
		Timeout:       1500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second},
	}}
}

// Ready сообщает, можно ли в данный момент использовать компонент или настроенную зависимость.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
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

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
func (h *HTTPReady) Close() { h.client.CloseIdleConnections() }
