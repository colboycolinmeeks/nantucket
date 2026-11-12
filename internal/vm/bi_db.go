// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"nantucket/internal/db"
	"nantucket/internal/value"
)

// ---------- work area helpers ----------

func (vm *VM) needWA(op string) *db.WorkArea {
	wa := vm.CurWA()
	if wa == nil {
		vm.Raise(&RTError{SubSystem: "DBCMD", GenCode: 15, SubCode: 2001, Description: "Workarea not in use", Operation: op})
	}
	return wa
}

func (vm *VM) dbErr(err error, op string) {
	if err == nil {
		return
	}
	if e, ok := err.(*RTError); ok {
		panic(e)
	}
	vm.Raise(dbError(err, op))
}

// dbError turns a database error into a runtime error, with the Clipper
// codes for the shared-use errors.
func dbError(err error, op string) *RTError {
	switch {
	case errors.Is(err, db.ErrLockRequired):
		return &RTError{SubSystem: "DBFNTX", GenCode: 38, SubCode: 1022, Description: "Lock required", Operation: op,
			Hint: "Lock the record with RLock() (or the table with FLock()) first, or open the table EXCLUSIVE.", CanDefault: true}
	case errors.Is(err, db.ErrExclusiveRequired):
		return &RTError{SubSystem: "DBFNTX", GenCode: 37, SubCode: 1023, Description: "Exclusive required", Operation: op,
			Hint: "Open the table with USE ... EXCLUSIVE (nobody else may have it open).", CanDefault: true}
	case errors.Is(err, db.ErrLocked):
		return &RTError{SubSystem: "DBFNTX", GenCode: 41, SubCode: 1038, Description: "Record or file is locked by another user", Operation: op, CanDefault: true}
	}
	return errDB(1001, err.Error(), op)
}

// openMode is how USE opens a table: shared is the SHARED / EXCLUSIVE
// clause (NIL: follow SET EXCLUSIVE).
func (vm *VM) openMode(shared value.Value) db.OpenMode {
	sh := !vm.sets[8].Truthy()
	if shared.K == value.LOGICAL {
		sh = shared.B
	}
	if sh {
		return db.OpenShared
	}
	return db.OpenExclusive
}

// areaEval evaluates a block with area n selected.
func (vm *VM) areaEval(n int, b value.Value) value.Value {
	save := vm.curArea
	vm.curArea = n
	defer func() { vm.curArea = save }()
	return vm.EvalBlock(b)
}

func (vm *VM) freeArea() int {
	for i := 1; i < 250; i++ {
		if vm.areas[i] == nil {
			return i
		}
	}
	vm.Raise(errMsg("No free work area"))
	return 0
}

func aliasFromPath(p string) string {
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	var sb strings.Builder
	for _, c := range strings.ToUpper(base) {
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9' && sb.Len() > 0) {
			sb.WriteRune(c)
		}
	}
	if sb.Len() == 0 {
		return "AREA"
	}
	return sb.String()
}

// UseArea opens a DBF in the current (or a new) work area.
func (vm *VM) UseArea(newArea bool, file, alias string, readOnly bool) {
	vm.UseAreaMode(newArea, file, alias, readOnly, vm.openMode(value.Nil))
}

// UseAreaMode opens a table in the current (or a new) work area. When
// another session's use conflicts (the table is open EXCLUSIVE elsewhere,
// or this is an EXCLUSIVE open of a table in use), NetErr() is set and
// the work area is left empty rather than raising an error.
func (vm *VM) UseAreaMode(newArea bool, file, alias string, readOnly bool, mode db.OpenMode) {
	vm.netErr = false
	path := vm.FindFile(file, ".dbf")
	if path == "" {
		vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1001, Description: "Open error", Filename: vm.DefaultPath(file, ".dbf")})
		return
	}
	if newArea {
		vm.curArea = vm.freeArea()
	}
	if vm.areas[vm.curArea] != nil {
		vm.closeArea(vm.curArea)
	}
	if alias == "" {
		alias = aliasFromPath(path)
	}
	alias = strings.ToUpper(strings.TrimSpace(alias))
	for i, wa := range vm.areas {
		if wa != nil && wa.Alias == alias {
			vm.Raise(&RTError{SubSystem: "DBCMD", GenCode: 13, SubCode: 1011, Description: "Alias already in use", Operation: alias})
			_ = i
			return
		}
	}
	t, err := db.OpenTableMode(path, readOnly, mode)
	if errors.Is(err, db.ErrInUse) {
		vm.netErr = true
		vm.netErrNote = filepath.Base(path) + " is in use by another user (NetErr() is .T.)"
		return
	}
	if err != nil {
		vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1001, Description: "Open error: " + err.Error(), Filename: path})
		return
	}
	t.SetCodePage(vm.CodePage())
	wa := db.NewWorkArea(vm.curArea, alias, t, &vm.DBSet)
	vm.areas[vm.curArea] = wa
	vm.dbErr(wa.GoTop(), "USE")
}

