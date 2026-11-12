// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"fmt"
	"path/filepath"
	"strings"

	"nantucket/internal/value"
)

// AssertFailure describes a failed ASSERT or Assert*() call.
type AssertFailure struct {
	File    string // base name of the source file
	Line    int
	Proc    string
	Message string
}

func (f AssertFailure) String() string {
	loc := f.Proc
	if f.File != "" {
		loc = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return loc + ": " + f.Message
}

// SetAssertHandler sets a function that receives failed assertions (the
// test runner uses it), after which the program carries on. Without one, a
// failed assertion raises a runtime error that stops the program.
func (vm *VM) SetAssertHandler(h func(AssertFailure)) { vm.onAssertFail = h }

func (vm *VM) assertFail(msg string) value.Value {
	f := AssertFailure{Message: msg}
	if fr := vm.curFrame(); fr != nil {
		f.Proc, f.Line = vm.procName(fr), fr.line
		if fr.fn.File != "" {
			f.File = filepath.Base(fr.fn.File)
		}
	}
	if vm.onAssertFail != nil {
		vm.onAssertFail(f)
		return value.False
	}
	return vm.Raise(&RTError{SubSystem: "BASE", GenCode: 1, SubCode: 9100,
		Description: "Assertion failed: " + msg})
}

// ValueExpr shows a value the way it would be written in source, for
// assertion messages: strings quoted, dates as 0dYYYYMMDD, arrays in braces.
func (vm *VM) ValueExpr(v value.Value) string {
	v = v.Deref()
	switch v.K {
	case value.NIL:
		return "NIL"
	case value.STRING:
		return fmt.Sprintf("%q", v.S)
	case value.NUMBER:
		return strings.TrimSpace(vm.Display(v))
	case value.DATE:
		if v.J == 0 {
			return "0d00000000"
		}
		return "0d" + value.DTOS(v.J)
	case value.LOGICAL:
		if v.B {
			return ".T."
		}
		return ".F."
	case value.ARRAY:
		var parts []string
		for _, it := range v.Arr().Items {
			parts = append(parts, vm.ValueExpr(it))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case value.HASH:
		h := v.Hsh()
		var parts []string
		for i, k := range h.Keys {
			parts = append(parts, vm.ValueExpr(k)+" => "+vm.ValueExpr(h.Vals[i]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case value.BLOCK:
		return "{|| ... }"
	}
	return strings.TrimSpace(vm.Display(v))
}

// valuesEqual compares exactly, as == does, and element by element for
// arrays and hashes.
func valuesEqual(a, b value.Value) bool {
	a, b = a.Deref(), b.Deref()
	if a.K != b.K {
		return false
	}
	switch a.K {
	case value.NIL:
		return true
	case value.STRING:
		return a.S == b.S
	case value.NUMBER:
		return a.N == b.N
	case value.DATE:
		return a.J == b.J
	case value.LOGICAL:
		return a.B == b.B
	case value.ARRAY:
		x, y := a.Arr().Items, b.Arr().Items
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if !valuesEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case value.HASH:
		x, y := a.Hsh(), b.Hsh()
		if x.Len() != y.Len() {
			return false
		}
		for i, k := range x.Keys {
			v, ok := y.Get(k)
			if !ok || !valuesEqual(x.Vals[i], v) {
				return false
			}
		}
		return true
	}
	return a.P == b.P
}

func init() {
	// __Assert( lCond, cCondText, [cMessage] ) implements the ASSERT command.
	reg("__ASSERT", func(vm *VM, a []value.Value) value.Value {
		if arg(a, 0).Truthy() {
			return value.True
		}
		msg := argStr(a, 2, "")
		if msg == "" {
			msg = strings.TrimSpace(argStr(a, 1, ""))
		}
		return vm.assertFail(msg)
	})
	// AssertTrue( lCond, [cMessage] ) -> lPassed
	reg("ASSERTTRUE", func(vm *VM, a []value.Value) value.Value {
		if arg(a, 0).Truthy() {
			return value.True
		}
		return vm.assertFail(argStr(a, 1, "expected .T., got "+vm.ValueExpr(arg(a, 0))))
	})
	// AssertFalse( lCond, [cMessage] ) -> lPassed
	reg("ASSERTFALSE", func(vm *VM, a []value.Value) value.Value {
		if v := arg(a, 0).Deref(); v.K == value.LOGICAL && !v.B {
			return value.True
		}
		return vm.assertFail(argStr(a, 1, "expected .F., got "+vm.ValueExpr(arg(a, 0))))
	})
	// AssertEquals( xExpected, xActual, [cMessage] ) -> lPassed
	reg("ASSERTEQUALS", func(vm *VM, a []value.Value) value.Value {
		want, got := arg(a, 0), arg(a, 1)
		if valuesEqual(want, got) {
			return value.True
		}
		msg := fmt.Sprintf("expected %s, got %s", vm.ValueExpr(want), vm.ValueExpr(got))
		if m := argStr(a, 2, ""); m != "" {
			msg = m + ": " + msg
		}
		return vm.assertFail(msg)
	})
	// AssertError( bBlock, [cMessage] ) -> oError or NIL: passes when the
	// block raises a runtime error, and returns the Error object.
	reg("ASSERTERROR", func(vm *VM, a []value.Value) value.Value {
		b := arg(a, 0)
		var caught *RTError
		func() {
			saved := vm.errorBlock
			vm.errorBlock = value.Value{K: value.BLOCK, P: &value.Block{Fn: NativeBlock(func(vm *VM, args []value.Value) value.Value {
				if len(args) > 0 {
					if e, ok := args[0].P.(*RTError); ok {
						panic(&assertCaught{e})
					}
				}
				return value.Nil
			}), Src: "{|e| Break(e)}"}}
			sp, nf := len(vm.stack), len(vm.frames)
			defer func() {
				vm.errorBlock = saved
				if r := recover(); r != nil {
					c, ok := r.(*assertCaught)
					if !ok {
						panic(r)
					}
					caught = c.e
					vm.stack = vm.stack[:sp]
					vm.frames = vm.frames[:nf]
				}
			}()
			vm.EvalBlock(b)
		}()
		if caught != nil {
			return value.Obj(caught)
		}
		vm.assertFail(argStr(a, 1, "expected an error, but none was raised"))
		return value.Nil
	})
}

type assertCaught struct{ e *RTError }
