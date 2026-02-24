package messages

import "encoding/json"

// ClientMessage represents a message from frontend client
type ClientMessage struct {
	Type    string          `json:"type"` // "audio", "config", "control"
	Payload json.RawMessage `json:"payload"`
}

// AudioPayload contains audio data from client
type AudioPayload struct {
	Data string `json:"data"` // Base64-encoded PCM audio
}

// ConfigPayload contains session configuration
type ConfigPayload struct {
	SystemPrompt string `json:"systemPrompt,omitempty"`
}

// ControlPayload contains control commands
type ControlPayload struct {
	Action string `json:"action"` // "ping", "end_turn"
}
