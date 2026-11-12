// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

// Breakpoints for the debugger: on a line of a program file or on a
// procedure, each with an optional condition and an optional hit count.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"nantucket/internal/compile"
	"nantucket/internal/value"
)

// HitCount says on which hits a breakpoint stops. A hit is each time the
// program reaches the breakpoint with its condition true (or with no
// condition).
type HitCount struct {
	Op byte // 0: every hit; '=': the Nth only; '>': the Nth and after; '%': every Nth
	N  int
}

// ParseHitCount reads a hit count: "5" (the 5th hit), ">= 5" (the 5th and
// every one after) or "% 5" (every 5th).
func ParseHitCount(s string) (HitCount, error) {
	s = strings.TrimSpace(s)
	h := HitCount{Op: '='}
	switch {
	case strings.HasPrefix(s, ">="):
		h.Op, s = '>', s[2:]
	case strings.HasPrefix(s, "%"):
		h.Op, s = '%', s[1:]
	case strings.HasPrefix(s, "="):
		s = s[1:]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		return HitCount{}, fmt.Errorf("a hit count is a number from 1 up, >= n or %% n")
	}
	h.N = n
	return h, nil
}

func (h HitCount) String() string {
	switch h.Op {
	case '=':
		return fmt.Sprintf("HIT %d", h.N)
	case '>':
		return fmt.Sprintf("HIT >= %d", h.N)
	case '%':
		return fmt.Sprintf("HIT %% %d", h.N)
	}
	return ""
}

// stops reports whether the hit-th hit stops the program.
func (h HitCount) stops(hit int) bool {
	switch h.Op {
	case '=':
		return hit == h.N
	case '>':
		return hit >= h.N
	case '%':
		return hit%h.N == 0
	}
	return true
}

type breakpoint struct {
	cond string // stop only when this is true ("" for always)
	hit  HitCount
	hits int    // hits so far in this run
	log  string // a logpoint: write this message instead of stopping
}

// Breakpoint describes a breakpoint for the debugger.
type Breakpoint struct {
	File string // base name in upper case, "" for a procedure
	Line int
	Proc string
	Cond string   // stop only when this is true ("" for always)
	Hit  HitCount // stop only on these hits
	Hits int      // hits so far in this run
	Log  string   // a logpoint's message ("" for a breakpoint)
}

// Rule writes the breakpoint's message, condition and hit count:
// `LOG "n={n}" IF n > 3 HIT 2`. It is "" for an ordinary breakpoint.
func (b Breakpoint) Rule() string {
	var parts []string
	if b.Log != "" {
		parts = append(parts, "LOG "+strconv.Quote(b.Log))
	}
	if b.Cond != "" {
		parts = append(parts, "IF "+b.Cond)
	}
	if b.Hit.Op != 0 {
		parts = append(parts, b.Hit.String())
	}
	return strings.Join(parts, " ")
}

func (b Breakpoint) String() string {
	s := fmt.Sprintf("%s(%d)", b.File, b.Line)
	if b.Proc != "" {
		s = b.Proc + "()"
	}
	if r := b.Rule(); r != "" {
		s += " " + r
	}
	if b.Hits > 0 {
		s += fmt.Sprintf(" [%d %s]", b.Hits, plural(b.Hits, "hit"))
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func breakKey(file string, line int) string {
	return fmt.Sprintf("%s:%d", strings.ToUpper(filepath.Base(file)), line)
}

func procKey(name string) string {
	return strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "()")))
}

// ToggleBreak sets a breakpoint on a line, or clears the one there,
// reporting whether it is now set.
func (vm *VM) ToggleBreak(file string, line int) bool {
	k := breakKey(file, line)
	if _, ok := vm.dbg.breaks[k]; ok {
		delete(vm.dbg.breaks, k)
		vm.updateTracing()
		return false
	}
	vm.SetBreak(file, line, "", HitCount{})
	return true
}

