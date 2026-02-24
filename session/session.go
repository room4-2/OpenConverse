package session

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"naboo-audio/functions"
	"naboo-audio/gemini"
	"naboo-audio/messages"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/genai"
)

var muLawToPcmTable [256]int16

const (
	writeBufferSize = 256
	writeTimeout    = 10 * time.Second
)

// ClientSession represents a single user's connection
type ClientSession struct {
	ID           string
	IsTwilio     bool   // Whether this is a Twilio voice call session
	StreamSid    string // Twilio stream SID (set on "start" event)
	ClientConn   *websocket.Conn
	GeminiProxy  *gemini.GeminiProxy
	AudioBuffer  *AudioBuffer // Buffer for incoming audio chunks
	CreatedAt    time.Time
	LastActivity time.Time

	// Use channels for non-blocking writes
	writeChan chan any

	mu        sync.RWMutex
	closed    bool
	CloseChan chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewClientSession creates a session with Gemini connection
func NewClientSession(id string, clientConn *websocket.Conn, geminiKey string, systemPrompt string, maxBufferSize int, tools []*genai.Tool) (*ClientSession, error) {
	proxy, err := gemini.NewGeminiProxy(geminiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini proxy: %w", err)
	}

	if err := proxy.Setup(systemPrompt, tools); err != nil {
		proxy.Close()
		return nil, fmt.Errorf("failed to setup Gemini session: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Configure WebSocket for better performance
	clientConn.SetReadLimit(512 * 1024) // 512KB max message
	clientConn.EnableWriteCompression(true)
	clientConn.SetCompressionLevel(6)

	session := &ClientSession{
		ID:           id,
		ClientConn:   clientConn,
		GeminiProxy:  proxy,
		AudioBuffer:  NewAudioBuffer(maxBufferSize),
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		writeChan:    make(chan any, writeBufferSize),
		CloseChan:    make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
	}

	return session, nil
}

// NewTwilioClientSession creates a session for Twilio voice calls
func NewTwilioClientSession(id string, clientConn *websocket.Conn, geminiKey string, systemPrompt string, maxBufferSize int, tools []*genai.Tool) (*ClientSession, error) {
	session, err := NewClientSession(id, clientConn, geminiKey, systemPrompt, maxBufferSize, tools)
	if err != nil {
		return nil, err
	}
	session.IsTwilio = true

	// Twilio doesn't support WebSocket compression
	clientConn.EnableWriteCompression(false)

	return session, nil
}

// Start begins the bidirectional message handling for standard WebSocket clients
func (cs *ClientSession) Start() {
	go cs.writePump()
	cs.setupGeminiCallbacks()
	cs.GeminiProxy.StartReceiving(cs.ctx)
	cs.queueMessage(messages.NewStatusMessage(cs.ID, "connected", "Session established"))
	go cs.handleClientMessages()
}

// StartTwilio begins the bidirectional message handling for Twilio voice calls
func (cs *ClientSession) StartTwilio() {
	go cs.writePump()
	cs.setupTwilioGeminiCallbacks()
	cs.GeminiProxy.StartReceiving(cs.ctx)
	go cs.handleClientMessagesFromTwilio()
}

// setupGeminiCallbacks configures callbacks for standard WebSocket clients
func (cs *ClientSession) setupGeminiCallbacks() {
	cs.GeminiProxy.OnAudioRaw = func(base64Data string) {
		cs.queueMessage(messages.NewAudioMessage(cs.ID, base64Data))
	}

	cs.GeminiProxy.OnText = func(text string) {
		cs.queueMessage(messages.NewTextMessage(cs.ID, text))
	}

	cs.GeminiProxy.OnComplete = func() {
		cs.queueMessage(messages.NewStatusMessage(cs.ID, "turn_complete", ""))
	}

	cs.setupGeminiErrorCallback()

	cs.GeminiProxy.OnToolCall = func(functionCalls []*genai.FunctionCall) {
		cs.handleToolCalls(functionCalls)
	}
}

// setupTwilioGeminiCallbacks configures callbacks for Twilio voice call sessions
func (cs *ClientSession) setupTwilioGeminiCallbacks() {
	cs.GeminiProxy.OnAudioRaw = func(base64Data string) {
		cs.mu.RLock()
		streamSid := cs.StreamSid
		cs.mu.RUnlock()

		if streamSid == "" {
			log.Printf("⚠️ [%s] Received audio from Gemini but no StreamSid set yet", cs.ID[:8])
			return
		}

		// Decode Gemini's PCM audio (24kHz, 16-bit, little-endian)
		pcmData, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			log.Printf("❌ [%s] Failed to decode base64 audio: %v", cs.ID[:8], err)
			return
		}

		// Downsample 24kHz -> 8kHz (take every 3rd sample) and convert PCM -> mu-law
		sampleCount := len(pcmData) / 2
		muLawData := make([]byte, 0, sampleCount/3+1)
		for i := 0; i < sampleCount; i += 3 {
			offset := i * 2
			if offset+1 >= len(pcmData) {
				break
			}
			sample := int16(binary.LittleEndian.Uint16(pcmData[offset : offset+2]))
			muLawData = append(muLawData, PcmToMuLawByte(sample))
		}

		// Send mu-law audio back to Twilio as base64
		encoded := base64.StdEncoding.EncodeToString(muLawData)
		cs.queueMessage(messages.NewTwilioMessageBack(streamSid, encoded))
	}

	cs.GeminiProxy.OnText = func(text string) {
		log.Printf("📝 [%s] Gemini text (Twilio session): %s", cs.ID[:8], text)
	}

	cs.GeminiProxy.OnComplete = func() {
		log.Printf("✅ [%s] Gemini turn complete (Twilio session)", cs.ID[:8])
	}

	cs.setupGeminiErrorCallback()

	cs.GeminiProxy.OnToolCall = func(functionCalls []*genai.FunctionCall) {
		cs.handleToolCalls(functionCalls)
	}
}

// setupGeminiErrorCallback sets up error handling common to both session types
func (cs *ClientSession) setupGeminiErrorCallback() {
	cs.GeminiProxy.OnError = func(err error) {
		log.Printf("❌ [%s] Gemini error: %v", cs.ID[:8], err)
		if !cs.IsTwilio {
			cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeGeminiError, err.Error()))
		}
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) ||
			websocket.IsUnexpectedCloseError(err) {
			log.Printf("🔌 [%s] Closing session due to Gemini connection error", cs.ID[:8])
			cs.Close()
		}
	}
}

