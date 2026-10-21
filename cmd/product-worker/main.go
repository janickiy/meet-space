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

// main запускает рабочий процесс продукта и сообщает о сбоях при запуске или выполнении.
func main() {
	if app.RunProductWorker() != nil {
		slog.Error("product worker stopped", "event_type", "product.bootstrap.failed")
		os.Exit(1)
	}
}
