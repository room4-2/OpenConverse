package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/room4-2/OpenConverse/config"
	"github.com/room4-2/OpenConverse/functions"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"google.golang.org/genai"
)

// Manager manages all client sessions
type Manager struct {
	sessions  map[string]*ClientSession
	mu        sync.RWMutex
	redis     *redis.Client
	config    *config.Config
	geminiKey string
}

// NewManager creates a session manager with Redis connection
func NewManager(cfg *config.Config) (*Manager, error) {
	var redisClient *redis.Client

	// Try to connect to Redis, but don't fail if unavailable
	redisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.RedisURL,
		Password: cfg.RedisPassword,
		DB:       0,
	})

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		// Redis unavailable, continue without it
		redisClient = nil
	}

	return &Manager{
		sessions:  make(map[string]*ClientSession),
		redis:     redisClient,
		config:    cfg,
		geminiKey: cfg.GeminiAPIKey,
	}, nil
}

func buildTools() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				functions.GetCompanyInformationsDocsFunctionDeclaration(),
			},
		},
	}
}

// CreateSession creates a new client session
func (sm *Manager) CreateSession(ctx context.Context, clientConn *websocket.Conn) (*ClientSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(sm.sessions) >= sm.config.MaxSessions {
		return nil, fmt.Errorf("maximum sessions reached")
	}

	sessionID := uuid.New().String()

	session, err := NewClientSession(ctx, sessionID, clientConn, sm.geminiKey, DefaultSystemPrompt, sm.config.MaxBufferSize, buildTools())
	if err != nil {
		return nil, err
	}

	sm.storeSession(ctx, sessionID, session)
	return session, nil
}

// CreateTwilioSession creates a new Twilio voice call session
func (sm *Manager) CreateTwilioSession(ctx context.Context, clientConn *websocket.Conn) (*ClientSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(sm.sessions) >= sm.config.MaxSessions {
		return nil, fmt.Errorf("maximum sessions reached")
	}

	sessionID := uuid.New().String()

	session, err := NewTwilioClientSession(ctx, sessionID, clientConn, sm.geminiKey, DefaultSystemPrompt, sm.config.MaxBufferSize, buildTools())
	if err != nil {
		return nil, err
	}

	sm.storeSession(ctx, sessionID, session)
	return session, nil
}

// storeSession saves a session to memory and Redis
func (sm *Manager) storeSession(ctx context.Context, sessionID string, session *ClientSession) {
	sm.sessions[sessionID] = session

	if sm.redis != nil {
		sm.redis.HSet(ctx, "session:"+sessionID, map[string]interface{}{
			"created_at":    session.CreatedAt.Format(time.RFC3339),
			"last_activity": session.LastActivity.Format(time.RFC3339),
			"status":        "active",
			"is_twilio":     session.IsTwilio,
		})
		sm.redis.SAdd(ctx, "active_sessions", sessionID)
		sm.redis.Expire(ctx, "session:"+sessionID, sm.config.SessionTimeout)
	}
}

// GetSession retrieves a session by ID
func (sm *Manager) GetSession(sessionID string) (*ClientSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	return session, exists
}

// RemoveSession cleans up and removes a session
func (sm *Manager) RemoveSession(ctx context.Context, sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil
	}

	session.Close()
	delete(sm.sessions, sessionID)

	if sm.redis != nil {
		sm.redis.Del(ctx, "session:"+sessionID)
		sm.redis.SRem(ctx, "active_sessions", sessionID)
	}

	return nil
}

// GetActiveSessionCount returns current session count
func (sm *Manager) GetActiveSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// CleanupInactiveSessions removes sessions that have been inactive
func (sm *Manager) CleanupInactiveSessions(ctx context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for id, session := range sm.sessions {
		if now.Sub(session.LastActivity) > sm.config.SessionTimeout {
			session.Close()
			delete(sm.sessions, id)

			if sm.redis != nil {
				sm.redis.Del(ctx, "session:"+id)
				sm.redis.SRem(ctx, "active_sessions", id)
			}
		}
	}
}

// StartCleanupRoutine starts periodic cleanup of inactive sessions
func (sm *Manager) StartCleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.CleanupInactiveSessions(ctx)
		}
	}
}

// Shutdown closes all sessions
func (sm *Manager) Shutdown() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, session := range sm.sessions {
		session.Close()
		delete(sm.sessions, id)
	}

	if sm.redis != nil {
		sm.redis.Close()
	}
}
