package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all server configuration
type Config struct {
	Port            int
	TwilioPort      int    // Port for Twilio server (used when ServerType is "both")
	ServerType      string // "websocket", "twilio", or "both"
	RedisURL        string
	RedisPassword   string
	MaxSessions     int
	SessionTimeout  time.Duration
	GeminiAPIKey    string
	AllowedOrigins  []string
	KeepAlivePeriod time.Duration
	MaxBufferSize   int // Maximum audio buffer size in bytes per session
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() (*Config, error) {
	// Load .env file if it exists (doesn't error if missing)
	_ = godotenv.Load()

	config := &Config{
		Port:            8080,
		TwilioPort:      8081,
		ServerType:      "websocket",
		RedisURL:        "localhost:6379",
		RedisPassword:   "",
		MaxSessions:     100,
		SessionTimeout:  30 * time.Minute,
		AllowedOrigins:  []string{"*"},
		KeepAlivePeriod: 30 * time.Second,
		MaxBufferSize:   5 * 1024 * 1024, // 5MB default
	}

	// Required: GEMINI_API_KEY
	config.GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	if config.GeminiAPIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is required")
	}

	// Optional: PORT
	if port := os.Getenv("PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT: %w", err)
		}
		config.Port = p
	}

	// Optional: REDIS_URL
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		config.RedisURL = redisURL
	}

	// Optional: REDIS_PASSWORD
	if redisPassword := os.Getenv("REDIS_PASSWORD"); redisPassword != "" {
		config.RedisPassword = redisPassword
	}

	// Optional: MAX_SESSIONS
	if maxSessions := os.Getenv("MAX_SESSIONS"); maxSessions != "" {
		m, err := strconv.Atoi(maxSessions)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_SESSIONS: %w", err)
		}
		config.MaxSessions = m
	}

	// Optional: SESSION_TIMEOUT (in minutes)
	if timeout := os.Getenv("SESSION_TIMEOUT"); timeout != "" {
		t, err := strconv.Atoi(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid SESSION_TIMEOUT: %w", err)
		}
		config.SessionTimeout = time.Duration(t) * time.Minute
	}

	// Optional: ALLOWED_ORIGINS (comma-separated)
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		config.AllowedOrigins = strings.Split(origins, ",")
	}

	// Optional: KEEPALIVE_PERIOD (in seconds)
	if keepalive := os.Getenv("KEEPALIVE_PERIOD"); keepalive != "" {
		k, err := strconv.Atoi(keepalive)
		if err != nil {
			return nil, fmt.Errorf("invalid KEEPALIVE_PERIOD: %w", err)
		}
		config.KeepAlivePeriod = time.Duration(k) * time.Second
	}

	// Optional: MAX_BUFFER_SIZE (in bytes)
	if bufferSize := os.Getenv("MAX_BUFFER_SIZE"); bufferSize != "" {
		b, err := strconv.Atoi(bufferSize)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_BUFFER_SIZE: %w", err)
		}
		config.MaxBufferSize = b
	}

	// Optional: SERVER_TYPE ("websocket", "twilio", or "both")
	if serverType := os.Getenv("SERVER_TYPE"); serverType != "" {
		switch serverType {
		case "websocket", "twilio", "both":
			config.ServerType = serverType
		default:
			return nil, fmt.Errorf("invalid SERVER_TYPE: must be 'websocket', 'twilio', or 'both'")
		}
	}

	// Optional: TWILIO_PORT (used when SERVER_TYPE is "both")
	if twilioPort := os.Getenv("TWILIO_PORT"); twilioPort != "" {
		tp, err := strconv.Atoi(twilioPort)
		if err != nil {
			return nil, fmt.Errorf("invalid TWILIO_PORT: %w", err)
		}
		config.TwilioPort = tp
	}

	return config, nil
}
