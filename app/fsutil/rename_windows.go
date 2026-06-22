//go:build windows

package fsutil

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Win32 error codes (stable ABI) signalling that another process holds the
// rename target open — almost always an antivirus scanner or a sibling
// revdiff for a few milliseconds. ERROR_ACCESS_DENIED is deliberately excluded:
// it is usually a real permission problem (and is what MoveFileEx returns when
// the destination is a directory), so retrying it would only add latency to
// genuine failures.
const (
	errorSharingViolation = syscall.Errno(32)
	errorLockViolation    = syscall.Errno(33)
)

// renameWithRetry renames oldpath to newpath, retrying briefly when Windows
// reports a transient sharing/lock violation. os.Rename maps to MoveFileEx with
// MOVEFILE_REPLACE_EXISTING, so replacing an existing file already works; the
// retries only cover the window where an AV scanner or sibling process holds
// the target open.
func renameWithRetry(oldpath, newpath string) error {
	const attempts = 10
	delay := time.Millisecond
	var err error
	for range attempts {
		err = os.Rename(oldpath, newpath)
		if err == nil || !isTransientRenameErr(err) {
			return err
		}
		time.Sleep(delay)
		if delay < 50*time.Millisecond {
			delay *= 2
		}
	}
	return err
}

func isTransientRenameErr(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, errorLockViolation)
}
