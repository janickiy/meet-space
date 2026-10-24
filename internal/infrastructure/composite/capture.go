// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package composite

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

// CaptureOptions ограничивает количество исходных фрагментов, буферизацию пакетов и захват
// закодированных дорожек.
type CaptureOptions struct {
	Mode            string
	SegmentDuration time.Duration
	MaxBytes        int64
	MaxTracks       int
	OnStarted       func() error
}

var ErrEgressEnded = errors.New("media egress ended")

// OnlyEgressEnded отличает обычное завершение передачи медиаданных от сбоя при записи.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func OnlyEgressEnded(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface {
		// Unwrap возвращает основную причину, чтобы errors.Is и errors.As могли ее проверить.
		//
		// @returns:
		//   - result1: Обернутая диагностическая причина, сохраненная для проверки
		//   errors.Is и errors.As.
		Unwrap() []error
	}); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !OnlyEgressEnded(child) {
				return false
			}
		}
		return true
	}
	return errors.Is(err, ErrEgressEnded)
}

// Capture — захват закодированных медиаданных в архивы для записи по каждому источнику для последующей
// компоновки.
//
// @Parameters:
//   - captureCtx: Контекст, контролирующий отмену, сроки и продолжительность выполнения
//     операции.
//   - workCtx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - reader: Поток исходного контента или устройство чтения карт памяти, в зависимости от его
//     типа.
//   - dir: Проверенный локальный каталог.
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Composer) Capture(captureCtx, workCtx context.Context, reader io.Reader, dir string, options CaptureOptions) error {
	if options.SegmentDuration < time.Second || options.SegmentDuration > 30*time.Second {
		return fmt.Errorf("invalid segment duration")
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = 10 << 30
	}
	if options.MaxTracks <= 0 {
		options.MaxTracks = 64
	}
	if err := os.MkdirAll(filepath.Join(dir, "sources"), 0o750); err != nil {
		return err
	}
	jobs := make(chan Chunk, 4)
	composed := make(chan error, 1)
	composeFailed := make(chan error, 1)
	go func() {
		var result error
		for chunk := range jobs {
			if result == nil {
				result = c.Compose(workCtx, dir, chunk)
				if result != nil {
					composeFailed <- result
				}
			}
		}
		composed <- result
	}()
	active := map[string]media.EgressTrack{}
	writers := map[string]*sourceWriter{}
	states := map[string]*sourceState{}
	var epoch, last int64
	var sequence uint64
	var bytesReceived int64
	index := 0
	started := false

	closeChunk := func(end int64, final bool) error {
		chunk := Chunk{Index: index, Mode: options.Mode, StartedAtNS: epoch, Duration: float64(end-epoch) / 1e9, Sources: []Source{}}
		var closeErr error
		// Активный видеоисточник в браузере может не отправлять пакеты в течение всего
		// сегмента, например при показе статичного изображения. Сохраните последнюю
		// декодируемую группу кадров; отсутствие последних пакетов не означает, что источник
		// завершил передачу.
		for id, track := range active {
			if track.Kind != media.KindVideo || writers[id] != nil || states[id] == nil {
				continue
			}
			writer, err := newSourceWriterState(dir, index, track, states[id])
			if err != nil {
				closeErr = errors.Join(closeErr, err)
				continue
			}
			writers[id] = writer
		}
		for id, writer := range writers {
			_, stillActive := active[id]
			// После окна джиттера очистите память от последнего полного кадра редко
			// обновляемого видео; SampleBuilder обычно ожидает следующей метки времени RTP.
			quietVideo := writer.track.Kind == media.KindVideo && end-writer.state.lastPacketAt >= int64(sourceJitterWindow)
			if err := writer.closeChunk(final || !stillActive || quietVideo); err != nil {
				closeErr = errors.Join(closeErr, err)
			}
			if writer.samples > 0 {
				sourceEnd := min(chunk.Duration, float64(writer.lastAt-epoch)/1e9+writer.lastDuration.Seconds())
				if writer.track.Kind == media.KindVideo {
					sourceEnd = chunk.Duration
					if writer.endedAt != 0 {
						sourceEnd = min(chunk.Duration, float64(writer.endedAt-epoch)/1e9)
					}
				}
				chunk.Sources = append(chunk.Sources, Source{Track: writer.track.Track, File: filepath.Base(writer.path), Offset: float64(writer.firstAt-epoch) / 1e9, End: sourceEnd})
			}
			delete(writers, id)
			if !stillActive {
				delete(states, id)
			}
		}
		if closeErr != nil {
			return closeErr
		}
		if chunk.Duration < 0.05 {
			return nil
		}
		sort.Slice(chunk.Sources,

			func(i, j int) bool { return chunk.Sources[i].Track.ID < chunk.Sources[j].Track.ID })
		tracks := make([]media.Track, 0, len(chunk.Sources))
		for _, source := range chunk.Sources {
			tracks = append(tracks, source.Track)
		}
		chunk.Layout = GridLayout(tracks, c.Width, c.Height)
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, fmt.Sprintf("chunk_%06d.json", index))
		if err := os.WriteFile(path+".partial", data, 0o640); err != nil {
			return err
		}
		if err := os.Rename(path+".partial", path); err != nil {
			return err
		}
		select {
		case jobs <- chunk:
		case <-workCtx.Done():
			return workCtx.Err()
		default:
			return fmt.Errorf("compositor backlog limit reached; durable chunks retained")
		}
		index++
		epoch = end
		return nil
	}
	// Считывайте и декодируйте данные в пределах ограниченной очереди.  При закрытии потока
	// HTTP не следует отбрасывать кадры, полученные во время закрытия исходного файла.  Эта
	// очередь предназначена только для записи; дисковый ввод-вывод никогда не используется на
	// пути пересылки SFU.
	readCtx, stopReading := context.WithCancel(workCtx)
	defer stopReading()
	frames := make(chan media.EgressFrame, 512)
	readDone := make(chan error, 1)
	go func() {
		var readErr error
		defer func() { readDone <- readErr; close(frames) }()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64<<10), 256<<10)
		for scanner.Scan() {
			var frame media.EgressFrame
			if json.Unmarshal(scanner.Bytes(), &frame) != nil {
				if captureCtx.Err() != nil {
					return
				} // При остановке может прерваться передача одного кадра HTTP.
				readErr = fmt.Errorf("invalid egress frame")
				return
			}
			select {
			case frames <- frame:
			case <-readCtx.Done():
				return
			}
		}
		if captureCtx.Err() == nil {
			readErr = ErrEgressEnded
			if scanner.Err() != nil {
				if errors.Is(scanner.Err(), bufio.ErrTooLong) {
					readErr = fmt.Errorf("egress frame exceeded size limit")
				} else {
					readErr = fmt.Errorf("%w: transport closed", ErrEgressEnded)
				}
			}
		}
	}()
	var captureErr error
