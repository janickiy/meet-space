// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/operations"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

// LiveAudio декодирует одну дорожку в формате Opus в отдельном однопоточном процессе CPU. Общий
// пул активных рабочих ограничивает параллелизм, не допуская неограниченного количества
// горутин-издателей.
type LiveAudio struct{ Binary string }

// Decode — декодер декодирует ограниченный закодированный аудиопоток в кадры PCM для конвейера
// субтитров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - track: Метаданные медиадорожки или согласованный экземпляр дорожки.
//   - packets: Входной канал закодированных медиакадров.
//   - pcm: Обратный вызов, получающий каждый декодированный аудиокадр PCM и сообщающий об
//     ошибке доставки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (d LiveAudio) Decode(ctx context.Context, track media.EgressTrack, packets <-chan media.EgressFrame, pcm func([]byte) error) error {
	if track.MimeType != "audio/opus" || track.ClockRate != 48000 || track.Channels < 1 || track.Channels > 2 {
		return fmt.Errorf("unsupported live audio codec")
	}
	op, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(op, d.Binary, "-hide_banner", "-loglevel", "error", "-nostdin", "-threads", "1", "-probesize", "4096", "-analyzeduration", "0", "-f", "ogg", "-i", "pipe:0", "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-threads", "1", "-f", "s16le", "-flush_packets", "1", "pipe:1")
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	started := time.Now()
	defer func() {
		operations.Observe("live_decode", time.Since(started).Seconds())
		if cmd.ProcessState != nil {
			operations.Observe("live_decode_cpu", (cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Seconds())
		}
	}()
	readDone := make(chan error, 1)
	outputDone := make(chan struct{})
	go func() {
		var result error
		defer func() {
			readDone <- result
			close(outputDone)
			if result != nil {
				cancel()
			}
		}()
		buffer := make([]byte, 640)
		for {
			n, e := io.ReadFull(output, buffer)
			if n > 0 {
				if result = pcm(buffer[:n]); result != nil {
					return
				}
			}
			if e != nil {
				if e != io.EOF && e != io.ErrUnexpectedEOF {
					result = e
				}
				return
			}
		}
	}()
	writer, err := oggwriter.NewWith(input, 48000, track.Channels)
	if err != nil {
		cancel()
		input.Close()
		<-readDone
		cmd.Wait()
		return err
	}
	builder := samplebuilder.New(10, &codecs.OpusPacket{}, 48000)
	var stamp uint32
	var initialized bool
	writeSample := func() error {
		for sample := builder.Pop(); sample != nil; sample = builder.Pop() {
			if err := writer.WriteRTP(&rtp.Packet{Header: rtp.Header{Timestamp: stamp}, Payload: sample.Data}); err != nil {
				return err
			}
			stamp += uint32(sample.Duration.Seconds() * 48000)
		}
		return nil
	}
loop:
	for {
		select {
		case <-op.Done():
			break loop
		case <-outputDone:
			// Дешифратор может завершить работу при получении неверных входных данных, пока
			// очередь RTP остается открытой.  Освободите слот и вызовите Wait, не дожидаясь
			// поступления следующего пакета.
			break loop
		case frame, ok := <-packets:
			if !ok {
				builder.Flush()
				err = writeSample()
				break loop
			}
			var packet rtp.Packet
			if packet.Unmarshal(frame.RTP) != nil {
				continue
			}
			if !initialized {
				stamp = packet.Timestamp
				initialized = true
			}
			builder.Push(&packet)
			if err = writeSample(); err != nil {
				break loop
			}
		}
	}
	_ = writer.Close()
	_ = input.Close()
	if err != nil {
		cancel()
	}
	readErr := <-readDone
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	// Обычный EOF не отменяет процесс, поэтому ошибка с кодом завершения остается видимой.
	if waitErr != nil {
		return fmt.Errorf("live decoder failed")
	}
	return nil
}
