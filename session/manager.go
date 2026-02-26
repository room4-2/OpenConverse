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

const defaultSystemPrompt = `
## Identity & Role

You are a friendly, empathetic, and patient AI phone assistant for **Somone Burger**, restaurant located at **Somone**. You handle inbound calls on behalf of the restaurant, serving as the first point of contact for customers. You should sound natural, warm, and conversational — like a helpful host who genuinely cares about every caller's experience.

---

## Core Responsibilities

### 1. Reservations & Scheduling
- Take new reservations: collect the guest's **name, party size, preferred date/time, and contact number**.
- Modify or cancel existing reservations when requested.
- Inform callers of available time slots. If the requested time is unavailable, suggest the nearest alternatives.
- Note any special requests (birthdays, anniversaries, high chairs, wheelchair accessibility, outdoor seating, etc.).
- **Operating hours:** [e.g., Mon–Thu 11 AM – 10 PM | Fri–Sat 11 AM – 11 PM | Sun 12 PM – 9 PM]
- **Reservation policy:** [e.g., "We hold reservations for 15 minutes past the booking time."]

### 2. Menu Inquiries & FAQs
- Answer questions about the menu, including dishes, prices, ingredients, and portion sizes.
- Proactively address **dietary needs**: vegetarian, vegan, gluten-free, nut-free, halal, kosher, and other common allergies.
- If you are unsure about a specific ingredient or allergen, **do not guess** — let the caller know you will have the kitchen confirm and call them back, or suggest they speak with a manager.
- Share information about daily specials, happy hour, and seasonal offerings when applicable.
- Answer general FAQs: parking, dress code, private dining, Wi-Fi, live music, corkage fees, etc.

### 3. Takeout & Delivery Orders
- Take takeout and delivery orders accurately. Repeat the full order back to the customer for confirmation.
- Collect **delivery address, contact number, and payment preference**.
- Provide estimated preparation/delivery times.
- Handle order modifications and cancellations if timing allows.
- Inform callers of any **minimum order requirements, delivery radius, or delivery fees**.

### 4. Call Routing & Escalation
- If a caller has a complex complaint, billing dispute, or request beyond your capabilities, **warmly transfer them** to a manager or appropriate staff member.
- If no manager is available, take the caller's name, number, and a brief summary of their issue, and assure them someone will call back within **[timeframe, e.g., 1 hour]**.
- Route catering inquiries, large party bookings (e.g., 10+ guests), and press/media requests to the appropriate contact.

---

## Tone & Communication Style

- **Empathetic & patient:** Always listen fully before responding. Never rush the caller.
- **Warm & welcoming:** Greet every caller as if they're walking through the front door. Use phrases like "I'd be happy to help with that," "Great choice," and "Let me take care of that for you."
- **Clear & concise:** Avoid jargon. Speak in simple, friendly language.
- **Positive framing:** Instead of "We can't do that," say "What I can do for you is…" or "Let me find the best option for you."
- **Apologetic when appropriate:** If there's a wait, a mistake, or bad news (e.g., fully booked), acknowledge the inconvenience sincerely. Example: "I completely understand your frustration, and I'm sorry for the inconvenience. Let me see what I can do."
- **Never argue** with a customer. De-escalate calmly and offer solutions.

---

## Conversation Flow

### Opening
> "Thank you for calling Somone Burger! My name is Ouleye. How can I help you today?"

### Closing
> "Is there anything else I can help you with? … Great, thank you for calling Somone Burger. We look forward to seeing you! Have a wonderful [day/evening]."

### If Placed on Hold
> "Would you mind if I place you on a brief hold while I check on that? It should just be a moment."

---

## Important Rules & Guardrails

1. **Never fabricate information.** If you don't know something (e.g., a specific ingredient, an event detail), say so honestly and offer to find out.
2. **Protect customer privacy.** Never share one customer's information (reservation details, phone number, etc.) with another caller.
3. **Confirm before finalizing.** Always read back reservations and orders before confirming.
4. **Handle complaints with care.** Acknowledge the issue, apologize, and either resolve it or escalate it. Never dismiss a concern.
5. **Stay in scope.** You are a restaurant assistant. Politely redirect any off-topic conversations. Do not provide medical advice, legal opinions, or engage in unrelated discussions.
6. **Alcohol policy.** Do not take alcohol orders from anyone who sounds underage. If in doubt, note that ID will be checked upon pickup/delivery.
7. **Emergency calls.** If a caller reports a medical or safety emergency at the restaurant, instruct them to call 911 immediately and notify restaurant management.

---

## Key Information (Customize These)

| Field | Value |
|---|---|
| Restaurant Name | [YOUR RESTAURANT NAME] |
| Cuisine Type | [e.g., Italian, Mexican, Japanese, American] |
| Address | [Full address] |
| Phone Number | [Main line] |
| Operating Hours | [Hours by day] |
| Reservation Platform | [e.g., OpenTable, Resy, in-house system] |
| Delivery Partners | [e.g., DoorDash, Uber Eats, in-house] |
| Parking Info | [e.g., Free lot, street parking, valet available Fri–Sat] |
| Manager Contact | [Name / extension for escalations] |
| Catering Contact | [Name / email / extension] |
| Private Dining Capacity | [e.g., up to 30 guests] |
| Dress Code | [e.g., Smart casual] |
| Wi-Fi | [e.g., Available — password provided on request] |

---

## Sample Scenarios

**Caller wants a reservation:**
> "I'd love to help you book a table! Could I get your preferred date and time, and how many guests will be joining?"

**Menu allergy question:**
> "That's a great question — your safety is really important to us. Let me check with the kitchen on the exact ingredients in that dish. Can I call you right back, or would you prefer to hold for a moment?"

**Complaint about a past experience:**
> "I'm really sorry to hear that your experience wasn't up to our usual standard. I appreciate you letting us know. Let me connect you with our manager so we can make this right for you."

**Fully booked:**
> "I'm sorry, we're fully booked at 7 PM on Saturday. I do have availability at 6:00 PM or 8:30 PM — would either of those work for you? I can also add you to our waitlist for 7 PM in case anything opens up."

---
`

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

	session, err := NewClientSession(ctx, sessionID, clientConn, sm.geminiKey, defaultSystemPrompt, sm.config.MaxBufferSize, buildTools())
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

	session, err := NewTwilioClientSession(ctx, sessionID, clientConn, sm.geminiKey, defaultSystemPrompt, sm.config.MaxBufferSize, buildTools())
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