readLoop:
	for frame := range frames {
		select {
		case captureErr = <-composeFailed:
			break readLoop
		default:
		}
		if frame.CapturedAt <= 0 || (sequence != 0 && frame.Sequence != sequence+1) {
			captureErr = fmt.Errorf("egress sequence or timestamp invalid")
			break
		}
		sequence = frame.Sequence
		if epoch == 0 {
			if frame.Type != "hello" {
				captureErr = fmt.Errorf("egress hello required")
				break
			}
			epoch = frame.CapturedAt
			last = epoch
			// Установленная подписка позволяет записывать хронологию конференции, в том числе
			// периоды тишины.  Если дождаться RTP, то комнаты с отключённым звуком останутся в
			// состоянии «зависания» при запуске.
			if options.OnStarted != nil {
				if err := options.OnStarted(); err != nil {
					captureErr = err
					break
				}
			}
			started = true
			continue
		}
		// Параллельные источники SFU могут устанавливать временные метки до получения
		// блокировки потока.  Устраняйте незначительные инверсии порядка поступления данных,
		// сверяясь с упорядоченными временными метками потока.
		if frame.CapturedAt < last {
			frame.CapturedAt = last
		}
		if frame.CapturedAt-last > int64(time.Minute) {
			captureErr = fmt.Errorf("egress clock discontinuity")
			break
		}
		for frame.CapturedAt-epoch >= int64(options.SegmentDuration) {
			if err := closeChunk(epoch+int64(options.SegmentDuration), false); err != nil {
				captureErr = err
				break readLoop
			}
		}
		last = frame.CapturedAt
		switch frame.Type {
		case "track":
			if frame.Track == nil {
				captureErr = fmt.Errorf("missing egress track")
				break readLoop
			}
			track := *frame.Track
			if _, err := uuid.Parse(track.ID); err != nil {
				captureErr = fmt.Errorf("invalid egress track id")
				break readLoop
			}
			if len(active) >= options.MaxTracks {
				captureErr = fmt.Errorf("recording track limit reached")
				break readLoop
			}
			if !validCodec(track) {
				captureErr = fmt.Errorf("unsupported recording codec")
				break readLoop
			}
			if _, exists := active[track.ID]; exists {
				captureErr = fmt.Errorf("duplicate egress track")
				break readLoop
			}
			active[track.ID] = track
		case "rtp":
			track, ok := active[frame.TrackID]
			if !ok {
				captureErr = fmt.Errorf("RTP without egress track")
				break readLoop
			}
			if options.Mode == "audio_only" && track.Kind != media.KindAudio {
				continue
			}
			bytesReceived += int64(len(frame.RTP))
			if bytesReceived > options.MaxBytes {
				captureErr = fmt.Errorf("recording byte limit reached")
				break readLoop
			}
			var packet rtp.Packet
			if err := packet.Unmarshal(frame.RTP); err != nil {
				captureErr = fmt.Errorf("invalid egress RTP")
				break readLoop
			}
			writer := writers[track.ID]
			if writer == nil {
				// Ended Треки могут сохранять запись до закрытия сегмента. Учитывайте и эти
				// буферы, чтобы при быстрой смене устройств не было превышения лимита активных
				// дорожек и не выделялись неограниченные SampleBuilders.
				if states[track.ID] == nil && len(states) >= options.MaxTracks {
					captureErr = fmt.Errorf("recording retained track limit reached")
					break readLoop
				}
				var err error
				writer, err = newSourceWriterState(dir, index, track, states[track.ID])
				if err != nil {
					captureErr = err
					break readLoop
				}
				writers[track.ID] = writer
				states[track.ID] = writer.state
			}
			if err := writer.WriteRTP(&packet, frame.CapturedAt); err != nil {
				captureErr = err
				break readLoop
			}
		case "track.end":
			track, exists := active[frame.TrackID]
			if exists && track.Kind == media.KindVideo && writers[frame.TrackID] == nil && states[frame.TrackID] != nil {
				// Сохраняйте неактивный источник до тех пор, пока он не завершит передачу, а не
				// просто до получения последнего пакета в предыдущем сегменте.
				writer, err := newSourceWriterState(dir, index, track, states[frame.TrackID])
				if err != nil {
					captureErr = err
					break readLoop
				}
				writers[frame.TrackID] = writer
			}
			if writer := writers[frame.TrackID]; writer != nil {
				writer.endedAt = frame.CapturedAt
			}
			delete(active, frame.TrackID)
			// Неактивная дорожка может завершиться после закрытия предыдущей записывающей
			// стороны. В таком случае в текущем сегменте не будет данных, для которых требуется
			// сохраненный начальный кадр.
			if writers[frame.TrackID] == nil {
				delete(states, frame.TrackID)
			}
		case "ping":
		case "error":
			captureErr = fmt.Errorf("media egress failed: %s", frame.Code)
			break readLoop
		default:
			captureErr = fmt.Errorf("unknown egress frame")
			break readLoop
		}
	}
	stopReading()
	if closer, ok := reader.(io.Closer); ok {
		_ = closer.Close()
	}
	captureErr = errors.Join(captureErr, <-readDone)
	if epoch != 0 {
		captureErr = errors.Join(captureErr, closeChunk(last, true))
	}
	close(jobs)
	captureErr = errors.Join(captureErr, <-composed)
	if !started && captureErr == nil {
		captureErr = fmt.Errorf("recording received no media")
	}
	return captureErr
}

