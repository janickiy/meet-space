package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// RTPTrack описывает кодек и локальный входной сигнал RTP для одной записанной дорожки.
type RTPTrack struct {
	Kind        string
	MimeType    string
	PayloadType uint8
	ClockRate   uint32
	Channels    uint16
	Port        int
	FmtpLine    string
}

// SegmentRecorder настраивает и запускает захват сегментов FFmpeg из потоков RTP.
type SegmentRecorder struct {
	path   string
	logger *log.Logger
}

// SegmentProcess управляет процессом записи сегментов и отслеживает его завершение.
type SegmentProcess struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	waitErr  error
	stopOnce sync.Once
	stderr   logTail
	stdin    io.WriteCloser
	logger   *log.Logger
}

// NewSegmentRecorder создаёт модуль записи сегментов с зависимостями и ограничениями,
// необходимыми для записи и постобработки FFmpeg.
//
// @Parameters:
//   - path: Проверенный путь к локальной файловой системе или объекту.
//   - logger: Ограниченный диагностический логгер.
//
// @returns:
//   - result1: Созданный или загруженный модуль записи сегментов.
func NewSegmentRecorder(path string, logger *log.Logger) *SegmentRecorder {
	if path == "" {
		path = "ffmpeg"
	}
	if logger == nil {
		logger = log.Default()
	}

	return &SegmentRecorder{path: path, logger: logger}
}

// Start запускает запись и постобработку FFmpeg и подготавливает ресурсы, необходимые для этой
// операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
//   - tracks: Согласованные или архивированные дорожки, включенные в эту операцию.
//   - workDir: Проверенный рабочий каталог процесса.
//   - outputDir: Проверенный каталог для сгенерированных артефактов.
//   - segmentDurationSec: Запрошенная продолжительность сегмента в секундах.
//
// @returns:
//   - result1: Процесс создания или загрузки сегмента.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SegmentRecorder) Start(ctx context.Context, recordID string, tracks []RTPTrack, workDir string, outputDir string, segmentDurationSec int) (*SegmentProcess, error) {
	if len(tracks) == 0 {
		return nil, fmt.Errorf("ffmpeg needs at least one RTP track")
	}
	if segmentDurationSec <= 0 {
		segmentDurationSec = 5
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create ffmpeg work dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create ffmpeg output dir: %w", err)
	}
	sdp, err := buildRTPInputSDP(tracks)
	if err != nil {
		return nil, err
	}
	sdpPath := filepath.Join(workDir, "input.sdp")
	if err := os.WriteFile(sdpPath, []byte(sdp), 0o644); err != nil {
		return nil, fmt.Errorf("write ffmpeg sdp: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	segmentTpl := filepath.Join(outputDir, "segment_%06d.mkv")
	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-protocol_whitelist", "file,udp,rtp",
		"-fflags", "+genpts",
		"-f", "sdp",
		"-i", sdpPath,
		"-map", "0:v:0?",
		"-map", "0:a:0?",
		"-c", "copy",
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", segmentDurationSec),
		"-segment_format", "matroska",
		"-reset_timestamps", "1",
		segmentTpl,
	}
	cmd := exec.CommandContext(runCtx, r.path, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stdin: %w", err)
	}
	process := &SegmentProcess{
		cmd:    cmd,
		cancel: cancel,
		done:   make(chan struct{}),
		stdin:  stdin,
		logger: r.logger,
	}
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start ffmpeg segment recorder: %w", err)
	}
	go func() {
		process.waitErr = cmd.Wait()
		close(process.done)
	}()

	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-process.done:
		cancel()
		return nil, fmt.Errorf("ffmpeg exited during startup: %v; stderr=%s", process.waitErr, process.Stderr())
	case <-timer.C:
		r.logger.Printf("record %s ffmpeg segment recorder started", recordID)
		return process, nil
	case <-ctx.Done():
		cancel()
		// Дочерний процесс принадлежит программе Startup до тех пор, пока она не передаст его
		// процессу сеанса.  Присоединиться к ожиданию здесь, чтобы отменённый процесс Startup
		// не пережил остановку/завершение работы.
		<-process.done
		return nil, ctx.Err()
	}
}

// Stop останавливает активную операцию ffmpeg и освобождает принадлежащие ей ресурсы.
//
// @Parameters:
//   - timeout: Максимальное время, отведенное на выполнение этой операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *SegmentProcess) Stop(timeout time.Duration) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	p.stopOnce.Do(func() { p.stop(timeout) })
	return nil
}

