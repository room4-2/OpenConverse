package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"

	"google.golang.org/genai"
)

const (
	modelName = "models/gemini-2.5-flash-native-audio-preview-12-2025"
	//modelName = "models/gemini-3-flash-preview"
)

// Proxy manages the connection to Gemini Live API using the official SDK
type Proxy struct {
	client  *genai.Client
	session *genai.Session

	// Callbacks for handling responses
	OnAudio    func(data []byte)       // Decoded audio bytes
	OnAudioRaw func(base64Data string) // Raw base64 (avoids re-encoding)
	OnText     func(text string)
	OnComplete func()
	OnToolCall func(functionCalls []*genai.FunctionCall) // Tool/function calls from model
	OnError    func(err error)

	mu     sync.RWMutex
	closed bool
}

// NewProxy creates and connects to Gemini Live API
func NewProxy(ctx context.Context, apiKey string) (*Proxy, error) {
	// Initialize the Client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create GenAI client: %w", err)
	}

	return &Proxy{
		client: client,
	}, nil
}

// Setup establishes the Live session
func (gp *Proxy) Setup(ctx context.Context, systemPrompt string, tools []*genai.Tool) error {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if gp.closed {
		return fmt.Errorf("proxy is closed")
	}

	// Configure the Live Session
	config := &genai.LiveConnectConfig{
		ResponseModalities: []genai.Modality{"AUDIO"},
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemPrompt},
			},
		},
		Tools: tools,
		// Configure voice for TTS
		SpeechConfig: &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
					VoiceName: "Zephyr", // Available voices: Puck, Charon, Kore, Fenrir, Aoede, Leda, Orus, Zephyr
				},
			},
		},
	}

	// Connect to the Live API
	session, err := gp.client.Live.Connect(ctx, modelName, config)
	if err != nil {
		return fmt.Errorf("failed to connect to Live API: %w", err)
	}

	gp.session = session
	log.Printf("✅ Connected to Gemini Live via SDK (%s)", modelName)
	return nil
}

// StartReceiving begins listening for Gemini responses
func (gp *Proxy) StartReceiving(ctx context.Context) {
	go func() {
		defer func() {
			if gp.OnError != nil {
				gp.OnError(fmt.Errorf("gemini receiver closed"))
			}
		}()

		for {
			gp.mu.RLock()
			if gp.closed || gp.session == nil {
				gp.mu.RUnlock()
				return
			}
			session := gp.session
			gp.mu.RUnlock()

			// Receive blocks until a message arrives or error occurs
			resp, err := session.Receive()
			if err != nil {
				gp.mu.RLock()
				closed := gp.closed
				gp.mu.RUnlock()

				if !closed {
					log.Printf("❌ Gemini receive error: %v", err)
					if gp.OnError != nil {
						gp.OnError(err)
					}
				}
				return
			}

			gp.handleResponse(resp)
		}
	}()
}

func (gp *Proxy) handleResponse(resp *genai.LiveServerMessage) {
	// Handle Tool Calls
	if resp.ToolCall != nil && len(resp.ToolCall.FunctionCalls) > 0 {
		log.Printf("📥 Received from Gemini: %d function call(s)", len(resp.ToolCall.FunctionCalls))
		if gp.OnToolCall != nil {
			gp.OnToolCall(resp.ToolCall.FunctionCalls)
		}
	}

	// Handle Server Content
	if resp.ServerContent != nil {
		if resp.ServerContent.ModelTurn != nil {
			for _, part := range resp.ServerContent.ModelTurn.Parts {
				if part.Text != "" && gp.OnText != nil {
					log.Printf("📥 Received from Gemini: text '%s'", part.Text)
					gp.OnText(part.Text)
				}
				if part.InlineData != nil {
					// SDK provides raw bytes in InlineData.Data
					log.Printf("📥 Received from Gemini: %d bytes audio", len(part.InlineData.Data))

					if gp.OnAudioRaw != nil {
						encoded := base64.StdEncoding.EncodeToString(part.InlineData.Data)
						gp.OnAudioRaw(encoded)
					} else if gp.OnAudio != nil {
						gp.OnAudio(part.InlineData.Data)
					}
				}
			}
		}

		if resp.ServerContent.TurnComplete && gp.OnComplete != nil {
			log.Println("📥 Received from Gemini: turn complete")
			gp.OnComplete()
		}
	}
}