// validCodec проверяет, поддерживается ли кодек дорожки записывающим устройством архиватора.
//
// @Parameters:
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func validCodec(track media.EgressTrack) bool {
	return (track.Kind == media.KindVideo && strings.EqualFold(track.MimeType, "video/VP8") && track.ClockRate == 90000) || (track.Kind == media.KindAudio && strings.EqualFold(track.MimeType, "audio/opus") && track.ClockRate == 48000 && track.Channels <= 2)
}

// sourceWriter объединяет один закодированный элементарный поток исходного материала в
// устойчивые фрагменты записи.
type sourceWriter struct {
	track          media.EgressTrack
	path           string
	state          *sourceState
	file           *os.File
	ogg            *oggwriter.OggWriter
	firstTimestamp uint32
	firstAt        int64
	lastAt         int64
	endedAt        int64
	lastDuration   time.Duration
	samples        uint32
	keyframe       bool
}

// encodedSample содержит закодированные медиаданные и метаданные синхронизации контейнера.
type encodedSample struct {
	data      []byte
	timestamp uint32
	at        int64
	duration  time.Duration
}

// sourceState сохраняет состояние источника между пакетами и границами фрагментов.
type sourceState struct {
	builder      *samplebuilder.SampleBuilder
	arrivals     map[uint32]int64
	preroll      []encodedSample
	bytes        int
	lastPacketAt int64
}

