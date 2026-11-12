// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nantucket/internal/value"
)

// Settings shared by all work areas.
type Settings struct {
	Deleted  bool // SET DELETED ON hides deleted records
	SoftSeek bool
	// OnFieldWrite, when set, is called after each write to a field with
	// its value before and after (the debugger's data breakpoints).
	OnFieldWrite func(wa *WorkArea, field int, old, new value.Value)
}

type Relation struct {
	Child   *WorkArea
	KeyFn   func() (value.Value, error)
	KeyText string
}

type WorkArea struct {
	Num      int
	Alias    string
	T        *Table
	Set      *Settings
	buf      []byte
	Recno    int
	EOF      bool
	BOF      bool
	Found    bool
	dirty    bool
	Indexes  []*Index
	Order    int // 1-based, 0 = natural
	pos      int // position in current order (-1 unknown)
	origKeys []string
	inFor    []bool

	Filter     func() bool
	FilterText string
	Relations  []*Relation

	// LOCATE state
	LocateFor   func() bool
	LocateWhile func() bool
	LocateEnd   int // remaining NEXT count (-1 unlimited)

	Exclusive bool

	lastRefresh time.Time // when refresh last looked for other sessions' changes
}

func NewWorkArea(num int, alias string, t *Table, s *Settings) *WorkArea {
	wa := &WorkArea{Num: num, Alias: strings.ToUpper(alias), T: t, Set: s, pos: -1}
	wa.buf = t.BlankRecord()
	return wa
}

func (wa *WorkArea) Buf() []byte { return wa.buf }

func (wa *WorkArea) GetField(i int) value.Value { return wa.T.GetField(wa.buf, i) }

func (wa *WorkArea) PutField(i int, v value.Value) error {
	if wa.EOF && wa.Recno > wa.T.RecCount() {
		return nil // writes to the phantom record are ignored
	}
	if wa.T.ReadOnly {
		return errors.New("table is read-only")
	}
	if !wa.T.CanWrite(wa.Recno) {
		return ErrLockRequired
	}
	hook := wa.Set != nil && wa.Set.OnFieldWrite != nil
	var old value.Value
	if hook {
		old = wa.GetField(i)
	}
	if err := wa.T.PutField(wa.buf, i, v); err != nil {
		return err
	}
	wa.dirty = true
	if hook {
		wa.Set.OnFieldWrite(wa, i, old, wa.GetField(i))
	}
	return nil
}

func (wa *WorkArea) Deleted() bool { return len(wa.buf) > 0 && wa.buf[0] == '*' }

func (wa *WorkArea) SetDeleted(del bool) error {
	if wa.Recno > wa.T.RecCount() || wa.Recno < 1 {
		return nil
	}
	if !wa.T.CanWrite(wa.Recno) {
		return ErrLockRequired
	}
	if del {
		wa.buf[0] = '*'
	} else {
		wa.buf[0] = ' '
	}
	wa.dirty = true
	return nil
}

func (wa *WorkArea) CurIndex() *Index {
	if wa.Order < 1 || wa.Order > len(wa.Indexes) {
		return nil
	}
	return wa.Indexes[wa.Order-1]
}

// ---------- keys ----------

func (wa *WorkArea) evalKey(ix *Index) (string, bool, error) {
	if ix.ForFn != nil {
		ok, err := ix.ForFn()
		if err != nil {
			return "", false, err
		}
		if !ok {
			return "", false, nil
		}
	}
	v, err := ix.KeyFn()
	if err != nil {
		return "", false, err
	}
	return ix.KeyString(v), true, nil
}

func (wa *WorkArea) computeKeys() error {
	if len(wa.Indexes) == 0 {
		return nil
	}
	if len(wa.origKeys) != len(wa.Indexes) {
		wa.origKeys = make([]string, len(wa.Indexes))
		wa.inFor = make([]bool, len(wa.Indexes))
	}
	for i, ix := range wa.Indexes {
		k, in, err := wa.evalKey(ix)
		if err != nil {
			return err
		}
		wa.origKeys[i], wa.inFor[i] = k, in
	}
	return nil
}