// writePump handles all outgoing messages in a single goroutine
func (cs *ClientSession) writePump() {
	defer func() {
		// Send close message before exiting
		cs.ClientConn.SetWriteDeadline(time.Now().Add(writeTimeout))
		cs.ClientConn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
	}()

	for {
		select {
		case <-cs.CloseChan:
			return
		case msg, ok := <-cs.writeChan:
			if !ok {
				// Channel closed, exit gracefully
				return
			}

			cs.ClientConn.SetWriteDeadline(time.Now().Add(writeTimeout))

			if err := cs.ClientConn.WriteJSON(msg); err != nil {
				return
			}

			n := len(cs.writeChan)
			for i := 0; i < n; i++ {
				select {
				case msg, ok := <-cs.writeChan:
					if !ok {
						return
					}
					if err := cs.ClientConn.WriteJSON(msg); err != nil {
						return
					}
				default:
					// No more messages, continue outer loop
				}
			}
		}
	}
}

// queueMessage adds a message to the write queue (non-blocking)
func (cs *ClientSession) queueMessage(msg any) {
	cs.mu.RLock()
	closed := cs.closed
	cs.mu.RUnlock()
	if closed {
		return
	}
	select {
	case cs.writeChan <- msg:
		cs.mu.Lock()
		cs.LastActivity = time.Now()
		cs.mu.Unlock()
	default:
		// Queue full, drop message (shouldn't happen with proper sizing)
	}
}

// SendToClient sends a message to the frontend client (legacy, use queueMessage)
func (cs *ClientSession) SendToClient(msg *messages.ServerMessage) error {
	cs.queueMessage(msg)
	return nil
}

