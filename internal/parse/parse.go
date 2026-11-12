// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

// Package parse builds an AST from preprocessed statements.
package parse

import (
	"fmt"
	"path/filepath"
	"strings"

	"nantucket/internal/ast"
	"nantucket/internal/lex"
	"nantucket/internal/value"
)

type Error struct {
	File string
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.File != "" {
		return fmt.Sprintf("%s(%d): %s", e.File, e.Line, e.Msg)
	}
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

type parser struct {
	stmts []lex.Stmt
	si    int
	toks  []lex.Token
	p     int
	line  int
	file  string
}

type bailout struct{ err *Error }

func (ps *parser) fail(f string, a ...any) {
	panic(bailout{&Error{File: ps.file, Line: ps.line, Msg: fmt.Sprintf(f, a...)}})
}

func catch(err *error) {
	if r := recover(); r != nil {
		if b, ok := r.(bailout); ok {
			*err = b.err
			return
		}
		panic(r)
	}
}

// ---------- keyword helpers ----------

// kw reports whether ident text t matches keyword k (dBase 4-char abbreviation rule).
func kw(t lex.Token, k string) bool {
	if t.Kind != lex.Ident {
		return false
	}
	if t.Text == k {
		return true
	}
	return len(t.Text) >= 4 && strings.HasPrefix(k, t.Text)
}

func (ps *parser) cur() lex.Token {
	if ps.p < len(ps.toks) {
		return ps.toks[ps.p]
	}
	return lex.Token{Kind: lex.EOF}
}
func (ps *parser) peekN(n int) lex.Token {
	if ps.p+n < len(ps.toks) {
		return ps.toks[ps.p+n]
	}
	return lex.Token{Kind: lex.EOF}
}
func (ps *parser) atEnd() bool { return ps.p >= len(ps.toks) }
func (ps *parser) isOp(op string) bool {
	return ps.cur().Is(op)
}
func (ps *parser) acceptOp(op string) bool {
	if ps.isOp(op) {
		ps.p++
		return true
	}
	return false
}
func (ps *parser) expectOp(op string) {
	if !ps.acceptOp(op) {
		ps.fail("syntax error: expected '%s' near %s", op, ps.near())
	}
}
func (ps *parser) acceptKw(k string) bool {
	if kw(ps.cur(), k) {
		ps.p++
		return true
	}
	return false
}
func (ps *parser) near() string {
	if ps.atEnd() {
		return "end of line"
	}
	return "'" + ps.cur().Raw + "'"
}
func (ps *parser) ident() string {
	t := ps.cur()
	if t.Kind != lex.Ident {
		ps.fail("syntax error: expected identifier near %s", ps.near())
	}
	ps.p++
	return t.Text
}
func (ps *parser) endStmt() {
	if !ps.atEnd() {
		ps.fail("syntax error near %s", ps.near())
	}
}

// ---------- entry points ----------

// ParseFile parses a whole program file. Top-level code before the first
// FUNCTION/PROCEDURE becomes an implicit procedure named after the file.
func ParseFile(stmts []lex.Stmt, file string) (f *ast.File, err error) {
	defer catch(&err)
	ps := &parser{stmts: stmts, file: file}
	f = &ast.File{Name: file}
	base := strings.ToUpper(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
	main := &ast.Func{Name: base, IsProc: true, Implicit: true, Line: 1}
	var curFn *ast.Func = main
	for ps.si < len(ps.stmts) {
		ps.load()
		if fn := ps.funcHeader(); fn != nil {
			curFn = fn
			f.Funcs = append(f.Funcs, fn)
			continue
		}
		if curFn == main && len(main.Body) == 0 && len(f.Funcs) == 0 {
			// file-level declarations
			if kw(ps.cur(), "STATIC") && !kw(ps.peekN(1), "FUNCTION") && !kw(ps.peekN(1), "PROCEDURE") {
				st := ps.localStmt(true)
				f.Statics = append(f.Statics, st.(*ast.LocalStmt))
				ps.si++
				continue
			}
		}
		if len(f.Funcs) > 0 && kw(ps.cur(), "STATIC") && curFn != main && len(curFn.Body) == 0 && false {
			// (function-level statics are handled inside bodies)
		}
		st, term := ps.statement(nil)
		if term != "" {
			ps.fail("%s without matching structure", term)
		}
		if st != nil {
			curFn.Body = append(curFn.Body, st)
		}
	}
	if len(main.Body) > 0 || len(f.Funcs) == 0 {
		f.Funcs = append([]*ast.Func{main}, f.Funcs...)
	}
	return f, nil
}

// ParseFragment parses statements typed at the dot prompt.
func ParseFragment(stmts []lex.Stmt) (body []ast.Stmt, err error) {
	defer catch(&err)
	ps := &parser{stmts: stmts}
	for ps.si < len(ps.stmts) {
		ps.load()
		if fn := ps.funcHeader(); fn != nil {
			ps.fail("FUNCTION/PROCEDURE not allowed here")
		}
		st, term := ps.statement(nil)
		if term != "" {
			ps.fail("%s without matching structure", term)
		}
		if st != nil {
			body = append(body, st)
		}
	}
	return body, nil
}

// ParseExpr parses a standalone expression (used by macros).
func ParseExpr(toks []lex.Token) (e ast.Expr, err error) {
	defer catch(&err)
	ps := &parser{toks: toks}
	e = ps.expr()
	ps.endStmt()
	return e, nil
}

// ParseExprList parses comma separated expressions (macro lists).
func ParseExprList(toks []lex.Token) (es []ast.Expr, err error) {
	defer catch(&err)
	ps := &parser{toks: toks}
	es = ps.args("")
	ps.endStmt()
	return es, nil
}

func (ps *parser) load() {
	st := ps.stmts[ps.si]
	ps.toks = st.Toks
	ps.p = 0
	ps.line = st.Line
	if st.File != "" {
		ps.file = st.File
	}
}

// IsBlockStart reports whether a statement opens a control structure;
// used by the dot prompt to collect multi-line input.
func IsBlockStart(toks []lex.Token) int {
	if len(toks) == 0 {
		return 0
	}
	t := toks[0]
	if len(toks) > 1 && isAssignOp(toks[1]) {
		return 0
	}
	switch {
	case kw(t, "IF") && !(len(toks) > 1 && toks[1].Is("(") && false):
		return 1
	case kw(t, "DO") && len(toks) > 1 && (kw(toks[1], "WHILE") || kw(toks[1], "CASE")):
		return 1
	case kw(t, "WHILE"), t.Kind == lex.Ident && t.Text == "FOR", kw(t, "SWITCH"), t.Kind == lex.Ident && t.Text == "TRY":
		return 1
	case kw(t, "BEGIN") && len(toks) > 1 && kw(toks[1], "SEQUENCE"):
		return 1
	case kw(t, "ENDIF"), kw(t, "ENDDO"), kw(t, "ENDCASE"), t.Kind == lex.Ident && (t.Text == "END" || t.Text == "NEXT"),
		kw(t, "ENDFOR"), kw(t, "ENDWHILE"), kw(t, "ENDSWITCH"), kw(t, "ENDSEQUENCE"), kw(t, "ENDTRY"):
		return -1
	}
	return 0
}

func isAssignOp(t lex.Token) bool {
	if t.Kind != lex.Op {
		return false
	}
	switch t.Text {
	case ":=", "=", "+=", "-=", "*=", "/=", "%=", "**=", "++", "--":
		return true
	}
	return false
}

// ---------- functions ----------

func (ps *parser) funcHeader() *ast.Func {
	t := ps.cur()
	if t.Kind != lex.Ident {
		return nil
	}
	if len(ps.toks) > 1 && isAssignOp(ps.toks[1]) {
		return nil
	}
	fn := &ast.Func{Line: ps.line}
	save := ps.p
	switch {
	case kw(t, "STATIC"):
		ps.p++
		fn.Static = true
	case t.Text == "INIT":
		ps.p++
		fn.Init = true
	case t.Text == "EXIT" && kw(ps.peekN(1), "PROCEDURE"):
		ps.p++
		fn.Exit = true
	}
	switch {
	case kw(ps.cur(), "FUNCTION"):
	case kw(ps.cur(), "PROCEDURE"):
		fn.IsProc = true
	default:
		ps.p = save
		return nil
	}
	ps.p++
	if ps.cur().Kind != lex.Ident {
		ps.p = save
		return nil
	}
	fn.Name = ps.ident()
	if ps.acceptOp("(") {
		if !ps.isOp(")") {
			for {
				if ps.acceptOp("@") {
				}
				fn.Params = append(fn.Params, ps.ident())
				ps.skipAsType()
				if !ps.acceptOp(",") {
					break
				}
			}
		}
		ps.expectOp(")")
	}
	ps.skipAsType()
	ps.endStmt()
	ps.si++
	return fn
}

func (ps *parser) skipAsType() {
	if kw(ps.cur(), "AS") {
		ps.p++
		for !ps.atEnd() && ps.cur().Kind == lex.Ident {
			ps.p++
		}
	}
}

// ---------- statements ----------

// statement parses one statement (possibly a whole structure). If the
// statement is one of the terminators, it is not consumed and its
// keyword is returned.
func (ps *parser) statement(terms []string) (ast.Stmt, string) {
	ps.load()
	t := ps.cur()
	line := ps.line
	if t.Kind == lex.Ident && !(len(ps.toks) > 1 && isAssignOp(ps.toks[1])) {
		for _, term := range terms {
			if matchTerm(ps.toks, term) {
				return nil, term
			}
		}
		for _, term := range []string{"ENDIF", "ENDDO", "ENDCASE", "ELSE", "ELSEIF", "OTHERWISE", "NEXT", "ENDFOR", "RECOVER", "ALWAYS", "CATCH", "FINALLY", "ENDSWITCH", "ENDTRY"} {
			if matchTerm(ps.toks, term) {
				return nil, term
			}
		}
		if t.Text == "END" {
			return nil, "END"
		}
		if fnh := ps.peekFuncHeader(); fnh {
			return nil, "FUNCTION"
		}
	}
	ps.si++
	if t.Kind == lex.Ident && !(len(ps.toks) > 1 && isAssignOp(ps.toks[1])) {
		switch {
		case t.Text == "IF" || (kw(t, "IF") && false):
			if !(len(ps.toks) > 1 && ps.toks[1].Is("(") && ps.isIIfCall()) {
				return ps.ifStmt(line), ""
			}
		case kw(t, "DO") && len(t.Text) <= 2:
			return ps.doStmt(line), ""
		case kw(t, "WHILE"):
			ps.p++
			return ps.whileBody(line), ""
		case t.Text == "FOR":
			return ps.forStmt(line), ""
		case kw(t, "SWITCH"):
			return ps.switchStmt(line), ""
		case kw(t, "BEGIN") && kw(ps.peekN(1), "SEQUENCE"):
			return ps.seqStmt(line, false), ""
		case t.Text == "TRY" && len(ps.toks) == 1:
			return ps.seqStmt(line, true), ""
		case kw(t, "RETURN"):
			ps.p++
			r := &ast.ReturnStmt{Line: line}
			if !ps.atEnd() {
				r.X = ps.expr()
			}
			ps.endStmt()
			return r, ""
		case t.Text == "EXIT" && len(ps.toks) == 1:
			return &ast.ExitStmt{Line: line}, ""
		case t.Text == "LOOP" && len(ps.toks) == 1:
			return &ast.LoopStmt{Line: line}, ""
		case t.Text == "BREAK" && !ps.peekN(1).Is("("):
			ps.p++
			b := &ast.BreakStmt{Line: line}
			if !ps.atEnd() {
				b.X = ps.expr()
			}
			ps.endStmt()
			return b, ""
		case t.Text == "LOCAL":
			return ps.localStmt(false), ""
		case kw(t, "STATIC"):
			return ps.localStmt(true), ""
		case kw(t, "PRIVATE"):
			return ps.memvarStmt(false), ""
		case kw(t, "PUBLIC"):
			return ps.memvarStmt(true), ""
		case kw(t, "DECLARE") || kw(t, "DIMENSION"):
			return ps.memvarStmt(false), ""
		case kw(t, "PARAMETERS"):
			ps.p++
			pr := &ast.ParamStmt{Line: line}
			for {
				pr.Names = append(pr.Names, ps.ident())
				ps.skipAsType()
				if !ps.acceptOp(",") {
					break
				}
			}
			ps.endStmt()
			return pr, ""
		case kw(t, "MEMVAR"):
			ps.p++
			d := &ast.DeclStmt{Line: line}
			d.Names = ps.nameList()
			ps.endStmt()
			return d, ""
		case t.Text == "FIELD" && ps.peekN(1).Kind == lex.Ident:
			ps.p++
			d := &ast.DeclStmt{Line: line, Field: true}
			d.Names = ps.nameList()
			if ps.acceptKw("IN") {
				d.Alias = ps.ident()
			}
			ps.endStmt()
			return d, ""
		case t.Text == "ANNOUNCE" || t.Text == "REQUEST" || t.Text == "EXTERNAL":
			return &ast.NopStmt{}, ""
		}
	}
	return ps.exprStmt(line), ""
}

func (ps *parser) isIIfCall() bool {
	// "IF(" at statement start: treat as statement unless the whole
	// statement is a single call expression IF(a,b,c)
	if !ps.toks[1].Is("(") {
		return false
	}
	depth := 0
	for i := 1; i < len(ps.toks); i++ {
		t := ps.toks[i]
		if t.Is("(") || t.Is("[") || t.Is("{") {
			depth++
		} else if t.Is(")") || t.Is("]") || t.Is("}") {
			depth--
			if depth == 0 {
				return i == len(ps.toks)-1 && hasTopComma(ps.toks[2:i])
			}
		}
	}
	return false
}

func hasTopComma(toks []lex.Token) bool {
	d := 0
	for _, t := range toks {
		switch {
		case t.Is("("), t.Is("["), t.Is("{"):
			d++
		case t.Is(")"), t.Is("]"), t.Is("}"):
			d--
		case t.Is(","):
			if d == 0 {
				return true
			}
		}
	}
	return false
}

func (ps *parser) peekFuncHeader() bool {
	t := ps.cur()
	switch {
	case kw(t, "FUNCTION"), kw(t, "PROCEDURE"):
		return ps.peekN(1).Kind == lex.Ident
	case kw(t, "STATIC") || t.Text == "INIT" || t.Text == "EXIT":
		n := ps.peekN(1)
		return (kw(n, "FUNCTION") || kw(n, "PROCEDURE")) && ps.peekN(2).Kind == lex.Ident
	}
	return false
}

func matchTerm(toks []lex.Token, term string) bool {
	t := toks[0]
	switch term {
	case "END":
		return t.Kind == lex.Ident && t.Text == "END"
	case "NEXT":
		return t.Kind == lex.Ident && t.Text == "NEXT"
	case "CASE":
		return t.Kind == lex.Ident && t.Text == "CASE"
	case "ELSE":
		return t.Kind == lex.Ident && t.Text == "ELSE"
	case "ELSEIF":
		return t.Kind == lex.Ident && t.Text == "ELSEIF"
	case "CATCH", "FINALLY", "ALWAYS":
		return t.Kind == lex.Ident && t.Text == term
	case "OTHERWISE":
		return kw(t, "OTHERWISE")
	case "RECOVER":
		return kw(t, "RECOVER")
	}
	return kw(t, term)
}

// block parses statements until one of the terminators.
func (ps *parser) block(startLine int, what string, terms ...string) ([]ast.Stmt, string) {
	var body []ast.Stmt
	for {
		if ps.si >= len(ps.stmts) {
			ps.line = startLine
			ps.fail("%s not closed (missing %s)", what, terms[0])
		}
		st, term := ps.statement(terms)
		if term != "" {
			ok := false
			for _, t := range terms {
				if t == term {
					ok = true
				}
			}
			if !ok {
				if term == "FUNCTION" {
					ps.line = startLine
					ps.fail("%s not closed (missing %s)", what, terms[0])
				}
				ps.fail("%s without matching structure", term)
			}
			return body, term
		}
		if st != nil {
			body = append(body, st)
		}
	}
}

// consumeTerm advances past a terminator statement.
func (ps *parser) consumeTerm() {
	ps.si++
}

func (ps *parser) ifStmt(line int) ast.Stmt {
	ps.p++
	st := &ast.IfStmt{Line: line}
	st.Conds = append(st.Conds, ps.expr())
	ps.endStmt()
	for {
		body, term := ps.block(line, "IF", "ENDIF", "ELSEIF", "ELSE", "END")
		if len(st.Bodies) < len(st.Conds) {
			st.Bodies = append(st.Bodies, body)
		} else {
			st.Else = body
		}
		switch term {
		case "ELSEIF":
			ps.load()
			ps.p++
			st.Conds = append(st.Conds, ps.expr())
			ps.endStmt()
			ps.si++
		case "ELSE":
			if st.Else != nil || len(st.Bodies) > len(st.Conds) {
				ps.fail("duplicate ELSE")
			}
			ps.si++
			body, term2 := ps.block(line, "IF", "ENDIF", "END")
			st.Else = body
			if st.Else == nil {
				st.Else = []ast.Stmt{}
			}
			_ = term2
			ps.consumeTerm()
			return st
		default:
			ps.consumeTerm()
			return st
		}
	}
}

func (ps *parser) doStmt(line int) ast.Stmt {
	ps.p++
	if kw(ps.cur(), "WHILE") {
		ps.p++
		return ps.whileBody(line)
	}
	if kw(ps.cur(), "CASE") {
		ps.p++
		ps.endStmt()
		return ps.caseBody(line)
	}
	d := &ast.DoStmt{Line: line}
	switch t := ps.cur(); {
	case t.Kind == lex.Macro:
		d.Macro = &ast.Macro{Text: t.Text}
		ps.p++
	case t.Is("("):
		ps.p++
		d.Macro = ps.expr()
		ps.expectOp(")")
	case t.Kind == lex.Ident:
		d.Name = t.Text
		ps.p++
		// DO name.prg
		if ps.isOp(".") && ps.peekN(1).Kind == lex.Ident {
			ps.p += 2
		}
	default:
		ps.fail("syntax error in DO near %s", ps.near())
	}
	if ps.acceptKw("WITH") {
		d.Args = ps.args("")
		// variables passed to DO are by reference
		for i, a := range d.Args {
			switch a.(type) {
			case *ast.Ident, *ast.MemvarRef:
				d.Args[i] = &ast.RefArg{X: a}
			}
		}
	}
	ps.endStmt()
	return d
}

func (ps *parser) whileBody(line int) ast.Stmt {
	st := &ast.WhileStmt{Line: line}
	st.Cond = ps.expr()
	ps.endStmt()
	st.Body, _ = ps.block(line, "DO WHILE", "ENDDO", "END", "ENDWHILE")
	ps.consumeTerm()
	return st
}

func (ps *parser) caseBody(line int) ast.Stmt {
	st := &ast.CaseStmt{Line: line}
	// skip anything before first CASE
	_, term := ps.block(line, "DO CASE", "ENDCASE", "CASE", "OTHERWISE", "END")
	for {
		switch term {
		case "CASE":
			ps.load()
			ps.p++
			st.Conds = append(st.Conds, ps.expr())
			ps.endStmt()
			ps.si++
			var body []ast.Stmt
			body, term = ps.block(line, "DO CASE", "ENDCASE", "CASE", "OTHERWISE", "END")
			st.Bodies = append(st.Bodies, body)
		case "OTHERWISE":
			ps.si++
			st.Other, term = ps.block(line, "DO CASE", "ENDCASE", "END", "CASE")
			if st.Other == nil {
				st.Other = []ast.Stmt{}
			}
			if term == "CASE" {
				ps.fail("CASE after OTHERWISE")
			}
		default:
			ps.consumeTerm()
			return st
		}
	}
}

func (ps *parser) switchStmt(line int) ast.Stmt {
	ps.p++
	st := &ast.SwitchStmt{Line: line, X: ps.expr()}
	ps.endStmt()
	_, term := ps.block(line, "SWITCH", "END", "ENDSWITCH", "CASE", "OTHERWISE")
	for {
		switch term {
		case "CASE":
			ps.load()
			ps.p++
			st.Vals = append(st.Vals, ps.expr())
			ps.endStmt()
			ps.si++
			var body []ast.Stmt
			body, term = ps.block(line, "SWITCH", "END", "ENDSWITCH", "CASE", "OTHERWISE")
			st.Bodies = append(st.Bodies, body)
		case "OTHERWISE":
			ps.si++
			st.Other, term = ps.block(line, "SWITCH", "END", "ENDSWITCH")
			if st.Other == nil {
				st.Other = []ast.Stmt{}
			}
		default:
			ps.consumeTerm()
			return st
		}
	}
}

func (ps *parser) forStmt(line int) ast.Stmt {
	ps.p++
	if ps.cur().Kind == lex.Ident && ps.cur().Text == "EACH" {
		ps.p++
		st := &ast.ForEachStmt{Line: line}
		st.Var = ps.postfix()
		if !ps.acceptKw("IN") {
			ps.fail("expected IN in FOR EACH")
		}
		st.In = ps.expr()
		if ps.acceptKw("DESCEND") {
			st.Desc = true
		}
		ps.endStmt()
		st.Body, _ = ps.block(line, "FOR EACH", "NEXT", "ENDFOR", "END")
		ps.consumeTerm()
		return st
	}
	st := &ast.ForStmt{Line: line}
	st.Var = ps.postfix()
	if !ps.acceptOp(":=") && !ps.acceptOp("=") {
		ps.fail("expected := in FOR")
	}
	st.From = ps.expr()
	if !ps.acceptKw("TO") {
		ps.fail("expected TO in FOR")
	}
	st.To = ps.expr()
	if ps.acceptKw("STEP") {
		st.Step = ps.expr()
	}
	ps.endStmt()
	st.Body, _ = ps.block(line, "FOR", "NEXT", "ENDFOR", "END")
	ps.consumeTerm()
	return st
}

func (ps *parser) seqStmt(line int, try bool) ast.Stmt {
	st := &ast.SeqStmt{Line: line, Try: try}
	if try {
		var term string
		st.Body, term = ps.block(line, "TRY", "END", "ENDTRY", "CATCH", "FINALLY")
		for {
			switch term {
			case "CATCH":
				ps.load()
				ps.p++
				st.HasRecover = true
				if !ps.atEnd() {
					st.RecoverVar = ps.postfix()
				}
				ps.endStmt()
				ps.si++
				st.Recover, term = ps.block(line, "TRY", "END", "ENDTRY", "FINALLY")
			case "FINALLY":
				ps.si++
				st.Always, term = ps.block(line, "TRY", "END", "ENDTRY")
			default:
				ps.consumeTerm()
				return st
			}
		}
	}
	ps.p += 2
	if ps.acceptKw("WITH") {
		st.With = ps.expr()
	}
	ps.endStmt()
	var term string
	st.Body, term = ps.block(line, "BEGIN SEQUENCE", "END", "ENDSEQUENCE", "RECOVER", "ALWAYS")
	for {
		switch term {
		case "RECOVER":
			ps.load()
			ps.p++
			st.HasRecover = true
			if ps.acceptKw("USING") {
				st.RecoverVar = ps.postfix()
			}
			ps.endStmt()
			ps.si++
			st.Recover, term = ps.block(line, "BEGIN SEQUENCE", "END", "ENDSEQUENCE", "ALWAYS")
		case "ALWAYS":
			ps.si++
			st.Always, term = ps.block(line, "BEGIN SEQUENCE", "END", "ENDSEQUENCE")
		default:
			ps.consumeTerm()
			return st
		}
	}
}

func (ps *parser) nameList() []string {
	var names []string
	for {
		names = append(names, ps.ident())
		if !ps.acceptOp(",") {
			break
		}
	}
	return names
}

func (ps *parser) dims() []ast.Expr {
	var dims []ast.Expr
	for ps.isOp("[") || ps.isOp("(") {
		closer := "]"
		if ps.isOp("(") {
			closer = ")"
		}
		ps.p++
		for {
			dims = append(dims, ps.expr())
			if !ps.acceptOp(",") {
				break
			}
		}
		ps.expectOp(closer)
	}
	return dims
}

func (ps *parser) localStmt(static bool) ast.Stmt {
	st := &ast.LocalStmt{Line: ps.line, Static: static}
	ps.p++
	for {
		v := ast.VarDecl{Name: ps.ident()}
		v.Dims = ps.dims()
		ps.skipAsType()
		if ps.acceptOp(":=") || ps.acceptOp("=") {
			v.Init = ps.expr()
		}
		ps.skipAsType()
		st.Vars = append(st.Vars, v)
		if !ps.acceptOp(",") {
			break
		}
	}
	ps.endStmt()
	return st
}

func (ps *parser) memvarStmt(public bool) ast.Stmt {
	st := &ast.MemvarStmt{Line: ps.line, Public: public}
	ps.p++
	if kw(ps.cur(), "ALL") {
		// PRIVATE ALL [LIKE/EXCEPT skel] - privatizing patterns is a no-op here
		ps.p = len(ps.toks)
		return &ast.NopStmt{}
	}
	for {
		if ps.cur().Kind == lex.Macro {
			st.Macro = append(st.Macro, &ast.Macro{Text: ps.cur().Text})
			ps.p++
		} else {
			v := ast.VarDecl{Name: ps.ident()}
			v.Dims = ps.dims()
			ps.skipAsType()
			if ps.acceptOp(":=") {
				v.Init = ps.expr()
			}
			st.Vars = append(st.Vars, v)
		}
		if !ps.acceptOp(",") {
			break
		}
	}
	ps.endStmt()
	return st
}

func (ps *parser) exprStmt(line int) ast.Stmt {
	// statement-level assignment with '='
	save := ps.p
	if isLvalueStart(ps.cur()) {
		lhs := ps.postfixSafe()
		if lhs != nil && ps.isOp("=") && isLvalue(lhs) {
			ps.p++
			rhs := ps.expr()
			ps.endStmt()
			return &ast.ExprStmt{Line: line, X: &ast.Assign{Op: ":=", Target: lhs, X: rhs}}
		}
	}
	ps.p = save
	x := ps.expr()
	ps.endStmt()
	return &ast.ExprStmt{Line: line, X: x}
}

func isLvalueStart(t lex.Token) bool {
	return t.Kind == lex.Ident || t.Kind == lex.Macro
}

// postfixSafe parses a postfix expression, returning nil on syntax error.
func (ps *parser) postfixSafe() (e ast.Expr) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bailout); ok {
				e = nil
				return
			}
			panic(r)
		}
	}()
	return ps.postfix()
}

