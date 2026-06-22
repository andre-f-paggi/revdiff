//go:build windows

package main

import (
	"fmt"
	"os"
)

// openTTY opens the controlling terminal for interactive key input. In stdin
// mode the diff payload arrives on os.Stdin, so bubbletea reads keys from the
// console instead. On Windows the console input handle is the CONIN$ pseudo
// device, opened read-write to match bubbletea's own openInputTTY().
func openTTY() (*os.File, error) {
	tty, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open CONIN$: %w", err)
	}
	return tty, nil
}
