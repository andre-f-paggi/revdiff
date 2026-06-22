//go:build windows

package main

import "testing"

// mkfifo is unavailable on Windows; the FIFO/non-regular-file tests skip.
func mkfifo(t *testing.T, _ string) {
	t.Helper()
	t.Skip("FIFOs are not supported on Windows")
}