func isLvalue(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident, *ast.FieldRef, *ast.MemvarRef, *ast.Index, *ast.Macro:
		return true
	case *ast.Send:
		return x.IsVar
	}
	return false
}

// ---------- expressions ----------

func (ps *parser) expr() ast.Expr {
	return ps.assignExpr()
}

func (ps *parser) assignExpr() ast.Expr {
	lhs := ps.orExpr()
	t := ps.cur()
	if t.Kind == lex.Op {
		switch t.Text {
		case ":=", "+=", "-=", "*=", "/=", "%=", "**=":
			if !isLvalue(lhs) {
				ps.fail("invalid assignment target")
			}
			ps.p++
			rhs := ps.assignExpr()
			return &ast.Assign{Op: t.Text, Target: lhs, X: rhs}
		}
	}
	return lhs
}

func (ps *parser) orExpr() ast.Expr {
	l := ps.andExpr()
	for ps.isOp(".OR.") {
		ps.p++
		l = &ast.Binary{Op: ".OR.", L: l, R: ps.andExpr()}
	}
	return l
}

func (ps *parser) andExpr() ast.Expr {
	l := ps.notExpr()
	for ps.isOp(".AND.") {
		ps.p++
		l = &ast.Binary{Op: ".AND.", L: l, R: ps.notExpr()}
	}
	return l
}

