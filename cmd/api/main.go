package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flag_backend/internal/bootstrap"
	apphttp "flag_backend/internal/platform/http"
)

func main() {
	ctx := context.Background()

	// Initialize application dependencies.
	app, err := bootstrap.New(ctx)
	if err != nil {
		log.Fatal("Application startup failed: ", err)
	}
	defer app.Close()

	server := apphttp.NewServer(":8080", apphttp.NewRouter(app.DB, app.Config))
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatal("HTTP server failed: ", err)
		}
	case <-stop:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server shutdown failed: ", err)
		}
	}
}
