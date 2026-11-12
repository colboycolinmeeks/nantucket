// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package compile turns an AST into bytecode for the VM.
package compile

import (
	"fmt"
	"strings"

	"nantucket/internal/value"
)

type Op uint8

const (
	NOP Op = iota
	PUSHNIL
	PUSHTRUE
	PUSHFALSE
	PUSHCONST // A=const
	PUSHLOCAL // A=slot
	POPLOCAL
	PUSHLOCALREF
	PUSHCAPT
	POPCAPT
	PUSHCAPTREF
	PUSHSTATIC // A=module static index
	POPSTATIC
	PUSHSTATICREF
	PUSHMEMVAR // A=name
	POPMEMVAR
	PUSHMEMVARREF
	PUSHVAR // ambiguous: field or memvar
	POPVAR
	PUSHVARREF
	PUSHFIELD // A=name, B=alias name+1 (0=current)
	POPFIELD
	PUSHFIELDX // alias value on stack
	POPFIELDX  // stack: value, alias
	PUSHALIAS  // A=name: select area, save previous
	PUSHALIASX // alias value on stack
	POPALIAS   // restore area (keeps top value)
	MACROPUSH  // A=const text
	MACROPUSHX // string on stack
	MACROPOP   // A=const text; value on stack (consumed)
	MACROPOPX  // stack: value, name
	MACROTEXT  // A=const text -> substituted string
	ARRAYGET
	ARRAYSET // arr idx val -> val
	MKARRAY  // A=n
	MKHASH   // A=n pairs
	MKBLOCK  // A=block index
	CALL     // A=name B=argc
	CALLX    // B=argc, callee below args
	SEND     // A=msg B=argc
	SENDSET  // A=msg: obj val -> val
	DO       // A=name B=argc
	DOX      // B=argc
	POP
	DUP
	DUP2
	ADD
	SUB
	MUL
	DIV
	MOD
	POW
	NEG
	NOT
	EQ
	EXACTEQ
	NE
	LT
	LE
	GT
	GE
	INSTR
	JMP // A=target
	JF  // pop, jump if false
	JT  // pop, jump if true
	JFK // jump if false keeping value, else pop
	JTK // jump if true keeping value, else pop
	RET // return top
	RETNIL
	LINE     // A=line
	SEQBEGIN // A=recover pc, B=1 for TRY
	SEQEND
	PRIVATE  // A=name
	PRIVATEX // name on stack
	PUBLIC   // A=name
	PARAM    // A=name B=arg index
	FORTEST  // var to step -> bool
	FOREACHGET
	TOLOGIC // check logical for AND/OR results
)

var opNames = [...]string{"NOP", "PUSHNIL", "PUSHTRUE", "PUSHFALSE", "PUSHCONST", "PUSHLOCAL", "POPLOCAL", "PUSHLOCALREF",
	"PUSHCAPT", "POPCAPT", "PUSHCAPTREF", "PUSHSTATIC", "POPSTATIC", "PUSHSTATICREF", "PUSHMEMVAR", "POPMEMVAR",
	"PUSHMEMVARREF", "PUSHVAR", "POPVAR", "PUSHVARREF", "PUSHFIELD", "POPFIELD", "PUSHFIELDX", "POPFIELDX",
	"PUSHALIAS", "PUSHALIASX", "POPALIAS", "MACROPUSH", "MACROPUSHX", "MACROPOP", "MACROPOPX", "MACROTEXT",
	"ARRAYGET", "ARRAYSET", "MKARRAY", "MKHASH", "MKBLOCK", "CALL", "CALLX", "SEND", "SENDSET", "DO", "DOX",
	"POP", "DUP", "DUP2", "ADD", "SUB", "MUL", "DIV", "MOD", "POW", "NEG", "NOT", "EQ", "EXACTEQ", "NE", "LT", "LE",
	"GT", "GE", "INSTR", "JMP", "JF", "JT", "JFK", "JTK", "RET", "RETNIL", "LINE", "SEQBEGIN", "SEQEND", "PRIVATE",
	"PRIVATEX", "PUBLIC", "PARAM", "FORTEST", "FOREACHGET", "TOLOGIC"}

func (o Op) String() string {
	if int(o) < len(opNames) {
		return opNames[o]
	}
	return fmt.Sprintf("OP%d", o)
}

type Instr struct {
	Op Op
	A  int32
	B  int32
}

// Capture describes where a code block gets a captured variable from
// the frame that creates it.
type Capture struct {
	FromLocal bool // true: enclosing frame's local slot; false: enclosing block's capture
	Index     int
	Name      string
}

type Func struct {
	Name    string
	Code    []Instr
	Consts  []value.Value
	Names   []string
	Blocks  []*Func
	NParams int
	NLocals int
	// LocalNames names each local slot, for the debugger. Hidden
	// temporaries start with a NUL byte.
	LocalNames []string
	Captures   []Capture
	IsProc     bool
	Static     bool
	IsBlock    bool
	Init       bool
	Exit       bool
	Src        string
	File       string
	Line       int
	Module     *Module
	// Cache is VM-owned per-instruction call resolution.
	Cache []any
}

// Module is one compiled source file.
type Module struct {
	Name        string
	Funcs       []*Func
	NStatics    int
	StaticNames []string
	Init        *Func // static initializers
	StaticBase  int   // assigned by VM when loaded
	Loaded      bool
}

// Disasm returns a readable listing (debug aid).
func (f *Func) Disasm() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "FUNC %s params=%d locals=%d\n", f.Name, f.NParams, f.NLocals)
	for i, in := range f.Code {
		arg := ""
		switch in.Op {
		case PUSHCONST, MACROPUSH, MACROPOP, MACROTEXT:
			arg = f.Consts[in.A].String()
		case PUSHMEMVAR, POPMEMVAR, PUSHVAR, POPVAR, PUSHFIELD, POPFIELD, CALL, SEND, DO, PRIVATE, PUBLIC, PARAM, PUSHALIAS, PUSHMEMVARREF, PUSHVARREF, SENDSET:
			arg = f.Names[in.A]
		}
		fmt.Fprintf(&sb, "%4d %-12s %d %d %s\n", i, in.Op, in.A, in.B, arg)
	}
	for _, b := range f.Blocks {
		sb.WriteString(b.Disasm())
	}
	return sb.String()
}
