//go:build !windows

package fsutil

import "os"

// renameWithRetry renames oldpath to newpath. On non-Windows platforms
// os.Rename is atomic and does not hit the transient sharing-violation races
// that Windows does, so this is a thin passthrough.
func renameWithRetry(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
