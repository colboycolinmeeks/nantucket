// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

// Watchpoints: the debugger stops the program when the value of an
// expression (usually a variable) changes. Each watched expression is
// evaluated before every program line while any watchpoint is set, so a
// program with watchpoints runs much more slowly.

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"nantucket/internal/lex"
	"nantucket/internal/value"
)

type watchpoint struct {
	expr string
	// frame is the call whose LOCALs the expression uses (local), or a
	// stand-in for its procedure that only gives access to the STATICs
	// of its file.
	frame *Frame
	local bool
	proc  string
	fp    string // fingerprint of the last value seen
	shown string // the last value, for messages
}

// Watchpoint describes a watchpoint for the debugger.
type Watchpoint struct {
	Expr  string
	Proc  string // the procedure whose LOCALs it uses, or ""
	Value string // its value when last checked
}

func (w Watchpoint) String() string {
	if w.Proc != "" {
		return w.Expr + " in " + w.Proc + "()"
	}
	return w.Expr
}

// SetWatchpoint stops the program whenever the value of expr changes.
// The expression is evaluated in the call f: if it uses that call's LOCAL
// variables, the watchpoint lasts until the call returns. Setting one that
// is already there does nothing.
func (vm *VM) SetWatchpoint(f DebugFrame, expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("WP needs an expression to watch")
	}
	if _, err := vm.compileMacro(expr, false); err != nil {
		return fmt.Errorf("not a valid expression: %s", expr)
	}
	for _, w := range vm.dbg.watchpoints {
		if strings.EqualFold(w.expr, expr) {
			return nil
		}
	}
	w := &watchpoint{expr: expr}
	if f.fr != nil {
		w.frame = &Frame{fn: f.fr.fn}
		if usesLocals(vm, f, expr) {
			w.frame, w.local, w.proc = f.fr, true, f.Proc
		}
	}
	w.fp, w.shown = vm.watchValue(w)
	vm.dbg.watchpoints = append(vm.dbg.watchpoints, w)
	vm.updateTracing()
	return nil
}

// usesLocals reports whether an expression names one of the call's LOCAL
// variables.
func usesLocals(vm *VM, f DebugFrame, expr string) bool {
	toks, err := lex.LexLine(expr)
	if err != nil {
		return false
	}
	locals := map[string]bool{}
	for _, v := range vm.DebugLocals(f) {
		if v.Scope == "LOCAL" {
			locals[v.Name] = true
		}
	}
	for _, t := range toks {
		if t.Kind == lex.Ident && locals[t.Text] {
			return true
		}
	}
	return false
}

// ClearWatchpoint removes the watchpoint on expr, reporting whether there
// was one.
func (vm *VM) ClearWatchpoint(expr string) bool {
	for i, w := range vm.dbg.watchpoints {
		if strings.EqualFold(w.expr, strings.TrimSpace(expr)) {
			vm.dbg.watchpoints = append(vm.dbg.watchpoints[:i], vm.dbg.watchpoints[i+1:]...)
			vm.updateTracing()
			return true
		}
	}
	return false
}

// ClearWatchpoints removes every watchpoint.
func (vm *VM) ClearWatchpoints() {
	vm.dbg.watchpoints = nil
	vm.updateTracing()
}

// Watchpoints lists the watchpoints, in the order they were set.
func (vm *VM) Watchpoints() []Watchpoint {
	var out []Watchpoint
	for _, w := range vm.dbg.watchpoints {
		p := ""
		if w.local {
			p = w.proc
		}
		out = append(out, Watchpoint{Expr: w.expr, Proc: p, Value: w.shown})
	}
	return out
}

// TakeDebugNotes returns messages for the debugger (watchpoints removed
// because their procedure returned) and forgets them.
func (vm *VM) TakeDebugNotes() []string {
	n := vm.dbg.notes
	vm.dbg.notes = nil
	return n
}

// watchValue evaluates a watchpoint, returning a fingerprint of its value
// (to compare) and the value as shown in messages.
func (vm *VM) watchValue(w *watchpoint) (string, string) {
	// nothing the expression runs may open the debugger
	active := vm.dbg.active
	vm.dbg.active = true
	v, err := vm.DebugEval(DebugFrame{fr: w.frame}, w.expr)
	vm.dbg.active = active
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return "!" + err.Error(), "(undefined)"
		}
		return "!" + err.Error(), "(" + err.Error() + ")"
	}
	var sb strings.Builder
	n := 0
	fingerprint(&sb, v, &n, 0)
	return sb.String(), vm.debugShow(v)
}

// rememberWatchpoints takes the watched values as they are now, so that
// changes made in the debugger itself don't stop the program.
func (vm *VM) rememberWatchpoints() {
	for _, w := range vm.dbg.watchpoints {
		w.fp, w.shown = vm.watchValue(w)
	}
}

