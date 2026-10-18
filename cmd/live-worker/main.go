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
