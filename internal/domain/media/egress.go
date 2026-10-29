// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package media

// EgressRequest описывает внутренний запрос на запись потока, защищённый авторизацией и арендой
// права владения медиакомнатой.
type EgressRequest struct {
	RequestID          string `json:"requestId"`
	ConferenceID       string `json:"conferenceId"`
	RecordingID        string `json:"recordingId"`
	Route              Route  `json:"route"`
	SegmentDurationSec int    `json:"segmentDurationSec"`
}

// EgressTrack содержит данные о выходе из системы, используемые для управления медиакомнатой с
// аутентификацией.
type EgressTrack struct {
	Track
	MimeType    string `json:"mimeType"`
	ClockRate   uint32 `json:"clockRate"`
	Channels    uint16 `json:"channels"`
	PayloadType uint8  `json:"payloadType"`
	SSRC        uint32 `json:"ssrc"`
}

// EgressFrame содержит закодированный пакет RTP и временную метку захвата по рабочим часам.
type EgressFrame struct {
	Type       string       `json:"type"`
	Sequence   uint64       `json:"sequence"`
	CapturedAt int64        `json:"capturedAt"`
	Track      *EgressTrack `json:"track,omitempty"`
	TrackID    string       `json:"trackId,omitempty"`
	RTP        []byte       `json:"rtp,omitempty"`
	Code       string       `json:"code,omitempty"`
}

// EgressSubscription определяет заменяемый контракт зависимости для управления медиакомнатой с
// аутентификацией.
type EgressSubscription interface {
	// Frames возвращает канал подписки, по которому передаются закодированные медиафреймы.
	//
	// @returns:
	//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
	Frames() <-chan EgressFrame
	// Done возвращает канал, который закрывается при завершении собственного процесса или
	// подписки.
	//
	// @returns:
	//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
	Done() <-chan struct{}
	// Err возвращает ошибку терминальной подписки, если таковая была зафиксирована.
	//
	// @returns:
	//   - result1: Ошибка завершения собственного процесса или подписки, либо nil после
	//   нормального завершения.
	Err() error
	// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
	Close()
	// Keyframes запрашивает видеокадр из подписки на запись.
	Keyframes()
	// Ping обновляет информацию о доступности текущей подписки на запись.
	Ping()
}
