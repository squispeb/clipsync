package main

// Board abstracts clipboard operations.
// SystemBoard implements this with the real clipboard.
// MockBoard implements this for testing.
type Board interface {
	ReadText() ([]byte, error)
	ReadImage() ([]byte, error)
	WriteText(data []byte) error
	WriteImage(data []byte) error
}
