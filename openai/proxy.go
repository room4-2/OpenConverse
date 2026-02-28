package openai

import (
	"context"
	"fmt"
	"log"
	"sync"

	"encoding/base64"
	"net/http"
	"net/url"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"github.com/openai/openai-go/v3/responses"
	openai_function "github.com/room4-2/OpenConverse/functions/openai"
)

const (
	baseURL = "wss://api.openai.com/v1/realtime"
)

// Proxy handles communication between the server and OpenAI Realtime API.
type Proxy struct {
	openaiClient *websocket.Conn
	sessionID    string

	OnAudio    func(data []byte)       // Decoded audio bytes
	OnAudioRaw func(base64Data string) // Raw base64 (avoids re-encoding)
	OnText     func(text string)
	OnComplete func()
	OnToolCall func(functionCalls []*responses.ToolUnionParam) // Tool/function calls from model
	OnError    func(err error)

	mu     sync.RWMutex
	closed bool

	// Accumulate function call arguments across delta events
	funcCallArgs   map[string]*pendingFuncCall
	funcCallArgsMu sync.Mutex
}

// pendingFuncCall accumulates streamed function call data
type pendingFuncCall struct {
	CallID    string
	Name      string
	Arguments string
}

// NewProxy creates a new Proxy instance and establishes a connection to OpenAI.
func NewProxy(ctx context.Context, apiKey string, model string) (*Proxy, error) {
	headers := http.Header{}
	params := url.Values{}
	headers.Add("Authorization", "Bearer "+apiKey)
	headers.Add("OpenAI-Beta", "realtime=v1")
	u, _ := url.Parse(baseURL)
	params.Add("model", model)
	u.RawQuery = params.Encode()
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create websocket connection: %w", err)
	}
	return &Proxy{
		openaiClient: conn,
		funcCallArgs: make(map[string]*pendingFuncCall),
	}, nil
}

// AudioFormatType defines the audio encoding format for OpenAI realtime sessions.
type AudioFormatType string

const (
	// AudioFormatMuLaw is mu-law encoding at 8kHz — used for Twilio voice calls.
	AudioFormatMuLaw AudioFormatType = "audio/pcmu"
	// AudioFormatPCM is raw PCM encoding at 24kHz, 16-bit — used for regular WebSocket clients.
	AudioFormatPCM AudioFormatType = "audio/pcm"
)

// Setup configures the OpenAI realtime session with instructions, voice, and audio format.
func (op *Proxy) Setup(ctx context.Context, systemPrompt string, voice string, audioFormat AudioFormatType) error {
	op.mu.Lock()
	defer op.mu.Unlock()

	if op.closed {
		return fmt.Errorf("proxy is closed")
	}

	toolsOpenai := []responses.ToolUnionParam{{OfFunction: openai_function.GetHangUpFunction()}}
	var tools []map[string]any
	for _, toolOpenai := range toolsOpenai {
		toolByte, _ := toolOpenai.MarshalJSON()
		var tool map[string]any
		_ = sonic.Unmarshal(toolByte, &tool)
		tools = append(tools, tool)
	}

	// Map audio format to v1 API format strings
	var v1AudioFormat string
	switch audioFormat {
	case AudioFormatMuLaw:
		v1AudioFormat = "g711_ulaw"
	case AudioFormatPCM:
		v1AudioFormat = "pcm16"
	default:
		v1AudioFormat = "pcm16"
	}

	SessionInformation := Session{
		Instructions:      systemPrompt,
		Voice:             voice,
		InputAudioFormat:  v1AudioFormat,
		OutputAudioFormat: v1AudioFormat,
		TurnDetection:     map[string]any{"type": "semantic_vad", "eagerness": "high"},
		Tools:             tools,
		ToolChoice:        "auto",
	}

	payload := SessionUpdate{
		Type:    "session.update",
		Session: SessionInformation,
		EventID: op.sessionID,
	}
	return op.openaiClient.WriteJSON(payload)
}

