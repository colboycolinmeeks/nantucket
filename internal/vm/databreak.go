// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

// Data breakpoints: the debugger stops the program when a field of a
// table changes, in any record and however it is written (REPLACE, an
// assignment, APPEND FROM, a GET...). Unlike watchpoints they cost
// nothing per line: the table layer reports each field write, and only
// while a data breakpoint is set.

import (
	"fmt"
	"path/filepath"
	"strings"

	"nantucket/internal/db"
	"nantucket/internal/value"
)

type dataBreak struct {
	table string // the table's file, base name in upper case
	field string // field name in upper case, or "*" for any field
	name  string // ALIAS->FIELD, as set
	bp    *breakpoint
}

// DataBreak describes a data breakpoint for the debugger.
type DataBreak struct {
	Name  string // ALIAS->FIELD or ALIAS->*
	Table string // the table's file
	Cond  string
	Hit   HitCount
	Hits  int
}

func (b DataBreak) String() string {
	s := b.Name
	if r := (Breakpoint{Cond: b.Cond, Hit: b.Hit}).Rule(); r != "" {
		s += " " + r
	}
	if b.Hits > 0 {
		s += fmt.Sprintf(" [%d %s]", b.Hits, plural(b.Hits, "hit"))
	}
	return s
}

// resolveField reads "alias->field", "alias->*" or "field" (a field of
// the current work area), for a table that is open.
func (vm *VM) resolveField(spec string) (table, field, name string, err error) {
	spec = strings.ToUpper(strings.TrimSpace(spec))
	alias, field := "", spec
	if i := strings.Index(spec, "->"); i >= 0 {
		alias, field = strings.TrimSpace(spec[:i]), strings.TrimSpace(spec[i+2:])
	}
	var wa *db.WorkArea
	if alias == "" {
		wa = vm.CurWA()
		if wa == nil {
			return "", "", "", fmt.Errorf("no table in use; name one as alias->field")
		}
	} else if n := vm.areaByAlias(alias); n > 0 {
		wa = vm.areas[n]
	}
	if wa == nil {
		return "", "", "", fmt.Errorf("no table open with the alias %s", alias)
	}
	if field == "" {
		return "", "", "", fmt.Errorf("name a field, or * for every field")
	}
	if field != "*" {
		i := wa.T.FieldIndex(field)
		if i < 0 {
			return "", "", "", fmt.Errorf("%s has no field %s", wa.Alias, field)
		}
		field = wa.T.Fields[i].Name
	}
	return strings.ToUpper(filepath.Base(wa.T.Path)), field, wa.Alias + "->" + field, nil
}

// SetDataBreak stops the program when a field changes ("alias->field",
// "alias->*" for any field of the table, or a field of the current work
// area). cond and hit say when, as for SetBreak; the condition is
// evaluated in the program's current call, with the changed record
// current. A data breakpoint already on the field is replaced.
func (vm *VM) SetDataBreak(spec, cond string, hit HitCount) (string, error) {
	table, field, name, err := vm.resolveField(spec)
	if err != nil {
		return "", err
	}
	bp, err := vm.newBreak(cond, hit)
	if err != nil {
		return "", err
	}
	vm.ClearDataBreak(spec)
	vm.dbg.dataBreaks = append(vm.dbg.dataBreaks, &dataBreak{table: table, field: field, name: name, bp: bp})
	vm.DBSet.OnFieldWrite = vm.onFieldWrite
	return name, nil
}

// ClearDataBreak removes the data breakpoint on a field, reporting
// whether there was one.
func (vm *VM) ClearDataBreak(spec string) bool {
	table, field, _, err := vm.resolveField(spec)
	if err != nil {
		return false
	}
	for i, b := range vm.dbg.dataBreaks {
		if b.table == table && b.field == field {
			vm.dbg.dataBreaks = append(vm.dbg.dataBreaks[:i], vm.dbg.dataBreaks[i+1:]...)
			if len(vm.dbg.dataBreaks) == 0 {
				vm.DBSet.OnFieldWrite = nil
			}
			return true
		}
	}
	return false
}

// ClearDataBreaks removes every data breakpoint.
func (vm *VM) ClearDataBreaks() {
	vm.dbg.dataBreaks = nil
	vm.DBSet.OnFieldWrite = nil
}

// DataBreaks lists the data breakpoints, in the order they were set.
func (vm *VM) DataBreaks() []DataBreak {
	var out []DataBreak
	for _, b := range vm.dbg.dataBreaks {
		out = append(out, DataBreak{Name: b.name, Table: b.table, Cond: b.bp.cond, Hit: b.bp.hit, Hits: b.bp.hits})
	}
	return out
}

// onFieldWrite is called by the table layer after each field write while
// data breakpoints are set.
func (vm *VM) onFieldWrite(wa *db.WorkArea, field int, old, new value.Value) {
	if debugHook == nil || vm.dbg.active || len(vm.dbg.dataBreaks) == 0 {
		return
	}
	if sameValue(old, new) {
		return
	}
	table := strings.ToUpper(filepath.Base(wa.T.Path))
	name := wa.T.Fields[field].Name
	fr := vm.progLineFrame()
	if fr == nil {
		return // typed at the dot prompt: no program to stop
	}
	for _, b := range vm.dbg.dataBreaks {
		if b.table != table || (b.field != "*" && b.field != name) {
			continue
		}
		what := fmt.Sprintf("Data breakpoint: %s->%s, record %d, %s to %s",
			wa.Alias, name, wa.Recno, vm.debugShow(old.Deref()), vm.debugShow(new.Deref()))
		if why := vm.breakWhy(fr, what, b.bp); why != "" {
			vm.enterDebugger(why)
			return
		}
	}
}

func sameValue(a, b value.Value) bool {
	var sa, sb strings.Builder
	n, m := 0, 0
	fingerprint(&sa, a, &n, 0)
	fingerprint(&sb, b, &m, 0)
	return sa.String() == sb.String()
}

// progLineFrame returns the innermost call running a line of a program
// file, or nil. A code block that a command runs (REPLACE runs one to
// assign the field) has no line, so the call that ran the command is
// returned instead.
func (vm *VM) progLineFrame() *Frame {
	for i := len(vm.frames) - 1; i >= 0; i-- {
		if fr := vm.frames[i]; traceable(fr.fn) && fr.line > 0 {
			return fr
		}
	}
	return nil
}
