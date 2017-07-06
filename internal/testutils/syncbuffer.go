package testutils

import (
	"bytes"
	"sync"
)

// SyncBuffer is a bytes.Buffer protected by a mutex.
type SyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Read implements io.Reader for SyncBuffer.
func (b *SyncBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Read(p)
}

// Write implements io.Writer for SyncBuffer.
func (b *SyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the buffer's data as a string.
func (b *SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Bytes returns the buffer's data as raw bytes.
func (b *SyncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Bytes()
}
