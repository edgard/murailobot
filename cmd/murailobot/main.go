// Package main is the entry point for the MurailoBot Telegram bot application.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edgard/murailobot/internal/app"
)

func main() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing application: %v\n", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)

	go func() {
		if err := application.Start(errCh); err != nil {
			errCh <- err
		}
	}()

	var exitCode int
	select {
	case sig := <-quit:
		fmt.Printf("Received shutdown signal: %v\n", sig)

		// Create a context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := application.Stop(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
			exitCode = 1
		}
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)

		// Create a context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := application.Stop(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown after application error: %v\n", err)
		}

		exitCode = 1
	}

	os.Exit(exitCode)
}
