package messages

// Error codes
const (
	ErrCodeInvalidMessage   = "INVALID_MESSAGE"
	ErrCodeGeminiError      = "GEMINI_ERROR"
	ErrCodeSessionFailed    = "SESSION_FAILED"
	ErrCodeConnectionClosed = "CONNECTION_CLOSED"
	ErrCodeRateLimited      = "RATE_LIMITED"
	ErrCodeBufferFull       = "BUFFER_FULL"
)

// Message types
const (
	TypeAudio  = "audio"
	TypeText   = "text"
	TypeStatus = "status"
	TypeError  = "error"
)

// Media represents the audio payload for a Twilio message.
type Media struct {
	Payload string `json:"payload"` //  Base64-encoded mi-law audio data
}

// ServerMessage represents a message sent to frontend client.
type ServerMessage struct {
	Type      string      `json:"type"` // "audio", "text", "status", "error"
	SessionID string      `json:"sessionId,omitempty"`
	Payload   interface{} `json:"payload"`
}

// TwilioMessageBack represents the response message to be sent back to Twilio.
type TwilioMessageBack struct {
	Event     string `json:"event"`
	StreamSid string `json:"streamSid"`
	Media     Media  `json:"media"`
}

// AudioResponsePayload contains audio data for client
type AudioResponsePayload struct {
	Data     string `json:"data"`     // Base64-encoded PCM audio
	MimeType string `json:"mimeType"` // "audio/pcm;rate=24000"
}

// TextResponsePayload contains text response
type TextResponsePayload struct {
	Text string `json:"text"`
}

// StatusPayload contains status updates
type StatusPayload struct {
	Status  string `json:"status"` // "connected", "turn_complete", "disconnected"
	Message string `json:"message,omitempty"`
}

// ErrorPayload contains error information
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewTwilioMessageBack creates a new TwilioMessageBack with the provided stream SID and audio data.
func NewTwilioMessageBack(streamSid string, data string) *TwilioMessageBack {
	return &TwilioMessageBack{
		Event:     "media",
		StreamSid: streamSid,
		Media:     Media{Payload: data},
	}
}

// NewAudioMessage creates an audio response message
func NewAudioMessage(sessionID, data string) *ServerMessage {
	return &ServerMessage{
		Type:      TypeAudio,
		SessionID: sessionID,
		Payload: AudioResponsePayload{
			Data:     data,
			MimeType: "audio/pcm;rate=24000",
		},
	}
}

// NewTextMessage creates a text response message
func NewTextMessage(sessionID, text string) *ServerMessage {
	return &ServerMessage{
		Type:      TypeText,
		SessionID: sessionID,
		Payload: TextResponsePayload{
			Text: text,
		},
	}
}

// NewStatusMessage creates a status message
func NewStatusMessage(sessionID, status, message string) *ServerMessage {
	return &ServerMessage{
		Type:      TypeStatus,
		SessionID: sessionID,
		Payload: StatusPayload{
			Status:  status,
			Message: message,
		},
	}
}

// NewErrorMessage creates an error message
func NewErrorMessage(sessionID, code, message string) *ServerMessage {
	return &ServerMessage{
		Type:      TypeError,
		SessionID: sessionID,
		Payload: ErrorPayload{
			Code:    code,
			Message: message,
		},
	}
}
