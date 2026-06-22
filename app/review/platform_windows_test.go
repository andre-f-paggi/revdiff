//go:build windows

package review

import "testing"

// mkfifo is unavailable on Windows; the FIFO/non-regular-file tests skip.
func mkfifo(t *testing.T, _ string) {
	t.Helper()
	t.Skip("FIFOs are not supported on Windows")
}

// symlink is skipped on Windows: creating symlinks needs elevated privileges
// and the resolution semantics these tests assert are POSIX-specific.
func symlink(t *testing.T, _, _ string) {
	t.Helper()
	t.Skip("symlinks require elevated privileges on Windows")
}
