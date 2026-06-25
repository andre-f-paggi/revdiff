//go:build !windows

package review

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// mkfifo creates a named pipe for the non-regular-file tests. It skips (rather
// than fails) when the platform does not support FIFOs.
func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
}

// symlink creates a symbolic link for the workdir-safety tests.
func symlink(t *testing.T, oldname, newname string) {
	t.Helper()
	require.NoError(t, os.Symlink(oldname, newname))
}
