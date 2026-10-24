// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package mediaworker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/janickiy/meet-space/internal/domain/media"
)

// serviceCommand выполняет доверенную команду модерации или закрытия комнаты после проверки
// подлинности.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - r: Входящий запрос HTTP.
//   - operation: Фиксированный провайдер из списка разрешенных или операция протокола.
//   - cmd: Проверенная внутренняя команда для работы с медиа.
func (h *Handler) serviceCommand(w http.ResponseWriter, r *http.Request, operation string, cmd media.Command) {
	if !validUUID(cmd.ConferenceID) || !validUUID(cmd.Route.LeaseID) || cmd.Route.WorkerID != h.cfg.WorkerID || cmd.Route.Endpoint != h.cfg.WorkerInternalURL || cmd.MediaPeerID != "" || cmd.SDP != "" || len(cmd.Candidate) > 0 || cmd.Ticket != "" {
		h.fail(w, media.ErrInvalid)
		return
	}
	if !h.ready.Load() {
		h.fail(w, media.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.OperationTimeout)
	defer cancel()
	owner, err := h.registry.GetOwner(ctx, cmd.ConferenceID)
	if err != nil || !sameRoute(owner, cmd.Route) {
		h.fail(w, media.ErrOwnership)
		return
	}
	unlock, err := h.lockRoom(ctx, cmd.ConferenceID)
	if err != nil {
		h.fail(w, err)
		return
	}
	defer unlock()
	switch operation {
	case "policy":
		if !validUUID(cmd.ParticipantID) || cmd.Policy == nil || cmd.Policy.Version < 0 {
			h.fail(w, media.ErrInvalid)
			return
		}
		engine, ok := h.engine.(interface {
			// SetPolicy обновляет политику для медиаворкера.
			//
			// @Parameters:
			//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность
			//   выполнения операции.
			//   - arg2: Идентификатор конференции, ограничивающий область действия
			//   операции.
			//   - arg3: Идентификатор участника конференции.
			//   - arg4: Эффективная политика в отношении медиаданных на стороне сервера.
			//
			// @returns:
			//   - result1: nil — в случае успеха; в противном случае — ошибка валидации,
			//   доступа, передачи данных или сохранения.
			SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
		})
		if !ok {
			h.fail(w, media.ErrUnavailable)
			return
		}
		err = engine.SetPolicy(ctx, cmd.ConferenceID, cmd.ParticipantID, *cmd.Policy)
	case "close":
		err = h.engine.CloseConference(ctx, cmd.ConferenceID)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.json(w, http.StatusOK, media.Result{})
}

// egress открывает защищенный поток закодированных медиаданных для записи, сохраняя при этом
// право собственности на текущую комнату.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - r: Входящий запрос HTTP.
func (h *Handler) egress(w http.ResponseWriter, r *http.Request) {
	var req media.EgressRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	raw, err := io.ReadAll(r.Body)
	if err != nil || strictJSON(raw, &req) != nil || !validUUID(req.RequestID) || r.Header.Get("X-Request-ID") != req.RequestID || !validUUID(req.ConferenceID) || !validUUID(req.RecordingID) || !validUUID(req.Route.LeaseID) || req.Route.WorkerID != h.cfg.WorkerID || req.Route.Endpoint != h.cfg.WorkerInternalURL || req.SegmentDurationSec < 1 || req.SegmentDurationSec > 30 {
		h.fail(w, media.ErrInvalid)
		return
	}
	engine, ok := h.engine.(interface {
		// SubscribeRecording открывает пассивную ограниченную подписку на закодированные
		// медиаданные SFU для записи.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
		//   - arg3: Внешняя запись UUID.
		//
		// @returns:
		//   - result1: Результирующая подписка egress.
		//   - result2: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		SubscribeRecording(context.Context, string, string) (media.EgressSubscription, error)
	})
	if req.Purpose != "" && req.Purpose != "recording" && req.Purpose != "captions" {
		h.fail(w, media.ErrInvalid)
		return
	}
	if !ok || !h.ready.Load() {
		h.fail(w, media.ErrUnavailable)
		return
	}
	setup, cancel := context.WithTimeout(r.Context(), h.cfg.OperationTimeout)
	defer cancel()
	unlock, err := h.lockRoom(setup, req.ConferenceID)
	if err != nil {
		h.fail(w, err)
		return
	}
	owner, err := h.registry.GetOwner(setup, req.ConferenceID)
	if err != nil || !sameRoute(owner, req.Route) {
		unlock()
		h.fail(w, media.ErrOwnership)
		return
	}
	renewedAt := time.Now()
	if h.registry.Renew(setup, req.ConferenceID, req.Route, h.cfg.OwnershipTTL) != nil {
		unlock()
		h.fail(w, media.ErrOwnership)
		return
	}
	h.mu.Lock()
	old, exists := h.leases[req.ConferenceID]
	h.mu.Unlock()
	if exists && !sameRoute(old.route, req.Route) {
		_ = h.engine.CloseConference(setup, req.ConferenceID)
	}
	h.mu.Lock()
	admitted := !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && renewedAt.Add(h.cfg.OwnershipTTL).After(time.Now())
	if admitted {
		h.leases[req.ConferenceID] = roomLease{route: req.Route, deadline: renewedAt.Add(h.cfg.OwnershipTTL)}
	}
	h.mu.Unlock()
	if !admitted {
		unlock()
		h.fail(w, media.ErrUnavailable)
		return
	}
	var sub media.EgressSubscription
	if req.Purpose == "captions" {
		if audio, available := h.engine.(interface {
			// SubscribeAudio открывает отдельную ограниченную подписку только на аудио, не
			// занимая слот подписки на запись.
			//
			// @Parameters:
			//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность
			//   выполнения операции.
			//   - arg2: Идентификатор конференции, ограничивающий область действия
			//   операции.
			//   - arg3: Постоянный или физический идентификатор сессии, определяемый
			//   операцией.
			//
			// @returns:
			//   - result1: Результирующая подписка egress.
			//   - result2: nil — в случае успеха; в противном случае — ошибка валидации,
			//   доступа, передачи данных или сохранения.
			SubscribeAudio(context.Context, string, string) (media.EgressSubscription, error)
		}); available {
			sub, err = audio.SubscribeAudio(setup, req.ConferenceID, req.RecordingID)
		} else {
			err = media.ErrUnavailable
		}
	} else {
		sub, err = engine.SubscribeRecording(setup, req.ConferenceID, req.RecordingID)
	}
	unlock()
	if err != nil {
		h.fail(w, err)
		return
	}
	defer sub.Close()
	if !h.leaseValid(req.ConferenceID, req.Route) {
		h.fail(w, media.ErrOwnership)
		return
	}
	cancel()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Request-ID", req.RequestID)
	control := http.NewResponseController(w)
	encoder := json.NewEncoder(w)

	write := func(frame media.EgressFrame) bool {
		if err := control.SetWriteDeadline(time.Now().Add(h.cfg.OperationTimeout)); err != nil {
			return false
		}
		if encoder.Encode(frame) != nil {
			return false
		}
		return control.Flush() == nil
	}
	heartbeat := time.NewTicker(time.Second)
	defer heartbeat.Stop()
	keyframes := time.NewTicker(time.Duration(req.SegmentDurationSec) * time.Second)
	defer keyframes.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub.Done():
			if sub.Err() != nil {
				_ = write(media.EgressFrame{Type: "error", Code: "recording_egress_failed", CapturedAt: time.Now().UnixNano()})
			}
			return
		case frame := <-sub.Frames():
			if !h.leaseValid(req.ConferenceID, req.Route) {
				return
			}
			if !write(frame) {
				return
			}
		case <-heartbeat.C:
			if !h.leaseValid(req.ConferenceID, req.Route) {
				return
			}
			sub.Ping()
		case <-keyframes.C:
			if req.Purpose != "captions" {
				sub.Keyframes()
			}
		}
	}
}
