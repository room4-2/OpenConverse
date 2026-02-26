package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message types matching the server
type ClientMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type AudioPayload struct {
	Data string `json:"data"`
}

type ServerMessage struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

type AudioResponsePayload struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

type TextResponsePayload struct {
	Text string `json:"text"`
}

type StatusPayload struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// AudioPlayer streams audio via sox
type AudioPlayer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	closed bool
}

func NewAudioPlayer() *AudioPlayer {
	cmd := exec.Command("sox",
		"-t", "raw",
		"-r", "24000",
		"-b", "16",
		"-c", "1",
		"-e", "signed-integer",
		"-",
		"-d",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Println("sox stdin error:", err)
		return nil
	}

	if err := cmd.Start(); err != nil {
		log.Println("sox start error:", err)
		return nil
	}

	return &AudioPlayer{cmd: cmd, stdin: stdin}
}

func (p *AudioPlayer) Play(audioData []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.stdin == nil {
		return
	}
	_, _ = p.stdin.Write(audioData)
}

func (p *AudioPlayer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Wait()
	}
}

func main() {
	// Flags
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	audioFile := flag.String("file", "examples/user.pcm", "Audio file to send (PCM or WAV)")
	flag.Parse()

	log.Printf("🔌 Connecting to %s...", *serverURL)

	// Connect to server
	conn, _, err := websocket.DefaultDialer.Dial(*serverURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	log.Println("✅ Connected!")

	// Setup audio player
	player := NewAudioPlayer()
	if player == nil {
		log.Fatal("Failed to create audio player (is sox installed?)")
	}
	defer player.Close()

	// Handle interrupt
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	done := make(chan struct{})

	// Read responses from server
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return
			}

			var msg ServerMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Println("Parse error:", err)
				continue
			}

			switch msg.Type {
			case "audio":
				var payload AudioResponsePayload
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					log.Println("Parse audio payload error:", err)
					continue
				}
				audioBytes, err := base64.StdEncoding.DecodeString(payload.Data)
				if err == nil {
					log.Printf("🔊 Playing audio: %d bytes", len(audioBytes))
					player.Play(audioBytes)
				}

			case "text":
				var payload TextResponsePayload
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					log.Println("Parse text payload error:", err)
					continue
				}
				fmt.Printf("📝 %s\n", payload.Text)

			case "status":
				var payload StatusPayload
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					log.Println("Parse status payload error:", err)
					continue
				}
				log.Printf("📊 Status: %s %s", payload.Status, payload.Message)
				if payload.Status == "turn_complete" {
					log.Println("--- Turn complete ---")
				}

			case "error":
				log.Printf("❌ Error: %s", string(msg.Payload))
			}
		}
	}()

	// Wait for connected status
	time.Sleep(500 * time.Millisecond)

	// Load and send audio file
	log.Printf("📤 Sending audio file: %s", *audioFile)

	audioData, err := loadAudioFile(*audioFile)
	if err != nil {
		log.Fatalf("Failed to load audio: %v", err)
	}

	// Send audio in chunks (simulating real-time streaming)
	chunkSize := 3200 // 100ms at 16kHz
	for i := 0; i < len(audioData); i += chunkSize {
		end := i + chunkSize
		if end > len(audioData) {
			end = len(audioData)
		}
		chunk := audioData[i:end]

		// Send as binary (more efficient)
		if err := conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
			log.Printf("Send error: %v", err)
			break
		}

		log.Printf("📤 Sent chunk %d/%d (%d bytes)", i/chunkSize+1, (len(audioData)+chunkSize-1)/chunkSize, len(chunk))

		// Simulate real-time streaming pace
		time.Sleep(100 * time.Millisecond)
	}

	log.Println("✅ Audio sent, waiting for response...")

	// Wait for response or interrupt
	select {
	case <-done:
		log.Println("Connection closed")
	case <-interrupt:
		log.Println("\n👋 Interrupted, closing...")
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	case <-time.After(30 * time.Second):
		log.Println("⏰ Timeout waiting for response")
	}
}

// loadAudioFile loads PCM or WAV file and returns raw PCM bytes
func loadAudioFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Check if it's a WAV file (starts with "RIFF")
	if len(data) > 44 && string(data[0:4]) == "RIFF" {
		// Skip WAV header (44 bytes for standard WAV)
		log.Println("📁 Detected WAV file, skipping header")
		return data[44:], nil
	}

	// Assume raw PCM
	log.Println("📁 Detected raw PCM file")
	return data, nil
}
