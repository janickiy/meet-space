package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

// main запускает процесс API и сообщает о сбоях при запуске или выполнении.
func main() {
	if err := app.RunAPI(); err != nil {
		log.Fatalf("api: %v", err)
	}
}
