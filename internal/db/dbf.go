// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package db implements DBF tables, memo files, NTX indexes and work areas.
package db

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"nantucket/internal/value"
)

type FieldDef struct {
	Name   string
	Type   byte // C N D L M F I B Y T
	Len    int
	Dec    int
	Offset int
}

type Table struct {
	f        *os.File
	Path     string
	Version  byte
	hdrLen   int
	RecLen   int
	recCount int
	Fields   []FieldDef
	fieldIdx map[string]int
	memo     *Memo
	ReadOnly bool
	dirtyHdr bool
	vfp      bool
	ldid     byte      // language driver ID from the header
	cp       *CodePage // text conversion; nil stores bytes as they are

	mode       OpenMode
	recLocks   map[int]bool // records locked by RLock()
	fileLocked bool         // FLock() held
}

var ErrNotDBF = errors.New("not a valid DBF file")

func (t *Table) RecCount() int { return t.recCount }
func (t *Table) HasMemo() bool { return t.memo != nil }

// FieldIndex returns the 0-based index of a field or -1.
func (t *Table) FieldIndex(name string) int {
	if i, ok := t.fieldIdx[strings.ToUpper(name)]; ok {
		return i
	}
	// dBase allows 10 char field names; match truncated names
	n := strings.ToUpper(name)
	if len(n) > 10 {
		if i, ok := t.fieldIdx[n[:10]]; ok {
			return i
		}
	}
	return -1
}

// OpenTable opens a DBF file for exclusive use.
func OpenTable(path string, readOnly bool) (*Table, error) {
	return OpenTableMode(path, readOnly, OpenExclusive)
}

// OpenTableMode opens a DBF file. With OpenExclusive or OpenShared it takes
// the table's open lock (see lock.go), and returns ErrInUse when another
// session's use conflicts.
func OpenTableMode(path string, readOnly bool, mode OpenMode) (*Table, error) {
	flag := os.O_RDWR
	if readOnly {
		flag = os.O_RDONLY
	}
	f, err := os.OpenFile(path, flag, 0)
	if err != nil && !readOnly {
		// fall back to read-only
		f, err = os.OpenFile(path, os.O_RDONLY, 0)
		readOnly = true
	}
	if err != nil {
		return nil, err
	}
	if mode != OpenNoLock {
		// a read-only handle can only take a read lock
		if err := lockRange(f, openLock, 1, mode == OpenExclusive && !readOnly, false); err != nil {
			f.Close()
			if errors.Is(err, ErrLocked) {
				return nil, ErrInUse
			}
			return nil, err
		}
	}
	t := &Table{f: f, Path: path, ReadOnly: readOnly, fieldIdx: map[string]int{}, mode: mode, recLocks: map[int]bool{}}
	hdr := make([]byte, 32)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		f.Close()
		return nil, ErrNotDBF
	}
	t.Version = hdr[0]
	t.recCount = int(binary.LittleEndian.Uint32(hdr[4:8]))
	t.hdrLen = int(binary.LittleEndian.Uint16(hdr[8:10]))
	t.RecLen = int(binary.LittleEndian.Uint16(hdr[10:12]))
	t.ldid = hdr[29]
	t.vfp = t.Version == 0x30 || t.Version == 0x31 || t.Version == 0x32
	if t.hdrLen < 33 || t.RecLen < 1 {
		f.Close()
		return nil, ErrNotDBF
	}
	desc := make([]byte, t.hdrLen-32)
	if _, err := f.ReadAt(desc, 32); err != nil {
		f.Close()
		return nil, ErrNotDBF
	}
	off := 1
	for i := 0; i+32 <= len(desc); i += 32 {
		d := desc[i : i+32]
		if d[0] == 0x0D {
			break
		}
		name := strings.ToUpper(strings.TrimRight(string(d[0:11]), "\x00 "))
		if j := strings.IndexByte(name, 0); j >= 0 {
			name = name[:j]
		}
		fd := FieldDef{Name: name, Type: d[11], Len: int(d[16]), Dec: int(d[17]), Offset: off}
		if fd.Type == 'C' && fd.Dec > 0 {
			// Clipper long character fields
			fd.Len += fd.Dec << 8
			fd.Dec = 0
		}
		if fd.Type == '0' {
			// VFP _NullFlags
			off += fd.Len
			continue
		}
		off += fd.Len
		t.fieldIdx[name] = len(t.Fields)
		t.Fields = append(t.Fields, fd)
	}
	if err := t.checkFields(); err != nil {
		f.Close()
		return nil, err
	}
	// a file cut short holds fewer records than the header says: use the
	// complete records that are there
	if st, err := f.Stat(); err == nil {
		if avail := (st.Size() - int64(t.hdrLen)) / int64(t.RecLen); int64(t.recCount) > avail {
			t.recCount = int(max(avail, 0))
		}
	}
	if t.needsMemo() {
		mp, typ := memoPathFor(path, t.Version)
		m, err := OpenMemo(mp, typ, readOnly)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("cannot open memo file %s: %w", filepath.Base(mp), err)
		}
		m.shared = mode == OpenShared
		t.memo = m
	}
	return t, nil
}

