// Command kvstore starts the TCP key value server.
//
// Flags, wiring, and signal handling live here so the server package
// stays testable.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"kvstore/server"
	"kvstore/store"
)

func main() {
	addr := flag.String("addr", ":6380", "listen address")
	flag.Parse()

	srv := server.New(store.NewMemoryStore())
	if err := srv.Listen(*addr); err != nil {
		log.Fatal(err)
	}
	log.Printf("kvstore listening on %s", srv.Addr())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve() }()

	select {
	case err := <-errc:
		if err != nil {
			log.Fatal(err)
		}
	case <-ctx.Done():
		log.Print("shutting down")
	}

	// Five seconds is arbitrary but bounded, one hung client should not
	// keep the process alive forever.
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
