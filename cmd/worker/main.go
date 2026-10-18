package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

// main запускает рабочий процесс и сообщает о сбоях при запуске или выполнении.
func main() {
	if err := app.RunWorker(); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