// minWidth is the narrowest a field of each type can be and still be read.
var minWidth = map[byte]int{'L': 1, 'I': 4, 'B': 8, 'Y': 8, 'T': 8}

// checkFields makes sure every field lies inside the record and is wide
// enough for its type, so that a damaged header is reported instead of
// crashing when records are read.
func (t *Table) checkFields() error {
	for _, fd := range t.Fields {
		if fd.Len < minWidth[fd.Type] {
			return fmt.Errorf("%w: field %s is %d wide, too narrow for type %c", ErrNotDBF, fd.Name, fd.Len, fd.Type)
		}
		if fd.Offset+fd.Len > t.RecLen {
			return fmt.Errorf("%w: field %s goes past the end of the %d byte record", ErrNotDBF, fd.Name, t.RecLen)
		}
	}
	return nil
}

func (t *Table) needsMemo() bool {
	for _, fd := range t.Fields {
		if fd.Type == 'M' || fd.Type == 'G' || fd.Type == 'P' {
			if fd.Len == 10 || fd.Len == 4 {
				return true
			}
		}
	}
	return false
}

func memoPathFor(path string, version byte) (string, MemoType) {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	switch version {
	case 0xF5, 0x30, 0x31, 0x32:
		return findCase(base, ".fpt"), MemoFPT
	case 0x8B, 0x8E:
		return findCase(base, ".dbt"), MemoDBT4
	}
	return findCase(base, ".dbt"), MemoDBT3
}

// findCase returns base+ext, trying upper case extension if lower is missing.
func findCase(base, ext string) string {
	p := base + ext
	if _, err := os.Stat(p); err == nil {
		return p
	}
	up := base + strings.ToUpper(ext)
	if _, err := os.Stat(up); err == nil {
		return up
	}
	return p
}

// CreateTable creates a new empty DBF (dBase III / Clipper format).
//
// cp, if given, is the code page its text will be stored in; it is
// recorded in the header.
func CreateTable(path string, fields []FieldDef, cp ...*CodePage) error {
	if len(fields) == 0 {
		return errors.New("no fields defined")
	}
	hasMemo := false
	recLen := 1
	seen := map[string]bool{}
	for i := range fields {
		fd := &fields[i]
		fd.Name = strings.ToUpper(strings.TrimSpace(fd.Name))
		if fd.Name == "" || len(fd.Name) > 10 {
			return fmt.Errorf("invalid field name %q", fd.Name)
		}
		if seen[fd.Name] {
			return fmt.Errorf("duplicate field name %s", fd.Name)
		}
		seen[fd.Name] = true
		switch fd.Type {
		case 'C':
			if fd.Len < 1 || fd.Len > 65535 {
				return fmt.Errorf("invalid width for %s", fd.Name)
			}
			fd.Dec = 0
		case 'N', 'F':
			if fd.Len < 1 || fd.Len > 20 || fd.Dec < 0 || (fd.Dec > 0 && fd.Dec > fd.Len-2) {
				return fmt.Errorf("invalid width/decimals for %s", fd.Name)
			}
		case 'D':
			fd.Len, fd.Dec = 8, 0
		case 'L':
			fd.Len, fd.Dec = 1, 0
		case 'M':
			fd.Len, fd.Dec = 10, 0
			hasMemo = true
		default:
			return fmt.Errorf("invalid field type %c for %s", fd.Type, fd.Name)
		}
		fd.Offset = recLen
		recLen += fd.Len
	}
	hdrLen := 32 + 32*len(fields) + 1
	buf := make([]byte, hdrLen+1)
	ver := byte(0x03)
	if hasMemo {
		ver = 0x83
	}
	buf[0] = ver
	now := time.Now()
	buf[1], buf[2], buf[3] = byte(now.Year()-1900), byte(now.Month()), byte(now.Day())
	binary.LittleEndian.PutUint16(buf[8:], uint16(hdrLen))
	binary.LittleEndian.PutUint16(buf[10:], uint16(recLen))
	if len(cp) > 0 && cp[0] != nil {
		buf[29] = cp[0].LDID
	}
	for i, fd := range fields {
		d := buf[32+32*i:]
		copy(d[0:11], fd.Name)
		d[11] = fd.Type
		if fd.Type == 'C' && fd.Len > 255 {
			d[16] = byte(fd.Len & 0xFF)
			d[17] = byte(fd.Len >> 8)
		} else {
			d[16] = byte(fd.Len)
			d[17] = byte(fd.Dec)
		}
	}
	buf[hdrLen-1] = 0x0D
	buf[hdrLen] = 0x1A
	if err := os.WriteFile(path, buf, 0644); err != nil {
		return err
	}
	if hasMemo {
		mp, _ := memoPathFor(path, ver)
		if err := CreateMemo(mp, MemoDBT3); err != nil {
			return err
		}
	}
	return nil
}