func (ps *parser) notExpr() ast.Expr {
	if ps.isOp(".NOT.") {
		ps.p++
		return &ast.Unary{Op: ".NOT.", X: ps.notExpr()}
	}
	return ps.relExpr()
}

func (ps *parser) relExpr() ast.Expr {
	l := ps.addExpr()
	for {
		t := ps.cur()
		if t.Kind != lex.Op {
			return l
		}
		switch t.Text {
		case "=", "==", "!=", "<", ">", "<=", ">=", "$":
			ps.p++
			l = &ast.Binary{Op: t.Text, L: l, R: ps.addExpr()}
		default:
			return l
		}
	}
}

func (ps *parser) addExpr() ast.Expr {
	l := ps.mulExpr()
	for ps.isOp("+") || ps.isOp("-") {
		op := ps.cur().Text
		ps.p++
		l = &ast.Binary{Op: op, L: l, R: ps.mulExpr()}
	}
	return l
}

func (ps *parser) mulExpr() ast.Expr {
	l := ps.powExpr()
	for ps.isOp("*") || ps.isOp("/") || ps.isOp("%") {
		op := ps.cur().Text
		ps.p++
		l = &ast.Binary{Op: op, L: l, R: ps.powExpr()}
	}
	return l
}

func (ps *parser) powExpr() ast.Expr {
	l := ps.unary()
	for ps.isOp("**") {
		ps.p++
		l = &ast.Binary{Op: "**", L: l, R: ps.unary()}
	}
	return l
}