// ---------- record buffer ----------

// Commit writes the current record and updates indexes.
func (wa *WorkArea) Commit() error {
	if !wa.dirty {
		return nil
	}
	wa.dirty = false
	if wa.Recno < 1 || wa.Recno > wa.T.RecCount() {
		return nil
	}
	// work out which index keys change, from the record buffer
	type change struct {
		i       int
		key     string
		in      bool
		oldKey  string
		wasIn   bool
		changed bool
	}
	var changes []change
	if len(wa.Indexes) > 0 && len(wa.origKeys) != len(wa.Indexes) {
		wa.computeKeys()
	}
	var touched []*Index
	for i, ix := range wa.Indexes {
		k, in, err := wa.evalKey(ix)
		if err != nil {
			return err
		}
		if k != wa.origKeys[i] || in != wa.inFor[i] {
			changes = append(changes, change{i: i, key: k, in: in, oldKey: wa.origKeys[i], wasIn: wa.inFor[i]})
			touched = append(touched, ix)
		}
	}
	// mark the indexes as changing before the record reaches the disk,
	// so that if Nantucket is stopped in between, they are rebuilt
	if err := wa.beginAll(touched); err != nil {
		return err
	}
	if err := wa.T.WriteRecord(wa.Recno, wa.buf); err != nil {
		abortAll(touched)
		return err
	}
	rec := uint32(wa.Recno)
	for _, c := range changes {
		ix := wa.Indexes[c.i]
		if c.wasIn {
			ix.Remove(c.oldKey, rec)
		}
		if c.in {
			ix.Insert(c.key, rec)
		}
		wa.origKeys[c.i], wa.inFor[c.i] = c.key, c.in
		if c.i == wa.Order-1 {
			wa.pos = -1
		}
	}
	return endAll(touched, wa.T.RecCount())
}

// beginAll calls Begin on each index, in file name order so that sessions
// sharing indexes always lock them in the same order. A shared index that
// another session left part way through a change is rebuilt first.
func (wa *WorkArea) beginAll(ixs []*Index) error {
	sort.Slice(ixs, func(a, b int) bool { return ixs[a].Path < ixs[b].Path })
	for k, ix := range ixs {
		err := ix.Begin()
		if errors.Is(err, ErrIndexStale) {
			abortAll(ixs[:k]) // RebuildIndex re-reads the current record
			if err = wa.rebuildKeepingRecord(ix); err == nil {
				return wa.beginAll(ixs)
			}
		}
		if err != nil {
			abortAll(ixs[:k])
			return err
		}
	}
	return nil
}

// rebuildKeepingRecord rebuilds an index without losing unsaved changes
// in the record buffer.
func (wa *WorkArea) rebuildKeepingRecord(ix *Index) error {
	buf := append([]byte(nil), wa.buf...)
	err := wa.RebuildIndex(ix)
	copy(wa.buf, buf)
	return err
}

func abortAll(ixs []*Index) {
	for _, ix := range ixs {
		ix.Abort()
	}
}

