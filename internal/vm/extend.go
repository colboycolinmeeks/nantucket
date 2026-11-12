// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"nantucket/internal/compile"
	"nantucket/internal/db"
	"nantucket/internal/term"
	"nantucket/internal/value"
)

// Extending the runtime.
//
// The interpreter core lives in this package. Larger features (the full
// screen tools, the GET system, the database commands, report forms) live
// in their own packages, which import vm and add their built-in functions
// and per-session setup through this API from their init functions. The
// core never imports them, so the dependency only runs one way. A program
// links the features it wants with blank imports (see internal/features).

// Register adds a built-in function (callable from programs, and the
// target of commands defined in std.ch). Names are upper case.
func Register(name string, fn Builtin) { reg(name, fn) }

// RegisterRaw adds a built-in that receives by-reference arguments
// without dereferencing them.
func RegisterRaw(name string, fn Builtin) { regRaw(name, fn) }

// Ext returns this session's state for a feature package, creating it
// with make the first time. key is a value private to the package (an
// unexported type), so packages can't clash.
func (vm *VM) Ext(key any, make func() any) any {
	if vm.ext == nil {
		vm.ext = map[any]any{}
	}
	v, ok := vm.ext[key]
	if !ok {
		v = make()
		vm.ext[key] = v
	}
	return v
}

var newHooks []func(*VM)

// OnNew adds a function run for every new runtime, after it is set up.
func OnNew(f func(*VM)) { newHooks = append(newHooks, f) }

// ---------- arguments ----------

// Arg returns argument i (dereferenced), or NIL.
func Arg(a []value.Value, i int) value.Value { return arg(a, i) }

// ArgStr returns argument i if it is a string, else def.
func ArgStr(a []value.Value, i int, def string) string { return argStr(a, i, def) }

// ArgInt returns argument i if it is a number, else def.
func ArgInt(a []value.Value, i int, def int) int { return argInt(a, i, def) }

// ArgNum returns argument i if it is a number, else def.
func ArgNum(a []value.Value, i int, def float64) float64 { return argNum(a, i, def) }

// ArgBool returns argument i if it is a logical, else def.
func ArgBool(a []value.Value, i int, def bool) bool { return argBool(a, i, def) }

// Has reports whether argument i was passed (and is not NIL).
func Has(a []value.Value, i int) bool { return has(a, i) }

// ---------- text helpers ----------

// PadTo pads or cuts s to n characters: side 'R' pads on the right
// (text on the left), 'L' on the left, 'C' both.
func PadTo(s string, n int, side byte, fill string) string { return padTo(s, n, side, fill) }

// MemoLines word-wraps text to width, as memo fields are shown.
func MemoLines(s string, width int) []string { return memoLines(s, width) }

// ---------- output ----------

// WritePrinter sends text to the printer (file).
func (vm *VM) WritePrinter(s string) { vm.writePrinter(s) }

// ---------- work areas ----------

// Scope is a record scope: FOR, WHILE, NEXT, RECORD and REST.
type Scope = scope

// ScopeArgs reads a scope from five arguments starting at i: FOR block,
// WHILE block, NEXT count, RECORD number, REST flag.
func ScopeArgs(a []value.Value, i int) Scope { return scopeArgs(a, i) }

// EachRecord runs fn for each record in the scope; fn returns false to
// stop.
func (vm *VM) EachRecord(wa *db.WorkArea, s Scope, fn func() bool) { vm.eachRecord(wa, s, fn) }

// NeedWA returns the current work area, or raises "no table in use".
func (vm *VM) NeedWA(op string) *db.WorkArea { return vm.needWA(op) }

// Area returns work area n (1 to 255), or nil.
func (vm *VM) Area(n int) *db.WorkArea {
	if n < 1 || n >= len(vm.areas) {
		return nil
	}
	return vm.areas[n]
}

// AreaEval evaluates a block with work area n selected.
func (vm *VM) AreaEval(n int, b value.Value) value.Value { return vm.areaEval(n, b) }

// ---------- settings, colours and keys ----------

// Setting returns SET value n (see set.ch for the numbers).
func (vm *VM) Setting(n int) value.Value { return vm.sets[n] }

// SetSetting changes SET value n directly, without the checks and side
// effects of Set() (used for values the GET system keeps, such as the
// insert mode).
func (vm *VM) SetSetting(n int, v value.Value) { vm.sets[n] = v }

// Colors returns the current SET COLOR colours.
func (vm *VM) Colors() term.ColorSpec { return vm.colors }

