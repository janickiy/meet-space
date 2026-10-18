package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

// main starts the worker process and reports startup or execution failures.
func main() {
	if err := app.RunWorker(); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
