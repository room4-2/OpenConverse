package session

import (
	"errors"
	"sync"
)

// ErrBufferFull is returned when the buffer exceeds its maximum size
var ErrBufferFull = errors.New("audio buffer full")

// AudioBuffer accumulates audio chunks until flushed
type AudioBuffer struct {
	chunks    [][]byte
	totalSize int
	maxSize   int
	mu        sync.Mutex
}

// NewAudioBuffer creates a buffer with the specified maximum size in bytes
func NewAudioBuffer(maxSize int) *AudioBuffer {
	return &AudioBuffer{
		chunks:  make([][]byte, 0),
		maxSize: maxSize,
	}
}

// MaxSize returns the maximum buffer size
func (ab *AudioBuffer) MaxSize() int {
	return ab.maxSize
}

// Append adds an audio chunk to the buffer
// Returns ErrBufferFull if adding the chunk would exceed maxSize
func (ab *AudioBuffer) Append(chunk []byte) error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	newSize := ab.totalSize + len(chunk)
	if newSize > ab.maxSize {
		return ErrBufferFull
	}

	ab.chunks = append(ab.chunks, chunk)
	ab.totalSize = newSize
	return nil
}

// Flush concatenates all chunks in order and clears the buffer
// Returns the complete audio data
func (ab *AudioBuffer) Flush() []byte {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if len(ab.chunks) == 0 {
		return nil
	}

	// Pre-allocate result slice for efficiency
	result := make([]byte, 0, ab.totalSize)
	for _, chunk := range ab.chunks {
		result = append(result, chunk...)
	}

	// Clear the buffer
	ab.chunks = make([][]byte, 0)
	ab.totalSize = 0

	return result
}

// Clear empties the buffer without returning data
func (ab *AudioBuffer) Clear() {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	ab.chunks = make([][]byte, 0)
	ab.totalSize = 0
}

// Size returns the current total buffered bytes
func (ab *AudioBuffer) Size() int {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	return ab.totalSize
}

// IsEmpty returns true if no chunks are buffered
func (ab *AudioBuffer) IsEmpty() bool {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	return len(ab.chunks) == 0
}

// ChunkCount returns the number of chunks in the buffer
func (ab *AudioBuffer) ChunkCount() int {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	return len(ab.chunks)
}