// SetLogpoint sets a logpoint on a line: instead of stopping, the program
// adds msg to the debugger's log, with each {expression} in it replaced
// by its value. cond and hit say when, as for SetBreak. It replaces any
// breakpoint already on the line.
func (vm *VM) SetLogpoint(file string, line int, msg, cond string, hit HitCount) error {
	b, err := vm.newLogpoint(msg, cond, hit)
	if err != nil {
		return err
	}
	if vm.dbg.breaks == nil {
		vm.dbg.breaks = map[string]*breakpoint{}
	}
	vm.dbg.breaks[breakKey(file, line)] = b
	vm.updateTracing()
	return nil
}

// SetProcLogpoint sets a logpoint on a procedure or function: the message
// is logged at its first line, where its parameters are set.
func (vm *VM) SetProcLogpoint(name, msg, cond string, hit HitCount) error {
	b, err := vm.newLogpoint(msg, cond, hit)
	if err != nil {
		return err
	}
	if vm.dbg.procBreaks == nil {
		vm.dbg.procBreaks = map[string]*breakpoint{}
	}
	vm.dbg.procBreaks[procKey(name)] = b
	vm.updateTracing()
	return nil
}

func (vm *VM) newLogpoint(msg, cond string, hit HitCount) (*breakpoint, error) {
	if strings.TrimSpace(msg) == "" {
		return nil, fmt.Errorf("a logpoint needs a message")
	}
	if err := vm.checkLogMessage(msg); err != nil {
		return nil, err
	}
	b, err := vm.newBreak(cond, hit)
	if err != nil {
		return nil, err
	}
	b.log = msg
	return b, nil
}

// SetBreak sets a breakpoint on a line that stops only when cond is true
// ("" for always) and on the hits that hit says (every hit when it is
// zero), replacing any breakpoint already there. It fails if cond isn't
// an expression.
func (vm *VM) SetBreak(file string, line int, cond string, hit HitCount) error {
	b, err := vm.newBreak(cond, hit)
	if err != nil {
		return err
	}
	if vm.dbg.breaks == nil {
		vm.dbg.breaks = map[string]*breakpoint{}
	}
	vm.dbg.breaks[breakKey(file, line)] = b
	vm.updateTracing()
	return nil
}

// ClearBreak removes the breakpoint or logpoint on a line, reporting
// whether there was one.
func (vm *VM) ClearBreak(file string, line int) bool {
	k := breakKey(file, line)
	_, ok := vm.dbg.breaks[k]
	delete(vm.dbg.breaks, k)
	vm.updateTracing()
	return ok
}

// ClearProcBreak removes the breakpoint or logpoint on a procedure,
// reporting whether there was one.
func (vm *VM) ClearProcBreak(name string) bool {
	k := procKey(name)
	_, ok := vm.dbg.procBreaks[k]
	delete(vm.dbg.procBreaks, k)
	vm.updateTracing()
	return ok
}

// ToggleProcBreak sets a breakpoint on a procedure or function, or clears
// the one there, reporting whether it is now set.
func (vm *VM) ToggleProcBreak(name string) bool {
	name = procKey(name)
	if _, ok := vm.dbg.procBreaks[name]; ok {
		delete(vm.dbg.procBreaks, name)
		vm.updateTracing()
		return false
	}
	vm.SetProcBreak(name, "", HitCount{})
	return true
}

// SetProcBreak sets a breakpoint on a procedure or function, as SetBreak
// does for a line. The condition can use the procedure's parameters.
func (vm *VM) SetProcBreak(name, cond string, hit HitCount) error {
	b, err := vm.newBreak(cond, hit)
	if err != nil {
		return err
	}
	if vm.dbg.procBreaks == nil {
		vm.dbg.procBreaks = map[string]*breakpoint{}
	}
	vm.dbg.procBreaks[procKey(name)] = b
	vm.updateTracing()
	return nil
}

