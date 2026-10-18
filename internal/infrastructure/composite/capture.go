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

// CaptureOptions bounds source fragments, packet buffering and encoded track capture.
type CaptureOptions struct {
	Mode            string
	SegmentDuration time.Duration
	MaxBytes        int64
	MaxTracks       int
	OnStarted       func() error
}

var ErrEgressEnded = errors.New("media egress ended")

// OnlyEgressEnded distinguishes a normal media-egress completion from a recording failure.
//
// Parameters:
//   - err: Error to classify, wrap or report safely.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func OnlyEgressEnded(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface {
		// Unwrap returns the underlying cause so errors.Is and errors.As can inspect it.
		//
		// Returns:
		//   - result 1: The wrapped diagnostic cause, retained for errors.Is and errors.As inspection.
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

// Capture captures encoded media into per-source recording archives for later composition.
//
// Parameters:
//   - captureCtx: Context controlling cancellation, deadlines and operation lifetime.
//   - workCtx: Context controlling cancellation, deadlines and operation lifetime.
//   - reader: Source content stream or recording-card reader, as defined by its type.
//   - dir: Validated local directory.
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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
		// An active browser video source may send no packets for an entire segment, for example for a
		// static screen. Retain the last decodable frame group; no recent packet does not mean the
		// source ended.
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
			// Flush the last complete frame of infrequently updated video after the jitter window;
			// SampleBuilder normally waits for the next RTP timestamp.
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
	// Read and decode ahead into a bounded queue. Closing the HTTP stream must not discard frames
	// received while a source file is closing. This queue is recording-only; disk I/O never runs in
	// the SFU forwarding path.
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
				} // Stopping may interrupt one HTTP frame.
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
			// An established subscription records the conference timeline, including
			// silent intervals. Waiting for RTP leaves muted rooms stuck in starting.
			if options.OnStarted != nil {
				if err := options.OnStarted(); err != nil {
					captureErr = err
					break
				}
			}
			started = true
			continue
		}
		// Concurrent SFU publishers may timestamp before acquiring the stream lock. Clamp minor
		// arrival-order inversions to the ordered stream clock.
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
				// Ended tracks may retain a writer until the segment closes. Count those buffers too, so rapid
				// device changes cannot bypass the active-track limit or allocate unbounded SampleBuilders.
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
				// Retain a quiet source until it explicitly ends, not merely until the last packet in the
				// previous segment.
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
			// A quiet track may end after its previous writer closes. The current segment then has no data
			// that needs the retained initial frame.
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

// validCodec checks whether the track codec is supported by the archive writer.
//
// Parameters:
//   - track: Media-track metadata or negotiated track instance.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func validCodec(track media.EgressTrack) bool {
	return (track.Kind == media.KindVideo && strings.EqualFold(track.MimeType, "video/VP8") && track.ClockRate == 90000) || (track.Kind == media.KindAudio && strings.EqualFold(track.MimeType, "audio/opus") && track.ClockRate == 48000 && track.Channels <= 2)
}

// sourceWriter assembles one encoded elementary source stream into durable recording fragments.
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

// encodedSample contains encoded media and its container timing metadata.
type encodedSample struct {
	data      []byte
	timestamp uint32
	at        int64
	duration  time.Duration
}

// sourceState retains source state across packets and fragment boundaries.
type sourceState struct {
	builder      *samplebuilder.SampleBuilder
	arrivals     map[uint32]int64
	preroll      []encodedSample
	bytes        int
	lastPacketAt int64
}

// sourceJitterWindow bounds RTP reordering during ordinary packet capture.
const sourceJitterWindow = 200 * time.Millisecond

// Observed screen-share streams can exceed the requested capture frame rate.
// Keep enough frames between periodic keyframes without relaxing the byte cap.
const maxPrerollFrames = 4096

// newSourceWriterState creates the per-source state used to assemble incoming RTP into media
// samples.
//
// Parameters:
//   - dir: Validated local directory.
//   - index: Index of the item being processed.
//   - track: Media-track metadata or negotiated track instance.
//   - state: Current state or validated state update.
//
// Returns:
//   - result 1: The constructed or loaded source writer.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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
	// Reusing initial encoded data avoids waiting for a keyframe and losing Opus pre-skip state when
	// layouts change. The independent compositor trims and aligns negative offsets.
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

// WriteRTP queues an RTP packet for ordered sample assembly and track archiving.
//
// Parameters:
//   - packet: Encoded RTP or media packet.
//   - capturedAt: Capture timestamp used to align the encoded media.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// pop removes the next available item from the component-owned queue.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// writeSample writes one assembled media sample into the active recording chunk.
//
// Parameters:
//   - sample: Assembled encoded media sample.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// Close closes the component-owned resources and ends their lifecycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (w *sourceWriter) Close() error {
	return w.closeChunk(true)
}

// closeChunk finalizes the current media chunk and records its metadata.
//
// Parameters:
//   - final: Whether this is the final chunk and the archive must be closed.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (w *sourceWriter) closeChunk(final bool) error {
	var err error
	if final {
		// Flush already forces every buffered packet out. Disable its redundant
		// timestamp-age scans, then restore the capture jitter window for sources
		// reused after a quiet video chunk. No Push/Pop runs with the guard disabled.
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
		// Keep disk synchronization out of packet reception: the compositor syncs closed source files
		// before reading them. Otherwise macOS fsync can delay packet reads enough to lose the recording
		// tail during stop.
		err = errors.Join(err, countErr, w.file.Close())
	}
	return err
}
