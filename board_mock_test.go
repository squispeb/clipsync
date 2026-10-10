package main

import (
	"fmt"
	"sync"
	"time"
)

// MockBoard is safe for concurrent use: the server writes received content
// from a goroutine.
type MockBoard struct {
	mu        sync.Mutex
	text      []byte
	image     []byte
	readErr   error
	writeErr  error
	written   []byte
	sensitive bool
}

func (m *MockBoard) ReadText() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readErr != nil {
		return nil, m.readErr
	}
	if m.text == nil {
		return nil, fmt.Errorf("clipboard: no text content")
	}
	return m.text, nil
}

func (m *MockBoard) ReadImage() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readErr != nil {
		return nil, m.readErr
	}
	if m.image == nil {
		return nil, fmt.Errorf("clipboard: no image content")
	}
	return m.image, nil
}

func (m *MockBoard) WriteText(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.text = data
	m.written = data
	return nil
}

func (m *MockBoard) WriteImage(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.image = data
	m.written = data
	return nil
}

func (m *MockBoard) Sensitive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sensitive
}

func (m *MockBoard) setText(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text = data
}

// waitWritten polls until the server's async clipboard write lands.
func (m *MockBoard) waitWritten() []byte {
	for i := 0; i < 200; i++ {
		m.mu.Lock()
		w := m.written
		m.mu.Unlock()
		if w != nil {
			return w
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}