// sourceJitterWindow ограничивает переупорядочивание RTP при обычной записи пакетов.
const sourceJitterWindow = 200 * time.Millisecond

// Наблюдаемые потоки совместного использования экрана могут превышать запрошенную частоту
// кадров при захвате.  Сохраняйте достаточное количество кадров между периодическими ключевыми
// кадрами, не превышая ограничение по размеру в байтах.
const maxPrerollFrames = 4096

// newSourceWriterState создает состояние для каждого источника, используемое для объединения
// входящих RTP в медиасэмплы.
//
// @Parameters:
//   - dir: Проверенный локальный каталог.
//   - index: Индекс обрабатываемого элемента.
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
//   - state: Текущее состояние или обновлённое валидированное состояние.
//
// @returns:
//   - result1: Созданный или загруженный модуль записи исходного кода.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func newSourceWriterState(dir string, index int, track media.EgressTrack, state *sourceState) (*sourceWriter, error) {
	w := &sourceWriter{track: track, state: state}
	ext := ".ivf"
	var depacketizer rtp.Depacketizer = &codecs.VP8Packet{}
	if track.Kind == media.KindAudio {
		ext = ".ogg"
		depacketizer = &codecs.OpusPacket{}
	}
	w.path = filepath.Join(dir, "sources", fmt.Sprintf("%06d_%s%s", index, track.ID, ext))
	if w.state == nil {
		w.state = &sourceState{builder: samplebuilder.New(64, depacketizer, track.ClockRate, samplebuilder.WithMaxTimeDelay(sourceJitterWindow)), arrivals: make(map[uint32]int64)}
	}
	if track.Kind == media.KindAudio {
		channels := track.Channels
		if channels == 0 {
			channels = 2
		}
		var err error
		w.ogg, err = oggwriter.New(w.path, 48000, channels)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		w.file, err = os.Create(w.path)
		if err != nil {
			return nil, err
		}
		header := make([]byte, 32)
		copy(header, "DKIF")
		binary.LittleEndian.PutUint16(header[6:], 32)
		copy(header[8:], "VP80")
		binary.LittleEndian.PutUint32(header[16:], 90000)
		binary.LittleEndian.PutUint32(header[20:], 1)
		if _, err := w.file.Write(header); err != nil {
			w.file.Close()
			return nil, err
		}
	}
	// Повторное использование исходных закодированных данных позволяет избежать ожидания
	// ключевого кадра и потери состояния Opus до пропуска при смене макетов.  Независимый
	// модуль композитинга обрезает и выравнивает отрицательные смещения.
	for _, sample := range w.state.preroll {
		if err := w.writeSample(sample); err != nil {
			if w.file != nil {
				w.file.Close()
			}
			if w.ogg != nil {
				w.ogg.Close()
			}
			return nil, err
		}
	}
	return w, nil
}

// WriteRTP помещает в очередь пакет RTP для упорядоченной сборки сэмплов и архивирования
// дорожек.
//
// @Parameters:
//   - packet: Закодированный RTP или медиапакет.
//   - capturedAt: Временная метка захвата, используемая для выравнивания закодированных
//     медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (w *sourceWriter) WriteRTP(packet *rtp.Packet, capturedAt int64) error {
	w.state.lastPacketAt = max(w.state.lastPacketAt, capturedAt)
	if len(w.state.arrivals) > 256 {
		for key := range w.state.arrivals {
			delete(w.state.arrivals, key)
		}
	}
	if _, ok := w.state.arrivals[packet.Timestamp]; !ok {
		w.state.arrivals[packet.Timestamp] = capturedAt
	}
	w.state.builder.Push(packet)
	return w.pop()
}

