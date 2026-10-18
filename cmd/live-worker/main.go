package main

import (
	"github.com/janickiy/meet-space/internal/app"
	"log/slog"
	"os"
)

// main starts the live worker process and reports startup or execution failures.
func main() {
	if app.RunLiveWorker() != nil {
		slog.Error("live worker stopped", "event_type", "live.bootstrap.failed")
		os.Exit(1)
	}
}
