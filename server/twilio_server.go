package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"naboo-audio/config"
	"naboo-audio/session"

	"github.com/gorilla/websocket"
)

type WebsocketTwilio struct {
	httpServer     *http.Server
	upgrader       websocket.Upgrader
	sessionManager *session.Manager
	config         *config.Config
}

func NewWebsocketTwilio(cfg *config.Config, sessionManager *session.Manager) *WebsocketTwilio {
	s := &WebsocketTwilio{
		sessionManager: sessionManager,
		config:         cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  64 * 1024,
			WriteBufferSize: 64 * 1024,
			// Twilio doesn't support WebSocket compression
			EnableCompression: false,
			CheckOrigin: func(r *http.Request) bool {
				// Twilio connections don't send browser Origin headers.
				// Allow all origins for the Twilio server.
				return true
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", s.handleWebsocketTwilio)
	mux.HandleFunc("/voice", s.handleVoiceCall)
	mux.HandleFunc("/health", s.handleHealth)

	// Determine which port to use
	port := cfg.TwilioPort
	if cfg.ServerType == "twilio" {
		// When running as standalone Twilio server, use the main port
		port = cfg.Port
	}

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
		// No ReadTimeout/WriteTimeout — these interfere with long-lived WebSocket connections.
		// The WebSocket layer handles its own timeouts via SetWriteDeadline/SetReadDeadline.
	}

	return s
}

// Start begins listening for connections
func (s *WebsocketTwilio) Start() error {
	port := s.httpServer.Addr
	log.Printf("📞 Twilio WebSocket server starting on %s", port)
	log.Printf("📡 Twilio stream endpoint: ws://localhost%s/stream", port)
	log.Printf("📡 Twilio voice endpoint: http://localhost%s/voice", port)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *WebsocketTwilio) Shutdown(ctx context.Context) error {
	log.Println("Shutting down Twilio server...")
	return s.httpServer.Shutdown(ctx)
}

func (s *WebsocketTwilio) handleWebsocketTwilio(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Twilio WebSocket upgrade failed: %v", err)
		return
	}

	// Create Twilio-specific session
	clientSession, err := s.sessionManager.CreateTwilioSession(r.Context(), conn)
	if err != nil {
		log.Printf("Failed to create Twilio session: %v", err)
		conn.Close()
		return
	}

	log.Printf("📞 New Twilio session created: %s", clientSession.ID)

	// Start Twilio session (uses Twilio-specific message handler)
	clientSession.StartTwilio()

	// Wait for session to close
	<-clientSession.CloseChan

	// Clean up
	_ = s.sessionManager.RemoveSession(clientSession.ID)
	log.Printf("📞 Twilio session closed: %s", clientSession.ID)
}

func (s *WebsocketTwilio) handleVoiceCall(w http.ResponseWriter, r *http.Request) {
	wsURL := "wss://" + r.Host + "/stream"

	// TwiML to connect the call to the WebSocket stream
	xmlResponse := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
	<Say>Connecting to the assistant now.</Say>
	<Connect>
		<Stream url="%s" />
	</Connect>
</Response>`, wsURL)

	w.Header().Set("Content-Type", "text/xml")
	_, _ = w.Write([]byte(xmlResponse))
}

func (s *WebsocketTwilio) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","server":"twilio","sessions":%d}`, s.sessionManager.GetActiveSessionCount())
}

// GetAddr returns the server's listen address (for logging in main)
func (s *WebsocketTwilio) GetAddr() string {
	return s.httpServer.Addr
}
