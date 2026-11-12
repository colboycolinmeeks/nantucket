// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockRange locks n bytes at off with LockFileEx (see lock_linux.go).
// Windows locks belong to the file handle.
func lockRange(f *os.File, off, n int64, exclusive, wait bool) error {
	var flags uint32
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	ol := windows.Overlapped{Offset: uint32(off), OffsetHigh: uint32(off >> 32)}
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, uint32(n), uint32(n>>32), &ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return ErrLocked
	}
	return err
}

func unlockRange(f *os.File, off, n int64) error {
	ol := windows.Overlapped{Offset: uint32(off), OffsetHigh: uint32(off >> 32)}
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, uint32(n), uint32(n>>32), &ol)
}
