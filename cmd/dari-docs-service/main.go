package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mupt-ai/dari-docs/internal/managedservice"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := managedservice.ConfigFromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := managedservice.Run(ctx, cfg); err != nil {
		log.Fatalf("service: %v", err)
	}
}