func (t *Table) recOffset(n int) int64 { return int64(t.hdrLen) + int64(n-1)*int64(t.RecLen) }

// ReadRecord reads record n (1-based) into buf.
func (t *Table) ReadRecord(n int, buf []byte) error {
	if n < 1 || n > t.recCount {
		for i := range buf {
			buf[i] = ' '
		}
		return nil
	}
	_, err := t.f.ReadAt(buf, t.recOffset(n))
	return err
}

func (t *Table) WriteRecord(n int, buf []byte) error {
	if t.ReadOnly {
		return errors.New("table is read-only")
	}
	_, err := t.f.WriteAt(buf, t.recOffset(n))
	return err
}

// AppendRecord appends buf as a new record and returns its number. On a
// shared table it holds the append lock, picks up records other sessions
// have added, and locks the new record (releasing other record locks)
// unless the whole file is locked.
func (t *Table) AppendRecord(buf []byte) (int, error) {
	if t.ReadOnly {
		return 0, errors.New("table is read-only")
	}
	if t.mode == OpenShared {
		if err := lockRange(t.f, appendLock, 1, true, true); err != nil {
			return 0, err
		}
		defer unlockRange(t.f, appendLock, 1)
		t.Refresh()
		if !t.fileLocked {
			t.unlockRecords()
			if err := t.LockRecord(t.recCount + 1); err != nil {
				return 0, err
			}
		}
	}
	t.recCount++
	if err := t.WriteRecord(t.recCount, buf); err != nil {
		t.recCount--
		return 0, err
	}
	t.f.WriteAt([]byte{0x1A}, t.recOffset(t.recCount+1))
	t.dirtyHdr = true
	t.writeHeader()
	return t.recCount, nil
}

func (t *Table) writeHeader() {
	if t.ReadOnly {
		return
	}
	hdr := make([]byte, 8)
	hdr[0] = t.Version
	now := time.Now()
	hdr[1], hdr[2], hdr[3] = byte(now.Year()-1900), byte(now.Month()), byte(now.Day())
	binary.LittleEndian.PutUint32(hdr[4:], uint32(t.recCount))
	t.f.WriteAt(hdr, 0)
	t.dirtyHdr = false
}

// Truncate sets the record count (used by PACK/ZAP).
func (t *Table) Truncate(n int) error {
	t.recCount = n
	end := t.recOffset(n + 1)
	if err := t.f.Truncate(end); err != nil {
		return err
	}
	t.f.WriteAt([]byte{0x1A}, end)
	t.writeHeader()
	return nil
}

func (t *Table) Flush() {
	if t.dirtyHdr {
		t.writeHeader()
	}
	if !t.ReadOnly {
		t.f.Sync()
	}
}

func (t *Table) Close() {
	t.UnlockAll()
	t.Flush()
	t.f.Close()
	if t.memo != nil {
		t.memo.Close()
	}
}

func (t *Table) ZapMemo() error {
	if t.memo == nil {
		return nil
	}
	return t.memo.Zap()
}

// ---------- field conversion ----------