func (vm *VM) closeArea(n int) {
	wa := vm.areas[n]
	if wa == nil {
		return
	}
	// drop relations pointing at this area
	for _, other := range vm.areas {
		if other == nil {
			continue
		}
		rels := other.Relations[:0]
		for _, r := range other.Relations {
			if r.Child != wa {
				rels = append(rels, r)
			}
		}
		other.Relations = rels
	}
	err := wa.Close()
	vm.areas[n] = nil
	vm.dbErr(err, "CLOSE")
}

// CloseAll closes every work area.
// Close ends the runtime: it closes every table and every file opened
// with FOpen() or FCreate(). (CloseAll, for CLOSE DATABASES, closes only
// the tables.)
func (vm *VM) Close() {
	vm.RunExitProcs()
	vm.closeFiles()
	vm.SetDebugLogFile("")
}

// closeFiles closes the tables and every other open file.
func (vm *VM) closeFiles() {
	vm.CloseAll()
	for h, f := range vm.memFiles {
		f.Close()
		delete(vm.memFiles, h)
	}
	vm.endText()
}

func (vm *VM) CloseAll() {
	for i := range vm.areas {
		if vm.areas[i] != nil {
			vm.closeArea(i)
		}
	}
	vm.curArea = 1
}

func (vm *VM) openIndex(wa *db.WorkArea, file string) {
	path := vm.FindFile(file, ".ntx")
	if path == "" {
		if strings.EqualFold(filepath.Ext(file), ".cdx") || vm.FindFile(file, ".cdx") != "" {
			vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1003, Description: "CDX indexes are not supported yet", Filename: file})
			return
		}
		vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1003, Description: "Open error", Filename: vm.DefaultPath(file, ".ntx")})
		return
	}
	ix, err := db.OpenNTX(path, wa.T.Mode(), wa.T.ReadOnly)
	if err != nil {
		vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1012, Description: "Corruption detected: " + err.Error(), Filename: path})
		return
	}
	// the key and FOR expressions come from the file: check them first
	for _, ex := range []struct{ what, src string }{{"index key", ix.KeyExpr}, {"index FOR", ix.ForExpr}} {
		if err := vm.CheckDataExpr(ex.src); err != nil {
			ix.Close()
			vm.Raise(dataExprError(ex.what, path, ex.src, err))
			return
		}
	}
	if err := vm.bindIndex(wa, ix); err != nil {
		vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1012, Description: "Invalid index key expression: " + ix.KeyExpr, Filename: path})
		return
	}
	vm.dbErr(wa.AddIndex(ix), "SET INDEX")
	// an index that wasn't saved after its last change (Nantucket was
	// stopped), or that missed records added while it was closed, is
	// rebuilt so that it can't silently leave records out
	wa.T.Refresh()
	if stale, why := ix.Stale(wa.T.RecCount()); stale {
		vm.dbErr(wa.RebuildIndex(ix), "SET INDEX")
		vm.notes = append(vm.notes, fmt.Sprintf("Index %s was out of date (%s) and has been rebuilt", filepath.Base(path), why))
	}
	// as in Clipper, USE ... INDEX and SET INDEX TO go to the first record
	// in index order
	vm.dbErr(wa.GoTop(), "SET INDEX")
}

func (vm *VM) bindIndex(wa *db.WorkArea, ix *db.Index) error {
	kb, err := vm.CompileBlockText(ix.KeyExpr)
	if err != nil {
		return err
	}
	n := wa.Num
	ix.KeyFn = func() (value.Value, error) { return vm.areaEval(n, kb), nil }
	if strings.TrimSpace(ix.ForExpr) != "" {
		fb, err := vm.CompileBlockText(ix.ForExpr)
		if err != nil {
			return err
		}
		ix.ForFn = func() (bool, error) { return vm.areaEval(n, fb).Truthy(), nil }
	}
	return nil
}

func (vm *VM) blockOrText(b value.Value, text string) (value.Value, bool) {
	if b.K == value.BLOCK {
		return b, true
	}
	if strings.TrimSpace(text) != "" {
		bb, err := vm.CompileBlockText(text)
		if err == nil {
			return bb, true
		}
	}
	return value.Nil, false
}

// ---------- scoped iteration ----------

// scope is a record scope (see Scope).
type scope struct {
	For, While value.Value // FOR and WHILE blocks (NIL if not given)
	Next       int         // NEXT count, when HasNext
	HasNext    bool
	Rec        int // RECORD number, when HasRec
	HasRec     bool
	Rest       bool // REST
}

func scopeArgs(a []value.Value, i int) scope {
	s := scope{For: arg(a, i), While: arg(a, i+1)}
	if has(a, i+2) {
		s.HasNext, s.Next = true, argInt(a, i+2, 0)
	}
	if has(a, i+3) {
		s.HasRec, s.Rec = true, argInt(a, i+3, 0)
	}
	s.Rest = argBool(a, i+4, false)
	return s
}