// StartReceiving begins listening for OpenAI Realtime server events
func (op *Proxy) StartReceiving(ctx context.Context) {
	go func() {
		defer func() {
			if op.OnError != nil {
				op.OnError(fmt.Errorf("openai receiver closed"))
			}
		}()

		for {
			op.mu.RLock()
			if op.closed {
				op.mu.RUnlock()
				return
			}
			op.mu.RUnlock()

			_, msg, err := op.openaiClient.ReadMessage()
			if err != nil {
				op.mu.RLock()
				closed := op.closed
				op.mu.RUnlock()

				if !closed {
					log.Printf("❌ OpenAI receive error: %v", err)
					if op.OnError != nil {
						op.OnError(err)
					}
				}
				return
			}

			op.handleResponse(msg)
		}
	}()
}

func (op *Proxy) handleResponse(msg []byte) {
	// Peek at the event type
	var event ServerEvent
	if err := sonic.Unmarshal(msg, &event); err != nil {
		log.Printf("❌ Failed to parse OpenAI event: %v", err)
		return
	}

	switch event.Type {

	case "session.created":
		log.Println("📥 OpenAI: session created")

	case "session.updated":
		log.Println("📥 OpenAI: session updated")

	case "error":
		var errEvent ServerEventError
		_ = sonic.Unmarshal(msg, &errEvent)
		log.Printf("❌ OpenAI error: [%s] %s", errEvent.Error.Code, errEvent.Error.Message)
		if op.OnError != nil {
			op.OnError(fmt.Errorf("openai error [%s]: %s", errEvent.Error.Code, errEvent.Error.Message))
		}

	case "response.audio.delta":
		var audioDelta ServerEventResponseAudioDelta
		if err := sonic.Unmarshal(msg, &audioDelta); err != nil {
			log.Printf("❌ Failed to parse audio delta: %v", err)
			return
		}
		if audioDelta.Delta != "" {
			if op.OnAudioRaw != nil {
				op.OnAudioRaw(audioDelta.Delta)
			} else if op.OnAudio != nil {
				decoded, err := base64.StdEncoding.DecodeString(audioDelta.Delta)
				if err != nil {
					log.Printf("❌ Failed to decode audio base64: %v", err)
					return
				}
				op.OnAudio(decoded)
			}
		}

	case "response.audio.done":
		log.Println("📥 OpenAI: audio done")

	case "response.text.delta":
		var textDelta ServerEventResponseTextDelta
		if err := sonic.Unmarshal(msg, &textDelta); err != nil {
			log.Printf("❌ Failed to parse text delta: %v", err)
			return
		}
		if textDelta.Delta != "" && op.OnText != nil {
			op.OnText(textDelta.Delta)
		}

	case "response.text.done":
		var textDone ServerEventResponseTextDone
		if err := sonic.Unmarshal(msg, &textDone); err != nil {
			log.Printf("❌ Failed to parse text done: %v", err)
			return
		}
		log.Printf("📥 OpenAI: text done '%s'", textDone.Text)

	case "response.function_call_arguments.delta":
		var fcDelta ServerEventResponseFunctionCallArgsDelta
		if err := sonic.Unmarshal(msg, &fcDelta); err != nil {
			log.Printf("❌ Failed to parse function call args delta: %v", err)
			return
		}
		op.funcCallArgsMu.Lock()
		if _, ok := op.funcCallArgs[fcDelta.CallID]; !ok {
			op.funcCallArgs[fcDelta.CallID] = &pendingFuncCall{CallID: fcDelta.CallID}
		}
		op.funcCallArgs[fcDelta.CallID].Arguments += fcDelta.Delta
		op.funcCallArgsMu.Unlock()

	case "response.function_call_arguments.done":
		var fcDone ServerEventResponseFunctionCallArgsDone
		if err := sonic.Unmarshal(msg, &fcDone); err != nil {
			log.Printf("❌ Failed to parse function call args done: %v", err)
			return
		}
		log.Printf("📥 OpenAI: function call done: %s(%s)", fcDone.Name, fcDone.Arguments)

		// Clean up pending state
		op.funcCallArgsMu.Lock()
		delete(op.funcCallArgs, fcDone.CallID)
		op.funcCallArgsMu.Unlock()

	case "response.output_item.done":
		var itemDone ServerEventResponseOutputItemDone
		if err := sonic.Unmarshal(msg, &itemDone); err != nil {
			log.Printf("❌ Failed to parse output item done: %v", err)
			return
		}
		// If this is a function_call item, fire the OnToolCall callback
		if itemDone.Item.Type == "function_call" && op.OnToolCall != nil {
			fc := &responses.FunctionToolParam{
				Name: itemDone.Item.Name,
			}
			toolParam := responses.ToolUnionParam{OfFunction: fc}
			op.OnToolCall([]*responses.ToolUnionParam{&toolParam})
		}

	case "response.done":
		log.Println("📥 OpenAI: response done")
		if op.OnComplete != nil {
			op.OnComplete()
		}

	// Events we acknowledge but don't need to act on
	case "response.created",
		"response.output_item.added",
		"response.content_part.added",
		"response.content_part.done",
		"response.audio_transcript.delta",
		"response.audio_transcript.done",
		"conversation.created",
		"conversation.item.created",
		"conversation.item.input_audio_transcription.completed",
		"input_audio_buffer.committed",
		"input_audio_buffer.cleared",
		"input_audio_buffer.speech_started",
		"input_audio_buffer.speech_stopped",
		"rate_limits.updated":
		// Silently ignore these events

	default:
		log.Printf("📥 OpenAI: unhandled event type: %s", event.Type)
	}
}

