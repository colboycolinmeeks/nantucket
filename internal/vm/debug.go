// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

// Support for the debugger (internal/debugger): breakpoints, stepping,
// opening it with Alt-D or AltD(), and looking at a paused program's call
// stack and variables.
//
// The debugger is only consulted while tracing is on (see trace.go), so a
// program runs at full speed until a breakpoint is set or the debugger is
// opened; checking for Alt-D costs one atomic load per line.

import (
	"fmt"
	"sort"
	"strings"

	"nantucket/internal/value"
)

// debugHook shows the debugger. It is set by internal/debugger, and runs
// on the program's goroutine with the program paused before a line. It
// returns when the user lets the program go on, having said how with
// DebugStep or DebugRunTo (or not, to run freely).
var debugHook func(m *VM, why string)

// SetDebugHook installs the debugger.
func SetDebugHook(f func(m *VM, why string)) { debugHook = f }

// StepKind says where a paused program stops next.
type StepKind int

const (
	StepNone  StepKind = iota // run until a breakpoint
	StepInto                  // the next program line, in any procedure
	StepOver                  // the next line in this procedure or its callers
	StepOut                   // the next line after this procedure returns
	stepRunTo                 // a given line
)

type debugState struct {
	// breakpoints (breakpoint.go)
	breaks     map[string]*breakpoint // "FILE.PRG:12": base name in upper case, line
	procBreaks map[string]*breakpoint // procedure names, upper case
	step       StepKind
	depth      int          // call depth when the step started
	runTo      string       // break key for stepRunTo
	active     bool         // the debugger is on screen, or a condition is running
	procHit    string       // a procedure with a breakpoint was called
	procBreak  *breakpoint  // and its breakpoint
	log        debugLog     // logpoints (logpoint.go)
	dataBreaks []*dataBreak // data breakpoints (databreak.go)
	// watchpoints (watchpoint.go)
	watchpoints []*watchpoint
	notes       []string // for the debugger: watchpoints removed
	prevFile    string   // the program line that ran last, and its call,
	prevLine    int      // for saying where a watched value changed
	prevFrame   *Frame
}

func (vm *VM) debugWanted() bool {
	d := &vm.dbg
	return d.step != StepNone || len(d.breaks) > 0 || len(d.procBreaks) > 0 || len(d.watchpoints) > 0
}

// RequestDebug asks the running program to open the debugger (Alt-D).
// It is safe to call from another goroutine. It reports false when there
// is no debugger or AltD(0) turned Alt-D off, so the key should go to the
// program instead.
func (vm *VM) RequestDebug() bool {
	if debugHook == nil || vm.debugOff.Load() {
		return false
	}
	vm.debugReq.Store(true)
	return true
}

// DebugAtStart stops the next program to run at its first line
// (nantucket -debug).
func (vm *VM) DebugAtStart() {
	vm.dbg.step = StepInto
	vm.updateTracing()
}

// DebugStep says where to stop next when the debugger returns.
func (vm *VM) DebugStep(k StepKind) {
	vm.dbg.step = k
	vm.dbg.depth = vm.depth
}

// DebugRunTo runs until the program reaches a line of a file.
func (vm *VM) DebugRunTo(file string, line int) {
	vm.dbg.step = stepRunTo
	vm.dbg.runTo = breakKey(file, line)
}

// ResetDebugStep stops any step that is under way: the dot prompt calls
// it so that a step left over from a finished program doesn't stop the
// next one. Breakpoints stay, with their hit counts back at zero.
func (vm *VM) ResetDebugStep() {
	vm.debugReq.Store(false)
	vm.resetHits()
	if vm.dbg.step != StepNone {
		vm.dbg.step = StepNone
		vm.updateTracing()
	}
}