// Close terminates the session and cleans up resources
func (cs *ClientSession) Close() error {
	cs.mu.Lock()
	if cs.closed {
		cs.mu.Unlock()
		return nil
	}
	cs.closed = true
	cs.mu.Unlock()

	cs.cancel()

	// Close the write channel first to stop writePump
	close(cs.writeChan)

	// Signal close (for other goroutines waiting on this)
	close(cs.CloseChan)

	// Clear audio buffer
	if cs.AudioBuffer != nil {
		cs.AudioBuffer.Clear()
	}

	// Close Gemini connection
	if cs.GeminiProxy != nil {
		cs.GeminiProxy.Close()
	}

	// Close client connection - don't write close message as writePump is stopped
	if cs.ClientConn != nil {
		cs.ClientConn.Close()
	}

	return nil
}

// handleClientMessagesFromTwilio processes Twilio WebSocket protocol messages.
// Twilio sends: connected, start, media, stop events.
// Audio is streamed directly to Gemini (no buffering) — Gemini handles VAD.
func (cs *ClientSession) handleClientMessagesFromTwilio() {
	defer cs.Close()
	for {
		select {
		case <-cs.CloseChan:
			return
		default:
			_, message, err := cs.ClientConn.ReadMessage()
			if err != nil {
				if !cs.IsClosed() {
					log.Printf("❌ [%s] Twilio WebSocket read error: %v", cs.ID[:8], err)
				}
				return
			}

			cs.mu.Lock()
			cs.LastActivity = time.Now()
			cs.mu.Unlock()

			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("⚠️ [%s] Failed to parse Twilio message: %v", cs.ID[:8], err)
				continue
			}

			event, ok := msg["event"].(string)
			if !ok {
				log.Printf("⚠️ [%s] Twilio message missing 'event' field", cs.ID[:8])
				continue
			}

			switch event {
			case "connected":
				log.Printf("📞 [%s] Twilio stream connected", cs.ID[:8])

			case "start":
				startData, ok := msg["start"].(map[string]interface{})
				if !ok {
					log.Printf("⚠️ [%s] Twilio 'start' event missing start data", cs.ID[:8])
					continue
				}
				streamSid, ok := startData["streamSid"].(string)
				if !ok {
					log.Printf("⚠️ [%s] Twilio 'start' event missing streamSid", cs.ID[:8])
					continue
				}
				cs.mu.Lock()
				cs.StreamSid = streamSid
				cs.mu.Unlock()
				log.Printf("📞 [%s] Twilio stream started, StreamSid: %s", cs.ID[:8], streamSid)

			case "media":
				media, ok := msg["media"].(map[string]interface{})
				if !ok {
					continue
				}
				payloadStr, ok := media["payload"].(string)
				if !ok {
					continue
				}

				// Decode base64 mu-law audio from Twilio
				muLawData, err := base64.StdEncoding.DecodeString(payloadStr)
				if err != nil {
					log.Printf("⚠️ [%s] Failed to decode Twilio audio: %v", cs.ID[:8], err)
					continue
				}

				// Convert mu-law (8kHz) -> PCM (8kHz) -> upsample to PCM (16kHz) for Gemini
				pcmData := muLawToPCMUpsample(muLawData)

				// Stream directly to Gemini (no buffering — Gemini handles VAD)
				if err := cs.GeminiProxy.SendAudio(pcmData); err != nil {
					log.Printf("❌ [%s] Failed to send audio to Gemini: %v", cs.ID[:8], err)
				}

			case "stop":
				log.Printf("📞 [%s] Twilio stream stopped", cs.ID[:8])
				return

			case "mark":
				// Mark events are informational, ignore
				log.Printf("📞 [%s] Twilio mark event received", cs.ID[:8])

			default:
				log.Printf("⚠️ [%s] Unknown Twilio event: %s", cs.ID[:8], event)
			}
		}
	}
}

