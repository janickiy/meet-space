// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package main

import (
	"log/slog"
	"os"

	"github.com/janickiy/meet-space/internal/app"
)

// main запускает процесс API и сообщает о сбоях при запуске или выполнении.
func main() {
	if app.RunLiveWorker() != nil {
		slog.Error("live worker stopped", "event_type", "live.bootstrap.failed")
		os.Exit(1)
	}
}