func (vm *VM) newBreak(cond string, hit HitCount) (*breakpoint, error) {
	cond = strings.TrimSpace(cond)
	if cond != "" {
		if _, err := vm.compileMacro(cond, false); err != nil {
			return nil, fmt.Errorf("not a valid condition: %s", cond)
		}
	}
	if hit.Op != 0 && hit.N < 1 {
		return nil, fmt.Errorf("a hit count is a number from 1 up")
	}
	return &breakpoint{cond: cond, hit: hit}, nil
}

// BreakAt returns the breakpoint on a line, if there is one.
func (vm *VM) BreakAt(file string, line int) (Breakpoint, bool) {
	b, ok := vm.dbg.breaks[breakKey(file, line)]
	if !ok {
		return Breakpoint{}, false
	}
	return Breakpoint{File: strings.ToUpper(filepath.Base(file)), Line: line, Cond: b.cond, Hit: b.hit, Hits: b.hits, Log: b.log}, true
}

// HasBreak reports whether a line has a breakpoint.
func (vm *VM) HasBreak(file string, line int) bool {
	_, ok := vm.dbg.breaks[breakKey(file, line)]
	return ok
}

// ClearBreaks removes every breakpoint.
func (vm *VM) ClearBreaks() {
	vm.dbg.breaks, vm.dbg.procBreaks = nil, nil
	vm.updateTracing()
}

// resetHits sets every breakpoint's hits back to zero, for a new run.
func (vm *VM) resetHits() {
	for _, b := range vm.dbg.breaks {
		b.hits = 0
	}
	for _, b := range vm.dbg.procBreaks {
		b.hits = 0
	}
	for _, b := range vm.dbg.dataBreaks {
		b.bp.hits = 0
	}
}

// Breaks lists the breakpoints: procedures first, then lines in order.
func (vm *VM) Breaks() []Breakpoint {
	var out []Breakpoint
	for k, b := range vm.dbg.breaks {
		i := strings.LastIndexByte(k, ':')
		n, _ := strconv.Atoi(k[i+1:])
		out = append(out, Breakpoint{File: k[:i], Line: n, Cond: b.cond, Hit: b.hit, Hits: b.hits, Log: b.log})
	}
	for p, b := range vm.dbg.procBreaks {
		out = append(out, Breakpoint{Proc: p, Cond: b.cond, Hit: b.hit, Hits: b.hits, Log: b.log})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Proc < out[j].Proc
	})
	return out
}

// breakWhy counts a hit on a breakpoint and says why the program stops
// there, or "" if it doesn't: its condition is false, or this hit isn't
// one its hit count stops on. A condition that fails, or isn't logical,
// stops the program too, saying so, rather than hiding the mistake. A
// logpoint never stops the program: it logs its message (or the problem
// with its condition) instead.
func (vm *VM) breakWhy(fr *Frame, what string, b *breakpoint) string {
	if b.log != "" {
		if problem := vm.logpointHit(fr, b); problem != "" {
			vm.writeLog(fr, problem)
		}
		return ""
	}
	if b.cond != "" {
		// nothing the condition runs may open the debugger
		vm.dbg.active = true
		v, err := vm.DebugEval(DebugFrame{fr: fr}, b.cond)
		vm.dbg.active = false
		switch {
		case err != nil:
			return what + " (condition error: " + err.Error() + ")"
		case v.K != value.LOGICAL:
			return what + " (condition isn't logical: " + b.cond + ")"
		case !v.B:
			return ""
		}
		what += " IF " + b.cond
	}
	b.hits++
	if !b.hit.stops(b.hits) {
		return ""
	}
	if b.hit.Op != 0 {
		what += fmt.Sprintf(", hit %d (%s)", b.hits, b.hit)
	}
	return what
}

// debugCall notes a call to a procedure with a breakpoint: debugLine
// checks it at the procedure's first line, where its parameters are set.
func (vm *VM) debugCall(fn *compile.Func) {
	if len(vm.dbg.procBreaks) == 0 || vm.dbg.active || !traceable(fn) {
		return
	}
	if b, ok := vm.dbg.procBreaks[fn.Name]; ok {
		vm.dbg.procHit, vm.dbg.procBreak = fn.Name, b
	}
}
