package openai

// --- Client Events (sent to OpenAI) ---

type SessionUpdate struct {
	Type    string  `json:"type"`
	Session Session `json:"session"`
	EventID string  `json:"event_id"`
}

type Session struct {
	Instructions     string           `json:"instructions"`
	Voice            string           `json:"voice,omitempty"`
	InputAudioFormat string           `json:"input_audio_format,omitempty"`
	OutputAudioFormat string          `json:"output_audio_format,omitempty"`
	TurnDetection    map[string]any   `json:"turn_detection,omitempty"`
	Tools            []map[string]any `json:"tools"`
	ToolChoice       string           `json:"tool_choice"`
}

type InputAudioBufferAppend struct {
	Type    string `json:"type"`
	EventID string `json:"event_id,omitempty"`
	Audio   string `json:"audio"`
}

type ConversationItemCreate struct {
	Type    string                `json:"type"`
	EventID string               `json:"event_id,omitempty"`
	Item    ConversationItemParam `json:"item"`
}

type ConversationItemParam struct {
	Type    string        `json:"type"`
	ID      string        `json:"id,omitempty"`
	CallID  string        `json:"call_id,omitempty"`
	Role    string        `json:"role,omitempty"`
	Content []ContentPart `json:"content,omitempty"`
	Output  string        `json:"output,omitempty"`
}

type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

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

type ServerEventError struct {
	Type  string      `json:"type"`
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param"`
	EventID string `json:"event_id"`
}

type ServerEventSessionCreated struct {
	Type    string `json:"type"`
	Session any    `json:"session"`
}

type ServerEventResponseAudioDelta struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"` // Base64-encoded audio data
}

type ServerEventResponseAudioDone struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
}

type ServerEventResponseTextDelta struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type ServerEventResponseTextDone struct {
	Type         string `json:"type"`
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

type ServerEventResponseFunctionCallArgsDelta struct {
	Type        string `json:"type"`
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Delta       string `json:"delta"`
}

type ServerEventResponseFunctionCallArgsDone struct {
	Type        string `json:"type"`
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
}

type ServerEventResponseDone struct {
	Type     string           `json:"type"`
	Response ResponseObject   `json:"response"`
}

type ResponseObject struct {
	ID     string           `json:"id"`
	Status string           `json:"status"`
	Output []ResponseOutputItem `json:"output"`
}

type ResponseOutputItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`       // "message" or "function_call"
	Role      string `json:"role,omitempty"`
	Name      string `json:"name,omitempty"`      // function name
	CallID    string `json:"call_id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ServerEventResponseOutputItemDone struct {
	Type        string             `json:"type"`
	ResponseID  string             `json:"response_id"`
	OutputIndex int                `json:"output_index"`
	Item        ResponseOutputItem `json:"item"`
}
