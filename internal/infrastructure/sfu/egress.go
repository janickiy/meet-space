package sfu

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/rtp"
)

var errEgressOverflow = errors.New("recording_egress_overflow")

// egress пассивно отслеживает закодированные пакеты.  При переполнении очереди подписка на
// запись закрывается без задержки при пересылке SFU RTP.
type egress struct {
	audioOnly bool
	manager   *Manager
	room      *room
	id        string
	mu        sync.Mutex
	frames    chan media.EgressFrame
	done      chan struct{}
	err       error
	closed    bool
	sequence  uint64
	tracks    map[string]bool
	closeOnce sync.Once
}

// SubscribeRecording открывает пассивную ограниченную подписку на закодированные медиаданные
// SFU для записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordingID: Внешняя запись UUID.
//
// @returns:
//   - result1: Результирующая подписка egress.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (m *Manager) SubscribeRecording(ctx context.Context, conferenceID, recordingID string) (media.EgressSubscription, error) {
	return m.subscribeEgress(ctx, conferenceID, recordingID, false)
}

// SubscribeAudio открывает отдельную ограниченную подписку только на аудио, не занимая слот
// подписки на запись.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - sessionID: Постоянный или физический идентификатор сессии, определяемый операцией.
//
// @returns:
//   - result1: Результирующая подписка egress.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (m *Manager) SubscribeAudio(ctx context.Context, conferenceID, sessionID string) (media.EgressSubscription, error) {
	return m.subscribeEgress(ctx, conferenceID, sessionID, true)
}

// subscribeEgress резервирует не более одной подписки на комнату для каждой цели и привязывает
// ее к текущему идентификатору записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordingID: Внешняя запись UUID.
//   - audioOnly: Ограничивается ли подписка только вспомогательными аудиокадрами.
//
// @returns:
//   - result1: Результирующая подписка egress.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (m *Manager) subscribeEgress(ctx context.Context, conferenceID, recordingID string, audioOnly bool) (media.EgressSubscription, error) {
	for _, id := range []string{conferenceID, recordingID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return nil, media.ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || m.roomClosures[conferenceID] != nil {
		return nil, media.ErrUnavailable
	}
	r := m.rooms[conferenceID]
	if r == nil {
		if len(m.rooms) >= m.opts.MaxRooms {
			return nil, media.ErrLimit
		}
		r = newRoom(conferenceID)
		m.rooms[conferenceID] = r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, existing := range r.egresses {
		if id == recordingID || existing.audioOnly == audioOnly {
			return nil, media.ErrNegotiation
		}
	}
	queueSize := m.opts.EgressQueueSize
	if audioOnly {
		queueSize = min(queueSize, 256)
	}
	e := &egress{audioOnly: audioOnly, manager: m, room: r, id: recordingID, frames: make(chan media.EgressFrame, queueSize), done: make(chan struct{}), tracks: map[string]bool{}}
	e.mu.Lock()
	e.enqueueLocked(media.EgressFrame{Type: "hello"})
	e.mu.Unlock()
	for _, t := range r.tracks {
		e.addTrack(t)
		if !audioOnly {
			t.requestPLI()
		}
	}
	r.egresses[recordingID] = e
	return e, nil
}

// Frames возвращает канал подписки, по которому передаются закодированные медиафреймы.
//
// @returns:
//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
func (e *egress) Frames() <-chan media.EgressFrame { return e.frames }

// Done возвращает канал, который закрывается при завершении собственного процесса или подписки.
//
// @returns:
//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
func (e *egress) Done() <-chan struct{} { return e.done }

// Err возвращает ошибку терминальной подписки, если таковая была зафиксирована.
//
// @returns:
//   - result1: Ошибка завершения собственного процесса или подписки, либо nil после
//     нормального завершения.
func (e *egress) Err() error { e.mu.Lock(); defer e.mu.Unlock(); return e.err }

// enqueueLocked ставит медиакадр в очередь, удерживая состояние подписки и обеспечивая
// ограничение очереди.
//
// @Parameters:
//   - frame: Кодированный медиакадр с метаданными о времени.
func (e *egress) enqueueLocked(frame media.EgressFrame) {
	if e.closed {
		return
	}
	e.sequence++
	frame.Sequence = e.sequence
	if frame.CapturedAt == 0 {
		frame.CapturedAt = time.Now().UnixNano()
	}
	select {
	case e.frames <- frame:
	default:
		if e.audioOnly {
			e.manager.audioTapDropped.Add(1)
		} else {
			e.manager.egressDropped.Add(1)
		}
		e.closed = true
		e.err = errEgressOverflow
		close(e.done)
	}
}

// addTrack добавляет описание кодированной исходной дорожки в подписку на запись, удерживая
// блокировку состояния.
//
// @Parameters:
//   - t: Test дескриптор, используемый для утверждений, очистки и сообщений об ошибках.
func (e *egress) addTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.tracks[t.metadata.ID] || !t.permitted.Load() || (e.audioOnly && t.metadata.Kind != media.KindAudio) {
		return
	}
	e.tracks[t.metadata.ID] = true
	codec := t.remote.Codec()
	descriptor := media.EgressTrack{Track: t.metadata, MimeType: codec.MimeType, ClockRate: codec.ClockRate, Channels: codec.Channels, PayloadType: uint8(t.remote.PayloadType()), SSRC: uint32(t.remote.SSRC())}
	e.enqueueLocked(media.EgressFrame{Type: "track", Track: &descriptor, TrackID: t.metadata.ID})
}