// pop удаляет следующий доступный элемент из очереди, принадлежащей компоненту.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (w *sourceWriter) pop() error {
	for sample := w.state.builder.Pop(); sample != nil; sample = w.state.builder.Pop() {
		at := w.state.arrivals[sample.PacketTimestamp]
		delete(w.state.arrivals, sample.PacketTimestamp)
		if at == 0 {
			continue
		}
		frame := encodedSample{data: sample.Data, timestamp: sample.PacketTimestamp, at: at, duration: sample.Duration}
		if w.track.Kind == media.KindVideo {
			keyframe := len(frame.data) >= 10 && frame.data[0]&1 == 0
			if keyframe {
				w.state.preroll = nil
				w.state.bytes = 0
			}
			if !keyframe && len(w.state.preroll) == 0 {
				continue
			}
			w.state.preroll = append(w.state.preroll, frame)
			w.state.bytes += len(frame.data)
			if w.state.bytes > 4<<20 || len(w.state.preroll) > maxPrerollFrames {
				return fmt.Errorf("recording keyframe pre-roll limit exceeded")
			}
		} else {
			w.state.preroll = append(w.state.preroll, frame)
			if len(w.state.preroll) > 6 {
				w.state.preroll = w.state.preroll[len(w.state.preroll)-6:]
			}
		}
		if err := w.writeSample(frame); err != nil {
			return err
		}
	}
	return nil
}

// writeSample записывает один собранный медиафайл в активный блок записи.
//
// @Parameters:
//   - sample: Собранный закодированный медиафайл.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (w *sourceWriter) writeSample(sample encodedSample) error {
	if w.track.Kind == media.KindVideo && !w.keyframe {
		if len(sample.data) < 10 || sample.data[0]&1 != 0 {
			return nil
		}
		w.keyframe = true
		dimensions := []byte{sample.data[6], sample.data[7] & 0x3f, sample.data[8], sample.data[9] & 0x3f}
		if _, err := w.file.WriteAt(dimensions, 12); err != nil {
			return err
		}
	}
	if w.samples == 0 {
		w.firstTimestamp = sample.timestamp
		w.firstAt = sample.at
		if w.track.Kind == media.KindAudio {
			w.firstAt += int64(80 * time.Millisecond)
		}
	}
	w.lastAt = sample.at
	w.lastDuration = sample.duration
	if w.ogg != nil {
		if err := w.ogg.WriteRTP(&rtp.Packet{Header: rtp.Header{Timestamp: sample.timestamp}, Payload: sample.data}); err != nil {
			return err
		}
	} else {
		header := make([]byte, 12)
		binary.LittleEndian.PutUint32(header, uint32(len(sample.data)))
		binary.LittleEndian.PutUint64(header[4:], uint64(sample.timestamp-w.firstTimestamp))
		if _, err := w.file.Write(header); err != nil {
			return err
		}
		if _, err := w.file.Write(sample.data); err != nil {
			return err
		}
	}
	w.samples++
	return nil
}

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (w *sourceWriter) Close() error {
	return w.closeChunk(true)
}

// closeChunk завершает работу с текущим медиафайлом и записывает его метаданные.
//
// @Parameters:
//   - final: Определяет, является ли этот файл последним и нужно ли закрыть архив.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (w *sourceWriter) closeChunk(final bool) error {
	var err error
	if final {
		// Функция Flush уже принудительно выводит все буферизованные пакеты.  Отключите
		// избыточное сканирование временных меток, а затем восстановите окно джиттера захвата
		// для источников, повторно используемых после тихого видеофрагмента.  Функция Push/Pop
		// не запускается при отключенном контроле.
		samplebuilder.WithMaxTimeDelay(0)(w.state.builder)
		w.state.builder.Flush()
		samplebuilder.WithMaxTimeDelay(sourceJitterWindow)(w.state.builder)
		err = w.pop()
	}
	if w.ogg != nil {
		err = errors.Join(err, w.ogg.Close())
	}
	if w.file != nil {
		count := make([]byte, 4)
		binary.LittleEndian.PutUint32(count, w.samples)
		_, countErr := w.file.WriteAt(count, 24)
		// Не синхронизируйте диск с приемом пакетов: компоновщик синхронизирует закрытые
		// исходные файлы перед их чтением.  В противном случае функция fsync в macOS может
		// задержать чтение пакетов настолько, что при остановке записи будет потерян хвост.
		err = errors.Join(err, countErr, w.file.Close())
	}
	return err
}
