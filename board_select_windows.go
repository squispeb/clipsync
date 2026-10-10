package main

func NewBoard() (Board, error) {
	return NewSystemBoard()
}

func boardRequirement() (desc string, ok bool, hint string) {
	return "Windows clipboard", true, ""
}
