package main

import (
	"log/slog"
	"os"

	"github.com/janickiy/meet-space/internal/app"
)

// main starts the product worker process and reports startup or execution failures.
func main() {
	if app.RunProductWorker() != nil {
		slog.Error("product worker stopped", "event_type", "product.bootstrap.failed")
		os.Exit(1)
	}
}