// stop останавливает активную операцию ffmpeg и освобождает принадлежащие ей ресурсы.
//
// @Parameters:
//   - timeout: Максимальное время, отведенное на выполнение этой операции.
func (p *SegmentProcess) stop(timeout time.Duration) {
	defer p.cancel()
	select {
	case <-p.done:
		return
	default:
	}
	if p.stdin != nil {
		_, _ = io.WriteString(p.stdin, "q\n")
		_ = p.stdin.Close()
	} else {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.done:
		if p.waitErr != nil && p.logger != nil {
			p.logger.Printf("ffmpeg stopped with error: %v; stderr=%s", p.waitErr, p.Stderr())
		}
		return
	case <-timer.C:
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.done:
			if p.waitErr != nil && p.logger != nil {
				p.logger.Printf("ffmpeg stopped after SIGTERM: %v; stderr=%s", p.waitErr, p.Stderr())
			}
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
			p.cancel()
			select {
			case <-p.done:
				if p.waitErr != nil && p.logger != nil {
					p.logger.Printf("ffmpeg killed after stop timeout: %v; stderr=%s", p.waitErr, p.Stderr())
				}
			case <-time.After(2 * time.Second):
				if p.logger != nil {
					p.logger.Printf("ffmpeg kill wait timeout; stderr=%s", p.Stderr())
				}
			}
		}
		return
	}
}

// Done сигнализирует о завершении после того, как cmd.Wait завершает процесс и закрывает его
// каналы.
//
// @returns:
//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
func (p *SegmentProcess) Done() <-chan struct{} { return p.done }

// WaitErr возвращает результат процесса; вызывающие функции должны сначала дождаться сигнала
// Done.
//
// @returns:
//   - result1: Ошибка завершения собственного процесса или подписки, либо nil после
//     нормального завершения.
func (p *SegmentProcess) WaitErr() error { return p.waitErr }

// Stderr возвращает ограниченный диагностический хвост, полученный от процесса записи.
//
// @returns:
//   - result1: Ограниченный хвост диагностических данных процесса записи.
func (p *SegmentProcess) Stderr() string {
	if p == nil {
		return ""
	}

	return strings.TrimSpace(p.stderr.String())
}

// buildRTPInputSDP формирует локальное описание SDP, используемое для передачи захваченных
// данных RTP в FFmpeg.
//
// @Parameters:
//   - tracks: Согласованные или архивированные дорожки, включенные в эту операцию.
//
// @returns:
//   - result1: SDP описывает локальные входы RTP, используемые FFmpeg.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func buildRTPInputSDP(tracks []RTPTrack) (string, error) {
	var b strings.Builder
	b.WriteString("v=0\n")
	b.WriteString("o=- 0 0 IN IP4 127.0.0.1\n")
	b.WriteString("s=go-recorder\n")
	b.WriteString("c=IN IP4 127.0.0.1\n")
	b.WriteString("t=0 0\n")

	supported := 0
	for _, track := range tracks {
		codec := sdpCodecName(track.MimeType)
		if codec == "" {
			continue
		}
		supported++
		if track.ClockRate == 0 {
			track.ClockRate = defaultClockRate(track.Kind)
		}
		b.WriteString(fmt.Sprintf("m=%s %d RTP/AVP %d\n", track.Kind, track.Port, track.PayloadType))
		if track.Kind == "audio" && track.Channels > 0 {
			b.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d/%d\n", track.PayloadType, codec, track.ClockRate, track.Channels))
		} else {
			b.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d\n", track.PayloadType, codec, track.ClockRate))
		}
		if track.FmtpLine != "" {
			b.WriteString(fmt.Sprintf("a=fmtp:%d %s\n", track.PayloadType, track.FmtpLine))
		}
		b.WriteString("a=recvonly\n")
	}
	if supported == 0 {
		return "", fmt.Errorf("ffmpeg does not support received WebRTC codecs")
	}

	return b.String(), nil
}

// sdpCodecName сопоставляет согласованный кодек с его названием в кодировке SDP.
//
// @Parameters:
//   - mime: Тип медиа MIME или метка кодека.
//
// @returns:
//   - result1: Название кодировки SDP для медиакодека.
func sdpCodecName(mime string) string {
	switch strings.ToLower(mime) {
	case "video/vp8":
		return "VP8"
	case "video/vp9":
		return "VP9"
	case "video/h264":
		return "H264"
	case "audio/opus":
		return "opus"
	case "audio/pcmu":
		return "PCMU"
	case "audio/pcma":
		return "PCMA"
	default:
		return ""
	}
}

// defaultClockRate возвращает тактовую частоту RTP по умолчанию для выбранного медиакодека.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//
// @returns:
//   - result1: Результирующее значение типа uint32.
func defaultClockRate(kind string) uint32 {
	if kind == "audio" {
		return 48000
	}

	return 90000
}
