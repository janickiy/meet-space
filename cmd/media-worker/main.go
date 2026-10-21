// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

// main запускает процесс медиаворкера и сообщает о сбоях при запуске или выполнении.
func main() {
	if err := app.RunMediaWorker(); err != nil {
		log.Fatalf("media-worker: %v", err)
	}
}