func (ps *parser) unary() ast.Expr {
	t := ps.cur()
	if t.Kind == lex.Op {
		switch t.Text {
		case "-":
			ps.p++
			x := ps.unary()
			if l, ok := x.(*ast.Lit); ok && l.V.K == value.NUMBER {
				v := l.V
				v.N = -v.N
				return &ast.Lit{V: v}
			}
			return &ast.Unary{Op: "-", X: x}
		case "+":
			ps.p++
			return ps.unary()
		case ".NOT.":
			ps.p++
			return &ast.Unary{Op: ".NOT.", X: ps.unary()}
		case "++", "--":
			ps.p++
			x := ps.unary()
			if !isLvalue(x) {
				ps.fail("invalid %s target", t.Text)
			}
			return &ast.IncDec{Op: t.Text, Prefix: true, Target: x}
		case "@":
			ps.p++
			return &ast.RefArg{X: ps.postfix()}
		}
	}
	return ps.postfix()
}

func (ps *parser) postfix() ast.Expr {
	x := ps.primary()
	for {
		t := ps.cur()
		if t.Kind != lex.Op {
			return x
		}
		switch t.Text {
		case "[":
			ps.p++
			for {
				x = &ast.Index{X: x, Idx: ps.expr()}
				if !ps.acceptOp(",") {
					break
				}
			}
			ps.expectOp("]")
		case ":":
			if ps.peekN(1).Kind != lex.Ident {
				return x
			}
			ps.p++
			msg := ps.ident()
			s := &ast.Send{X: x, Msg: msg}
			if ps.isOp("(") && !ps.cur().Space {
				ps.p++
				s.Args = ps.args(")")
				ps.expectOp(")")
			} else if ps.isOp("(") {
				ps.p++
				s.Args = ps.args(")")
				ps.expectOp(")")
			} else {
				s.IsVar = true
			}
			x = s
		case "++", "--":
			if !isLvalue(x) {
				return x
			}
			ps.p++
			x = &ast.IncDec{Op: t.Text, Target: x}
		case "(":
			if m, ok := x.(*ast.Macro); ok {
				ps.p++
				args := ps.args(")")
				ps.expectOp(")")
				x = &ast.CallMacro{Callee: m, Args: args}
				continue
			}
			return x
		default:
			return x
		}
	}
}