// SendAudio forwards a base64-encoded audio chunk to OpenAI
func (op *Proxy) SendAudio(base64Audio string) error {
	op.mu.RLock()
	closed := op.closed
	op.mu.RUnlock()

	if closed {
		return fmt.Errorf("proxy is closed")
	}

	payload := InputAudioBufferAppend{
		Type:    "input_audio_buffer.append",
		EventID: op.sessionID,
		Audio:   base64Audio,
	}
	return op.openaiClient.WriteJSON(payload)
}

// SendAudioBytes forwards raw audio bytes (encodes to base64) to OpenAI
func (op *Proxy) SendAudioBytes(audioData []byte) error {
	encoded := base64.StdEncoding.EncodeToString(audioData)
	return op.SendAudio(encoded)
}

// SendText sends a text message by creating a conversation item
func (op *Proxy) SendText(text string) error {
	op.mu.RLock()
	closed := op.closed
	op.mu.RUnlock()

	if closed {
		return fmt.Errorf("proxy is closed")
	}

	// Create conversation item with user text
	payload := ConversationItemCreate{
		Type:    "conversation.item.create",
		EventID: op.sessionID,
		Item: ConversationItemParam{
			Type: "message",
			Role: "user",
			Content: []ContentPart{
				{Type: "input_text", Text: text},
			},
		},
	}
	if err := op.openaiClient.WriteJSON(payload); err != nil {
		return fmt.Errorf("failed to send text item: %w", err)
	}

	// Trigger a response
	resp := ResponseCreate{Type: "response.create", EventID: op.sessionID}
	if err := op.openaiClient.WriteJSON(resp); err != nil {
		return fmt.Errorf("failed to trigger response: %w", err)
	}

	log.Printf("📤 Sent text to OpenAI: %s", text)
	return nil
}

// SendToolResponse sends function call results back to OpenAI
func (op *Proxy) SendToolResponse(callID string, output string) error {
	op.mu.RLock()
	closed := op.closed
	op.mu.RUnlock()

	if closed {
		return fmt.Errorf("proxy is closed")
	}

	payload := ConversationItemCreate{
		Type:    "conversation.item.create",
		EventID: op.sessionID,
		Item: ConversationItemParam{
			Type:   "function_call_output",
			CallID: callID,
			Output: output,
		},
	}
	if err := op.openaiClient.WriteJSON(payload); err != nil {
		return fmt.Errorf("failed to send tool response: %w", err)
	}

	// Trigger a new response after providing the function output
	resp := ResponseCreate{Type: "response.create", EventID: op.sessionID}
	if err := op.openaiClient.WriteJSON(resp); err != nil {
		return fmt.Errorf("failed to trigger response after tool: %w", err)
	}

	log.Printf("📤 Sent tool response to OpenAI (call_id: %s)", callID)
	return nil
}

// Close terminates the OpenAI connection
func (op *Proxy) Close() error {
	op.mu.Lock()
	defer op.mu.Unlock()

	if op.closed {
		return nil
	}
	op.closed = true

	if op.openaiClient != nil {
		return op.openaiClient.Close()
	}
	return nil
}
