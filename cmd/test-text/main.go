package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/room4-2/OpenConverse/gemini"
)

func main() {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY not set")
	}

	proxy, err := gemini.NewProxy(context.Background(), apiKey)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}
	defer proxy.Close()

	// Set up callbacks
	proxy.OnAudio = func(data []byte) {
		log.Printf("🔊 Received audio: %d bytes", len(data))
	}
	proxy.OnText = func(text string) {
		log.Printf("💬 Received text: %s", text)
	}
	proxy.OnComplete = func() {
		log.Println("✅ Turn complete")
	}
	proxy.OnError = func(err error) {
		log.Printf("❌ Error: %v", err)
	}

	// Setup session (no tools for this test)
	err = proxy.Setup(context.Background(), "You are a helpful assistant. Keep responses brief.", "gemini-2.5-flash-native-audio-preview-12-2025", "Zephyr", nil)
	if err != nil {
		log.Fatalf("Failed to setup: %v", err)
	}

	// Start receiving
	ctx := context.Background()
	proxy.StartReceiving(ctx)

	// Send a text message
	err = proxy.SendText("Hello! Say hi back in one sentence.")
	if err != nil {
		log.Fatalf("Failed to send text: %v", err)
	}

	// Wait for response
	log.Println("Waiting for response...")
	time.Sleep(10 * time.Second)
	log.Println("Done")
}