// SendAudio forwards an audio chunk to Gemini
func (gp *Proxy) SendAudio(audioData []byte) error {
	return gp.sendRealtimeInput(audioData)
}

// SendAudioBatch sends complete batched audio data to Gemini
func (gp *Proxy) SendAudioBatch(audioData []byte) error {
	if len(audioData) == 0 {
		return nil
	}

	// 1. Send Audio
	err := gp.sendRealtimeInput(audioData)
	if err != nil {
		return fmt.Errorf("failed to send audio batch: %w", err)
	}

	// 2. Send Turn Complete
	return gp.sendTurnComplete()
}

// SendAudioBase64 forwards base64-encoded audio to Gemini
func (gp *Proxy) SendAudioBase64(encodedAudio string) error {
	data, err := base64.StdEncoding.DecodeString(encodedAudio)
	if err != nil {
		return fmt.Errorf("invalid base64: %w", err)
	}
	return gp.sendRealtimeInput(data)
}

// SendText sends a text message to Gemini (useful for testing)
func (gp *Proxy) SendText(text string) error {
	gp.mu.RLock()
	session := gp.session
	closed := gp.closed
	gp.mu.RUnlock()

	if closed || session == nil {
		return fmt.Errorf("proxy is closed or not connected")
	}

	turnComplete := true
	err := session.SendClientContent(genai.LiveSendClientContentParameters{
		Turns: []*genai.Content{
			{
				Role:  "user",
				Parts: []*genai.Part{{Text: text}},
			},
		},
		TurnComplete: &turnComplete,
	})
	if err != nil {
		return fmt.Errorf("failed to send text: %w", err)
	}

	log.Printf("📤 Sent text to Gemini: %s", text)
	return nil
}

func (gp *Proxy) sendRealtimeInput(data []byte) error {
	gp.mu.RLock()
	session := gp.session
	closed := gp.closed
	gp.mu.RUnlock()

	if closed || session == nil {
		return fmt.Errorf("proxy is closed or not connected")
	}

	// Using Media field as identified via inspection
	err := session.SendRealtimeInput(genai.LiveRealtimeInput{
		Media: &genai.Blob{
			MIMEType: "audio/pcm;rate=16000",
			Data:     data,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to send audio: %w", err)
	}

	log.Printf("📤 Sent %d bytes audio to Gemini", len(data))
	return nil
}

func (gp *Proxy) sendTurnComplete() error {
	gp.mu.RLock()
	session := gp.session
	closed := gp.closed
	gp.mu.RUnlock()

	if closed || session == nil {
		return fmt.Errorf("proxy is closed or not connected")
	}

	// Signal to Gemini that the audio stream has ended
	// This triggers Gemini to process the accumulated audio and respond
	err := session.SendRealtimeInput(genai.LiveRealtimeInput{
		AudioStreamEnd: true,
	})
	if err != nil {
		return fmt.Errorf("failed to send audio stream end: %w", err)
	}

	log.Println("📤 Sent audio stream end to Gemini")
	return nil
}

// SendToolResponse sends function call responses back to Gemini
func (gp *Proxy) SendToolResponse(responses []*genai.FunctionResponse) error {
	gp.mu.RLock()
	session := gp.session
	closed := gp.closed
	gp.mu.RUnlock()

	if closed || session == nil {
		return fmt.Errorf("proxy is closed or not connected")
	}

	err := session.SendToolResponse(genai.LiveToolResponseInput{
		FunctionResponses: responses,
	})
	if err != nil {
		return fmt.Errorf("failed to send tool response: %w", err)
	}

	log.Printf("📤 Sent %d tool response(s) to Gemini", len(responses))
	return nil
}

// Close terminates the Gemini connection
func (gp *Proxy) Close() error {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if gp.closed {
		return nil
	}
	gp.closed = true

	if gp.session != nil {
		return gp.session.Close()
	}
	return nil
}
