// Command rockets-service consumes rocket messages and serves rocket state
// over a REST API.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"lunar-rockets/internal/httpapi"
	"lunar-rockets/internal/store"
)

const statsInterval = 2 * time.Second

func main() {
	addr := flag.String("addr", ":8088", "address to listen on")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(*addr, logger); err != nil {
		logger.Error("rockets-service stopped", "error", err)
		os.Exit(1)
	}
}

func run(addr string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st := store.New(logger)
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(st, logger),
		ReadHeaderTimeout: 5 * time.Second, // don't let slow clients hold connections open
	}
	srv.SetKeepAlivesEnabled(false) // close each connection after replying

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	logger.Info("listening", "addr", addr)
	go logStats(ctx, logger, st, statsInterval)

	select {
	case err := <-serveErr:
		return err // e.g. the port is already in use
	case <-ctx.Done():
	}

	// Finish in-flight requests before exiting: a message is only safe once
	// its 2xx has been sent (see PLAN.md §2).
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// logStats logs progress every interval until ctx is cancelled.
func logStats(ctx context.Context, logger *slog.Logger, st *store.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := st.Stats()
			logger.Info("stats",
				"messages", stats.Messages,
				"rockets", stats.Rockets,
				"goroutines", runtime.NumGoroutine())
		}
	}
}
