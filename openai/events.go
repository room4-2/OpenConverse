package openai

// --- Client Events (sent to OpenAI) ---

// SessionUpdate represents a session update event sent to OpenAI.
type SessionUpdate struct {
	Type    string  `json:"type"`
	Session Session `json:"session"`
	EventID string  `json:"event_id"`
}

// Session contains the configuration for an OpenAI realtime session.
type Session struct {
	Instructions      string           `json:"instructions"`
	Voice             string           `json:"voice,omitempty"`
	InputAudioFormat  string           `json:"input_audio_format,omitempty"`
	OutputAudioFormat string           `json:"output_audio_format,omitempty"`
	TurnDetection     map[string]any   `json:"turn_detection,omitempty"`
	Tools             []map[string]any `json:"tools"`
	ToolChoice        string           `json:"tool_choice"`
}

// InputAudioBufferAppend represents an event to append audio to the input buffer.
type InputAudioBufferAppend struct {
	Type    string `json:"type"`
	EventID string `json:"event_id,omitempty"`
	Audio   string `json:"audio"`
}

// ConversationItemCreate represents an event to create a new item in the conversation.
type ConversationItemCreate struct {
	Type    string                `json:"type"`
	EventID string                `json:"event_id,omitempty"`
	Item    ConversationItemParam `json:"item"`
}

// ConversationItemParam contains parameters for a conversation item.
type ConversationItemParam struct {
	Type    string        `json:"type"`
	ID      string        `json:"id,omitempty"`
	CallID  string        `json:"call_id,omitempty"`
	Role    string        `json:"role,omitempty"`
	Content []ContentPart `json:"content,omitempty"`
	Output  string        `json:"output,omitempty"`
}

// ContentPart represents a part of the content in a conversation item.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ResponseCreate represents an event to trigger a response from the model.
type ResponseCreate struct {
	Type    string `json:"type"`
	EventID string `json:"event_id,omitempty"`
}

// --- Server Events (received from OpenAI) ---

// ServerEvent is used to peek at the "type" field of incoming messages.
type ServerEvent struct {
	Type    string `json:"type"`
	EventID string `json:"event_id,omitempty"`
}

// ServerEventError represents an error event received from OpenAI.
type ServerEventError struct {
	Type  string      `json:"type"`
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains detailed information about an error from OpenAI.
type ErrorDetail struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param"`
	EventID string `json:"event_id"`
}

// ServerEventSessionCreated represents a session created event received from OpenAI.
type ServerEventSessionCreated struct {
	Type    string `json:"type"`
	Session any    `json:"session"`
}

// ServerEventResponseAudioDelta represents an audio delta event received from OpenAI.
type ServerEventResponseAudioDelta struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"` // Base64-encoded audio data
}

// ServerEventResponseAudioDone represents an audio done event received from OpenAI.
type ServerEventResponseAudioDone struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
}

// ServerEventResponseTextDelta represents a text delta event received from OpenAI.
type ServerEventResponseTextDelta struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// ServerEventResponseTextDone represents a text done event received from OpenAI.
type ServerEventResponseTextDone struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

// ServerEventResponseFunctionCallArgsDelta represents a function call arguments delta event.
type ServerEventResponseFunctionCallArgsDelta struct {
	Type        string `json:"type"`
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Delta       string `json:"delta"`
}

// ServerEventResponseFunctionCallArgsDone represents a function call arguments done event.
type ServerEventResponseFunctionCallArgsDone struct {
	Type        string `json:"type"`
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
}

// ServerEventResponseDone represents a response done event received from OpenAI.
type ServerEventResponseDone struct {
	Type     string         `json:"type"`
	Response ResponseObject `json:"response"`
}

// ResponseObject contains the result of a model response.
type ResponseObject struct {
	ID     string               `json:"id"`
	Status string               `json:"status"`
	Output []ResponseOutputItem `json:"output"`
}

// ResponseOutputItem represents an item in the model's output.
type ResponseOutputItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "message" or "function_call"
	Role      string `json:"role,omitempty"`
	Name      string `json:"name,omitempty"` // function name
	CallID    string `json:"call_id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ServerEventResponseOutputItemDone represents an output item done event.
type ServerEventResponseOutputItemDone struct {
	Type        string             `json:"type"`
	ResponseID  string             `json:"response_id"`
	OutputIndex int                `json:"output_index"`
	Item        ResponseOutputItem `json:"item"`
}
