// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// lockRange locks n bytes at off: exclusive is a write lock, otherwise a
// read lock. With wait it blocks until the lock is free; without, it
// returns ErrLocked straight away. Linux open file description locks are
// used (see lock.go).
func lockRange(f *os.File, off, n int64, exclusive, wait bool) error {
	lk := unix.Flock_t{Type: unix.F_RDLCK, Whence: 0, Start: off, Len: n}
	if exclusive {
		lk.Type = unix.F_WRLCK
	}
	cmd := unix.F_OFD_SETLK
	if wait {
		cmd = unix.F_OFD_SETLKW
	}
	for {
		err := unix.FcntlFlock(f.Fd(), cmd, &lk)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EACCES) {
			return ErrLocked
		}
		return err
	}
}

func unlockRange(f *os.File, off, n int64) error {
	lk := unix.Flock_t{Type: unix.F_UNLCK, Whence: 0, Start: off, Len: n}
	return unix.FcntlFlock(f.Fd(), unix.F_OFD_SETLK, &lk)
}
