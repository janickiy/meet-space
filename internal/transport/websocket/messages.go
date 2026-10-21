// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	mediadomain "github.com/janickiy/meet-space/internal/domain/media"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/operations"
)

// handleMessage отвечает за диспетчеризацию протокола.  false завершает работу модуля чтения;
// при восстанавливаемых ошибках запроса используется существующая оболочка ошибки, а сокет
// продолжает работать.  Сроки подтверждения активности, ограничение частоты запросов и разрыв
// соединения остаются в модуле чтения.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (c *client) handleMessage(raw []byte) bool {
	event, code := decodeEnvelope(raw, c.session.ConferenceID)
	if code != "" {
		c.failure(event.ID, code)
		return true
	}
	if c.authorize() != nil {
		return false
	}
	operations.WSMessage(event.Type)
	if strings.HasPrefix(event.Type, "media.") {
		c.handleMedia(event)
		return true
	}
	signal, code := decodeSignal(event, c.session, c.handler.cfg)
	if code != "" {
		c.failure(event.ID, code)
		return true
	}
	forward := domain.Event(event.Type, c.session.ConferenceID, signal)
	forward.ReplyTo = event.ID
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := c.handler.hub.SendToConnection(ctx, c.session, signal.TargetConnectionID, forward)
	cancel()
	if err != nil {
		if errors.Is(err, apperrors.ErrForbidden) || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
			c.failure(event.ID, "target_unavailable")
			return true
		}
		c.Stop("broker_unavailable")
		return false
	}
	ack := domain.Event("ack", c.session.ConferenceID, map[string]string{"type": event.Type})
	ack.ReplyTo = event.ID
	c.Offer(ack)
	return true
}

// handleMedia отправляет медиакоманду, привязанную к аутентифицированному сеансу WebSocket.
//
// @Parameters:
//   - event: Доверенное событие для публикации или обработки.
func (c *client) handleMedia(event domain.Envelope) {
	if c.handler.media == nil {
		c.failure(event.ID, "media_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(operations.WithID(context.Background(), event.ID), 10*time.Second)
	err := c.handler.media.Handle(ctx, c.session, c.expiresAt, event)
	cancel()
	if err != nil {
		c.failure(event.ID, mediadomain.ErrorCode(err))
	}
}

// decodeEnvelope строго декодирует огибающую сигнала в реальном времени перед отправкой
// полезной нагрузки.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Результирующий конверт.
//   - result2: Код безопасной ошибки или пустая строка в случае успешного выполнения.
func decodeEnvelope(raw []byte, conferenceID string) (domain.Envelope, string) {
	var event domain.Envelope
	if !utf8.Valid(raw) || strictJSON(raw, &event) != nil {
		// Отбрасывает частично декодированный IDs, как это сделал считывающий модуль перед
		// извлечением.
		return domain.Envelope{}, "invalid_message"
	}
	if parsed, err := uuid.Parse(event.ID); err != nil || parsed == uuid.Nil || event.Version != 1 || event.ConferenceID != conferenceID || event.Timestamp.IsZero() || event.ReplyTo != "" {
		return event, "invalid_envelope"
	}
	return event, ""
}

// decodeSignal проверяет устаревшие сигналы, нормализует целевую систему и подставляет только
// аутентифицированный идентификатор физического сеанса.  Никаких побочных эффектов на
// транспортном уровне.
//
// @Parameters:
//   - event: Доверенное событие для публикации или обработки.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Результирующий сигнал.
//   - result2: Код безопасной ошибки при передаче сигнала или пустая строка в случае
//     успешного выполнения.
func decodeSignal(event domain.Envelope, session domain.Session, cfg config.RealtimeConfig) (domain.Signal, string) {
	if event.Type != "webrtc.offer" && event.Type != "webrtc.answer" && event.Type != "webrtc.ice" {
		return domain.Signal{}, "unsupported_event"
	}
	var signal domain.Signal
	if strictJSON(event.Data, &signal) != nil || signal.SenderConnectionID != "" || signal.SenderParticipantID != "" {
		return domain.Signal{}, "invalid_signal"
	}
	target, err := uuid.Parse(signal.TargetConnectionID)
	if err != nil || target == uuid.Nil || target.String() == session.ConnectionID {
		return domain.Signal{}, "invalid_target"
	}
	signal.TargetConnectionID = target.String()
	if event.Type == "webrtc.ice" {
		var candidate map[string]json.RawMessage
		if signal.SDP != "" || len(signal.Candidate) == 0 || len(signal.Candidate) > cfg.ICEBytes || json.Unmarshal(signal.Candidate, &candidate) != nil || candidate == nil {
			return domain.Signal{}, "invalid_ice"
		}
	} else if len(signal.Candidate) != 0 || len(signal.SDP) == 0 || len(signal.SDP) > cfg.SDPBytes {
		return domain.Signal{}, "invalid_sdp"
	}
	signal.SenderConnectionID = session.ConnectionID
	signal.SenderParticipantID = session.ParticipantID
	return signal, ""
}
