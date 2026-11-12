// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"fmt"
	"strings"

	"nantucket/internal/ast"
	"nantucket/internal/lex"
	"nantucket/internal/parse"
	"nantucket/internal/value"
)

// Expressions stored in data files.
//
// An index (.ntx) keeps its key and FOR expressions as text, and report
// and label forms (.frm, .lbl) keep theirs. They are run when the file is
// used, so a file from someone else could carry code: an index key that
// runs a shell command, say. Before such an expression is compiled,
// CheckDataExpr makes sure it only reads data:
//
//   - fields, memory variables (read only), literals, operators, array
//     indexing and IIF;
//   - built-in functions that have no side effects (safeFunctions);
//   - functions defined by the program that is running: its code is
//     already trusted, and a file can only call functions that exist.
//
// Anything else (macros, assignments, code blocks, object messages, and
// built-ins that touch files, the shell, the screen or variables) is
// refused, unless SET TRUSTED is ON.

// safeFunctions are the built-ins allowed in data file expressions.
var safeFunctions = map[string]bool{}

func init() {
	for _, f := range strings.Fields(`
		UPPER LOWER TRIM RTRIM LTRIM ALLTRIM SUBSTR LEFT RIGHT STR STRZERO VAL
		PADL PADR PADC PAD SPACE REPLICATE LEN AT RAT STUFF STRTRAN CHR ASC
		ISALPHA ISDIGIT ISUPPER ISLOWER SOUNDEX DESCEND EMPTY TRANSFORM
		DTOS DTOC CTOD STOD DATE YEAR MONTH DAY DOW CDOW CMONTH TIME SECONDS
		ABS INT ROUND MAX MIN MOD SQRT LOG EXP VALTYPE
		DELETED RECNO RECCOUNT LASTREC FOUND EOF BOF FIELD FIELDNAME FIELDGET FIELDPOS
		HB_NTOS HB_VALTOSTR NTOC`) {
		safeFunctions[f] = true
	}
	// SET TRUSTED ON | OFF: allow any expression in data files
	reg("__SETTRUSTED", func(vm *VM, a []value.Value) value.Value {
		old := vm.trusted
		switch v := arg(a, 0); v.K {
		case value.LOGICAL:
			vm.trusted = v.B
		case value.STRING:
			vm.trusted = strings.EqualFold(strings.TrimSpace(v.S), "ON")
		}
		return value.Bool(old)
	})
}

// CheckDataExpr reports why an expression read from a data file isn't
// allowed, or nil if it is (or SET TRUSTED is ON).
func (vm *VM) CheckDataExpr(src string) error {
	if vm.trusted || strings.TrimSpace(src) == "" {
		return nil
	}
	toks, err := lex.LexLine(src)
	if err != nil {
		return err
	}
	e, err := parse.ParseExpr(toks)
	if err != nil {
		return err
	}
	return vm.checkExpr(e)
}

func (vm *VM) checkExpr(e ast.Expr) error {
	all := func(xs ...ast.Expr) error {
		for _, x := range xs {
			if x == nil {
				continue
			}
			if err := vm.checkExpr(x); err != nil {
				return err
			}
		}
		return nil
	}
	switch x := e.(type) {
	case nil, *ast.Lit, *ast.Ident, *ast.MemvarRef, *ast.Empty:
		return nil
	case *ast.FieldRef:
		return all(x.AliasExpr)
	case *ast.AliasExpr:
		return all(x.AliasExpr, x.X)
	case *ast.Index:
		return all(x.X, x.Idx)
	case *ast.Unary:
		return all(x.X)
	case *ast.Binary:
		return all(x.L, x.R)
	case *ast.IIf:
		return all(x.C, x.T, x.F)
	case *ast.ArrayLit:
		return all(x.Items...)
	case *ast.HashLit:
		if err := all(x.Keys...); err != nil {
			return err
		}
		return all(x.Vals...)
	case *ast.Call:
		name := strings.ToUpper(x.Name)
		if name != "IIF" && name != "IF" && !safeFunctions[name] && vm.funcs[name] == nil {
			return fmt.Errorf("calls %s(), which isn't allowed in a data file", x.Name)
		}
		return all(x.Args...)
	case *ast.Macro, *ast.CallMacro:
		return fmt.Errorf("uses a macro (&), which isn't allowed in a data file")
	case *ast.Assign, *ast.IncDec:
		return fmt.Errorf("changes a variable, which isn't allowed in a data file")
	case *ast.Block:
		return fmt.Errorf("contains a code block, which isn't allowed in a data file")
	case *ast.Send:
		return fmt.Errorf("sends an object message, which isn't allowed in a data file")
	case *ast.RefArg:
		return fmt.Errorf("passes a variable by reference, which isn't allowed in a data file")
	}
	return fmt.Errorf("uses something that isn't allowed in a data file")
}

// dataExprError is the runtime error for a refused expression.
func dataExprError(what, file, expr string, err error) *RTError {
	return &RTError{SubSystem: "BASE", GenCode: 1, SubCode: 2016,
		Description: fmt.Sprintf("The %s expression in this file %s", what, err.Error()),
		Filename:    file, Operation: expr,
		Hint: "Files can carry code. If you trust this file, SET TRUSTED ON and open it again."}
}

// DataExprError is dataExprError for feature packages (report forms).
func DataExprError(what, file, expr string, err error) *RTError {
	return dataExprError(what, file, expr, err)
}