// endAll calls End on each index, returning the first error.
func endAll(ixs []*Index, recs int) error {
	var first error
	for _, ix := range ixs {
		if err := ix.End(recs); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// readRaw loads a record without side effects (used for scanning).
func (wa *WorkArea) readRaw(n int) error {
	wa.Recno = n
	return wa.T.ReadRecord(n, wa.buf)
}

func (wa *WorkArea) load(n int) error {
	if err := wa.readRaw(n); err != nil {
		return err
	}
	wa.EOF, wa.BOF = false, false
	if err := wa.computeKeys(); err != nil {
		return err
	}
	return nil
}

func (wa *WorkArea) phantom() {
	wa.Recno = wa.T.RecCount() + 1
	wa.T.ReadRecord(wa.Recno, wa.buf) // fills with blanks
	wa.EOF = true
	if ix := wa.CurIndex(); ix != nil {
		wa.pos = ix.Len()
	}
	wa.origKeys = nil
}

func (wa *WorkArea) visible() bool {
	if wa.Set.Deleted && wa.Deleted() {
		return false
	}
	if wa.Filter != nil && !wa.Filter() {
		return false
	}
	return true
}

// Sync refreshes child work areas after the record pointer moves.
func (wa *WorkArea) Sync() error {
	for _, r := range wa.Relations {
		if wa.EOF {
			r.Child.GoTo(0)
			continue
		}
		v, err := r.KeyFn()
		if err != nil {
			return err
		}
		if cix := r.Child.CurIndex(); cix != nil {
			r.Child.Seek(cix.SeekString(v), false, false)
		} else if v.K == value.NUMBER {
			r.Child.GoTo(int(v.N))
		} else {
			r.Child.GoTo(0)
		}
	}
	return nil
}

// ---------- navigation ----------

// refresh picks up what other sessions have done to a shared table: new
// records, and changed indexes.
func (wa *WorkArea) refresh() error {
	if !wa.T.Shared() {
		return nil
	}
	wa.lastRefresh = time.Now()
	wa.T.Refresh()
	for i, ix := range wa.Indexes {
		changed, err := ix.Refresh()
		if err != nil {
			return err
		}
		if changed && i == wa.Order-1 {
			wa.pos = -1
		}
	}
	return nil
}

// refreshInterval is how often SKIP looks for other sessions' changes on a
// shared table. Scanning commands (LIST, COUNT, SUM, REPLACE ALL...) move
// with SKIP; checking the headers on every step made them two and a half
// times slower. GO, SEEK and SKIP 0 always check.
const refreshInterval = 50 * time.Millisecond

// refreshSoon is refresh for SKIP: at most once per refreshInterval.
func (wa *WorkArea) refreshSoon() error {
	if !wa.T.Shared() || time.Since(wa.lastRefresh) < refreshInterval {
		return nil
	}
	return wa.refresh()
}

// Refresh is refresh for callers outside the package (LastRec(), SKIP 0).
func (wa *WorkArea) Refresh() error { return wa.refresh() }

func (wa *WorkArea) GoTo(n int) error {
	if err := wa.Commit(); err != nil {
		return err
	}
	if err := wa.refresh(); err != nil {
		return err
	}
	wa.Found = false
	if n < 1 || n > wa.T.RecCount() {
		wa.phantom()
		wa.BOF = wa.T.RecCount() == 0
		return wa.Sync()
	}
	if err := wa.load(n); err != nil {
		return err
	}
	wa.pos = -1
	return wa.Sync()
}

func (wa *WorkArea) GoTop() error {
	if err := wa.Commit(); err != nil {
		return err
	}
	if err := wa.refresh(); err != nil {
		return err
	}
	wa.Found = false
	if ix := wa.CurIndex(); ix != nil {
		for p := 0; p < ix.Len(); p++ {
			if err := wa.load(int(ix.At(p).Rec)); err != nil {
				return err
			}
			wa.pos = p
			if wa.visible() {
				return wa.Sync()
			}
		}
	} else {
		for r := 1; r <= wa.T.RecCount(); r++ {
			if err := wa.load(r); err != nil {
				return err
			}
			if wa.visible() {
				return wa.Sync()
			}
		}
	}
	wa.phantom()
	wa.BOF = true
	return wa.Sync()
}

func (wa *WorkArea) GoBottom() error {
	if err := wa.Commit(); err != nil {
		return err
	}
	if err := wa.refresh(); err != nil {
		return err
	}
	wa.Found = false
	if ix := wa.CurIndex(); ix != nil {
		for p := ix.Len() - 1; p >= 0; p-- {
			if err := wa.load(int(ix.At(p).Rec)); err != nil {
				return err
			}
			wa.pos = p
			if wa.visible() {
				return wa.Sync()
			}
		}
	} else {
		for r := wa.T.RecCount(); r >= 1; r-- {
			if err := wa.load(r); err != nil {
				return err
			}
			if wa.visible() {
				return wa.Sync()
			}
		}
	}
	wa.phantom()
	wa.BOF = true
	return wa.Sync()
}

// ensurePos locates the current record within the current order.
func (wa *WorkArea) ensurePos(ix *Index) {
	if wa.pos >= 0 && wa.pos < ix.Len() && int(ix.At(wa.pos).Rec) == wa.Recno {
		return
	}
	if wa.EOF {
		wa.pos = ix.Len()
		return
	}
	key := ""
	if wa.origKeys != nil && wa.Order-1 < len(wa.origKeys) {
		key = wa.origKeys[wa.Order-1]
	} else if k, _, err := wa.evalKey(ix); err == nil {
		key = k
	}
	wa.pos = ix.LowerBound(key, uint32(wa.Recno))
}

func (wa *WorkArea) Skip(n int) error {
	if err := wa.Commit(); err != nil {
		return err
	}
	refresh := wa.refreshSoon
	if n == 0 {
		refresh = wa.refresh // SKIP 0: re-read now
	}
	if err := refresh(); err != nil {
		return err
	}
	wa.Found = false
	if n == 0 {
		if wa.Recno >= 1 && wa.Recno <= wa.T.RecCount() {
			if err := wa.load(wa.Recno); err != nil {
				return err
			}
		}
		return wa.Sync()
	}
	ix := wa.CurIndex()
	if ix != nil {
		wa.ensurePos(ix)
	}
	wa.BOF = false
	for n > 0 {
		if wa.EOF {
			break
		}
		if !wa.stepFwd(ix) {
			wa.phantom()
			break
		}
		n--
	}
	for n < 0 {
		start := wa.Recno
		wasEOF := wa.EOF
		startPos := wa.pos
		if !wa.stepBack(ix) {
			// hit top: stay on first visible record
			if wasEOF {
				wa.phantom()
				wa.BOF = true
			} else {
				if ix != nil {
					wa.pos = startPos
				}
				wa.load(start)
				wa.BOF = true
			}
			break
		}
		n++
	}
	return wa.Sync()
}

func (wa *WorkArea) stepFwd(ix *Index) bool {
	if ix != nil {
		for p := wa.pos + 1; p < ix.Len(); p++ {
			wa.load(int(ix.At(p).Rec))
			wa.pos = p
			if wa.visible() {
				return true
			}
		}
		return false
	}
	for r := wa.Recno + 1; r <= wa.T.RecCount(); r++ {
		wa.load(r)
		if wa.visible() {
			return true
		}
	}
	return false
}

func (wa *WorkArea) stepBack(ix *Index) bool {
	if ix != nil {
		for p := wa.pos - 1; p >= 0; p-- {
			wa.load(int(ix.At(p).Rec))
			wa.pos = p
			if wa.visible() {
				return true
			}
		}
		return false
	}
	start := wa.Recno
	if start > wa.T.RecCount()+1 {
		start = wa.T.RecCount() + 1
	}
	for r := start - 1; r >= 1; r-- {
		wa.load(r)
		if wa.visible() {
			return true
		}
	}
	return false
}

// Seek positions on key using the controlling index.
func (wa *WorkArea) Seek(key string, soft, last bool) (bool, error) {
	if err := wa.Commit(); err != nil {
		return false, err
	}
	if err := wa.refresh(); err != nil {
		return false, err
	}
	ix := wa.CurIndex()
	if ix == nil {
		return false, errors.New("no index is in use")
	}
	p := ix.SeekPos(key)
	if last {
		q := p
		for q < ix.Len() && ix.Matches(q, key) {
			q++
		}
		if q > p {
			p = q - 1
		}
	}
	for p < ix.Len() && p >= 0 {
		wa.load(int(ix.At(p).Rec))
		wa.pos = p
		if wa.visible() {
			break
		}
		if last {
			p--
		} else {
			p++
		}
	}
	if p >= 0 && p < ix.Len() && ix.Matches(p, key) {
		wa.Found = true
		return true, wa.Sync()
	}
	wa.Found = false
	if (soft || wa.Set.SoftSeek) && p >= 0 && p < ix.Len() {
		return false, wa.Sync()
	}
	wa.phantom()
	return false, wa.Sync()
}

// Append adds a blank record and makes it current.
func (wa *WorkArea) Append() error {
	if err := wa.Commit(); err != nil {
		return err
	}
	if wa.T.ReadOnly {
		return errors.New("table is read-only")
	}
	touched := append([]*Index(nil), wa.Indexes...)
	if err := wa.beginAll(touched); err != nil {
		return err
	}
	n, err := wa.T.AppendRecord(wa.T.BlankRecord())
	if err != nil {
		abortAll(touched)
		return err
	}
	if err := wa.load(n); err != nil {
		abortAll(touched)
		return err
	}
	for i, ix := range wa.Indexes {
		if wa.inFor[i] {
			ix.Insert(wa.origKeys[i], uint32(n))
		}
	}
	if err := endAll(touched, wa.T.RecCount()); err != nil {
		return err
	}
	wa.pos = -1
	wa.Found = false
	return wa.Sync()
}

// Pack removes deleted records.
func (wa *WorkArea) Pack() error {
	if wa.T.Shared() {
		return ErrExclusiveRequired
	}
	if err := wa.Commit(); err != nil {
		return err
	}
	t := wa.T
	out := 0
	buf := make([]byte, t.RecLen)
	for r := 1; r <= t.RecCount(); r++ {
		if err := t.ReadRecord(r, buf); err != nil {
			return err
		}
		if buf[0] == '*' {
			continue
		}
		out++
		if out != r {
			if err := t.WriteRecord(out, buf); err != nil {
				return err
			}
		}
	}
	if err := t.Truncate(out); err != nil {
		return err
	}
	if err := wa.Reindex(); err != nil {
		return err
	}
	return wa.GoTop()
}

func (wa *WorkArea) Zap() error {
	if wa.T.Shared() {
		return ErrExclusiveRequired
	}
	wa.dirty = false
	if err := wa.T.Truncate(0); err != nil {
		return err
	}
	wa.T.ZapMemo()
	for _, ix := range wa.Indexes {
		ix.SetEntries(nil)
		ix.TableRecs = 0
		ix.Save()
	}
	return wa.GoTop()
}

// buildIndex scans all records to fill ix.
func (wa *WorkArea) buildIndex(ix *Index) error {
	var list []Entry
	for r := 1; r <= wa.T.RecCount(); r++ {
		if err := wa.readRaw(r); err != nil {
			return err
		}
		k, in, err := wa.evalKey(ix)
		if err != nil {
			return err
		}
		if in {
			list = append(list, Entry{k, uint32(r)})
		}
	}
	ix.SetEntries(list)
	ix.TableRecs = wa.T.RecCount()
	return ix.Save()
}

func (wa *WorkArea) Reindex() error {
	if wa.T.Shared() {
		return ErrExclusiveRequired
	}
	if err := wa.Commit(); err != nil {
		return err
	}
	for _, ix := range wa.Indexes {
		if err := wa.buildIndex(ix); err != nil {
			return err
		}
	}
	wa.origKeys = nil
	return wa.GoTop()
}

// CreateIndex builds a new index and makes it the controlling order.
func (wa *WorkArea) CreateIndex(ix *Index) error {
	if err := wa.Commit(); err != nil {
		return err
	}
	// determine key format from the first record (or a blank one)
	if wa.T.RecCount() > 0 {
		wa.readRaw(1)
	} else {
		wa.phantom()
	}
	v, err := ix.KeyFn()
	if err != nil {
		return err
	}
	ix.CP = wa.T.CodePage()
	ix.SetKeyFormat(v)
	if err := wa.buildIndex(ix); err != nil {
		return err
	}
	if wa.T.Shared() {
		if err := ix.Share(wa.T.ReadOnly); err != nil {
			return err
		}
	}
	// replace an index with the same path
	for i, old := range wa.Indexes {
		if samePath(old.Path, ix.Path) {
			old.Close()
			wa.Indexes = append(wa.Indexes[:i], wa.Indexes[i+1:]...)
			break
		}
	}
	wa.Indexes = append(wa.Indexes, ix)
	wa.Order = len(wa.Indexes)
	wa.origKeys = nil
	return wa.GoTop()
}

func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}

// AddIndex attaches an already opened index.
func (wa *WorkArea) AddIndex(ix *Index) error {
	ix.CP = wa.T.CodePage()
	if ix.KeyType == 0 {
		if wa.T.RecCount() > 0 {
			save := wa.Recno
			wa.readRaw(1)
			if v, err := ix.KeyFn(); err == nil {
				switch v.K {
				case value.STRING:
					ix.KeyType = 'C'
				case value.NUMBER:
					ix.KeyType = 'N'
				case value.DATE:
					ix.KeyType = 'D'
				case value.LOGICAL:
					ix.KeyType = 'L'
				}
			}
			wa.readRaw(save)
		}
	}
	wa.Indexes = append(wa.Indexes, ix)
	if len(wa.Indexes) == 1 {
		wa.Order = 1
	}
	wa.origKeys = nil
	if wa.Recno >= 1 && wa.Recno <= wa.T.RecCount() {
		wa.computeKeys()
	}
	return nil
}

func (wa *WorkArea) ClearIndexes() error {
	if err := wa.Commit(); err != nil {
		return err
	}
	for _, ix := range wa.Indexes {
		wa.closeIndex(ix)
	}
	wa.Indexes = nil
	wa.Order = 0
	wa.origKeys = nil
	wa.pos = -1
	return nil
}

func (wa *WorkArea) SetOrder(n int) error {
	if err := wa.Commit(); err != nil {
		return err
	}
	if n < 0 || n > len(wa.Indexes) {
		return fmt.Errorf("invalid order %d", n)
	}
	wa.Order = n
	wa.pos = -1
	return nil
}

// Flush saves the record and dirty indexes.
func (wa *WorkArea) Flush() error {
	if err := wa.Commit(); err != nil {
		return err
	}
	// COMMIT: write index changes as change log entries, not whole files
	for _, ix := range wa.Indexes {
		if ix.f == nil {
			ix.TableRecs = wa.T.RecCount()
			if err := ix.Persist(); err != nil {
				return err
			}
		}
	}
	wa.T.Flush()
	return nil
}

func (wa *WorkArea) Close() error {
	err := wa.Flush()
	for _, ix := range wa.Indexes {
		wa.closeIndex(ix)
	}
	wa.T.Close()
	return err
}

// RecordVisible reports whether the current record passes filter/deleted.
func (wa *WorkArea) RecordVisible() bool { return wa.visible() }

// FileExists helper.
func FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// RebuildIndex rebuilds an index that Stale reports out of date. On a
// read-only table the rebuilt keys are only kept in memory.
func (wa *WorkArea) RebuildIndex(ix *Index) error {
	save := wa.Recno
	if ix.f != nil && !wa.T.ReadOnly {
		if err := lockRange(ix.f, indexLock, 1, true, true); err != nil {
			return err
		}
		defer unlockRange(ix.f, indexLock, 1)
		// another session may have been adding a record when we looked:
		// look again now that nobody can change the index
		wa.T.Refresh()
		if err := ix.reload(); err != nil {
			return err
		}
		if stale, _ := ix.Stale(wa.T.RecCount()); !stale {
			return nil
		}
	}
	var list []Entry
	for r := 1; r <= wa.T.RecCount(); r++ {
		if err := wa.readRaw(r); err != nil {
			return err
		}
		k, in, err := wa.evalKey(ix)
		if err != nil {
			return err
		}
		if in {
			list = append(list, Entry{k, uint32(r)})
		}
	}
	ix.SetEntries(list)
	ix.TableRecs = wa.T.RecCount()
	if save >= 1 && save <= wa.T.RecCount() {
		wa.readRaw(save)
	}
	if wa.T.ReadOnly {
		ix.diskDirty = false
		return nil
	}
	return ix.Save()
}

// closeIndex writes an index out and closes it: an exclusive one is
// written whole, leaving a plain NTX file; a shared one only if its change
// log has grown long.
func (wa *WorkArea) closeIndex(ix *Index) {
	if ix.f != nil {
		ix.CloseShared(wa.T.ReadOnly)
		return
	}
	if !wa.T.ReadOnly {
		ix.TableRecs = wa.T.RecCount()
		ix.Compact()
	}
	ix.Close()
}
