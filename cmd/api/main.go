package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

// main starts the API process and reports startup or execution failures.
func main() {
	if err := app.RunAPI(); err != nil {
		log.Fatalf("api: %v", err)
	}
}
