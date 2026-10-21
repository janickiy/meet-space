// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package composite

import (
	"fmt"
	"github.com/janickiy/meet-space/internal/domain/media"
	"path/filepath"
	"strconv"
	"strings"
)

// audioArguments формирует аудиовходы и фильтры FFmpeg для составной временной шкалы.
//
// @Parameters:
//   - dir: Проверенный локальный каталог.
//   - chunk: Ограниченный входной фрагмент и его метаданные для временной шкалы.
//   - output: Целевой или сгенерированный результат, необходимый для выполнения этой операции.
//
// @returns:
//   - result1: Собранные строковые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Composer) audioArguments(dir string, chunk Chunk, output string) ([]string, error) {
	duration := strconv.FormatFloat(chunk.Duration, 'f', 6, 64)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-filter_complex_threads", "1"}
	sources := []Source{}
	for _, source := range chunk.Sources {
		if source.Track.Kind != media.KindAudio {
			continue
		}
		if filepath.Base(source.File) != source.File || source.File == "." {
			return nil, fmt.Errorf("invalid audio source")
		}
		args = append(args, "-threads", "1", "-i", filepath.Join(dir, "sources", source.File))
		sources = append(sources, source)
	}
	graph := []string{fmt.Sprintf("anullsrc=r=48000:cl=stereo,atrim=duration=%s[silence]", duration)}
	labels := "[silence]"
	for i, source := range sources {
		graph = append(graph, fmt.Sprintf("[%d:a]aresample=48000:async=1:first_pts=0,aformat=sample_fmts=fltp:channel_layouts=stereo,atrim=start=%.6f,asetpts=PTS-STARTPTS,adelay=%d:all=1,apad,atrim=duration=%s[a%d]", i, max(0, -source.Offset), int(max(0, source.Offset)*1000+.5), duration, i))
		labels += fmt.Sprintf("[a%d]", i)
	}
	graph = append(graph, fmt.Sprintf("%samix=inputs=%d:duration=first:dropout_transition=0:normalize=1,alimiter=limit=0.95:latency=1[aout]", labels, len(sources)+1))
	return append(args, "-filter_complex", strings.Join(graph, ";"), "-map", "[aout]", "-vn", "-t", duration, "-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2", "-threads", "1", "-movflags", "+faststart", output), nil
}