// args parses a comma separated argument list allowing empty items.
func (ps *parser) args(closer string) []ast.Expr {
	var args []ast.Expr
	if closer != "" && ps.isOp(closer) {
		return nil
	}
	for {
		if ps.isOp(",") || (closer != "" && ps.isOp(closer)) || ps.atEnd() {
			args = append(args, &ast.Empty{})
		} else {
			args = append(args, ps.expr())
		}
		if !ps.acceptOp(",") {
			break
		}
	}
	return args
}

func (ps *parser) primary() ast.Expr {
	t := ps.cur()
	switch t.Kind {
	case lex.Number:
		ps.p++
		return &ast.Lit{V: value.Num(t.Num, t.Dec)}
	case lex.String:
		ps.p++
		return &ast.Lit{V: value.Str(t.Text)}
	case lex.Logical:
		ps.p++
		return &ast.Lit{V: value.Bool(t.Text == ".T.")}
	case lex.Date:
		ps.p++
		return &ast.Lit{V: value.Date(value.STOD(t.Text))}
	case lex.Macro:
		ps.p++
		m := &ast.Macro{Text: t.Text}
		if ps.isOp("->") {
			// &alias->field
			ps.p++
			name := ps.ident()
			return &ast.FieldRef{AliasExpr: &ast.Call{Name: "__MACROSTR", Args: []ast.Expr{&ast.Lit{V: value.Str(t.Text)}}}, Name: name}
		}
		return m
	case lex.Ident:
		return ps.identExpr()
	case lex.Op:
		switch t.Text {
		case "(":
			ps.p++
			items := ps.args(")")
			ps.expectOp(")")
			var x ast.Expr
			if len(items) == 1 {
				x = items[0]
			} else {
				x = &ast.Call{Name: "__EXPRLIST", Args: items}
			}
			if ps.isOp("->") {
				ps.p++
				return ps.aliasTail("", x)
			}
			return x
		case "{":
			return ps.brace()
		case "&":
			if ps.peekN(1).Is("(") {
				ps.p += 2
				x := ps.expr()
				ps.expectOp(")")
				return &ast.Macro{X: x}
			}
		}
	}
	if ps.atEnd() {
		ps.fail("syntax error: incomplete expression")
	}
	ps.fail("syntax error near %s", ps.near())
	return nil
}

