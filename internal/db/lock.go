// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import "errors"

// Locking
//
// Tables are locked with operating-system byte-range locks, so separate
// Nantucket sessions (on one machine or over a shared drive) see each
// other's locks. The locked bytes lie far beyond the end of any real file,
// as Clipper and Harbour do, so they never block reading or writing data.
//
//	openLock          held while a table is open: a read (shared) lock
//	                  for USE ... SHARED or READONLY, a write (exclusive)
//	                  lock for USE ... EXCLUSIVE
//	lockBase + n      record n, locked by RLock()
//	lockBase ..       the whole range, locked by FLock()
//	  lockBase+lockSpan-1
//	appendLock        held briefly while a record is added
//
// Index (.ntx) and memo files lock indexLock while they are being changed
// (and an index takes a read lock on it while being reloaded).
//
// On Linux the locks belong to the open file, not the process (open file
// description locks), so two work areas in one session lock independently,
// and closing some other handle on the same file never drops them.
const (
	lockBase   = 1_000_000_000
	lockSpan   = 1_000_000_000
	openLock   = 2_100_000_000
	appendLock = 2_100_000_001
	indexLock  = 1_000_000_000
)

// Errors for shared use.
var (
	// ErrInUse means the table is open elsewhere in a way that conflicts
	// (USE sets NetErr() rather than failing).
	ErrInUse = errors.New("file is in use by another user")
	// ErrLocked means a record or file lock is held by someone else.
	ErrLocked = errors.New("locked by another user")
	// ErrLockRequired means a shared table was changed without a lock.
	ErrLockRequired = errors.New("lock required")
	// ErrExclusiveRequired means the operation needs the table opened
	// EXCLUSIVE (PACK, ZAP, REINDEX...).
	ErrExclusiveRequired = errors.New("exclusive use required")
)

// OpenMode says how a table is opened.
type OpenMode int

const (
	OpenExclusive OpenMode = iota // only this work area may use it
	OpenShared                    // others may open it SHARED too
	OpenNoLock                    // look only (DIR, APPEND FROM a source)
)