// checkWatchpoints says why the program should stop because a watched
// value changed, or "". Watchpoints on LOCALs of calls that have returned
// are removed.
func (vm *VM) checkWatchpoints(fr *Frame) string {
	d := &vm.dbg
	why := ""
	kept := d.watchpoints[:0]
	for _, w := range d.watchpoints {
		if w.local && !vm.frameAlive(w.frame) {
			d.notes = append(d.notes, fmt.Sprintf("Watchpoint on %s removed: %s() returned", w.expr, w.proc))
			continue
		}
		kept = append(kept, w)
		fp, shown := vm.watchValue(w)
		if fp == w.fp {
			continue
		}
		if why == "" {
			why = fmt.Sprintf("Watchpoint: %s changed from %s to %s", w.expr, w.shown, shown)
			if file, line := vm.changedAt(fr); file != "" {
				why += fmt.Sprintf(" at %s(%d)", filepath.Base(file), line)
			}
		}
		w.fp, w.shown = fp, shown
	}
	for i := len(kept); i < len(d.watchpoints); i++ {
		d.watchpoints[i] = nil
	}
	if len(kept) != len(d.watchpoints) {
		d.watchpoints = kept
		vm.updateTracing()
	}
	return why
}

// changedAt says which line changed a watched value, found before line
// fr runs: the line that ran last, unless a call has since returned to
// fr, in which case it is fr's line that made the call (the change was
// made by it, or by what it called).
func (vm *VM) changedAt(fr *Frame) (string, int) {
	d := &vm.dbg
	if d.prevFrame != nil && d.prevFrame != fr && fr.prevLine > 0 {
		return fr.fn.File, fr.prevLine
	}
	return d.prevFile, d.prevLine
}

func (vm *VM) frameAlive(fr *Frame) bool {
	for i := len(vm.frames) - 1; i >= 0; i-- {
		if vm.frames[i] == fr {
			return true
		}
	}
	return false
}

// fingerprint writes a value's contents, arrays and hashes included, so
// that a change to an element counts as a change.
func fingerprint(sb *strings.Builder, v value.Value, n *int, depth int) {
	v = v.Deref()
	*n++
	if *n > 10000 || depth > 50 {
		sb.WriteString("~")
		return
	}
	switch v.K {
	case value.NIL:
		sb.WriteString("U")
	case value.LOGICAL:
		sb.WriteString(map[bool]string{true: "T", false: "F"}[v.B])
	case value.NUMBER:
		sb.WriteString("N" + strconv.FormatFloat(v.N, 'g', -1, 64))
	case value.STRING:
		sb.WriteString("C" + strconv.Quote(v.S))
	case value.DATE:
		sb.WriteString("D" + strconv.Itoa(int(v.J)))
	case value.ARRAY:
		sb.WriteString(fmt.Sprintf("A%p{", v.P))
		if a := v.Arr(); a != nil {
			for _, it := range a.Items {
				fingerprint(sb, it, n, depth+1)
				sb.WriteByte(',')
			}
		}
		sb.WriteByte('}')
	case value.HASH:
		sb.WriteString(fmt.Sprintf("H%p{", v.P))
		if h := v.Hsh(); h != nil {
			for i := range h.Keys {
				fingerprint(sb, h.Keys[i], n, depth+1)
				sb.WriteString("=>")
				fingerprint(sb, h.Vals[i], n, depth+1)
				sb.WriteByte(',')
			}
		}
		sb.WriteByte('}')
	default:
		sb.WriteString(fmt.Sprintf("%d:%p", v.K, v.P))
	}
}

// debugShow writes a value briefly, for debugger messages.
func (vm *VM) debugShow(v value.Value) string {
	switch v.K {
	case value.NIL:
		return "NIL"
	case value.STRING:
		s := v.S
		if r := []rune(s); len(r) > 30 {
			s = string(r[:30]) + "..."
		}
		return `"` + s + `"`
	case value.LOGICAL:
		if v.B {
			return ".T."
		}
		return ".F."
	case value.ARRAY:
		if a := v.Arr(); a != nil {
			// short arrays in full, so a changed element shows
			var parts []string
			for _, it := range a.Items {
				it = it.Deref()
				if it.K == value.ARRAY || it.K == value.HASH {
					parts = append(parts, "{...}")
				} else {
					parts = append(parts, vm.debugShow(it))
				}
			}
			if s := "{" + strings.Join(parts, ", ") + "}"; len(s) <= 40 {
				return s
			}
			return fmt.Sprintf("an array (%d)", len(a.Items))
		}
	case value.HASH:
		if h := v.Hsh(); h != nil {
			return fmt.Sprintf("a hash (%d)", len(h.Keys))
		}
	}
	return strings.TrimSpace(vm.Display(v))
}
