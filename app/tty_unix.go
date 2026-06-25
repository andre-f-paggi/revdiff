//go:build !windows

package main

import (
	"fmt"
	"os"
)

// openTTY opens the controlling terminal for interactive key input. In stdin
// mode the diff payload arrives on os.Stdin, so bubbletea reads keys from the
// terminal device instead. On Unix that device is /dev/tty.
func openTTY() (*os.File, error) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return nil, fmt.Errorf("open /dev/tty: %w", err)
	}
	return tty, nil
}