// debugLine is called before each program line while tracing, and opens
// the debugger when the program should stop there.
func (vm *VM) debugLine(fr *Frame) {
	if debugHook == nil || vm.dbg.active {
		return
	}
	d := &vm.dbg
	why := ""
	if len(d.watchpoints) > 0 {
		why = vm.checkWatchpoints(fr)
	}
	d.prevFile, d.prevLine, d.prevFrame = fr.fn.File, fr.line, fr
	// a breakpoint on this line, or on the procedure that just started;
	// both are checked even when the program stops for another reason, so
	// that their hits are counted
	if len(d.breaks) > 0 {
		if b, ok := d.breaks[breakKey(fr.fn.File, fr.line)]; ok {
			if w := vm.breakWhy(fr, "Breakpoint", b); why == "" {
				why = w
			}
		}
	}
	if d.procHit != "" {
		name, b := d.procHit, d.procBreak
		d.procHit, d.procBreak = "", nil
		if w := vm.breakWhy(fr, "Breakpoint at "+name+"()", b); why == "" {
			why = w
		}
	}
	if why == "" {
		switch d.step {
		case StepInto:
			why = "Step"
		case StepOver:
			if vm.depth <= d.depth {
				why = "Step"
			}
		case StepOut:
			if vm.depth < d.depth {
				why = "Step"
			}
		case stepRunTo:
			if breakKey(fr.fn.File, fr.line) == d.runTo {
				why = "Run to line"
			}
		}
	}
	if why != "" {
		vm.enterDebugger(why)
	}
}

// checkDebugRequest handles Alt-D at a program line.
func (vm *VM) checkDebugRequest(fr *Frame) {
	vm.debugReq.Store(false)
	if traceable(fr.fn) {
		vm.enterDebugger("Alt-D")
	}
}

// enterDebugger shows the debugger, then carries on as it says.
func (vm *VM) enterDebugger(why string) {
	if debugHook == nil || vm.dbg.active || !vm.Con.Interactive() || vm.ProgFrame() < 0 {
		return
	}
	vm.dbg.active = true
	vm.dbg.step = StepNone
	vm.dbg.procHit, vm.dbg.procBreak = "", nil
	defer func() {
		vm.rememberWatchpoints()
		vm.dbg.active = false
		vm.updateTracing()
	}()
	debugHook(vm, why)
}

// handleDebugKey is called when Inkey() reads the key that Alt-D sends to
// wake a program waiting for input.
func (vm *VM) handleDebugKey() {
	if vm.debugReq.Swap(false) && !vm.dbg.active {
		vm.enterDebugger("Alt-D")
	}
}

// CancelProgram stops the paused program, as Alt-C does.
func (vm *VM) CancelProgram() { panic(&CancelSignal{Interrupted: true}) }

// ---------- looking at a paused program ----------

// DebugFrame is one call on the stack.
type DebugFrame struct {
	Proc  string // procedure or function name; "(b)NAME" for a block
	File  string // source file ("" or "(...)" when there is none)
	Line  int
	Block bool
	fr    *Frame
}

// Program reports whether the frame runs code from a program file.
func (f DebugFrame) Program() bool { return f.fr != nil && traceable(f.fr.fn) }

// DebugFrames returns the call stack, innermost first.
func (vm *VM) DebugFrames() []DebugFrame {
	var out []DebugFrame
	for i := len(vm.frames) - 1; i >= 0; i-- {
		fr := vm.frames[i]
		out = append(out, DebugFrame{Proc: vm.procName(fr), File: fr.fn.File, Line: fr.line, Block: fr.fn.IsBlock, fr: fr})
	}
	return out
}

// ProgFrame returns the index in DebugFrames of the innermost frame
// running a program file, or -1.
func (vm *VM) ProgFrame() int {
	for i := len(vm.frames) - 1; i >= 0; i-- {
		if traceable(vm.frames[i].fn) {
			return len(vm.frames) - 1 - i
		}
	}
	return -1
}

// DebugVar is a variable the debugger shows.
type DebugVar struct {
	Name  string
	Scope string // LOCAL, STATIC, PRIVATE or PUBLIC
	Ref   *value.Ref
}

