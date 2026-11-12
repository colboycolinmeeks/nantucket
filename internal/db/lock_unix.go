// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !linux

package db

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// lockRange locks n bytes at off (see lock_linux.go). Systems other than
// Linux use classic POSIX record locks, which belong to the process: two
// work areas in one session share them, and closing any handle on the
// file releases them.
func lockRange(f *os.File, off, n int64, exclusive, wait bool) error {
	lk := unix.Flock_t{Type: unix.F_RDLCK, Whence: 0, Start: off, Len: n}
	if exclusive {
		lk.Type = unix.F_WRLCK
	}
	cmd := unix.F_SETLK
	if wait {
		cmd = unix.F_SETLKW
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
	return unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lk)
}