// muLawToPCMUpsample converts mu-law 8kHz audio to PCM 16kHz (16-bit LE) for Gemini
func muLawToPCMUpsample(muLawData []byte) []byte {
	// Each mu-law byte -> 1 PCM sample (8kHz)
	// Upsample 8kHz -> 16kHz by duplicating each sample
	// Output: 2 bytes per sample * 2 samples per input byte = 4 bytes per mu-law byte
	pcmData := make([]byte, len(muLawData)*4)
	for i, b := range muLawData {
		pcmVal := muLawToPcmTable[b]
		sample := make([]byte, 2)
		binary.LittleEndian.PutUint16(sample, uint16(pcmVal))
		// Write sample twice (duplicate for 8kHz -> 16kHz upsampling)
		copy(pcmData[i*4:i*4+2], sample)
		copy(pcmData[i*4+2:i*4+4], sample)
	}
	return pcmData
}

func (cs *ClientSession) handleClientMessages() {
	defer cs.Close()

	for {
		select {
		case <-cs.CloseChan:
			return
		default:
			messageType, message, err := cs.ClientConn.ReadMessage()
			if err != nil {
				return
			}

			cs.mu.Lock()
			cs.LastActivity = time.Now()
			cs.mu.Unlock()

			// Handle binary messages (raw PCM audio) - buffer instead of sending immediately
			if messageType == websocket.BinaryMessage {
				log.Printf("🎤 [%s] Buffering binary audio: %d bytes from client", cs.ID[:8], len(message))
				if err := cs.AudioBuffer.Append(message); err != nil {
					cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeBufferFull,
						fmt.Sprintf("Audio buffer full (max %d bytes)", cs.AudioBuffer.MaxSize())))
				}
				continue
			}

			// Handle text messages (JSON)
			var clientMsg messages.ClientMessage
			if err := json.Unmarshal(message, &clientMsg); err != nil {
				cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeInvalidMessage, "Invalid message format"))
				continue
			}

			cs.processClientMessage(&clientMsg)
		}
	}
}

func (cs *ClientSession) processClientMessage(msg *messages.ClientMessage) {
	switch msg.Type {
	case "audio":
		var payload messages.AudioPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeInvalidMessage, "Invalid audio payload"))
			return
		}
		// Decode base64 and buffer the audio
		audioBytes, err := base64.StdEncoding.DecodeString(payload.Data)
		if err != nil {
			cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeInvalidMessage, "Invalid base64 audio data"))
			return
		}
		log.Printf("🎤 [%s] Buffering JSON audio: %d bytes from client", cs.ID[:8], len(audioBytes))
		if err := cs.AudioBuffer.Append(audioBytes); err != nil {
			cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeBufferFull,
				fmt.Sprintf("Audio buffer full (max %d bytes)", cs.AudioBuffer.MaxSize())))
		}

	case "audio_binary":
		// Handle binary audio (more efficient)
		var payload messages.AudioPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return
		}
		// Decode and buffer
		audioBytes, err := base64.StdEncoding.DecodeString(payload.Data)
		if err != nil {
			return
		}
		cs.AudioBuffer.Append(audioBytes)

	case "control":
		var payload messages.ControlPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeInvalidMessage, "Invalid control payload"))
			return
		}
		cs.handleControlMessage(&payload)

	default:
		cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeInvalidMessage, "Unknown message type: "+msg.Type))
	}
}

func (cs *ClientSession) handleControlMessage(payload *messages.ControlPayload) {
	switch payload.Action {
	case "ping":
		cs.queueMessage(messages.NewStatusMessage(cs.ID, "pong", ""))
	case "end_turn":
		// Flush buffered audio and send to Gemini as a batch
		cs.handleEndTurn()
	default:
		cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeInvalidMessage, "Unknown control action: "+payload.Action))
	}
}

// handleEndTurn flushes the audio buffer and sends to Gemini
func (cs *ClientSession) handleEndTurn() {
	if cs.AudioBuffer.IsEmpty() {
		log.Printf("⚠️ [%s] end_turn received but buffer is empty, ignoring", cs.ID[:8])
		return
	}
	// Get chunk count before flushing (Flush clears the buffer)
	chunkCount := cs.AudioBuffer.ChunkCount()

	// Flush all buffered audio
	audioData := cs.AudioBuffer.Flush()
	log.Printf("📤 [%s] Sending batch audio to Gemini: %d bytes (%d chunks)", cs.ID[:8], len(audioData), chunkCount)

	if err := cs.GeminiProxy.SendAudioBatch(audioData); err != nil {
		log.Printf("❌ [%s] Failed to send audio to Gemini: %v", cs.ID[:8], err)
		cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeGeminiError, err.Error()))
	}
}

