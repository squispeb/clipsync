package main

import "fmt"

type MockBoard struct {
	text     []byte
	image    []byte
	readErr  error
	writeErr error
	written  []byte
}

func (m *MockBoard) ReadText() ([]byte, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	if m.text == nil {
		return nil, fmt.Errorf("clipboard: no text content")
	}
	return m.text, nil
}

func (m *MockBoard) ReadImage() ([]byte, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	if m.image == nil {
		return nil, fmt.Errorf("clipboard: no image content")
	}
	return m.image, nil
}

func (m *MockBoard) WriteText(data []byte) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.text = data
	m.written = data
	return nil
}

func (m *MockBoard) WriteImage(data []byte) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.image = data
	m.written = data
	return nil
}
