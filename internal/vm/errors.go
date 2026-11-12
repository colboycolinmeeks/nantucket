// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"fmt"
	"strings"

	"nantucket/internal/value"
)

// RTError is a runtime error; it doubles as the Error object.
type RTError struct {
	SubSystem   string
	GenCode     int
	SubCode     int
	Description string
	Operation   string
	Filename    string
	Severity    int
	Args        []value.Value
	CanRetry    bool
	CanDefault  bool
	CanSubst    bool
	Cargo       value.Value
	OsCode      int
	Proc        string
	// Hint says how to fix the problem. It is shown with the message but
	// is not part of the description programs see (e:description).
	Hint  string
	Line  int
	Trace []string
}

func (e *RTError) ClassName() string { return "ERROR" }

func (e *RTError) Error() string {
	var sb strings.Builder
	sub := e.SubSystem
	if sub == "" {
		sub = "BASE"
	}
	fmt.Fprintf(&sb, "Error %s/%d  %s", sub, e.SubCode, e.Description)
	if e.Operation != "" {
		sb.WriteString(": " + e.Operation)
	}
	if e.Filename != "" {
		sb.WriteString(": " + e.Filename)
	}
	if e.Hint != "" {
		sb.WriteString("\n  " + e.Hint)
	}
	return sb.String()
}

func (e *RTError) Send(vm *VM, msg string, args []value.Value) value.Value {
	set := len(args) > 0
	var a value.Value
	if set {
		a = args[0]
	}
	switch strings.TrimPrefix(msg, "_") {
	case "DESCRIPTION":
		if set {
			e.Description = a.S
		}
		return value.Str(e.Description)
	case "OPERATION":
		if set {
			e.Operation = a.S
		}
		return value.Str(e.Operation)
	case "SUBSYSTEM":
		if set {
			e.SubSystem = a.S
		}
		return value.Str(e.SubSystem)
	case "GENCODE":
		if set {
			e.GenCode = a.Int()
		}
		return value.Int(e.GenCode)
	case "SUBCODE":
		if set {
			e.SubCode = a.Int()
		}
		return value.Int(e.SubCode)
	case "SEVERITY":
		if set {
			e.Severity = a.Int()
		}
		return value.Int(e.Severity)
	case "FILENAME":
		if set {
			e.Filename = a.S
		}
		return value.Str(e.Filename)
	case "ARGS":
		if set && a.K == value.ARRAY {
			e.Args = a.Arr().Items
		}
		return value.NewArray(append([]value.Value(nil), e.Args...))
	case "CANRETRY":
		if set {
			e.CanRetry = a.B
		}
		return value.Bool(e.CanRetry)
	case "CANDEFAULT":
		if set {
			e.CanDefault = a.B
		}
		return value.Bool(e.CanDefault)
	case "CANSUBSTITUTE":
		if set {
			e.CanSubst = a.B
		}
		return value.Bool(e.CanSubst)
	case "CARGO":
		if set {
			e.Cargo = a
		}
		return e.Cargo
	case "OSCODE":
		if set {
			e.OsCode = a.Int()
		}
		return value.Int(e.OsCode)
	case "TRIES":
		return value.Int(0)
	case "CLASSNAME":
		return value.Str("ERROR")
	}
	vm.Raise(&RTError{SubSystem: "BASE", GenCode: 5, SubCode: 1004, Description: "No exported method", Operation: msg})
	return value.Nil
}

// BreakSignal unwinds to the nearest BEGIN SEQUENCE.
type BreakSignal struct{ V value.Value }

// QuitSignal ends the program (QUIT).
type QuitSignal struct{}

// CancelSignal returns to the dot prompt (CANCEL, Ctrl-C).
type CancelSignal struct{ Interrupted bool }

// Common error constructors.
func errArg(op string, args ...value.Value) *RTError {
	return &RTError{SubSystem: "BASE", GenCode: 1, SubCode: 1099, Description: "Argument error", Operation: op, Args: args, CanSubst: true}
}

func errArgCode(code int, op string, args ...value.Value) *RTError {
	return &RTError{SubSystem: "BASE", GenCode: 1, SubCode: code, Description: "Argument error", Operation: op, Args: args, CanSubst: true}
}

func errNoVar(name string) *RTError {
	return &RTError{SubSystem: "BASE", GenCode: 3, SubCode: 1003, Description: "Variable does not exist", Operation: name}
}

func errNoFunc(name string) *RTError {
	return &RTError{SubSystem: "BASE", GenCode: 4, SubCode: 1001, Description: "Undefined function", Operation: name}
}

func errDB(code int, desc, op string) *RTError {
	return &RTError{SubSystem: "DBFNTX", GenCode: 21, SubCode: code, Description: desc, Operation: op}
}

func errMsg(desc string) *RTError {
	return &RTError{SubSystem: "BASE", GenCode: 1, SubCode: 9999, Description: desc}
}
