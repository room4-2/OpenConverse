# OpenConverse

A real-time, open-source voice assistant server built in Go that bridges web clients and phone calls to Google's Gemini Live API. Deploy once, talk anywhere.

## What It Does

OpenConverse turns any WebSocket or Twilio phone call into a live voice conversation with a Gemini AI assistant. Audio flows in both directions in real time — the server handles all the complexity of format conversion, session management, and streaming so you don't have to.

- **Web clients** send 16kHz PCM audio over WebSocket and get 24kHz PCM responses back
- **Phone callers** (via Twilio) get the same AI, with automatic mu-law ↔ PCM conversion
- **Configurable system prompts** let you shape the AI's personality and domain

## Architecture

```
Web Browser ──── WebSocket (PCM 16kHz) ────┐
                                           │
                                    ┌──────▼──────────┐
                                    │  OpenConverse   │
                                    │  Go Server      │──── Gemini Live API
                                    │                 │
                                    └──────▲──────────┘
Phone Caller ──── Twilio ─── mu-law 8kHz ──┘
```

### Server Modes

| Mode | Port | Use Case |
|---|---|---|
| `websocket` (default) | 8080 | Web clients, browser-based frontends |
| `twilio` | 8081 | Phone calls via Twilio |
| `both` | 8080 + 8081 | Hybrid deployments |

## Quick Start

### Prerequisites

