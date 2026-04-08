package main

import (
	"fmt"

	"golang.design/x/clipboard"
)

type SystemBoard struct{}

func NewSystemBoard() (*SystemBoard, error) {
	if err := clipboard.Init(); err != nil {
		return nil, fmt.Errorf("clipboard init failed: %w", err)
	}
	return &SystemBoard{}, nil
}

func (b *SystemBoard) ReadText() ([]byte, error) {
	data := clipboard.Read(clipboard.FmtText)
	if data == nil {
		return nil, fmt.Errorf("clipboard: no text content")
	}
	return data, nil
}

func (b *SystemBoard) ReadImage() ([]byte, error) {
	data := clipboard.Read(clipboard.FmtImage)
	if data == nil {
		return nil, fmt.Errorf("clipboard: no image content")
	}
	return data, nil
}

func (b *SystemBoard) WriteText(data []byte) error {
	clipboard.Write(clipboard.FmtText, data)
	return nil
}

func (b *SystemBoard) WriteImage(data []byte) error {
	clipboard.Write(clipboard.FmtImage, data)
	return nil
}
