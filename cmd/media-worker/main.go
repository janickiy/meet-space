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