// endTrack помечает источник записи как завершенный и освобождает состояние активной дорожки.
//
// @Parameters:
//   - t: Test дескриптор, используемый для утверждений, очистки и сообщений об ошибках.
func (e *egress) endTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] {
		return
	}
	delete(e.tracks, t.metadata.ID)
	e.enqueueLocked(media.EgressFrame{Type: "track.end", TrackID: t.metadata.ID})
}

// packet копирует закодированный пакет в ограниченную очередь подписки на запись.
//
// @Parameters:
//   - t: Test дескриптор, используемый для утверждений, очистки и сообщений об ошибках.
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//   - at: Метка времени события или границы состояния.
func (e *egress) packet(t *publishedTrack, raw []byte, at int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] || !t.permitted.Load() {
		return
	}
	e.enqueueLocked(media.EgressFrame{Type: "rtp", TrackID: t.metadata.ID, RTP: raw, CapturedAt: at})
}

// fail регистрирует сбой безопасного медиа-соединения и запускает процесс очистки собственных
// ресурсов.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
func (e *egress) fail(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.err = err
		e.closed = true
		close(e.done)
	}
}

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
func (e *egress) Close() {
	e.closeOnce.Do(func() {
		e.fail(nil)
		e.manager.mu.Lock()
		e.room.mu.Lock()
		if e.room.egresses[e.id] == e {
			delete(e.room.egresses, e.id)
		}
		if len(e.room.peers) == 0 && len(e.room.egresses) == 0 && e.manager.rooms[e.room.id] == e.room {
			delete(e.manager.rooms, e.room.id)
		}
		e.room.mu.Unlock()
		e.manager.mu.Unlock()
	})
}

// Keyframes запрашивает видеокадр из подписки на запись.
func (e *egress) Keyframes() {
	e.room.mu.Lock()
	defer e.room.mu.Unlock()
	for _, t := range e.room.tracks {
		t.requestPLI()
	}
}

// Ping обновляет информацию о доступности текущей подписки на запись.
func (e *egress) Ping() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enqueueLocked(media.EgressFrame{Type: "ping"})
}

// recordPacket копирует закодированный пакет в исходящие подписки на запись.
//
// @Parameters:
//   - t: Test дескриптор, используемый для утверждений, очистки и сообщений об ошибках.
//   - packet: Закодированный RTP или медиапакет.
func (m *Manager) recordPacket(t *publishedTrack, packet *rtp.Packet) {
	t.publisher.room.mu.Lock()
	egresses := make([]*egress, 0, len(t.publisher.room.egresses))
	for _, e := range t.publisher.room.egresses {
		if e.audioOnly && t.metadata.Kind != media.KindAudio {
			continue
		}
		egresses = append(egresses, e)
	}
	t.publisher.room.mu.Unlock()
	if len(egresses) == 0 {
		return
	}
	raw, err := packet.Marshal()
	if err != nil {
		return
	}
	at := time.Now().UnixNano()
	for _, e := range egresses {
		e.packet(t, raw, at)
	}
}