// eachRecord iterates the scope; fn returns false to stop.
func (vm *VM) eachRecord(wa *db.WorkArea, s scope, fn func() bool) {
	test := func(b value.Value) bool {
		if b.K != value.BLOCK {
			return true
		}
		return vm.EvalBlock(b).Truthy()
	}
	if s.HasRec {
		vm.dbErr(wa.GoTo(s.Rec), "GOTO")
		if !wa.EOF && test(s.For) {
			fn()
		}
		return
	}
	if !(s.While.K == value.BLOCK || s.HasNext || s.Rest) {
		vm.dbErr(wa.GoTop(), "GOTOP")
	}
	count := 0
	for !wa.EOF {
		if vm.terminate.Load() {
			panic(&QuitSignal{})
		}
		if vm.interrupt.Load() {
			vm.interrupt.Store(false)
			panic(&CancelSignal{Interrupted: true})
		}
		if s.HasNext && count >= s.Next {
			break
		}
		if s.While.K == value.BLOCK && !test(s.While) {
			break
		}
		if test(s.For) {
			if !fn() {
				return
			}
		}
		count++
		vm.dbErr(wa.Skip(1), "SKIP")
	}
}

// ---------- formatting ----------

func (vm *VM) fieldDisplay(wa *db.WorkArea, i int, forList bool) string {
	fd := wa.T.Fields[i]
	v := wa.GetField(i)
	switch fd.Type {
	case 'M':
		if forList {
			if v.S == "" {
				return "memo"
			}
			return "Memo"
		}
		return v.S
	case 'L':
		if v.B {
			return ".T."
		}
		return ".F."
	case 'D':
		return vm.DTOC(v.J)
	}
	if v.K == value.NUMBER {
		return value.FormatNum(v.N, fd.Len, fd.Dec)
	}
	return vm.Display(v)
}

func fieldWidth(vm *VM, fd db.FieldDef) int {
	switch fd.Type {
	case 'M':
		return 4
	case 'L':
		return 3
	case 'D':
		return len(vm.DTOC(0))
	}
	return fd.Len
}

