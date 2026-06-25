//go:build !windows

package main

import (
	"syscall"
	"testing"
)

// mkfifo creates a named pipe for the non-regular-file tests. It skips (rather
// than fails) when the platform does not support FIFOs.
func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
}
