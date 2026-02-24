package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"naboo-audio/config"
	"naboo-audio/server"
	"naboo-audio/session"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create session manager
	sessionManager, err := session.NewSessionManager(cfg)
	if err != nil {
		log.Fatalf("Failed to create session manager: %v", err)
	}

	// Start cleanup routine
	ctx, cancel := context.WithCancel(context.Background())
	go sessionManager.StartCleanupRoutine(ctx)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	switch cfg.ServerType {
	case "websocket":
		srv := server.NewServerWebsocket(cfg, sessionManager)

		go func() {
			<-sigChan
			log.Println("\nReceived shutdown signal...")
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Server shutdown error: %v", err)
			}
		}()

		if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("Server error: %v", err)
		}

	case "twilio":
		twilioSrv := server.NewServerWebsocketTwilio(cfg, sessionManager)

		go func() {
			<-sigChan
			log.Println("\nReceived shutdown signal...")
			cancel()
			sessionManager.Shutdown()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := twilioSrv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Twilio server shutdown error: %v", err)
			}
		}()

		if err := twilioSrv.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("Twilio server error: %v", err)
		}

	case "both":
		srv := server.NewServerWebsocket(cfg, sessionManager)
		twilioSrv := server.NewServerWebsocketTwilio(cfg, sessionManager)

		go func() {
			<-sigChan
			log.Println("\nReceived shutdown signal...")
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Printf("WebSocket server shutdown error: %v", err)
			}
			if err := twilioSrv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Twilio server shutdown error: %v", err)
			}
		}()

		// Start Twilio server in background
		go func() {
			if err := twilioSrv.Start(); err != nil && err.Error() != "http: Server closed" {
				log.Fatalf("Twilio server error: %v", err)
			}
		}()

		// Start WebSocket server (blocks)
		if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("WebSocket server error: %v", err)
		}

	default:
		log.Fatalf("Unknown SERVER_TYPE: %s", cfg.ServerType)
	}

	log.Println("Server stopped")
}
