//go:build windows

package main

func NewBoard() (Board, error) {
	return NewSystemBoard()
}
