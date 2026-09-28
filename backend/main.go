package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/embedded"
)

// embeddedHostArg selects the packaged-binary entry point the Android app runs
// (ADR 0028 Decision 2). With it, the process reads a HostConfig JSON document
// from stdin — carrying the app-private paths and the Keystore-unwrapped JWT
// secret and at-rest master key — and serves the embedded deployment on a Unix
// socket, instead of loading configuration from the environment.
const embeddedHostArg = "--embedded-host"

// main is intentionally thin (issue #1257): it loads configuration from the
// environment, starts the server through the library entry point, blocks until
// SIGINT/SIGTERM, and stops it. Everything the server does on the way up and
// down lives in embedded.Start/Stop, which is also what an embedding host (the
// Android app, ADR 0028) calls with a programmatic Config instead of env vars.
func main() {
	if len(os.Args) > 1 && os.Args[1] == embeddedHostArg {
		runEmbeddedHost()
		return
	}

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

// runEmbeddedHost is the process entry point the Android host execs. It reads
// the host payload from stdin (never the environment — the secrets live there),
// then runs the embedded server until SIGINT/SIGTERM. Process.destroy() sends
// SIGTERM, which cancels the context and runs the same graceful Stop as a
// served shutdown. # pragma: no cover — process entry point; embedded.ReadHostConfig
// and embedded.RunHosted are tested directly.
func runEmbeddedHost() {
	hc, err := embedded.ReadHostConfig(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Announce readiness on stdout — read only by the parent app over its private
	// pipe, never a log and never a file — so the host can pick up the session
	// token the embedded server minted for its single local user. The marker key
	// (host_ready) is unique among the server's own structured log lines, which
	// also go to stdout.
	if err := embedded.RunHosted(ctx, hc, func(srv *embedded.Server) {
		handshake := struct {
			HostReady    bool   `json:"host_ready"`
			SessionToken string `json:"session_token"`
		}{HostReady: true, SessionToken: srv.LocalSessionToken()}
		// #nosec G117 -- the session token is deliberately handed to the parent
		// app over this process's private stdin pipe; it is never persisted or
		// logged, and stdout is read only by the host that started us.
		line, mErr := json.Marshal(handshake)
		if mErr != nil {
			log.Fatal(mErr)
		}
		if _, wErr := fmt.Fprintln(os.Stdout, string(line)); wErr != nil {
			log.Fatal(wErr)
		}
	}); err != nil {
		log.Fatal(err)
	}
}