func (ps *parser) aliasTail(alias string, aliasExpr ast.Expr) ast.Expr {
	t := ps.cur()
	switch {
	case t.Is("("):
		ps.p++
		items := ps.args(")")
		ps.expectOp(")")
		var x ast.Expr
		if len(items) == 1 {
			x = items[0]
		} else {
			x = &ast.Call{Name: "__EXPRLIST", Args: items}
		}
		return &ast.AliasExpr{Alias: alias, AliasExpr: aliasExpr, X: x}
	case t.Kind == lex.Ident:
		ps.p++
		return &ast.FieldRef{Alias: alias, AliasExpr: aliasExpr, Name: t.Text}
	case t.Kind == lex.Macro:
		ps.p++
		return &ast.FieldRef{Alias: alias, AliasExpr: aliasExpr, Name: "&" + t.Text}
	}
	ps.fail("syntax error after ->")
	return nil
}

func (ps *parser) identExpr() ast.Expr {
	t := ps.cur()
	ps.p++
	name := t.Text
	if name == "NIL" {
		return &ast.Lit{V: value.Nil}
	}
	if name == "SELF" {
		return &ast.Call{Name: "__SELF"}
	}
	if ps.isOp("(") {
		ps.p++
		args := ps.args(")")
		ps.expectOp(")")
		if name == "IIF" || name == "IF" {
			if len(args) != 3 {
				ps.fail("IIF requires 3 arguments")
			}
			return &ast.IIf{C: args[0], T: args[1], F: args[2]}
		}
		return &ast.Call{Name: name, Args: args}
	}
	if ps.isOp("->") {
		ps.p++
		switch name {
		case "M", "MEMVAR":
			return &ast.MemvarRef{Name: ps.ident()}
		case "FIELD", "_FIELD":
			// _FIELD->alias->name
			if ps.peekN(1).Is("->") && ps.cur().Kind == lex.Ident {
				alias := ps.ident()
				ps.p++
				return ps.aliasTail(alias, nil)
			}
			if ps.cur().Kind == lex.Macro {
				m := ps.cur().Text
				ps.p++
				return &ast.FieldRef{Name: "&" + m}
			}
			return &ast.FieldRef{Name: ps.ident()}
		}
		return ps.aliasTail(name, nil)
	}
	return &ast.Ident{Name: name}
}