// SetCodePage sets how the table's text is converted: the code page named
// in its header if it has one Nantucket knows, otherwise def.
func (t *Table) SetCodePage(def *CodePage) {
	if cp := codePageForLDID(t.ldid); cp != nil {
		t.cp = cp
	} else {
		t.cp = def
	}
}

// CodePage is the table's code page (nil: none set).
func (t *Table) CodePage() *CodePage { return t.cp }

// GetField decodes field i from a record buffer.
func (t *Table) GetField(buf []byte, i int) value.Value {
	fd := &t.Fields[i]
	raw := buf[fd.Offset : fd.Offset+fd.Len]
	switch fd.Type {
	case 'C':
		return value.Str(t.cp.Decode(raw))
	case 'N', 'F':
		s := strings.TrimSpace(string(raw))
		n, _ := strconv.ParseFloat(s, 64)
		return value.NumW(n, fd.Len, fd.Dec)
	case 'D':
		return value.Date(value.STOD(string(raw)))
	case 'L':
		c := raw[0]
		return value.Bool(c == 'T' || c == 't' || c == 'Y' || c == 'y')
	case 'M', 'G', 'P':
		if t.memo == nil {
			return value.MemoStr("")
		}
		blk := memoBlock(raw)
		if blk == 0 {
			return value.MemoStr("")
		}
		s, _ := t.memo.Read(blk)
		if fd.Type == 'M' {
			s = t.cp.Decode([]byte(s))
		}
		return value.MemoStr(s)
	case 'I':
		n := int32(binary.LittleEndian.Uint32(raw))
		return value.NumW(float64(n), 10, 0)
	case 'B':
		n := math.Float64frombits(binary.LittleEndian.Uint64(raw))
		return value.NumW(n, 20, fd.Dec)
	case 'Y':
		n := int64(binary.LittleEndian.Uint64(raw))
		return value.NumW(float64(n)/10000, 20, 4)
	case 'T':
		j := int32(binary.LittleEndian.Uint32(raw[0:4]))
		return value.Date(j)
	}
	return value.Str(string(raw))
}

