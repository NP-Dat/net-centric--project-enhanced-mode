package main

import (
	"enhanced-tcr-udp/internal/server"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("Starting Enhanced TCR Server...")

	// Initialize the main server
	srv := server.NewServer("localhost:8080") // Use default or configure via env/args

	// Start the global UDP echo server (optional, for basic UDP tests)
	// This runs on a different port than game-specific UDP.
	go server.StartGlobalUDPEchoServer("localhost:8081")

	// Channel to listen for OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Goroutine to start the main TCP server
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server failed to start: %v", err)
			// If server Start exits, main might exit too, or we can handle restart logic.
			// For now, fatal error will stop the program.
		}
	}()

	log.Println("Server is running. Press Ctrl+C to exit.")

	// Wait for a signal
	<-sigChan

	// Signal received, initiate graceful shutdown
	log.Println("Shutdown signal received, stopping server...")
	srv.Stop()
	log.Println("Server stopped gracefully.")
}