func init() {
	reg("DBUSEAREA", func(vm *VM, a []value.Value) value.Value {
		vm.UseAreaMode(argBool(a, 0, false), argStr(a, 2, ""), argStr(a, 3, ""), argBool(a, 5, false), vm.openMode(arg(a, 4)))
		return value.Nil
	})
	reg("DBCLOSEAREA", func(vm *VM, a []value.Value) value.Value {
		vm.closeArea(vm.curArea)
		return value.Nil
	})
	reg("DBCLOSEALL", func(vm *VM, a []value.Value) value.Value {
		save := vm.curArea
		vm.CloseAll()
		vm.curArea = save
		return value.Nil
	})
	reg("DBSELECTAREA", func(vm *VM, a []value.Value) value.Value {
		v := arg(a, 0)
		n := 0
		switch v.K {
		case value.NUMBER:
			n = v.Int()
			if n == 0 {
				n = vm.freeArea()
			}
		case value.STRING:
			if strings.TrimSpace(v.S) == "0" { // SELECT 0: the first free area
				n = vm.freeArea()
				break
			}
			n = vm.areaByAlias(v.S)
			if n == 0 {
				vm.aliasErr(v.S)
				return value.Nil
			}
		}
		if n < 1 || n >= len(vm.areas) {
			return vm.Raise(errArg("DBSELECTAREA", v))
		}
		vm.curArea = n
		return value.Nil
	})
	reg("SELECT", func(vm *VM, a []value.Value) value.Value {
		if has(a, 0) {
			return value.Int(vm.areaFromValue(arg(a, 0)))
		}
		return value.Int(vm.curArea)
	})
	reg("ALIAS", func(vm *VM, a []value.Value) value.Value {
		n := argInt(a, 0, vm.curArea)
		if n >= 1 && n < len(vm.areas) && vm.areas[n] != nil {
			return value.Str(vm.areas[n].Alias)
		}
		return value.Str("")
	})
	reg("USED", func(vm *VM, a []value.Value) value.Value { return value.Bool(vm.CurWA() != nil) })
	reg("DBAPPEND", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("APPEND"); wa != nil {
			vm.netErr = false
			err := wa.Append()
			if errors.Is(err, db.ErrLocked) {
				vm.netErr = true
				return value.Nil
			}
			vm.dbErr(err, "APPEND")
		}
		return value.Nil
	})
	reg("DBGOTO", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("GOTO"); wa != nil {
			vm.dbErr(wa.GoTo(argInt(a, 0, 0)), "GOTO")
		}
		return value.Nil
	})
	reg("DBGOTOP", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("GOTOP"); wa != nil {
			vm.dbErr(wa.GoTop(), "GOTOP")
		}
		return value.Nil
	})
	reg("DBGOBOTTOM", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("GOBOTTOM"); wa != nil {
			vm.dbErr(wa.GoBottom(), "GOBOTTOM")
		}
		return value.Nil
	})
	reg("DBSKIP", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("SKIP"); wa != nil {
			vm.dbErr(wa.Skip(argInt(a, 0, 1)), "SKIP")
		}
		return value.Nil
	})
	reg("DBSEEK", func(vm *VM, a []value.Value) value.Value {
		wa := vm.needWA("SEEK")
		if wa == nil {
			return value.False
		}
		ix := wa.CurIndex()
		if ix == nil {
			return vm.Raise(&RTError{SubSystem: "DBCMD", GenCode: 20, SubCode: 1201, Description: "No controlling index", Operation: "SEEK"})
		}
		soft := argBool(a, 1, vm.DBSet.SoftSeek)
		found, err := wa.Seek(ix.SeekString(arg(a, 0)), soft, argBool(a, 2, false))
		vm.dbErr(err, "SEEK")
		return value.Bool(found)
	})
	reg("__DBFIND", func(vm *VM, a []value.Value) value.Value {
		s := strings.TrimSpace(vm.MacroSubst(argStr(a, 0, ""), false))
		if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
			s = s[1 : len(s)-1]
		}
		wa := vm.needWA("FIND")
		if wa == nil || wa.CurIndex() == nil {
			return vm.Raise(&RTError{SubSystem: "DBCMD", GenCode: 20, SubCode: 1201, Description: "No controlling index", Operation: "FIND"})
		}
		ix := wa.CurIndex()
		key := value.Str(s)
		if ix.KeyType == 'N' {
			key = vm.MacroEval(s)
		}
		_, err := wa.Seek(ix.SeekString(key), vm.DBSet.SoftSeek, false)
		vm.dbErr(err, "FIND")
		return value.Nil
	})
	reg("EOF", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Bool(wa.EOF)
		}
		return value.True
	})
	reg("BOF", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Bool(wa.BOF)
		}
		return value.True
	})
	reg("FOUND", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Bool(wa.Found)
		}
		return value.False
	})
	reg("RECNO", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Int(wa.Recno)
		}
		return value.Int(0)
	})
	recCount := func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			wa.T.Refresh()
			return value.Int(wa.T.RecCount())
		}
		return value.Int(0)
	}
	reg("RECCOUNT", recCount)
	reg("LASTREC", recCount)
	reg("HEADER", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Int(wa.T.HdrLen())
		}
		return value.Int(0)
	})
	reg("RECSIZE", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Int(wa.T.RecLen)
		}
		return value.Int(0)
	})
	reg("LUPDATE", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			y, m, d := wa.T.LastUpdate()
			if value.ValidDate(y, m, d) {
				return value.Date(value.ToJulian(y, m, d))
			}
		}
		return value.Date(0)
	})
	reg("DELETED", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Bool(wa.Deleted())
		}
		return value.False
	})
	reg("DBDELETE", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("DELETE"); wa != nil {
			vm.dbErr(wa.SetDeleted(true), "DELETE")
		}
		return value.Nil
	})
	reg("DBRECALL", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("RECALL"); wa != nil {
			vm.dbErr(wa.SetDeleted(false), "RECALL")
		}
		return value.Nil
	})
	reg("__DBPACK", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("PACK"); wa != nil {
			before := wa.T.RecCount()
			vm.dbErr(wa.Pack(), "PACK")
			if vm.sets[100].Truthy() || vm.Con.Interactive() && len(vm.frames) <= 2 {
				vm.Println(fmt.Sprintf("%d records copied, %d removed", wa.T.RecCount(), before-wa.T.RecCount()))
			}
		}
		return value.Nil
	})
	reg("__DBZAP", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("ZAP"); wa != nil {
			vm.dbErr(wa.Zap(), "ZAP")
		}
		return value.Nil
	})
	reg("DBCOMMIT", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			vm.dbErr(wa.Flush(), "COMMIT")
		}
		return value.Nil
	})
	reg("DBCOMMITALL", func(vm *VM, a []value.Value) value.Value {
		for _, wa := range vm.areas {
			if wa != nil {
				vm.dbErr(wa.Flush(), "COMMIT")
			}
		}
		return value.Nil
	})
	// Locks. On a table opened SHARED these are real locks that other
	// sessions see; on one opened EXCLUSIVE they always succeed.
	reg("DBUNLOCK", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			vm.dbErr(wa.Commit(), "UNLOCK")
			wa.T.UnlockAll()
		}
		return value.Nil
	})
	reg("DBUNLOCKALL", func(vm *VM, a []value.Value) value.Value {
		for _, wa := range vm.areas {
			if wa != nil {
				vm.dbErr(wa.Commit(), "UNLOCK ALL")
				wa.T.UnlockAll()
			}
		}
		return value.Nil
	})
	// dbRUnlock([nRecord]) releases one record lock, or all of them.
	reg("DBRUNLOCK", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			vm.dbErr(wa.Commit(), "UNLOCK")
			if has(a, 0) {
				wa.T.UnlockRecord(argInt(a, 0, 0))
			} else {
				for _, n := range wa.T.LockedRecords() {
					wa.T.UnlockRecord(n)
				}
			}
		}
		return value.Nil
	})
	// RLock() / Lock(): release this area's record locks, then lock the
	// current record.
	rlock := func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil || wa.EOF {
			return value.False
		}
		vm.dbErr(wa.Commit(), "RLOCK")
		if wa.T.FileLocked() {
			return value.True
		}
		for _, n := range wa.T.LockedRecords() {
			if n != wa.Recno {
				wa.T.UnlockRecord(n)
			}
		}
		return vm.lockResult(wa, wa.T.LockRecord(wa.Recno))
	}
	reg("RLOCK", rlock)
	reg("LOCK", rlock)
	// dbRLock([nRecord]) adds a lock on a record (default: the current
	// one) without releasing others.
	reg("DBRLOCK", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.False
		}
		n := argInt(a, 0, wa.Recno)
		if n < 1 || n > wa.T.RecCount() {
			return value.False
		}
		return vm.lockResult(wa, wa.T.LockRecord(n))
	})
	reg("FLOCK", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.False
		}
		vm.dbErr(wa.Commit(), "FLOCK")
		return vm.lockResult(wa, wa.T.LockFile())
	})
	// NetErr([lNew]) reports whether the last USE or APPEND BLANK failed
	// because of another session.
	reg("NETERR", func(vm *VM, a []value.Value) value.Value {
		vm.netErrNote = "" // the program is handling it
		old := vm.netErr
		if arg(a, 0).K == value.LOGICAL {
			vm.netErr = arg(a, 0).B
		}
		return value.Bool(old)
	})
	reg("DBRLOCKLIST", func(vm *VM, a []value.Value) value.Value {
		var out []value.Value
		if wa := vm.CurWA(); wa != nil {
			for _, n := range wa.T.LockedRecords() {
				out = append(out, value.Int(n))
			}
		}
		return value.NewArray(out)
	})
	// IsLocked([nRecord]) / dbRecordInfo-style check: is the record (or,
	// with FLock(), the file) locked by this session?
	reg("ISLOCKED", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.False
		}
		return value.Bool(wa.T.RecordLocked(argInt(a, 0, wa.Recno)))
	})
	reg("ISSHARED", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		return value.Bool(wa != nil && wa.T.Shared())
	})

	// fields
	reg("FCOUNT", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Int(len(wa.T.Fields))
		}
		return value.Int(0)
	})
	fieldName := func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 0)
		if wa != nil && n >= 1 && n <= len(wa.T.Fields) {
			return value.Str(wa.T.Fields[n-1].Name)
		}
		return value.Str("")
	}
	reg("FIELDNAME", fieldName)
	reg("FIELD", fieldName)
	reg("FIELDPOS", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Int(wa.T.FieldIndex(strings.TrimSpace(argStr(a, 0, ""))) + 1)
		}
		return value.Int(0)
	})
	reg("FIELDGET", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 0)
		if wa != nil && n >= 1 && n <= len(wa.T.Fields) {
			return wa.GetField(n - 1)
		}
		return value.Nil
	})
	reg("FIELDPUT", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 0)
		if wa != nil && n >= 1 && n <= len(wa.T.Fields) {
			vm.putField(wa, wa.T.Fields[n-1].Name, arg(a, 1))
			return arg(a, 1)
		}
		return value.Nil
	})
	reg("FIELDTYPE", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 0)
		if wa != nil && n >= 1 && n <= len(wa.T.Fields) {
			return value.Str(string(wa.T.Fields[n-1].Type))
		}
		return value.Str("")
	})
	reg("FIELDLEN", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 0)
		if wa != nil && n >= 1 && n <= len(wa.T.Fields) {
			return value.Int(wa.T.Fields[n-1].Len)
		}
		return value.Int(0)
	})
	reg("FIELDDEC", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 0)
		if wa != nil && n >= 1 && n <= len(wa.T.Fields) {
			return value.Int(wa.T.Fields[n-1].Dec)
		}
		return value.Int(0)
	})
	reg("FIELDBLOCK", func(vm *VM, a []value.Value) value.Value {
		name := strings.ToUpper(strings.TrimSpace(argStr(a, 0, "")))
		return value.Value{K: value.BLOCK, P: &value.Block{Src: "{|x| FIELD->" + name + "}", Fn: NativeBlock(func(vm *VM, args []value.Value) value.Value {
			if len(args) > 0 && args[0].Deref().K != value.NIL {
				vm.putField(vm.CurWA(), name, args[0])
				return args[0].Deref()
			}
			return vm.getField(vm.CurWA(), name)
		})}}
	})
	reg("FIELDWBLOCK", func(vm *VM, a []value.Value) value.Value {
		name := strings.ToUpper(strings.TrimSpace(argStr(a, 0, "")))
		area := argInt(a, 1, vm.curArea)
		return value.Value{K: value.BLOCK, P: &value.Block{Fn: NativeBlock(func(vm *VM, args []value.Value) value.Value {
			wa := vm.areas[area]
			if len(args) > 0 && args[0].Deref().K != value.NIL {
				vm.putField(wa, name, args[0])
				return args[0].Deref()
			}
			return vm.getField(wa, name)
		})}}
	})
	reg("MEMVARBLOCK", func(vm *VM, a []value.Value) value.Value {
		name := strings.ToUpper(strings.TrimSpace(argStr(a, 0, "")))
		if vm.findMemvar(name) == nil {
			return value.Nil
		}
		return value.Value{K: value.BLOCK, P: &value.Block{Fn: NativeBlock(func(vm *VM, args []value.Value) value.Value {
			if len(args) > 0 && args[0].Deref().K != value.NIL {
				vm.SetMemvar(name, args[0].Deref())
			}
			v, _ := vm.GetMemvar(name)
			return v
		})}}
	})
	regRaw("AFIELDS", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.Int(0)
		}
		n := len(wa.T.Fields)
		for k := 0; k < 4; k++ {
			arr := arg(a, k)
			if arr.K != value.ARRAY {
				continue
			}
			it := arr.Arr().Items
			for i := 0; i < n && i < len(it); i++ {
				fd := wa.T.Fields[i]
				switch k {
				case 0:
					it[i] = value.Str(fd.Name)
				case 1:
					it[i] = value.Str(string(fd.Type))
				case 2:
					it[i] = value.Int(fd.Len)
				case 3:
					it[i] = value.Int(fd.Dec)
				}
			}
		}
		return value.Int(n)
	})
	reg("DBSTRUCT", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.ArrayOf(0)
		}
		return structArray(wa.T.Fields)
	})
	reg("DBCREATE", func(vm *VM, a []value.Value) value.Value {
		file := argStr(a, 0, "")
		fields, err := structFromArray(arg(a, 1))
		if err != nil {
			return vm.Raise(errArg("DBCREATE: "+err.Error(), arg(a, 1)))
		}
		path := vm.DefaultPath(file, ".dbf")
		if err := db.CreateTable(path, fields, vm.CodePage()); err != nil {
			return vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 20, SubCode: 1004, Description: "Create error: " + err.Error(), Filename: path})
		}
		if has(a, 3) {
			vm.UseArea(argBool(a, 3, false), path, argStr(a, 4, ""), false)
		}
		return value.True
	})
	reg("DBEVAL", func(vm *VM, a []value.Value) value.Value {
		wa := vm.needWA("DBEVAL")
		if wa == nil {
			return value.Nil
		}
		b := arg(a, 0)
		vm.eachRecord(wa, scopeArgs(a, 1), func() bool {
			vm.EvalBlock(b)
			return true
		})
		return value.Nil
	})

	// indexes
	reg("DBCREATEINDEX", func(vm *VM, a []value.Value) value.Value {
		wa := vm.needWA("INDEX")
		if wa == nil {
			return value.Nil
		}
		keyText := argStr(a, 1, "")
		kb, ok := vm.blockOrText(arg(a, 2), keyText)
		if !ok {
			return vm.Raise(errArg("INDEX ON", arg(a, 1)))
		}
		path := vm.DefaultPath(argStr(a, 0, ""), ".ntx")
		ix := &db.Index{Path: path, KeyExpr: keyText, ForExpr: argStr(a, 4, ""),
			Unique: argBool(a, 3, vm.sets[10].Truthy()), Descend: argBool(a, 6, false)}
		n := wa.Num
		ix.KeyFn = func() (value.Value, error) { return vm.areaEval(n, kb), nil }
		if fb, ok := vm.blockOrText(arg(a, 5), ix.ForExpr); ok && strings.TrimSpace(ix.ForExpr) != "" {
			ix.ForFn = func() (bool, error) { return vm.areaEval(n, fb).Truthy(), nil }
		}
		vm.dbErr(wa.CreateIndex(ix), "INDEX")
		if vm.sets[100].Truthy() {
			vm.Println(fmt.Sprintf("%d records indexed", ix.Len()))
		}
		return value.Nil
	})
	setIndex := func(vm *VM, a []value.Value) value.Value {
		wa := vm.needWA("SET INDEX")
		if wa == nil {
			return value.Nil
		}
		f := argStr(a, 0, "")
		if strings.TrimSpace(f) == "" {
			return value.Nil
		}
		vm.openIndex(wa, f)
		return value.Nil
	}
	reg("DBSETINDEX", setIndex)
	reg("ORDLISTADD", setIndex)
	clearIndex := func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			vm.dbErr(wa.ClearIndexes(), "CLOSE INDEXES")
		}
		return value.Nil
	}
	reg("DBCLEARINDEX", clearIndex)
	reg("ORDLISTCLEAR", clearIndex)
	reg("DBSETORDER", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("SET ORDER"); wa != nil {
			vm.dbErr(wa.SetOrder(argInt(a, 0, 0)), "SET ORDER")
		}
		return value.Nil
	})
	reg("ORDSETFOCUS", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.Str("")
		}
		old := ""
		if ix := wa.CurIndex(); ix != nil {
			old = strings.ToUpper(aliasFromPath(ix.Path))
		}
		v := arg(a, 0)
		switch v.K {
		case value.NUMBER:
			vm.dbErr(wa.SetOrder(v.Int()), "ORDSETFOCUS")
		case value.STRING:
			for i, ix := range wa.Indexes {
				if strings.EqualFold(aliasFromPath(ix.Path), strings.TrimSpace(v.S)) {
					vm.dbErr(wa.SetOrder(i+1), "ORDSETFOCUS")
				}
			}
		}
		return value.Str(old)
	})
	reg("INDEXORD", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Int(wa.Order)
		}
		return value.Int(0)
	})
	reg("ORDNUMBER", builtinTable["INDEXORD"].fn)
	reg("INDEXKEY", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.Str("")
		}
		n := argInt(a, 0, 0)
		if n == 0 {
			n = wa.Order
		}
		if n >= 1 && n <= len(wa.Indexes) {
			return value.Str(wa.Indexes[n-1].KeyExpr)
		}
		return value.Str("")
	})
	reg("ORDKEY", builtinTable["INDEXKEY"].fn)
	reg("ORDNAME", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		if wa == nil {
			return value.Str("")
		}
		n := argInt(a, 0, wa.Order)
		if n >= 1 && n <= len(wa.Indexes) {
			return value.Str(aliasFromPath(wa.Indexes[n-1].Path))
		}
		return value.Str("")
	})
	reg("ORDCOUNT", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Int(len(wa.Indexes))
		}
		return value.Int(0)
	})
	reg("ORDKEYCOUNT", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil && wa.CurIndex() != nil {
			return value.Int(wa.CurIndex().Len())
		}
		return value.Int(0)
	})
	reg("DBREINDEX", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.needWA("REINDEX"); wa != nil {
			vm.dbErr(wa.Reindex(), "REINDEX")
		}
		return value.Nil
	})

	// filters and relations
	reg("DBSETFILTER", func(vm *VM, a []value.Value) value.Value {
		wa := vm.needWA("SET FILTER")
		if wa == nil {
			return value.Nil
		}
		text := argStr(a, 1, "")
		b, ok := vm.blockOrText(arg(a, 0), text)
		if !ok {
			return vm.Raise(errArg("SET FILTER", arg(a, 0)))
		}
		n := wa.Num
		wa.Filter = func() bool { return vm.areaEval(n, b).Truthy() }
		wa.FilterText = text
		return value.Nil
	})
	reg("DBCLEARFILTER", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			wa.Filter, wa.FilterText = nil, ""
		}
		return value.Nil
	})
	reg("DBFILTER", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			return value.Str(wa.FilterText)
		}
		return value.Str("")
	})
	reg("DBSETRELATION", func(vm *VM, a []value.Value) value.Value {
		wa := vm.needWA("SET RELATION")
		if wa == nil {
			return value.Nil
		}
		cn := vm.areaFromValue(arg(a, 0))
		if cn == 0 || vm.areas[cn] == nil {
			vm.aliasErr(arg(a, 0).String())
			return value.Nil
		}
		text := argStr(a, 2, "")
		b, ok := vm.blockOrText(arg(a, 1), text)
		if !ok {
			return vm.Raise(errArg("SET RELATION", arg(a, 1)))
		}
		n := wa.Num
		wa.Relations = append(wa.Relations, &db.Relation{Child: vm.areas[cn], KeyText: text,
			KeyFn: func() (value.Value, error) { return vm.areaEval(n, b), nil }})
		vm.dbErr(wa.Sync(), "SET RELATION")
		return value.Nil
	})
	reg("DBCLEARRELATION", func(vm *VM, a []value.Value) value.Value {
		if wa := vm.CurWA(); wa != nil {
			wa.Relations = nil
		}
		return value.Nil
	})
	reg("DBCLEARREL", builtinTable["DBCLEARRELATION"].fn)
	reg("DBRELATION", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 1)
		if wa != nil && n >= 1 && n <= len(wa.Relations) {
			return value.Str(wa.Relations[n-1].KeyText)
		}
		return value.Str("")
	})
	reg("DBRSELECT", func(vm *VM, a []value.Value) value.Value {
		wa := vm.CurWA()
		n := argInt(a, 0, 1)
		if wa != nil && n >= 1 && n <= len(wa.Relations) {
			return value.Int(wa.Relations[n-1].Child.Num)
		}
		return value.Int(0)
	})

}

