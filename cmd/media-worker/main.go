package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

// main starts the media worker process and reports startup or execution failures.
func main() {
	if err := app.RunMediaWorker(); err != nil {
		log.Fatalf("media-worker: %v", err)
	}
}
