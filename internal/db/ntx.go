// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"nantucket/internal/value"
)

const ntxPage = 1024

type Entry struct {
	Key string
	Rec uint32
}

// Index is an NTX index held in memory as a sorted key list (see
// entryList) and written back as a Clipper compatible NTX B-tree.
type Index struct {
	Path    string
	KeyExpr string
	ForExpr string
	KeyLen  int
	KeyDec  int
	KeyType byte // C N D L
	Unique  bool
	Descend bool
	entries entryList
	dirty   bool

	// shared use (see Begin and Refresh)
	f       *os.File // kept open while the table is shared
	version uint16   // change counter at offset 2 of the header

	// consistency check (see the marker comment below)
	TableRecs int  // table record count stored with the index
	marked    bool // the header has our marker
	diskDirty bool // the unsaved-changes flag is set on disk
	locked    bool // Begin holds the index lock

	// the change log (see the journal comment below)
	pending  []journalOp // changes made in memory, not yet on disk
	needFull bool        // the keys were replaced: write the whole file
	jStart   int         // where the log starts in the file
	jSeen    int         // log entries on disk that the keys include

	// CP is the code page character keys are kept in (the table's), so
	// they sort and match like keys other programs write. Nil: as is.
	CP *CodePage

	// set by the VM
	KeyFn func() (value.Value, error)
	ForFn func() (bool, error)
}

var ErrBadIndex = errors.New("index file is corrupt or not an NTX file")

// ErrIndexStale means a shared index was left part way through a change
// by a session that stopped; it must be rebuilt (WorkArea does this).
var ErrIndexStale = errors.New("index was left out of date by another session")

// The marker. Nantucket keeps a few bytes in an unused part of the NTX
// header (Clipper and Harbour ignore bytes 551 to 1023):
//
//	1000  "NTK1"
//	1004  the table's record count when the index was saved (4 bytes)
//	1008  1 while the index has changes not yet saved, else 0
//	1012  where the change log starts in the file (4 bytes)
//	1016  the number of entries in the change log (4 bytes)
//	1020  the header's version word when Nantucket last wrote the index
//
// The flag is written before a change reaches the table and cleared when
// the index is saved, so an index whose flag is set, or whose record count
// doesn't match the table, may be missing keys: Nantucket was stopped
// before saving it, or records were added while it wasn't open. Such an
// index is rebuilt when it is opened (see Stale).
const (
	markerOff    = 1000
	markerMagic  = "NTK1"
	markerRecs   = 1004
	markerDirty  = 1008
	markerJStart = 1012
	markerJCount = 1016
	markerVer    = 1020
)

// The change log. Rewriting the whole index after every change costs time
// in proportion to its size, so changes are instead appended to a log
// after the index pages: one entry per key added ('I') or removed ('D'),
// holding the record number and the key. The entry count in the header is
// written after the entries, so it is the point at which they count; a
// half-written entry is ignored. Clipper and Harbour read an index by
// walking its pages from the root and never see the log. The index is
// written out whole, with an empty log, when it is closed (exclusive use)
// or when the log grows long.
type journalOp struct {
	op  byte
	rec uint32
	key string
}

func (ix *Index) journalSize() int { return 5 + ix.KeyLen }

// journalLimit is how long the log may grow before the whole index is
// written out again.
func journalLimit(keys int) int { return max(1024, keys/4) }

// Stale reports whether the index may not match a table of recs records,
// and why. Indexes without the marker (written by other programs) can't be
// checked and are assumed good.
func (ix *Index) Stale(recs int) (bool, string) {
	switch {
	case !ix.marked:
		return false, ""
	case ix.diskDirty:
		return true, "it was not saved after its last change"
	case ix.TableRecs != recs:
		return true, fmt.Sprintf("it was saved with %d records but the table has %d", ix.TableRecs, recs)
	}
	return false, ""
}

// LoadNTX reads an NTX file.
func LoadNTX(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseNTX(path, data)
}