func (vm *VM) compareLess(x, y value.Value) bool {
	if x.K != y.K {
		return x.K < y.K
	}
	switch x.K {
	case value.STRING:
		return x.S < y.S
	case value.NUMBER:
		return x.N < y.N
	case value.DATE:
		return x.J < y.J
	case value.LOGICAL:
		return !x.B && y.B
	}
	return false
}

// AtPrompt reports whether code is running straight from the dot prompt.
func (vm *VM) AtPrompt() bool {
	n := 0
	for _, f := range vm.frames {
		if !f.transparent {
			n++
		}
	}
	return n <= 1 && vm.Con.Interactive()
}

func cloneDefs(d []db.FieldDef) []db.FieldDef {
	out := make([]db.FieldDef, len(d))
	copy(out, d)
	return out
}

// openSource opens a table that is only read (APPEND FROM, CREATE FROM)
// without locking it, so it can be open elsewhere at the same time.
// OpenSource opens a table that is only read (APPEND FROM, CREATE FROM)
// without locking it.
func (vm *VM) OpenSource(path string) int {
	return vm.openAux(path, true, db.OpenNoLock)
}

// openTemp opens a table that a command writes (COPY TO, SORT...).
func (vm *VM) openTemp(path string) int {
	return vm.openAux(path, false, db.OpenExclusive)
}