- Go 1.24.3+
- A [Google AI Studio](https://aistudio.google.com/) API key (Gemini)
- Redis (optional — for session persistence across restarts)

### Run

```bash
git clone https://github.com/room4-2/OpenConverse.git
cd OpenConverse

cp .env.example .env
# Edit .env and set your GEMINI_API_KEY

go run main.go
# Server starts at ws://localhost:8080/ws
```

### Build

```bash
go build -o openconverse main.go
./openconverse
```

## Configuration

All configuration is done via environment variables or a `.env` file in the project root.

| Variable | Default | Description |
|---|---|---|
| `GEMINI_API_KEY` | — | **Required.** Google AI API key |
| `SERVER_TYPE` | `websocket` | `websocket`, `twilio`, or `both` |
| `PORT` | `8080` | WebSocket server port |
| `TWILIO_PORT` | `8081` | Twilio server port (used when `SERVER_TYPE=both`) |
| `MAX_SESSIONS` | `100` | Maximum concurrent sessions |
| `SESSION_TIMEOUT` | `30` | Session timeout in minutes |
| `KEEPALIVE_PERIOD` | `30` | WebSocket ping interval in seconds |
| `MAX_BUFFER_SIZE` | `5242880` | Max audio buffer per session in bytes (5MB) |
| `ALLOWED_ORIGINS` | `*` | Comma-separated CORS allowed origins |
| `REDIS_URL` | `localhost:6379` | Redis address (optional) |
| `REDIS_PASSWORD` | — | Redis password (optional) |

**.env example:**
```env
GEMINI_API_KEY=your-key-here
SERVER_TYPE=websocket
PORT=8080
ALLOWED_ORIGINS=https://yourfrontend.com
```

## API Reference

### Endpoints

#### WebSocket Server
| Endpoint | Protocol | Description |
|---|---|---|
| `/ws` | WebSocket | Main voice session |
| `/health` | HTTP GET | Server health check |

#### Twilio Server
| Endpoint | Protocol | Description |
|---|---|---|
| `/stream` | WebSocket | Twilio media stream |
| `/voice` | HTTP GET | TwiML response (connect Twilio to `/stream`) |
| `/health` | HTTP GET | Server health check |

### WebSocket Message Protocol

#### Client → Server

**Send audio:**
```json
{
  "type": "audio",
  "payload": { "data": "<base64-encoded PCM 16kHz>" }
}
```

**Configure session:**
```json
{
  "type": "config",
  "payload": { "systemPrompt": "You are a helpful assistant..." }
}
```

**Signal end of speech turn:**
```json
{
  "type": "control",
  "payload": { "action": "end_turn" }
}
```

**Keepalive:**
```json
{
  "type": "control",
  "payload": { "action": "ping" }
}
```

#### Server → Client

**Audio response:**
```json
{
  "type": "audio",
  "sessionId": "uuid",
  "payload": {
    "data": "<base64-encoded PCM 24kHz>",
    "mimeType": "audio/pcm;rate=24000"
  }
}
```

**Text response:**
```json
{
  "type": "text",
  "sessionId": "uuid",
  "payload": { "text": "AI response text" }
}
```

**Status update:**
```json
{
  "type": "status",
  "sessionId": "uuid",
  "payload": { "status": "connected" | "turn_complete" | "disconnected" }
}
```

**Error:**
```json
{
  "type": "error",
  "sessionId": "uuid",
  "payload": { "code": "GEMINI_ERROR", "message": "details" }
}
```

## Audio Pipelines

### WebSocket Mode

Client captures microphone → encodes to 16kHz PCM → sends over WebSocket → server buffers → flushes to Gemini on `end_turn` → Gemini responds with 24kHz PCM → client decodes and plays.

### Twilio Mode

Phone caller speaks → Twilio sends mu-law 8kHz → server decodes and upsamples to 16kHz PCM → streams directly to Gemini (VAD handled by Gemini) → Gemini responds with 24kHz PCM → server downsamples to 8kHz → encodes to mu-law → sends to Twilio → played to caller.

## Frontend Integration (Next.js)

A typical Next.js frontend needs three things:

1. **WebSocket connection** to `ws://your-server/ws`
2. **Microphone capture** via `getUserMedia` (output: 16kHz PCM)
3. **Audio playback** from received 24kHz PCM chunks

Key libraries:
- [`@ricky0123/vad-web`](https://github.com/ricky0123/vad) — in-browser Voice Activity Detection
- Web Audio API — for encoding/decoding PCM

**Basic flow:**
1. VAD detects speech start → stream audio chunks to server
2. VAD detects silence → send `{ type: "control", payload: { action: "end_turn" } }`
3. Receive audio/text responses → play audio or display text

## Twilio Setup

1. Get a Twilio account and phone number
2. Configure the number's voice webhook:
   - URL: `https://your-domain.com/voice`
   - Method: `HTTP GET`
3. Start OpenConverse with `SERVER_TYPE=twilio` (or `both`)
4. Call your Twilio number — the call connects to Gemini

For local development, use [ngrok](https://ngrok.com/) to expose your local server:
```bash
ngrok http 8081
# Use the ngrok URL as your Twilio webhook
```

## Testing

### CLI Test Clients

```bash
# Full bidirectional audio test (requires ffmpeg + sox)
go run ./cmd/test/main.go

# Text-only test — no microphone needed
go run ./cmd/test-text/main.go
```

## Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o openconverse main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/openconverse .
EXPOSE 8080 8081
ENTRYPOINT ["./openconverse"]
```

```bash
docker build -t openconverse .
docker run -e GEMINI_API_KEY=your-key -p 8080:8080 openconverse
```

## Project Structure

```
openconverse/
├── main.go                  # Entry point
├── config/
│   └── config.go            # Config loading
├── server/
│   ├── websocket_server.go  # WebSocket HTTP server
│   └── twilio_server.go     # Twilio server + TwiML
├── session/
│   ├── session.go           # Per-connection session handler
│   ├── manager.go           # Session pool and lifecycle
│   ├── buffer.go            # Audio buffering
│   └── prompt.go            # System prompts
├── messages/
│   ├── client.go            # Client → server message types
│   └── server.go            # Server → client message types
├── gemini/
│   ├── proxy.go             # Gemini Live API wrapper
│   └── messages.go          # Gemini message structures
├── functions/
│   └── company_docs.go      # Example tool/function definition
└── cmd/
    ├── test/                # Full audio test client
    └── inspect/             # Inspetest-text/           # Text-only test client
```

## Contributing

Contributions are welcome. Please open an issue to discuss significant changes before submitting a pull request.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes
4. Push and open a pull request

## License

MIT License — see [LICENSE](LICENSE) for details.