// DebugLocals returns a frame's local variables (parameters first, then
// those of the blocks' enclosing procedure that a block uses) and the
// static variables of its source file.
func (vm *VM) DebugLocals(f DebugFrame) []DebugVar {
	if f.fr == nil {
		return nil
	}
	fr, fn := f.fr, f.fr.fn
	var out []DebugVar
	for i, ref := range fr.locals {
		if i < len(fn.LocalNames) && !strings.HasPrefix(fn.LocalNames[i], "\x00") && ref != nil {
			out = append(out, DebugVar{Name: strings.ToUpper(fn.LocalNames[i]), Scope: "LOCAL", Ref: ref})
		}
	}
	for i, c := range fn.Captures {
		if i < len(fr.capt) && fr.capt[i] != nil && !strings.HasPrefix(c.Name, "\x00") {
			out = append(out, DebugVar{Name: strings.ToUpper(c.Name), Scope: "LOCAL", Ref: fr.capt[i]})
		}
	}
	if m := fn.Module; m != nil && m.Loaded {
		for i, name := range m.StaticNames {
			if j := m.StaticBase + i; j < len(vm.statics) && !strings.HasPrefix(name, "\x00") {
				out = append(out, DebugVar{Name: strings.ToUpper(name), Scope: "STATIC", Ref: vm.statics[j]})
			}
		}
	}
	return out
}

// DebugMemvars returns the PRIVATE and PUBLIC variables visible now,
// sorted by name.
func (vm *VM) DebugMemvars() []DebugVar {
	var out []DebugVar
	for name, st := range vm.memvars {
		if len(st) == 0 || strings.HasPrefix(name, "\x00") {
			continue
		}
		mv := st[len(st)-1]
		scope := "PRIVATE"
		if mv.owner == nil {
			scope = "PUBLIC"
		}
		out = append(out, DebugVar{Name: name, Scope: scope, Ref: mv.ref})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// DebugEval evaluates an expression as if it were written in the frame:
// the frame's locals and statics can be read and assigned (x := 5). Errors
// are returned rather than going to the program's error handler.
func (vm *VM) DebugEval(f DebugFrame, src string) (v value.Value, err error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return value.Nil, nil
	}
	fn, err := vm.compileMacro(src, false)
	if err != nil {
		return value.Nil, err
	}
	// The frame's locals become memory variables sharing their storage
	// while the expression runs, so the macro sees (and changes) them.
	type pushed struct {
		name string
		mv   *memvar
	}
	var added []pushed
	seen := map[string]bool{}
	for _, lv := range vm.DebugLocals(f) {
		if seen[lv.Name] {
			continue
		}
		seen[lv.Name] = true
		mv := &memvar{ref: lv.Ref, owner: f.fr}
		vm.memvars[lv.Name] = append(vm.memvars[lv.Name], mv)
		added = append(added, pushed{lv.Name, mv})
	}
	saveEB, saveAlias, saveDepth := vm.errorBlock, vm.aliasStack, vm.depth
	vm.errorBlock = value.Value{K: value.BLOCK, P: &value.Block{Fn: NativeBlock(func(vm *VM, a []value.Value) value.Value {
		if len(a) > 0 {
			if e, ok := a[0].Deref().P.(*RTError); ok {
				panic(e)
			}
		}
		panic(errMsg("error"))
	})}}
	defer func() {
		vm.errorBlock, vm.aliasStack, vm.depth = saveEB, saveAlias, saveDepth
		for i := len(added) - 1; i >= 0; i-- {
			st := vm.memvars[added[i].name]
			for j := len(st) - 1; j >= 0; j-- {
				if st[j] == added[i].mv {
					st = append(st[:j], st[j+1:]...)
					break
				}
			}
			if len(st) == 0 {
				delete(vm.memvars, added[i].name)
			} else {
				vm.memvars[added[i].name] = st
			}
		}
	}()
	err = vm.Protect(func() { v = vm.Call(fn, nil, nil, true).Deref() })
	if e, ok := err.(*RTError); ok {
		return value.Nil, fmt.Errorf("%s", e.Description+describeOperand(e))
	}
	return v, err
}

func describeOperand(e *RTError) string {
	if e.Operation != "" {
		return ": " + e.Operation
	}
	return ""
}

// SourceLines returns the lines of a program file, as the debugger shows
// them.
func (vm *VM) SourceLines(file string) []string {
	vm.sourceLine(file, 1) // loads the cache
	return vm.srcCache[file]
}

func init() {
	// AltD(): open the debugger. AltD(0) turns Alt-D off, AltD(1) on.
	reg("ALTD", func(vm *VM, a []value.Value) value.Value {
		if v := arg(a, 0); v.K == value.NUMBER {
			vm.debugOff.Store(v.N == 0)
			return value.Nil
		}
		vm.enterDebugger("AltD()")
		return value.Nil
	})
}