// SetColorString applies a SET COLOR string ("" for the theme's colours).
func (vm *VM) SetColorString(s string) { vm.setColors(s) }

// LastKey is the last key read (LastKey()).
func (vm *VM) LastKey() int { return vm.lastKey }

// SetLastKey records the last key read.
func (vm *VM) SetLastKey(k int) { vm.lastKey = k }

// DispBox draws a box (as @ ... BOX / DispBox()).
func (vm *VM) DispBox(t, l, b, r int, spec value.Value, at term.Attr) {
	vm.dispBox(t, l, b, r, spec, at)
}

// CompareLess orders two values as ASort() does.
func (vm *VM) CompareLess(x, y value.Value) bool { return vm.compareLess(x, y) }

// ---------- errors ----------

// ErrArg is an argument error (EG_ARG) with a Clipper sub-code.
func ErrArg(code int, op string, args ...value.Value) *RTError { return errArgCode(code, op, args...) }

// ErrMsg is a general runtime error with a description.
func ErrMsg(desc string) *RTError { return errMsg(desc) }

// DBErr raises a database error (with Clipper codes for locking errors),
// doing nothing if err is nil.
func (vm *VM) DBErr(err error, op string) { vm.dbErr(err, op) }

// ---------- work areas (continued) ----------

// CurArea is the current work area number.
func (vm *VM) CurArea() int { return vm.curArea }

// SelectArea makes work area n current.
func (vm *VM) SelectArea(n int) { vm.curArea = n }

// SetArea puts a work area in slot n (nil empties it).
func (vm *VM) SetArea(n int, wa *db.WorkArea) { vm.areas[n] = wa }

// CloseArea closes the table in work area n.
func (vm *VM) CloseArea(n int) { vm.closeArea(n) }

// OpenIndex opens an index file for a work area (SET INDEX).
func (vm *VM) OpenIndex(wa *db.WorkArea, file string) { vm.openIndex(wa, file) }

// OpenTemp opens a table in a spare work area for a command that writes
// it, returning the area number; CloseTemp closes it.
func (vm *VM) OpenTemp(path string) int { return vm.openTemp(path) }

// CloseTemp closes a table opened with OpenTemp.
func (vm *VM) CloseTemp(n int) { vm.closeTemp(n) }

// GetField reads a field by name, raising an error if it doesn't exist.
func (vm *VM) GetField(wa *db.WorkArea, name string) value.Value { return vm.getField(wa, name) }

// PutField writes a field by name, raising an error on failure.
func (vm *VM) PutField(wa *db.WorkArea, name string, v value.Value) { vm.putField(wa, name, v) }

// FieldDisplay shows field i of the current record as LIST does.
func (vm *VM) FieldDisplay(wa *db.WorkArea, i int, forList bool) string {
	return vm.fieldDisplay(wa, i, forList)
}

// FieldWidth is how wide a field is shown in BROWSE and LIST.
func (vm *VM) FieldWidth(fd db.FieldDef) int { return fieldWidth(vm, fd) }

// CloneDefs copies a list of field definitions.
func CloneDefs(d []db.FieldDef) []db.FieldDef { return cloneDefs(d) }

// WhenIdle runs f only if the runtime is waiting for input (so it isn't
// drawing or running code), and reports whether it did. Tests use it to
// read a simulated screen safely while a program runs.
func (vm *VM) WhenIdle(f func()) bool {
	if !vm.busy.TryLock() {
		return false
	}
	defer vm.busy.Unlock()
	f()
	return true
}

// ---------- values ----------

// Arith applies an arithmetic operator (compile.ADD, SUB, MUL, DIV...) as
// the language does, raising an error for unsuitable operands.
func (vm *VM) Arith(op compile.Op, a, b value.Value) value.Value { return vm.arith(op, a, b) }

// EqualExact compares two values with ==.
func (vm *VM) EqualExact(a, b value.Value) bool { return vm.equalExact(a, b) }

// MemoWidth is SET MEMOWIDTH (at least 4).
func (vm *VM) MemoWidth() int { return vm.memoWidth() }

// ErrNoVar is "variable does not exist".
func ErrNoVar(name string) *RTError { return errNoVar(name) }

// ErrDB is a database error with a Clipper sub-code.
func ErrDB(code int, desc, op string) *RTError { return errDB(code, desc, op) }

// Areas returns the open work areas, in work area order.
func (vm *VM) Areas() []*db.WorkArea {
	var out []*db.WorkArea
	for _, wa := range vm.areas {
		if wa != nil {
			out = append(out, wa)
		}
	}
	return out
}
