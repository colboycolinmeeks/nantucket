// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"nantucket/internal/compile"
	"nantucket/internal/term"
	"nantucket/internal/value"
)

// Tracing, for following what a program does:
//
//   - nantucket -trace <file> logs every procedure call and program line,
//     with its source text, to a file ("-" for standard error);
//   - SET ECHO ON shows each program line on screen as it runs, as dBase
//     did;
//   - SET STEP ON pauses before each line until a key is pressed (Esc
//     stops stepping).
//
// Lines typed at the dot prompt are not traced, only program files.

// SetTrace sends a trace of calls and lines to w (nil stops it).
func (vm *VM) SetTrace(w io.Writer) {
	vm.traceW = w
	vm.updateTracing()
}

func (vm *VM) updateTracing() {
	vm.tracing = vm.traceW != nil || vm.sets[106].Truthy() || vm.stepping || vm.debugWanted()
}

// sourceLine returns line n of a program file, for showing in a trace.
func (vm *VM) sourceLine(file string, n int) string {
	if vm.srcCache == nil {
		vm.srcCache = map[string][]string{}
	}
	lines, ok := vm.srcCache[file]
	if !ok {
		var src string
		key := strings.ToUpper(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
		if s, ok := vm.Embedded[key]; ok && vm.Packaged {
			src = s
		} else if b, err := os.ReadFile(file); err == nil {
			src = string(b)
		}
		lines = strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
		vm.srcCache[file] = lines
	}
	if n >= 1 && n <= len(lines) {
		return strings.TrimSpace(lines[n-1])
	}
	return ""
}

func traceable(fn *compile.Func) bool {
	return fn.File != "" && fn.File != "(prompt)" && !strings.HasPrefix(fn.File, "(")
}

// traceLine is called before each line of a program runs, when tracing.
func (vm *VM) traceLine(fr *Frame) {
	if !traceable(fr.fn) {
		return
	}
	msg := fmt.Sprintf("%s(%d) %s: %s", filepath.Base(fr.fn.File), fr.line, fr.fn.Name, vm.sourceLine(fr.fn.File, fr.line))
	if vm.traceW != nil {
		fmt.Fprintln(vm.traceW, strings.Repeat("  ", max(vm.depth-1, 0))+msg)
	}
	if vm.sets[106].Truthy() || vm.stepping {
		vm.Println(msg)
	}
	if vm.stepping {
		vm.Out("  (any key: next line, Esc: stop stepping)", false)
		vm.Con.Flush()
		k, ok := vm.Con.ReadKey(-1)
		if !ok || k == term.KEsc {
			vm.stepping = false
			vm.updateTracing()
		}
	}
	vm.debugLine(fr)
}

// traceCall logs a call to a procedure or function.
func (vm *VM) traceCall(fn *compile.Func, nargs int) {
	if vm.traceW == nil || fn.IsBlock || !traceable(fn) {
		return
	}
	fmt.Fprintf(vm.traceW, "%s-> %s(%d argument(s))\n", strings.Repeat("  ", max(vm.depth-1, 0)), fn.Name, nargs)
}

func init() {
	// __SetStep(cOnOff | lOn): SET STEP ON / OFF.
	reg("__SETSTEP", func(vm *VM, a []value.Value) value.Value {
		old := vm.stepping
		switch v := arg(a, 0); v.K {
		case value.LOGICAL:
			vm.stepping = v.B
		case value.STRING:
			vm.stepping = strings.EqualFold(strings.TrimSpace(v.S), "ON")
		}
		vm.updateTracing()
		return value.Bool(old)
	})
}
