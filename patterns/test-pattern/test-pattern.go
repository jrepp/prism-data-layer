package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Get configuration from environment
	patternName := os.Getenv("PATTERN_NAME")
	namespace := os.Getenv("NAMESPACE")
	sessionID := os.Getenv("SESSION_ID")
	grpcPort := os.Getenv("GRPC_PORT")
	healthPort := os.Getenv("HEALTH_PORT")

	log.Printf("Starting test pattern: pattern=%s, namespace=%s, session=%s, grpc=%s, health=%s",
		patternName, namespace, sessionID, grpcPort, healthPort)

	// Create HTTP server for health check
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	healthServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", healthPort),
		Handler: mux,
	}

	// Start health server in background
	go func() {
		log.Printf("Health server listening on :%s", healthPort)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Health server error: %v", err)
		}
	}()

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Test pattern running, waiting for shutdown signal...")
	<-sigCh

	log.Printf("Shutdown signal received, gracefully stopping...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := healthServer.Shutdown(ctx); err != nil {
		log.Printf("Health server shutdown error: %v", err)
	}

	log.Printf("Test pattern stopped")
}