func memoBlock(raw []byte) int {
	if len(raw) == 4 {
		return int(binary.LittleEndian.Uint32(raw))
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return n
}

// PutField encodes v into field i of buf.
func (t *Table) PutField(buf []byte, i int, v value.Value) error {
	fd := &t.Fields[i]
	raw := buf[fd.Offset : fd.Offset+fd.Len]
	v = v.Deref()
	switch fd.Type {
	case 'C':
		if v.K != value.STRING {
			return typeMismatch(fd)
		}
		s := t.cp.Encode(v.S)
		if len(s) > fd.Len {
			s = s[:fd.Len]
		}
		copy(raw, s)
		for k := len(s); k < fd.Len; k++ {
			raw[k] = ' '
		}
	case 'N', 'F':
		if v.K != value.NUMBER {
			return typeMismatch(fd)
		}
		s := value.FormatNum(v.N, fd.Len, fd.Dec)
		copy(raw, s)
	case 'D':
		if v.K != value.DATE {
			return typeMismatch(fd)
		}
		copy(raw, value.DTOS(v.J))
	case 'L':
		if v.K != value.LOGICAL {
			return typeMismatch(fd)
		}
		if v.B {
			raw[0] = 'T'
		} else {
			raw[0] = 'F'
		}
	case 'M':
		if v.K != value.STRING {
			return typeMismatch(fd)
		}
		if t.memo == nil {
			return errors.New("no memo file")
		}
		old := memoBlock(raw)
		blk, err := t.memo.Write(old, string(t.cp.Encode(v.S)))
		if err != nil {
			return err
		}
		if len(raw) == 4 {
			binary.LittleEndian.PutUint32(raw, uint32(blk))
		} else if blk == 0 {
			copy(raw, strings.Repeat(" ", fd.Len))
		} else {
			copy(raw, fmt.Sprintf("%*d", fd.Len, blk))
		}
	case 'I':
		if v.K != value.NUMBER {
			return typeMismatch(fd)
		}
		binary.LittleEndian.PutUint32(raw, uint32(int32(v.N)))
	case 'B':
		if v.K != value.NUMBER {
			return typeMismatch(fd)
		}
		binary.LittleEndian.PutUint64(raw, math.Float64bits(v.N))
	case 'Y':
		if v.K != value.NUMBER {
			return typeMismatch(fd)
		}
		binary.LittleEndian.PutUint64(raw, uint64(int64(math.Round(v.N*10000))))
	default:
		return fmt.Errorf("field type %c is read-only", fd.Type)
	}
	return nil
}

type FieldTypeError struct{ Field string }

func (e *FieldTypeError) Error() string { return "data type mismatch: " + e.Field }

func typeMismatch(fd *FieldDef) error { return &FieldTypeError{fd.Name} }

// BlankRecord returns an empty record buffer.
func (t *Table) BlankRecord() []byte {
	b := make([]byte, t.RecLen)
	for i := range b {
		b[i] = ' '
	}
	for _, fd := range t.Fields {
		switch fd.Type {
		case 'I', 'B', 'Y', 'T':
			for k := 0; k < fd.Len; k++ {
				b[fd.Offset+k] = 0
			}
		}
	}
	return b
}

// EmptyValue returns the blank value for field i.
func (t *Table) EmptyValue(i int) value.Value {
	return t.GetField(t.BlankRecord(), i)
}

func (t *Table) HdrLen() int { return t.hdrLen }

// LastUpdate returns the y, m, d stored in the header.
func (t *Table) LastUpdate() (int, int, int) {
	h := make([]byte, 4)
	t.f.ReadAt(h, 0)
	return 1900 + int(h[1]), int(h[2]), int(h[3])
}

// ---------- shared use ----------

// Mode is how the table was opened.
func (t *Table) Mode() OpenMode { return t.mode }

// Shared reports whether the table was opened for shared use.
func (t *Table) Shared() bool { return t.mode == OpenShared }

// Refresh re-reads the record count from the file header, picking up
// records other sessions have added to a shared table.
func (t *Table) Refresh() {
	if t.mode != OpenShared {
		return
	}
	var b [4]byte
	if _, err := t.f.ReadAt(b[:], 4); err == nil {
		t.recCount = int(binary.LittleEndian.Uint32(b[:]))
	}
}

// CanWrite reports whether record n may be changed: always on a table
// opened exclusively, and on a shared one when the record or the file is
// locked.
func (t *Table) CanWrite(n int) bool {
	return t.mode != OpenShared || t.fileLocked || t.recLocks[n]
}

// LockRecord locks record n (RLock). It returns ErrLocked if another
// session holds it. On a table that is not shared it only records the
// lock.
func (t *Table) LockRecord(n int) error {
	if t.fileLocked || t.recLocks[n] {
		return nil
	}
	if t.mode == OpenShared && !t.ReadOnly {
		if err := lockRange(t.f, lockBase+int64(n), 1, true, false); err != nil {
			return err
		}
	}
	t.recLocks[n] = true
	return nil
}

// UnlockRecord releases a record lock.
func (t *Table) UnlockRecord(n int) {
	if !t.recLocks[n] {
		return
	}
	if t.mode == OpenShared && !t.ReadOnly {
		unlockRange(t.f, lockBase+int64(n), 1)
	}
	delete(t.recLocks, n)
}

func (t *Table) unlockRecords() {
	for n := range t.recLocks {
		t.UnlockRecord(n)
	}
}

// LockFile locks the whole table (FLock), releasing any record locks
// first. It returns ErrLocked if another session holds a lock.
func (t *Table) LockFile() error {
	if t.fileLocked {
		return nil
	}
	t.unlockRecords()
	if t.mode == OpenShared && !t.ReadOnly {
		if err := lockRange(t.f, lockBase, lockSpan, true, false); err != nil {
			return err
		}
	}
	t.fileLocked = true
	return nil
}

// UnlockAll releases the file lock and every record lock.
func (t *Table) UnlockAll() {
	t.unlockRecords()
	if t.fileLocked {
		if t.mode == OpenShared && !t.ReadOnly {
			unlockRange(t.f, lockBase, lockSpan)
		}
		t.fileLocked = false
	}
}

// FileLocked reports whether FLock() is held.
func (t *Table) FileLocked() bool { return t.fileLocked }

// RecordLocked reports whether this session has record n locked.
func (t *Table) RecordLocked(n int) bool { return t.fileLocked || t.recLocks[n] }

// LockedRecords lists the records locked with RLock(), in order.
func (t *Table) LockedRecords() []int {
	out := make([]int, 0, len(t.recLocks))
	for n := range t.recLocks {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}
