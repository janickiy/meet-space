// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/janickiy/meet-space/internal/infrastructure/sfu"
	"github.com/janickiy/meet-space/internal/operations"
)

// hostname возвращает метку хоста процесса, не раскрывая конфиденциальные данные конфигурации.
//
// @returns:
//   - result1: ограниченная метка хоста процесса.
func hostname() string {
	value, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return value
}

// runProfiling запускает прослушиватель ограниченного профилирования, если диагностика явно включена.
//
// @Parameters:
//   - ctx: контекст, управляющий отменой, дедлайнами и временем жизни операций.
//   - ops: операции процесса и среда выполнения готовности.
func runProfiling(ctx context.Context, ops *operations.Runtime) {
	if err := ops.Profiling(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("profiling unavailable", "event_type", "profiling.failed")
	}
}

// sampleDatabase обновляет метрики пула подключений к ограниченной базе данных.
//
// @Parameters:
//   - ctx: контекст, управляющий отменой, сроками и временем жизни операции.
//   - db: дескриптор базы данных, используемый для операции или транзакции.
func sampleDatabase(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		stats := db.Stats()
		operations.State("db_open", float64(stats.OpenConnections))
		operations.State("db_in_use", float64(stats.InUse))
		operations.State("db_wait_total", float64(stats.WaitCount))
		operations.State("db_wait_seconds", stats.WaitDuration.Seconds())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sampleMedia обновляет ограниченные показатели активности медиа-воркера.
//
// @Parameters:
//   - ctx: контекст, управляющий отменой, дедлайнами и временем жизни операции.
//   - engine: медиа-движок, которому принадлежат комната и ресурсы пиров.
func sampleMedia(ctx context.Context, engine *sfu.Manager) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		s := engine.Snapshot()
		operations.State("media_relay_total", float64(s.RelayConnections))
		operations.State("media_direct_total", float64(s.DirectConnections))
		operations.State("audio_taps", float64(s.AudioTaps))
		operations.State("audio_tap_drops_total", float64(s.AudioTapDrops))
		for k, v := range map[string]float64{"media_rooms": float64(s.Rooms), "media_peers": float64(s.Peers), "media_tracks": float64(s.Tracks), "media_subscriptions": float64(s.Subscriptions), "media_connections_total": float64(s.PeerConnections), "media_failures_total": float64(s.Failures), "rtp_packets_total": float64(s.Packets), "rtp_bytes_total": float64(s.Bytes), "rtp_dropped_total": float64(s.Dropped), "recording_outputs": float64(s.RecordingOutputs), "recording_drops_total": float64(s.RecordingDrops)} {
			operations.State(k, v)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