func (vm *VM) openAux(path string, readOnly bool, mode db.OpenMode) int {
	n := 0
	for i := 249; i > 200; i-- {
		if vm.areas[i] == nil {
			n = i
			break
		}
	}
	if n == 0 {
		vm.Raise(errMsg("No free work area"))
	}
	t, err := db.OpenTableMode(path, readOnly, mode)
	if err != nil {
		vm.Raise(&RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: 1001, Description: "Open error: " + err.Error(), Filename: path})
	}
	t.SetCodePage(vm.CodePage())
	wa := db.NewWorkArea(n, fmt.Sprintf("__TMP%d", n), t, &db.Settings{})
	vm.areas[n] = wa
	wa.GoTop()
	return n
}

func (vm *VM) closeTemp(n int) {
	if wa := vm.areas[n]; wa != nil {
		wa.Close()
		vm.areas[n] = nil
	}
}

func structArray(fields []db.FieldDef) value.Value {
	items := make([]value.Value, len(fields))
	for i, fd := range fields {
		items[i] = value.NewArray([]value.Value{value.Str(fd.Name), value.Str(string(fd.Type)), value.Int(fd.Len), value.Int(fd.Dec)})
	}
	return value.NewArray(items)
}

func structFromArray(v value.Value) ([]db.FieldDef, error) {
	if v.K != value.ARRAY {
		return nil, fmt.Errorf("structure must be an array")
	}
	var defs []db.FieldDef
	for _, it := range v.Arr().Items {
		if it.K != value.ARRAY || len(it.Arr().Items) < 3 {
			return nil, fmt.Errorf("bad field definition")
		}
		f := it.Arr().Items
		t := strings.ToUpper(f[1].Deref().S + " ")[0]
		dec := 0
		if len(f) > 3 {
			dec = f[3].Deref().Int()
		}
		defs = append(defs, db.FieldDef{Name: f[0].Deref().S, Type: t, Len: f[2].Deref().Int(), Dec: dec})
	}
	return defs, nil
}

// lockResult turns the result of a lock attempt into RLock()'s answer:
// .F. when someone else holds it, a runtime error for anything else.
func (vm *VM) lockResult(wa *db.WorkArea, err error) value.Value {
	if errors.Is(err, db.ErrLocked) {
		return value.False
	}
	vm.dbErr(err, "LOCK")
	return value.True
}

// TakeNotes returns, once, messages for the dot prompt: a USE that failed
// because another session has the table (see TakeNetErrNote) and indexes
// that were rebuilt because they were out of date.
func (vm *VM) TakeNotes() []string {
	var out []string
	if n := vm.TakeNetErrNote(); n != "" {
		out = append(out, n)
	}
	out = append(out, vm.notes...)
	out = append(out, vm.TakeDebugNotes()...)
	vm.notes = nil
	return out
}

// TakeNetErrNote returns, once, a message about a USE that failed because
// another session has the table, if no program has checked NetErr() since.
// The dot prompt shows it so that a failed USE isn't silent.
func (vm *VM) TakeNetErrNote() string {
	n := vm.netErrNote
	vm.netErrNote = ""
	return n
}
