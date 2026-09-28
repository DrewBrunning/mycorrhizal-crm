package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/embedded"
)

// main is intentionally thin (issue #1257): it loads configuration from the
// environment, starts the server through the library entry point, blocks until
// SIGINT/SIGTERM, and stops it. Everything the server does on the way up and
// down lives in embedded.Start/Stop, which is also what an embedding host (the
// Android app, ADR 0028) calls with a programmatic Config instead of env vars.
func main() {
	cfg := config.LoadConfig() // # pragma: no cover — process entry point; embedded.Start/Stop are tested directly

	srv, err := embedded.Start(context.Background(), cfg, embedded.Options{})
	if err != nil {
		log.Fatal(err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		log.Fatal(err)
	}
}