// OpenNTX opens an index for a table opened with mode. For a shared table
// the file stays open, so that changes can be saved under a lock and
// other sessions' changes picked up (see Update and Refresh).
func OpenNTX(path string, mode OpenMode, readOnly bool) (*Index, error) {
	if mode != OpenShared {
		return LoadNTX(path)
	}
	flag := os.O_RDWR
	if readOnly {
		flag = os.O_RDONLY
	}
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return nil, err
	}
	if err := lockRange(f, indexLock, 1, false, true); err != nil {
		f.Close()
		return nil, err
	}
	data, err := readAll(f)
	unlockRange(f, indexLock, 1)
	if err != nil {
		f.Close()
		return nil, err
	}
	ix, err := parseNTX(path, data)
	if err != nil {
		f.Close()
		return nil, err
	}
	ix.f = f
	return ix, nil
}

func readAll(f *os.File) ([]byte, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data := make([]byte, st.Size())
	n, err := f.ReadAt(data, 0)
	if n == len(data) {
		err = nil
	}
	return data[:n], err
}

func parseNTX(path string, data []byte) (*Index, error) {
	if len(data) < ntxPage {
		return nil, ErrBadIndex
	}
	ix := &Index{Path: path}
	ix.version = binary.LittleEndian.Uint16(data[2:])
	if string(data[markerOff:markerOff+4]) == markerMagic {
		ix.marked = true
		ix.TableRecs = int(binary.LittleEndian.Uint32(data[markerRecs:]))
		ix.diskDirty = data[markerDirty] != 0
	}
	root := int(binary.LittleEndian.Uint32(data[4:]))
	itemSize := int(binary.LittleEndian.Uint16(data[12:]))
	ix.KeyLen = int(binary.LittleEndian.Uint16(data[14:]))
	ix.KeyDec = int(binary.LittleEndian.Uint16(data[16:]))
	maxItem := int(binary.LittleEndian.Uint16(data[18:]))
	ix.KeyExpr = cstr(data[22:278])
	ix.Unique = data[278] != 0
	ix.Descend = data[280] != 0
	ix.ForExpr = cstr(data[282:538])
	if ix.KeyLen <= 0 || itemSize != ix.KeyLen+8 || maxItem <= 0 {
		return nil, ErrBadIndex
	}
	var list []Entry
	var walk func(off, depth int) error
	walk = func(off, depth int) error {
		if off == 0 {
			return nil
		}
		if depth > 64 || off < 0 || off+ntxPage > len(data) {
			return ErrBadIndex
		}
		pg := data[off : off+ntxPage]
		count := int(binary.LittleEndian.Uint16(pg))
		if count > maxItem {
			return ErrBadIndex
		}
		for i := 0; i <= count; i++ {
			io := int(binary.LittleEndian.Uint16(pg[2+2*i:]))
			if io+itemSize > ntxPage {
				return ErrBadIndex
			}
			item := pg[io : io+itemSize]
			left := int(binary.LittleEndian.Uint32(item))
			if err := walk(left, depth+1); err != nil {
				return err
			}
			if i < count {
				rec := binary.LittleEndian.Uint32(item[4:])
				list = append(list, Entry{Key: string(item[8 : 8+ix.KeyLen]), Rec: rec})
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	ix.entries.Reset(list)
	if ix.marked {
		ix.jStart = int(binary.LittleEndian.Uint32(data[markerJStart:]))
		count := int(binary.LittleEndian.Uint32(data[markerJCount:]))
		if count > 0 {
			// Clipper and Harbour don't know about the log: if one of
			// them has updated the index since (its version word changes,
			// or pages are added after the log), the log no longer
			// describes the file. Rebuild rather than replay it.
			if binary.LittleEndian.Uint16(data[markerVer:]) != ix.version || len(data) != ix.jStart+count*ix.journalSize() {
				ix.diskDirty = true
			} else if err := ix.replay(data, 0, count); err != nil {
				// a damaged log: keep the pages and have the index rebuilt
				ix.diskDirty = true
			}
		}
	}
	return ix, nil
}

// replay applies log entries from..to-1 found in data (the whole file).
func (ix *Index) replay(data []byte, from, to int) error {
	size := ix.journalSize()
	if ix.jStart < ntxPage || to < from || ix.jStart+to*size > len(data) {
		return ErrBadIndex
	}
	for n := from; n < to; n++ {
		e := data[ix.jStart+n*size:]
		rec := binary.LittleEndian.Uint32(e[1:])
		key := string(e[5 : 5+ix.KeyLen])
		switch e[0] {
		case 'I':
			ix.insert(key, rec)
		case 'D':
			ix.remove(key, rec)
		default:
			return ErrBadIndex
		}
	}
	ix.jSeen = to
	return nil
}

func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func (ix *Index) maxKeys() int {
	m := (ntxPage-2)/(ix.KeyLen+10) - 1
	if m%2 == 1 {
		m--
	}
	if m < 2 {
		m = 2
	}
	return m
}

// Save writes the whole index to disk as an NTX file with an empty change
// log, counting up its change counter. A shared index is written through
// its open file; the caller holds the index lock (see Begin).
func (ix *Index) Save() error {
	ix.version++
	data := ix.encode()
	ix.jStart, ix.jSeen, ix.pending, ix.needFull = len(data), 0, nil, false
	binary.LittleEndian.PutUint32(data[markerJStart:], uint32(ix.jStart))
	if ix.f != nil {
		if _, err := ix.f.WriteAt(data, 0); err != nil {
			return err
		}
		if err := ix.f.Truncate(int64(len(data))); err != nil {
			return err
		}
	} else if err := os.WriteFile(ix.Path, data, 0644); err != nil {
		return err
	}
	ix.dirty = false
	ix.marked, ix.diskDirty = true, false
	return nil
}

// Persist writes the changes made since the last save: as change log
// entries when it can, which takes time in proportion to the changes, or
// as a whole new file when the log is too long, the keys were replaced,
// or the file was written by another program. It also records the table's
// record count and clears the unsaved-changes flag.
func (ix *Index) Persist() error {
	if ix.needFull || !ix.marked || ix.jStart < ntxPage || ix.jSeen+len(ix.pending) > journalLimit(ix.Len()) {
		return ix.Save()
	}
	if len(ix.pending) == 0 && !ix.diskDirty && !ix.dirty {
		return nil
	}
	f := ix.f
	if f == nil {
		var err error
		if f, err = os.OpenFile(ix.Path, os.O_WRONLY, 0); err != nil {
			return err
		}
		defer f.Close()
	}
	size := ix.journalSize()
	if len(ix.pending) > 0 {
		buf := make([]byte, len(ix.pending)*size)
		for i, op := range ix.pending {
			e := buf[i*size:]
			e[0] = op.op
			binary.LittleEndian.PutUint32(e[1:], op.rec)
			copy(e[5:5+ix.KeyLen], op.key)
		}
		if _, err := f.WriteAt(buf, int64(ix.jStart+ix.jSeen*size)); err != nil {
			return err
		}
	}
	// the header last: record count, flag cleared, entry count
	var h [20]byte
	copy(h[:], markerMagic)
	binary.LittleEndian.PutUint32(h[4:], uint32(ix.TableRecs))
	binary.LittleEndian.PutUint32(h[12:], uint32(ix.jStart))
	binary.LittleEndian.PutUint32(h[16:], uint32(ix.jSeen+len(ix.pending)))
	if _, err := f.WriteAt(h[:], markerOff); err != nil {
		return err
	}
	ix.jSeen += len(ix.pending)
	ix.pending = nil
	ix.dirty, ix.diskDirty = false, false
	return nil
}

// Compact writes the whole index out with an empty change log if it has
// any changes logged or unsaved, leaving a plain NTX file (used when an
// exclusive table is closed).
func (ix *Index) Compact() error {
	if ix.jSeen > 0 || len(ix.pending) > 0 || ix.needFull || ix.dirty || ix.diskDirty || !ix.marked {
		if !ix.marked && !ix.dirty && len(ix.pending) == 0 {
			return nil // an untouched index from another program
		}
		return ix.Save()
	}
	return nil
}

// markDirty sets the unsaved-changes flag on disk, before a change
// reaches the table.
func (ix *Index) markDirty() error {
	if ix.diskDirty {
		return nil
	}
	b := make([]byte, 9)
	copy(b, markerMagic)
	binary.LittleEndian.PutUint32(b[4:], uint32(ix.TableRecs))
	b[8] = 1
	f := ix.f
	if f == nil {
		var err error
		if f, err = os.OpenFile(ix.Path, os.O_WRONLY, 0); err != nil {
			return err
		}
		defer f.Close()
	}
	if _, err := f.WriteAt(b, markerOff); err != nil {
		return err
	}
	ix.marked, ix.diskDirty = true, true
	return nil
}

// encode builds the NTX file.
func (ix *Index) encode() []byte {
	maxK := ix.maxKeys()
	itemSize := ix.KeyLen + 8
	var pages [][]byte
	newPage := func() (int, []byte) {
		pg := make([]byte, ntxPage)
		for i := 0; i <= maxK; i++ {
			binary.LittleEndian.PutUint16(pg[2+2*i:], uint16(2+2*(maxK+1)+i*itemSize))
		}
		pages = append(pages, pg)
		return len(pages) * ntxPage, pg // page 0 is the header
	}
	// capacity of a subtree of height h
	capOf := func(h int) int {
		c := 1
		for i := 0; i < h; i++ {
			c *= maxK + 1
			if c > 1<<40 {
				break
			}
		}
		return c - 1
	}
	var build func(items []Entry, h int) int
	build = func(items []Entry, h int) int {
		off, pg := newPage()
		putItem := func(i int, left int, e *Entry) {
			io := int(binary.LittleEndian.Uint16(pg[2+2*i:]))
			binary.LittleEndian.PutUint32(pg[io:], uint32(left))
			if e != nil {
				binary.LittleEndian.PutUint32(pg[io+4:], e.Rec)
				copy(pg[io+8:io+8+ix.KeyLen], e.Key)
			}
		}
		if h <= 1 {
			binary.LittleEndian.PutUint16(pg, uint16(len(items)))
			for i := range items {
				putItem(i, 0, &items[i])
			}
			return off
		}
		childCap := capOf(h - 1)
		n := len(items)
		k := (n + 1 + childCap) / (childCap + 1) // number of children
		if k < 2 {
			k = 2
		}
		rest := n - (k - 1)
		per, extra := rest/k, rest%k
		pos := 0
		binary.LittleEndian.PutUint16(pg, uint16(k-1))
		for c := 0; c < k; c++ {
			cnt := per
			if c < extra {
				cnt++
			}
			child := build(items[pos:pos+cnt], h-1)
			pos += cnt
			if c < k-1 {
				putItem(c, child, &items[pos])
				pos++
			} else {
				putItem(c, child, nil)
			}
		}
		return off
	}
	all := ix.entries.All()
	h := 1
	for capOf(h) < len(all) {
		h++
	}
	root := build(all, h)
	hdr := make([]byte, ntxPage)
	binary.LittleEndian.PutUint16(hdr[0:], 6)
	binary.LittleEndian.PutUint16(hdr[2:], ix.version)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(root))
	binary.LittleEndian.PutUint32(hdr[8:], 0)
	binary.LittleEndian.PutUint16(hdr[12:], uint16(itemSize))
	binary.LittleEndian.PutUint16(hdr[14:], uint16(ix.KeyLen))
	binary.LittleEndian.PutUint16(hdr[16:], uint16(ix.KeyDec))
	binary.LittleEndian.PutUint16(hdr[18:], uint16(maxK))
	binary.LittleEndian.PutUint16(hdr[20:], uint16(maxK/2))
	copy(hdr[22:277], ix.KeyExpr)
	if ix.Unique {
		hdr[278] = 1
	}
	if ix.Descend {
		hdr[280] = 1
	}
	copy(hdr[282:537], ix.ForExpr)
	copy(hdr[markerOff:], markerMagic)
	binary.LittleEndian.PutUint32(hdr[markerRecs:], uint32(ix.TableRecs))
	binary.LittleEndian.PutUint16(hdr[markerVer:], ix.version)
	var out bytes.Buffer
	out.Write(hdr)
	for _, p := range pages {
		out.Write(p)
	}
	return out.Bytes()
}

// ---------- shared use ----------

// Shared reports whether the index belongs to a shared table.
func (ix *Index) Shared() bool { return ix.f != nil }

// Share keeps an index that has just been written (INDEX ON) open for a
// shared table.
func (ix *Index) Share(readOnly bool) error {
	if ix.f != nil {
		return nil
	}
	flag := os.O_RDWR
	if readOnly {
		flag = os.O_RDONLY
	}
	f, err := os.OpenFile(ix.Path, flag, 0)
	if err != nil {
		return err
	}
	ix.f = f
	return nil
}

// Close releases the open file of a shared index.
func (ix *Index) Close() {
	if ix.f != nil {
		ix.f.Close()
		ix.f = nil
	}
}

// diskVersion reads the change counter from the file.
func (ix *Index) diskVersion() (uint16, error) {
	var b [2]byte
	if _, err := ix.f.ReadAt(b[:], 2); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b[:]), nil
}

// reload re-reads the keys from the open file.
func (ix *Index) reload() error {
	data, err := readAll(ix.f)
	if err != nil {
		return err
	}
	fresh, err := parseNTX(ix.Path, data)
	if err != nil {
		return err
	}
	ix.entries, ix.version, ix.dirty = fresh.entries, fresh.version, false
	ix.marked, ix.TableRecs, ix.diskDirty = fresh.marked, fresh.TableRecs, fresh.diskDirty
	ix.jStart, ix.jSeen, ix.pending, ix.needFull = fresh.jStart, fresh.jSeen, nil, false
	return nil
}

// header reads the change counter and the marker from the file.
func (ix *Index) header() (version uint16, m [20]byte, err error) {
	var v [2]byte
	if _, err = ix.f.ReadAt(v[:], 2); err != nil {
		return
	}
	_, err = ix.f.ReadAt(m[:], markerOff)
	return binary.LittleEndian.Uint16(v[:]), m, err
}

// catchUp picks up other sessions' changes: the whole file if it has been
// rewritten, otherwise just the new change log entries. The caller holds
// the index lock. It reports whether anything changed.
func (ix *Index) catchUp() (bool, error) {
	v, m, err := ix.header()
	if err != nil {
		return false, err
	}
	if v != ix.version {
		return true, ix.reload()
	}
	if string(m[:4]) != markerMagic {
		return false, nil
	}
	count := int(binary.LittleEndian.Uint32(m[16:]))
	ix.TableRecs = int(binary.LittleEndian.Uint32(m[4:]))
	if count <= ix.jSeen {
		return false, nil
	}
	size := ix.journalSize()
	data := make([]byte, ix.jStart+count*size)
	if _, err := ix.f.ReadAt(data[ix.jStart+ix.jSeen*size:], int64(ix.jStart+ix.jSeen*size)); err != nil {
		return false, ix.reload()
	}
	if err := ix.replay(data, ix.jSeen, count); err != nil {
		return true, ix.reload()
	}
	return true, nil
}

// Refresh picks up changes another session has made to a shared index,
// and reports whether there were any.
func (ix *Index) Refresh() (bool, error) {
	if ix.f == nil {
		return false, nil
	}
	v, m, err := ix.header()
	if err != nil {
		return false, err
	}
	if v == ix.version && (string(m[:4]) != markerMagic || int(binary.LittleEndian.Uint32(m[16:])) <= ix.jSeen) {
		return false, nil
	}
	if err := lockRange(ix.f, indexLock, 1, false, true); err != nil {
		return false, err
	}
	defer unlockRange(ix.f, indexLock, 1)
	return ix.catchUp()
}

// CloseShared closes a shared index, first writing it out whole if its
// change log has grown (so that it doesn't keep growing between uses).
func (ix *Index) CloseShared(readOnly bool) {
	if ix.f != nil && !readOnly && ix.jSeen > 256 {
		if lockRange(ix.f, indexLock, 1, true, true) == nil {
			if _, err := ix.catchUp(); err == nil {
				ix.Save()
			}
			unlockRange(ix.f, indexLock, 1)
		}
	}
	ix.Close()
}

// Begin prepares the index for a change that is about to reach the
// table: for a shared index it takes the index lock and picks up other
// sessions' changes; for any index it sets the unsaved-changes flag on
// disk. Every Begin must be followed by End (or Abort).
func (ix *Index) Begin() error {
	if ix.f != nil {
		if err := lockRange(ix.f, indexLock, 1, true, true); err != nil {
			return err
		}
		ix.locked = true
		if _, err := ix.catchUp(); err != nil {
			ix.Abort()
			return err
		}
		// the flag is set and cleared under this lock, so finding it set
		// means a session was stopped part way through a change (setting
		// the flag doesn't change the version, so read it directly)
		var m [9]byte
		if _, err := ix.f.ReadAt(m[:], markerOff); err == nil && string(m[:4]) == markerMagic && m[8] != 0 {
			ix.Abort()
			return ErrIndexStale
		}
	}
	if err := ix.markDirty(); err != nil {
		ix.Abort()
		return err
	}
	return nil
}

// End finishes a change. A shared index is saved straight away, so other
// sessions see it, and unlocked; an exclusive one is saved when the table
// is committed or closed.
func (ix *Index) End(recs int) error {
	ix.TableRecs = recs
	if ix.f == nil {
		return nil
	}
	err := ix.Persist()
	ix.Abort()
	return err
}

// Abort releases the lock Begin took, without saving.
func (ix *Index) Abort() {
	if ix.locked {
		unlockRange(ix.f, indexLock, 1)
		ix.locked = false
	}
}

// ---------- key handling ----------

// NtxNum formats a numeric key so byte order matches numeric order.
func NtxNum(n float64, length, dec int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := value.FormatNum(n, length, dec)
	b := []byte(s)
	for i := range b {
		if b[i] == ' ' {
			b[i] = '0'
		}
	}
	if neg {
		for i := range b {
			if b[i] >= '0' && b[i] <= '9' {
				b[i] = '0' + ('9' - b[i])
			}
		}
		b[0] = ','
	}
	return string(b)
}

// KeyString converts a key value to its stored form.
func (ix *Index) KeyString(v value.Value) string {
	v = v.Deref()
	switch v.K {
	case value.STRING:
		s := ix.CP.EncodeString(v.S)
		if len(s) >= ix.KeyLen {
			return s[:ix.KeyLen]
		}
		return s + strings.Repeat(" ", ix.KeyLen-len(s))
	case value.NUMBER:
		return NtxNum(v.N, ix.KeyLen, ix.KeyDec)
	case value.DATE:
		return value.DTOS(v.J)
	case value.LOGICAL:
		if v.B {
			return "T"
		}
		return "F"
	}
	return strings.Repeat(" ", ix.KeyLen)
}

// SeekString converts a seek value; strings are not padded (partial match).
func (ix *Index) SeekString(v value.Value) string {
	v = v.Deref()
	if v.K == value.STRING {
		s := ix.CP.EncodeString(v.S)
		if len(s) > ix.KeyLen {
			return s[:ix.KeyLen]
		}
		return s
	}
	return ix.KeyString(v)
}

// SetKeyFormat initializes key type/length from a sample key value.
func (ix *Index) SetKeyFormat(v value.Value) {
	v = v.Deref()
	switch v.K {
	case value.STRING:
		ix.KeyType = 'C'
		ix.KeyLen = len(ix.CP.Encode(v.S))
		if ix.KeyLen == 0 {
			ix.KeyLen = 1
		}
	case value.NUMBER:
		ix.KeyType = 'N'
		ix.KeyLen = v.DefaultWidth()
		ix.KeyDec = int(v.Dec)
	case value.DATE:
		ix.KeyType = 'D'
		ix.KeyLen = 8
	case value.LOGICAL:
		ix.KeyType = 'L'
		ix.KeyLen = 1
	default:
		ix.KeyType = 'C'
		ix.KeyLen = 10
	}
}

func (ix *Index) less(a, b Entry) bool {
	if a.Key != b.Key {
		if ix.Descend {
			return a.Key > b.Key
		}
		return a.Key < b.Key
	}
	return a.Rec < b.Rec
}

// Len is the number of keys in the index.
func (ix *Index) Len() int { return ix.entries.Len() }

// At returns the entry at position i in index order.
func (ix *Index) At(i int) Entry { return ix.entries.At(i) }

// SetEntries replaces the keys with list, sorting it (and dropping
// duplicate keys for a UNIQUE index).
func (ix *Index) SetEntries(list []Entry) {
	sort.SliceStable(list, func(i, j int) bool { return ix.less(list[i], list[j]) })
	if ix.Unique {
		out := list[:0]
		for i, e := range list {
			if i > 0 && e.Key == out[len(out)-1].Key {
				continue
			}
			out = append(out, e)
		}
		list = out
	}
	ix.entries.Reset(list)
	ix.dirty = true
	ix.needFull, ix.pending = true, nil
}

// Find returns the position of (key, rec) or -1.
func (ix *Index) Find(key string, rec uint32) int {
	e := Entry{key, rec}
	i := ix.LowerBound(key, rec)
	if i < ix.entries.Len() && ix.entries.At(i) == e {
		return i
	}
	return -1
}

// LowerBound returns the first position not less than (key, rec).
func (ix *Index) LowerBound(key string, rec uint32) int {
	e := Entry{key, rec}
	return ix.entries.Search(func(x Entry) bool { return !ix.less(x, e) })
}

// Insert adds a key (logging the change, see Persist).
func (ix *Index) Insert(key string, rec uint32) {
	ix.insert(key, rec)
	ix.pending = append(ix.pending, journalOp{'I', rec, key})
}

// Remove takes a key out (logging the change, see Persist).
func (ix *Index) Remove(key string, rec uint32) {
	ix.remove(key, rec)
	ix.pending = append(ix.pending, journalOp{'D', rec, key})
}

func (ix *Index) insert(key string, rec uint32) {
	if ix.Unique {
		i := ix.LowerBound(key, 0)
		if i < ix.entries.Len() && ix.entries.At(i).Key == key {
			return
		}
	}
	ix.entries.Insert(ix.LowerBound(key, rec), Entry{key, rec})
	ix.dirty = true
}

func (ix *Index) remove(key string, rec uint32) {
	i := ix.Find(key, rec)
	if i < 0 {
		return
	}
	ix.entries.Delete(i)
	ix.dirty = true
}

// SeekPos finds the first entry whose key starts at or after prefix.
func (ix *Index) SeekPos(prefix string) int {
	n := len(prefix)
	return ix.entries.Search(func(x Entry) bool {
		k := x.Key
		if len(k) > n {
			k = k[:n]
		}
		if ix.Descend {
			return k <= prefix
		}
		return k >= prefix
	})
}

// Matches reports whether entry i starts with prefix.
func (ix *Index) Matches(i int, prefix string) bool {
	if i < 0 || i >= ix.entries.Len() {
		return false
	}
	return strings.HasPrefix(ix.entries.At(i).Key, prefix)
}

func (ix *Index) Dirty() bool { return ix.dirty }
