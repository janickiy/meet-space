// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package operations

import (
	"encoding/json"
	"net/http"
)

// ConfigureDrain регистрирует принадлежащие процессу хуки graceful-drain, используемые при
// развертывании.
//
// @Parameters:
//   - begin: Функция обратного вызова, которая останавливает прием новых задач от компонентов.
//   - active: Функция обратного вызова, возвращающая количество активных операций, которые все
//     еще выполняются во время процесса graceful-drain.
func (r *Runtime) ConfigureDrain(begin func(), active func() int) {
	r.beginDrain, r.activeWork = begin, active
}

// DrainStatus возвращает текущее состояние процесса graceful-drain и количество активных задач.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - req: Входящий запрос HTTP.
func (r *Runtime) DrainStatus(w http.ResponseWriter, req *http.Request) {
	if !Authorized(req, r.Config.MetricsSecret) {
		http.NotFound(w, req)
		return
	}
	if req.Method == http.MethodPost {
		r.Drain()
		if r.beginDrain != nil {
			r.beginDrain()
		}
	}
	active := int(r.inflight.Load())
	if r.activeWork != nil {
		active += r.activeWork()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Draining bool `json:"draining"`
		Active   int  `json:"active"`
		Ready    bool `json:"ready"`
	}{r.draining.Load(), active, r.Ready()})
}