// IsClosed returns whether the session is closed
func (cs *ClientSession) IsClosed() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.closed
}

// handleToolCalls processes function calls from Gemini and sends responses
func (cs *ClientSession) handleToolCalls(functionCalls []*genai.FunctionCall) {
	var responses []*genai.FunctionResponse

	for _, fc := range functionCalls {
		log.Printf("🔧 [%s] Function call: %s (id: %s)", cs.ID[:8], fc.Name, fc.ID)

		var response map[string]any

		switch fc.Name {
		// Documentation function
		case "GetCompanyInformationsDocs":
			docs := functions.GetCompanyInformationsDocs()
			response = map[string]any{"output": docs}
			log.Printf("🔧 [%s] Returning company docs (%d chars)", cs.ID[:8], len(docs))

		default:
			response = map[string]any{"error": fmt.Sprintf("Unknown function: %s", fc.Name)}
			log.Printf("⚠️ [%s] Unknown function called: %s", cs.ID[:8], fc.Name)
		}

		responses = append(responses, &genai.FunctionResponse{
			ID:       fc.ID,
			Name:     fc.Name,
			Response: response,
		})
	}

	// Send all responses back to Gemini
	if err := cs.GeminiProxy.SendToolResponse(responses); err != nil {
		log.Printf("❌ [%s] Failed to send tool response: %v", cs.ID[:8], err)
		if !cs.IsTwilio {
			cs.queueMessage(messages.NewErrorMessage(cs.ID, messages.ErrCodeGeminiError, err.Error()))
		}
	}
}

func (cs *ClientSession) MuLawByteToPCMBytes(b byte) []byte {
	pcmVal := muLawToPcmTable[b]
	res := make([]byte, 2)
	binary.LittleEndian.PutUint16(res, uint16(pcmVal))
	return res
}

func init() {
	for i := 0; i < 256; i++ {
		muLawToPcmTable[i] = decodeMuLawByte(byte(i))
	}
}

// The Core Algorithm
// This logic is based on the Sun Microsystems G.711 reference implementation.
// ========================================================================
func decodeMuLawByte(uVal byte) int16 {
	// 1. Toggle bits (Mu-law definition requires inverting bits before processing)
	uVal = ^uVal

	// 2. Extract components
	// Sign bit (Mask 0x80)
	// Exponent (Mask 0x70)
	// Mantissa (Mask 0x0F)
	sign := uVal & 0x80
	exponent := (uVal >> 4) & 0x07
	mantissa := uVal & 0x0F

	// 3. Calculate sample location
	// The geometric bias for mu-law is 33 (0x21).
	// We shift the mantissa to align it, add the bias (132 or 0x84 due to alignment),
	// and then shift by the exponent.
	sample := int16((int32(mantissa)<<3 + 0x84) << exponent)

	// 4. Subtract the bias back out
	sample -= 0x84

	// 5. Apply the sign
	if sign != 0 {
		return -sample
	}
	return sample
}

func PcmToMuLawByte(pcm int16) byte {
	const (
		bias = 0x84 // 132
		clip = 32635
	)

	// 1. Get the sign bit
	sign := (pcm >> 8) & 0x80

	// 2. Magnitude (absolute value)
	if pcm < 0 {
		pcm = -pcm
	}

	// 3. Clip the magnitude
	if pcm > clip {
		pcm = clip
	}

	// 4. Add bias
	pcm += bias

	// 5. Calculate the exponent and mantissa
	exponent := 7
	// Move the exponent down until we find the highest bit
	for mask := 0x4000; (pcm&int16(mask)) == 0 && exponent > 0; mask >>= 1 {
		exponent--
	}

	mantissa := (pcm >> (exponent + 3)) & 0x0F

	// 6. Assemble the byte
	ulawByte := byte(sign | (int16(exponent) << 4) | mantissa)

	// 7. Invert bits (compressed format requirement)
	return ^ulawByte
}