func (ps *parser) brace() ast.Expr {
	start := ps.p
	ps.p++ // {
	if ps.isOp("|") {
		ps.p++
		b := &ast.Block{}
		if !ps.isOp("|") {
			for {
				b.Params = append(b.Params, ps.ident())
				ps.skipAsType()
				if !ps.acceptOp(",") {
					break
				}
			}
		}
		ps.expectOp("|")
		if ps.isOp("}") {
			b.Body = []ast.Expr{&ast.Lit{V: value.Nil}}
		} else {
			for {
				b.Body = append(b.Body, ps.expr())
				if !ps.acceptOp(",") {
					break
				}
			}
		}
		ps.expectOp("}")
		b.Src = lex.Join(ps.toks[start:ps.p])
		return b
	}
	if ps.isOp("=>") && ps.peekN(1).Is("}") {
		ps.p += 2
		return &ast.HashLit{}
	}
	if ps.isOp("}") {
		ps.p++
		return &ast.ArrayLit{}
	}
	first := ps.expr()
	if ps.isOp("=>") {
		h := &ast.HashLit{}
		ps.p++
		h.Keys = append(h.Keys, first)
		h.Vals = append(h.Vals, ps.expr())
		for ps.acceptOp(",") {
			h.Keys = append(h.Keys, ps.expr())
			ps.expectOp("=>")
			h.Vals = append(h.Vals, ps.expr())
		}
		ps.expectOp("}")
		return h
	}
	a := &ast.ArrayLit{Items: []ast.Expr{first}}
	for ps.acceptOp(",") {
		if ps.isOp(",") || ps.isOp("}") {
			a.Items = append(a.Items, &ast.Lit{V: value.Nil})
			continue
		}
		a.Items = append(a.Items, ps.expr())
	}
	ps.expectOp("}")
	return a
}
