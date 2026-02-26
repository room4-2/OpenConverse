package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"naboo-audio/config"
	"naboo-audio/messages"
	"naboo-audio/session"

	"github.com/gorilla/websocket"
)

type Server struct {
	httpServer     *http.Server
	upgrader       websocket.Upgrader
	sessionManager *session.Manager
	config         *config.Config
}

func NewServerWebsocket(cfg *config.Config, sessionManager *session.Manager) *Server {
	s := &Server{
		sessionManager: sessionManager,
		config:         cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:    64 * 1024, // 64KB for audio chunks
			WriteBufferSize:   64 * 1024, // 64KB for audio chunks
			EnableCompression: true,
			CheckOrigin: func(r *http.Request) bool {
				// Check allowed origins
				origin := r.Header.Get("Origin")
				for _, allowed := range cfg.AllowedOrigins {
					if allowed == "*" || allowed == origin {
						return true
					}
				}
				return false
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return s
}

// Start begins listening for connections
func (s *Server) Start() error {
	log.Printf("🚀 WebSocket server starting on port %d", s.config.Port)
	log.Printf("📡 WebSocket endpoint: ws://localhost:%d/ws", s.config.Port)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("🛑 Shutting down server...")
	s.sessionManager.Shutdown()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Create session
	clientSession, err := s.sessionManager.CreateSession(r.Context(), conn)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		// Send error and close
		errMsg := messages.NewErrorMessage("", messages.ErrCodeSessionFailed, err.Error())
		_ = conn.WriteJSON(errMsg)
		conn.Close()
		return
	}

	log.Printf("✅ New session created: %s", clientSession.ID)

	// Start session (handles messages in goroutines)
	clientSession.Start()

	// Wait for session to close
	<-clientSession.CloseChan

	// Clean up
	_ = s.sessionManager.RemoveSession(clientSession.ID)
	log.Printf("🔌 Session closed: %s", clientSession.ID)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","sessions":%d}`, s.sessionManager.GetActiveSessionCount())
}
