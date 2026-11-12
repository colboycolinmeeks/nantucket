// Copyright 2026 Colin Meeks
// SPDX-License-Identifier: Apache-2.0

package compile

import (
	"fmt"
	"strings"

	"nantucket/internal/ast"
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
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

type bail struct{ e *Error }

type compiler struct {
	mod         *Module
	file        string
	fileStatics map[string]int
	fileFields  map[string]string
	fileMemvars map[string]bool
	init        *fstate
	line        int
}

type loopCtx struct {
	exits    []int
	loops    []int
	seqDepth int
	isSwitch bool
}

type fstate struct {
	c        *compiler
	fn       *Func
	parent   *fstate
	locals   map[string]int
	captIdx  map[string]int
	statics  map[string]int
	fields   map[string]string
	memvars  map[string]bool
	loops    []*loopCtx
	seqDepth int
	consts   map[string]int
	names    map[string]int
	hidden   int
	noLocals bool // macro context
}

func (c *compiler) fail(f string, a ...any) {
	panic(bail{&Error{File: c.file, Line: c.line, Msg: fmt.Sprintf(f, a...)}})
}

func newFstate(c *compiler, fn *Func, parent *fstate) *fstate {
	return &fstate{c: c, fn: fn, parent: parent, locals: map[string]int{}, captIdx: map[string]int{},
		statics: map[string]int{}, fields: map[string]string{}, memvars: map[string]bool{},
		consts: map[string]int{}, names: map[string]int{}}
}

// CompileFile compiles a parsed file into a module.
func CompileFile(f *ast.File) (mod *Module, err error) {
	defer func() {
		if r := recover(); r != nil {
			if b, ok := r.(bail); ok {
				err = b.e
				return
			}
			panic(r)
		}
	}()
	c := &compiler{mod: &Module{Name: f.Name}, file: f.Name, fileStatics: map[string]int{},
		fileFields: map[string]string{}, fileMemvars: map[string]bool{}}
	initFn := &Func{Name: "(static init)", File: f.Name, Module: c.mod}
	c.init = newFstate(c, initFn, nil)
	c.init.noLocals = true
	c.mod.Init = initFn
	for _, st := range f.Statics {
		c.line = st.Line
		for _, v := range st.Vars {
			idx := c.newStatic(v.Name)
			c.fileStatics[v.Name] = idx
			c.staticInit(idx, v)
		}
	}
	for _, af := range f.Funcs {
		fn := c.compileFunc(af)
		c.mod.Funcs = append(c.mod.Funcs, fn)
	}
	c.init.emit(RETNIL, 0, 0)
	return c.mod, nil
}

// CompileFragment compiles dot-prompt statements into a procedure.
func CompileFragment(body []ast.Stmt, name string) (fn *Func, err error) {
	return CompileFile0(&ast.File{Name: name, Funcs: []*ast.Func{{Name: name, IsProc: true, Body: body, Implicit: true}}})
}

// CompileFile0 compiles and returns the first function (helper).
func CompileFile0(f *ast.File) (*Func, error) {
	m, err := CompileFile(f)
	if err != nil {
		return nil, err
	}
	return m.Funcs[0], nil
}

// CompileMacro compiles an expression evaluated by a macro. If assign is
// true the function takes one parameter and assigns it to the expression.
func CompileMacro(e ast.Expr, src string, assign bool) (fn *Func, err error) {
	defer func() {
		if r := recover(); r != nil {
			if b, ok := r.(bail); ok {
				err = b.e
				return
			}
			panic(r)
		}
	}()
	c := &compiler{mod: &Module{Name: "(macro)"}, file: "", fileStatics: map[string]int{},
		fileFields: map[string]string{}, fileMemvars: map[string]bool{}}
	fn = &Func{Name: "(macro)", Src: src, Module: c.mod}
	c.mod.Init = &Func{Name: "(init)", Module: c.mod, Code: []Instr{{Op: RETNIL}}}
	fs := newFstate(c, fn, nil)
	fs.noLocals = true
	if assign {
		fn.NParams = 1
		fn.NLocals = 1
		fs.emit(PUSHLOCAL, 0, 0)
		fs.assignTo(e)
		fs.emit(PUSHLOCAL, 0, 0)
		fs.emit(RET, 0, 0)
	} else {
		fs.expr(e)
		fs.emit(RET, 0, 0)
	}
	fs.finish()
	c.mod.Funcs = []*Func{fn}
	return fn, nil
}

func (c *compiler) newStatic(name string) int {
	idx := c.mod.NStatics
	c.mod.NStatics++
	c.mod.StaticNames = append(c.mod.StaticNames, name)
	return idx
}

func (c *compiler) staticInit(idx int, v ast.VarDecl) {
	fs := c.init
	if len(v.Dims) > 0 {
		for _, d := range v.Dims {
			fs.expr(d)
		}
		fs.emit(CALL, fs.name("ARRAY"), int32(len(v.Dims)))
		fs.emit(POPSTATIC, int32(idx), 0)
	} else if v.Init != nil {
		fs.expr(v.Init)
		fs.emit(POPSTATIC, int32(idx), 0)
	}
}

func (c *compiler) compileFunc(af *ast.Func) *Func {
	fn := &Func{Name: af.Name, IsProc: af.IsProc, Static: af.Static, Init: af.Init, Exit: af.Exit,
		File: c.file, Line: af.Line, Module: c.mod, NParams: len(af.Params)}
	fs := newFstate(c, fn, nil)
	for _, p := range af.Params {
		fs.newLocal(p)
	}
	fs.stmts(af.Body)
	fs.emit(RETNIL, 0, 0)
	fs.finish()
	return fn
}

func (fs *fstate) finish() {
	fs.fn.Cache = make([]any, len(fs.fn.Code))
}

// ---------- emit helpers ----------

func (fs *fstate) emit(op Op, a, b int32) int {
	fs.fn.Code = append(fs.fn.Code, Instr{Op: op, A: a, B: b})
	return len(fs.fn.Code) - 1
}
func (fs *fstate) here() int    { return len(fs.fn.Code) }
func (fs *fstate) patch(at int) { fs.fn.Code[at].A = int32(fs.here()) }

func (fs *fstate) konst(v value.Value) int32 {
	key := ""
	switch v.K {
	case value.STRING:
		key = "C" + v.S
	case value.NUMBER:
		key = fmt.Sprintf("N%v/%d/%d", v.N, v.Dec, v.Width)
	case value.DATE:
		key = fmt.Sprintf("D%d", v.J)
	}
	if key != "" {
		if i, ok := fs.consts[key]; ok {
			return int32(i)
		}
	}
	fs.fn.Consts = append(fs.fn.Consts, v)
	i := len(fs.fn.Consts) - 1
	if key != "" {
		fs.consts[key] = i
	}
	return int32(i)
}

func (fs *fstate) name(n string) int32 {
	n = strings.ToUpper(n)
	if i, ok := fs.names[n]; ok {
		return int32(i)
	}
	fs.fn.Names = append(fs.fn.Names, n)
	fs.names[n] = len(fs.fn.Names) - 1
	return int32(len(fs.fn.Names) - 1)
}

func (fs *fstate) newLocal(name string) int {
	idx := fs.fn.NLocals
	fs.fn.NLocals++
	fs.locals[name] = idx
	fs.fn.LocalNames = append(fs.fn.LocalNames, name)
	return idx
}

func (fs *fstate) hiddenLocal() int {
	fs.hidden++
	return fs.newLocal(fmt.Sprintf("\x00h%d", fs.hidden))
}

// ---------- variable resolution ----------

type varKind int

const (
	vLocal varKind = iota
	vCapt
	vStatic
	vField
	vMemvar
	vAmbig
)

type varRef struct {
	kind  varKind
	idx   int
	alias string
}

func (fs *fstate) resolve(name string) varRef {
	if i, ok := fs.locals[name]; ok {
		return varRef{kind: vLocal, idx: i}
	}
	if fs.fn.IsBlock {
		if i, ok := fs.captIdx[name]; ok {
			return varRef{kind: vCapt, idx: i}
		}
		if fs.parent != nil {
			if cap, ok := fs.parent.captureSource(name); ok {
				fs.fn.Captures = append(fs.fn.Captures, cap)
				i := len(fs.fn.Captures) - 1
				fs.captIdx[name] = i
				return varRef{kind: vCapt, idx: i}
			}
		}
	}
	for s := fs; s != nil; s = s.parent {
		if i, ok := s.statics[name]; ok {
			return varRef{kind: vStatic, idx: i}
		}
		if a, ok := s.fields[name]; ok {
			return varRef{kind: vField, alias: a}
		}
		if s.memvars[name] {
			return varRef{kind: vMemvar}
		}
	}
	if i, ok := fs.c.fileStatics[name]; ok {
		return varRef{kind: vStatic, idx: i}
	}
	if a, ok := fs.c.fileFields[name]; ok {
		return varRef{kind: vField, alias: a}
	}
	if fs.c.fileMemvars[name] {
		return varRef{kind: vMemvar}
	}
	return varRef{kind: vAmbig}
}

// captureSource finds name as a local (or capture) of fs for a nested block.
func (fs *fstate) captureSource(name string) (Capture, bool) {
	if i, ok := fs.locals[name]; ok {
		return Capture{FromLocal: true, Index: i, Name: name}, true
	}
	if fs.fn.IsBlock {
		if i, ok := fs.captIdx[name]; ok {
			return Capture{FromLocal: false, Index: i, Name: name}, true
		}
		if fs.parent != nil {
			if cap, ok := fs.parent.captureSource(name); ok {
				fs.fn.Captures = append(fs.fn.Captures, cap)
				i := len(fs.fn.Captures) - 1
				fs.captIdx[name] = i
				return Capture{FromLocal: false, Index: i, Name: name}, true
			}
		}
	}
	return Capture{}, false
}

func (fs *fstate) aliasIdx(a string) int32 {
	if a == "" {
		return 0
	}
	return fs.name(a) + 1
}

func (fs *fstate) pushVar(name string) {
	r := fs.resolve(name)
	switch r.kind {
	case vLocal:
		fs.emit(PUSHLOCAL, int32(r.idx), 0)
	case vCapt:
		fs.emit(PUSHCAPT, int32(r.idx), 0)
	case vStatic:
		fs.emit(PUSHSTATIC, int32(r.idx), 0)
	case vField:
		fs.emit(PUSHFIELD, fs.name(name), fs.aliasIdx(r.alias))
	case vMemvar:
		fs.emit(PUSHMEMVAR, fs.name(name), 0)
	default:
		fs.emit(PUSHVAR, fs.name(name), 0)
	}
}

func (fs *fstate) popVar(name string) {
	r := fs.resolve(name)
	switch r.kind {
	case vLocal:
		fs.emit(POPLOCAL, int32(r.idx), 0)
	case vCapt:
		fs.emit(POPCAPT, int32(r.idx), 0)
	case vStatic:
		fs.emit(POPSTATIC, int32(r.idx), 0)
	case vField:
		fs.emit(POPFIELD, fs.name(name), fs.aliasIdx(r.alias))
	case vMemvar:
		fs.emit(POPMEMVAR, fs.name(name), 0)
	default:
		fs.emit(POPVAR, fs.name(name), 0)
	}
}

func (fs *fstate) pushRef(e ast.Expr) {
	switch x := e.(type) {
	case *ast.Ident:
		r := fs.resolve(x.Name)
		switch r.kind {
		case vLocal:
			fs.emit(PUSHLOCALREF, int32(r.idx), 0)
		case vCapt:
			fs.emit(PUSHCAPTREF, int32(r.idx), 0)
		case vStatic:
			fs.emit(PUSHSTATICREF, int32(r.idx), 0)
		case vMemvar:
			fs.emit(PUSHMEMVARREF, fs.name(x.Name), 0)
		case vField:
			fs.emit(PUSHFIELD, fs.name(x.Name), fs.aliasIdx(r.alias))
		default:
			fs.emit(PUSHVARREF, fs.name(x.Name), 0)
		}
	case *ast.MemvarRef:
		fs.emit(PUSHMEMVARREF, fs.name(x.Name), 0)
	default:
		fs.expr(e)
	}
}

// ---------- statements ----------

func (fs *fstate) stmts(list []ast.Stmt) {
	for _, s := range list {
		fs.stmt(s)
	}
}

func (fs *fstate) line(l int) {
	if l > 0 {
		fs.c.line = l
		fs.emit(LINE, int32(l), 0)
	}
}

func (fs *fstate) stmt(s ast.Stmt) {
	switch st := s.(type) {
	case *ast.NopStmt:
	case *ast.ExprStmt:
		fs.line(st.Line)
		if a, ok := st.X.(*ast.Assign); ok {
			fs.assign(a, false)
			return
		}
		if id, ok := st.X.(*ast.IncDec); ok {
			fs.incdec(id, false)
			return
		}
		if m, ok := st.X.(*ast.Macro); ok && m.X == nil {
			// a bare &macro statement executes a whole command (dBase)
			fs.emit(MACROTEXT, fs.konst(value.Str(m.Text)), 0)
			fs.emit(CALL, fs.name("__EXECSTMT"), 1)
			fs.emit(POP, 0, 0)
			return
		}
		fs.expr(st.X)
		fs.emit(POP, 0, 0)
	case *ast.LocalStmt:
		fs.line(st.Line)
		for _, v := range st.Vars {
			if st.Static {
				idx := fs.c.newStatic(v.Name)
				fs.statics[v.Name] = idx
				fs.c.staticInit(idx, v)
				continue
			}
			if fs.noLocals {
				fs.c.fail("LOCAL not allowed here")
			}
			// evaluate initializer before the name is visible
			if len(v.Dims) > 0 {
				for _, d := range v.Dims {
					fs.expr(d)
				}
				fs.emit(CALL, fs.name("ARRAY"), int32(len(v.Dims)))
			} else if v.Init != nil {
				fs.expr(v.Init)
			} else {
				fs.emit(PUSHNIL, 0, 0)
			}
			idx := fs.newLocal(v.Name)
			fs.emit(POPLOCAL, int32(idx), 0)
		}
	case *ast.MemvarStmt:
		fs.line(st.Line)
		for _, m := range st.Macro {
			fs.emit(MACROTEXT, fs.konst(value.Str(m.(*ast.Macro).Text)), 0)
			fs.emit(PRIVATEX, 0, 0)
		}
		for _, v := range st.Vars {
			if st.Public {
				fs.emit(PUBLIC, fs.name(v.Name), 0)
			} else {
				fs.emit(PRIVATE, fs.name(v.Name), 0)
			}
			fs.memvars[v.Name] = true
			if len(v.Dims) > 0 {
				for _, d := range v.Dims {
					fs.expr(d)
				}
				fs.emit(CALL, fs.name("ARRAY"), int32(len(v.Dims)))
				fs.emit(POPMEMVAR, fs.name(v.Name), 0)
			} else if v.Init != nil {
				fs.expr(v.Init)
				fs.emit(POPMEMVAR, fs.name(v.Name), 0)
			}
		}
	case *ast.ParamStmt:
		fs.line(st.Line)
		for i, n := range st.Names {
			fs.emit(PARAM, fs.name(n), int32(i))
			fs.memvars[n] = true
		}
	case *ast.DeclStmt:
		for _, n := range st.Names {
			if st.Field {
				fs.fields[n] = st.Alias
			} else {
				fs.memvars[n] = true
			}
		}
	case *ast.IfStmt:
		fs.line(st.Line)
		var ends []int
		for i, c := range st.Conds {
			fs.expr(c)
			jf := fs.emit(JF, 0, 0)
			fs.stmts(st.Bodies[i])
			ends = append(ends, fs.emit(JMP, 0, 0))
			fs.patch(jf)
		}
		fs.stmts(st.Else)
		for _, e := range ends {
			fs.patch(e)
		}
	case *ast.WhileStmt:
		fs.line(st.Line)
		top := fs.here()
		fs.expr(st.Cond)
		jf := fs.emit(JF, 0, 0)
		lc := fs.pushLoop(false)
		fs.stmts(st.Body)
		fs.emit(JMP, int32(top), 0)
		fs.patch(jf)
		fs.popLoop(lc, top)
	case *ast.ForStmt:
		fs.forStmt(st)
	case *ast.ForEachStmt:
		fs.forEach(st)
	case *ast.CaseStmt:
		fs.line(st.Line)
		var ends []int
		for i, c := range st.Conds {
			fs.expr(c)
			jf := fs.emit(JF, 0, 0)
			fs.stmts(st.Bodies[i])
			ends = append(ends, fs.emit(JMP, 0, 0))
			fs.patch(jf)
		}
		fs.stmts(st.Other)
		for _, e := range ends {
			fs.patch(e)
		}
	case *ast.SwitchStmt:
		fs.switchStmt(st)
	case *ast.SeqStmt:
		fs.seqStmt(st)
	case *ast.ReturnStmt:
		fs.line(st.Line)
		if st.X != nil {
			fs.expr(st.X)
			fs.emit(RET, 0, 0)
		} else {
			fs.emit(RETNIL, 0, 0)
		}
	case *ast.ExitStmt:
		fs.line(st.Line)
		lc := fs.innermost(true)
		if lc == nil {
			fs.c.fail("EXIT outside of a loop")
		}
		fs.seqEnds(lc.seqDepth)
		lc.exits = append(lc.exits, fs.emit(JMP, 0, 0))
	case *ast.LoopStmt:
		fs.line(st.Line)
		lc := fs.innermost(false)
		if lc == nil {
			fs.c.fail("LOOP outside of a loop")
		}
		fs.seqEnds(lc.seqDepth)
		lc.loops = append(lc.loops, fs.emit(JMP, 0, 0))
	case *ast.BreakStmt:
		fs.line(st.Line)
		if st.X != nil {
			fs.expr(st.X)
		} else {
			fs.emit(PUSHNIL, 0, 0)
		}
		fs.emit(CALL, fs.name("BREAK"), 1)
		fs.emit(POP, 0, 0)
	case *ast.DoStmt:
		fs.line(st.Line)
		if st.Macro != nil {
			if m, ok := st.Macro.(*ast.Macro); ok && m.X == nil {
				fs.emit(MACROTEXT, fs.konst(value.Str(m.Text)), 0)
			} else {
				fs.expr(st.Macro)
			}
		}
		for _, a := range st.Args {
			fs.arg(a)
		}
		if st.Macro != nil {
			fs.emit(DOX, 0, int32(len(st.Args)))
		} else {
			fs.emit(DO, fs.name(st.Name), int32(len(st.Args)))
		}
	default:
		fs.c.fail("unsupported statement %T", s)
	}
}

func (fs *fstate) seqEnds(target int) {
	for d := fs.seqDepth; d > target; d-- {
		fs.emit(SEQEND, 0, 0)
	}
}

func (fs *fstate) pushLoop(isSwitch bool) *loopCtx {
	lc := &loopCtx{seqDepth: fs.seqDepth, isSwitch: isSwitch}
	fs.loops = append(fs.loops, lc)
	return lc
}

// popLoop patches EXIT to here and LOOP to cont.
func (fs *fstate) popLoop(lc *loopCtx, cont int) {
	fs.loops = fs.loops[:len(fs.loops)-1]
	for _, e := range lc.exits {
		fs.fn.Code[e].A = int32(fs.here())
	}
	for _, l := range lc.loops {
		fs.fn.Code[l].A = int32(cont)
	}
}

func (fs *fstate) innermost(exit bool) *loopCtx {
	for i := len(fs.loops) - 1; i >= 0; i-- {
		if fs.loops[i].isSwitch && !exit {
			continue
		}
		return fs.loops[i]
	}
	return nil
}

func (fs *fstate) forStmt(st *ast.ForStmt) {
	fs.line(st.Line)
	fs.expr(st.From)
	fs.assignTo(st.Var)
	stepSlot := -1
	dir := 1
	if st.Step != nil {
		if l, ok := st.Step.(*ast.Lit); ok && l.V.K == value.NUMBER {
			if l.V.N < 0 {
				dir = -1
			}
		} else {
			dir = 0
			stepSlot = fs.hiddenLocal()
			fs.expr(st.Step)
			fs.emit(POPLOCAL, int32(stepSlot), 0)
		}
	}
	top := fs.here()
	fs.expr(st.Var)
	fs.expr(st.To)
	switch dir {
	case 1:
		fs.emit(LE, 0, 0)
	case -1:
		fs.emit(GE, 0, 0)
	default:
		fs.emit(PUSHLOCAL, int32(stepSlot), 0)
		fs.emit(FORTEST, 0, 0)
	}
	jf := fs.emit(JF, 0, 0)
	lc := fs.pushLoop(false)
	fs.stmts(st.Body)
	cont := fs.here()
	fs.expr(st.Var)
	if st.Step == nil {
		fs.emit(PUSHCONST, fs.konst(value.Int(1)), 0)
	} else if stepSlot >= 0 {
		fs.emit(PUSHLOCAL, int32(stepSlot), 0)
	} else {
		fs.expr(st.Step)
	}
	fs.emit(ADD, 0, 0)
	fs.assignTo(st.Var)
	fs.emit(JMP, int32(top), 0)
	fs.patch(jf)
	fs.popLoop(lc, cont)
}

func (fs *fstate) forEach(st *ast.ForEachStmt) {
	fs.line(st.Line)
	coll := fs.hiddenLocal()
	idx := fs.hiddenLocal()
	fs.expr(st.In)
	fs.emit(POPLOCAL, int32(coll), 0)
	if st.Desc {
		fs.emit(PUSHLOCAL, int32(coll), 0)
		fs.emit(CALL, fs.name("LEN"), 1)
		fs.emit(PUSHCONST, fs.konst(value.Int(1)), 0)
		fs.emit(ADD, 0, 0)
	} else {
		fs.emit(PUSHCONST, fs.konst(value.Int(0)), 0)
	}
	fs.emit(POPLOCAL, int32(idx), 0)
	top := fs.here()
	fs.emit(PUSHLOCAL, int32(idx), 0)
	fs.emit(PUSHCONST, fs.konst(value.Int(1)), 0)
	if st.Desc {
		fs.emit(SUB, 0, 0)
	} else {
		fs.emit(ADD, 0, 0)
	}
	fs.emit(POPLOCAL, int32(idx), 0)
	fs.emit(PUSHLOCAL, int32(idx), 0)
	if st.Desc {
		fs.emit(PUSHCONST, fs.konst(value.Int(1)), 0)
		fs.emit(GE, 0, 0)
	} else {
		fs.emit(PUSHLOCAL, int32(coll), 0)
		fs.emit(CALL, fs.name("LEN"), 1)
		fs.emit(LE, 0, 0)
	}
	jf := fs.emit(JF, 0, 0)
	fs.emit(PUSHLOCAL, int32(coll), 0)
	fs.emit(PUSHLOCAL, int32(idx), 0)
	fs.emit(FOREACHGET, 0, 0)
	fs.assignTo(st.Var)
	lc := fs.pushLoop(false)
	fs.stmts(st.Body)
	fs.emit(JMP, int32(top), 0)
	fs.patch(jf)
	fs.popLoop(lc, top)
}

func (fs *fstate) switchStmt(st *ast.SwitchStmt) {
	fs.line(st.Line)
	sw := fs.hiddenLocal()
	fs.expr(st.X)
	fs.emit(POPLOCAL, int32(sw), 0)
	jumps := make([]int, len(st.Vals))
	for i, v := range st.Vals {
		fs.emit(PUSHLOCAL, int32(sw), 0)
		fs.expr(v)
		fs.emit(EXACTEQ, 0, 0)
		jumps[i] = fs.emit(JT, 0, 0)
	}
	jOther := fs.emit(JMP, 0, 0)
	lc := fs.pushLoop(true)
	for i, b := range st.Bodies {
		fs.patch(jumps[i])
		fs.stmts(b)
	}
	fs.patch(jOther)
	fs.stmts(st.Other)
	fs.popLoop(lc, fs.here())
}

func (fs *fstate) seqStmt(st *ast.SeqStmt) {
	fs.line(st.Line)
	try := int32(0)
	if st.Try {
		try = 1
	}
	if st.With != nil {
		fs.expr(st.With)
		try = 2
	}
	begin := fs.emit(SEQBEGIN, 0, try)
	fs.seqDepth++
	fs.stmts(st.Body)
	fs.seqDepth--
	fs.emit(SEQEND, 0, 0)
	jEnd := fs.emit(JMP, 0, 0)
	fs.patch(begin)
	if st.RecoverVar != nil {
		fs.assignTo(st.RecoverVar)
	} else {
		fs.emit(POP, 0, 0)
	}
	fs.stmts(st.Recover)
	fs.patch(jEnd)
	fs.stmts(st.Always)
}

// ---------- expressions ----------

func (fs *fstate) arg(a ast.Expr) {
	switch x := a.(type) {
	case *ast.RefArg:
		fs.pushRef(x.X)
	case *ast.Empty:
		fs.emit(PUSHNIL, 0, 0)
	default:
		fs.expr(a)
	}
}

func (fs *fstate) expr(e ast.Expr) {
	switch x := e.(type) {
	case *ast.Lit:
		switch {
		case x.V.K == value.NIL:
			fs.emit(PUSHNIL, 0, 0)
		case x.V.K == value.LOGICAL && x.V.B:
			fs.emit(PUSHTRUE, 0, 0)
		case x.V.K == value.LOGICAL:
			fs.emit(PUSHFALSE, 0, 0)
		default:
			fs.emit(PUSHCONST, fs.konst(x.V), 0)
		}
	case *ast.Empty:
		fs.emit(PUSHNIL, 0, 0)
	case *ast.Ident:
		fs.pushVar(x.Name)
	case *ast.MemvarRef:
		fs.emit(PUSHMEMVAR, fs.name(x.Name), 0)
	case *ast.FieldRef:
		if strings.HasPrefix(x.Name, "&") {
			fs.fieldMacro(x, nil)
			return
		}
		if x.AliasExpr != nil {
			fs.expr(x.AliasExpr)
			fs.emit(PUSHFIELDX, fs.name(x.Name), 0)
		} else {
			fs.emit(PUSHFIELD, fs.name(x.Name), fs.aliasIdx(x.Alias))
		}
	case *ast.AliasExpr:
		if x.AliasExpr != nil {
			fs.expr(x.AliasExpr)
			fs.emit(PUSHALIASX, 0, 0)
		} else {
			fs.emit(PUSHALIAS, fs.name(x.Alias), 0)
		}
		fs.expr(x.X)
		fs.emit(POPALIAS, 0, 0)
	case *ast.Macro:
		if x.X != nil {
			fs.expr(x.X)
			fs.emit(MACROPUSHX, 0, 0)
		} else {
			fs.emit(MACROPUSH, fs.konst(value.Str(x.Text)), 0)
		}
	case *ast.Call:
		if x.Name == "__EXPRLIST" {
			for i, a := range x.Args {
				fs.expr(a)
				if i < len(x.Args)-1 {
					fs.emit(POP, 0, 0)
				}
			}
			return
		}
		for _, a := range x.Args {
			fs.arg(a)
		}
		fs.emit(CALL, fs.name(x.Name), int32(len(x.Args)))
	case *ast.CallMacro:
		if m, ok := x.Callee.(*ast.Macro); ok && m.X == nil {
			fs.emit(MACROTEXT, fs.konst(value.Str(m.Text)), 0)
		} else if ok {
			fs.expr(m.X)
		}
		for _, a := range x.Args {
			fs.arg(a)
		}
		fs.emit(CALLX, 0, int32(len(x.Args)))
	case *ast.Index:
		fs.expr(x.X)
		fs.expr(x.Idx)
		fs.emit(ARRAYGET, 0, 0)
	case *ast.Send:
		fs.expr(x.X)
		for _, a := range x.Args {
			fs.arg(a)
		}
		fs.emit(SEND, fs.name(x.Msg), int32(len(x.Args)))
	case *ast.Unary:
		fs.expr(x.X)
		if x.Op == "-" {
			fs.emit(NEG, 0, 0)
		} else {
			fs.emit(NOT, 0, 0)
		}
	case *ast.Binary:
		switch x.Op {
		case ".AND.":
			fs.expr(x.L)
			fs.emit(TOLOGIC, 0, 0)
			j := fs.emit(JFK, 0, 0)
			fs.expr(x.R)
			fs.emit(TOLOGIC, 0, 0)
			fs.patch(j)
			return
		case ".OR.":
			fs.expr(x.L)
			fs.emit(TOLOGIC, 0, 0)
			j := fs.emit(JTK, 0, 0)
			fs.expr(x.R)
			fs.emit(TOLOGIC, 0, 0)
			fs.patch(j)
			return
		}
		fs.expr(x.L)
		fs.expr(x.R)
		fs.emit(binOp(x.Op), 0, 0)
	case *ast.Assign:
		fs.assign(x, true)
	case *ast.IncDec:
		fs.incdec(x, true)
	case *ast.ArrayLit:
		for _, it := range x.Items {
			fs.arg(it)
		}
		fs.emit(MKARRAY, int32(len(x.Items)), 0)
	case *ast.HashLit:
		for i := range x.Keys {
			fs.expr(x.Keys[i])
			fs.expr(x.Vals[i])
		}
		fs.emit(MKHASH, int32(len(x.Keys)), 0)
	case *ast.Block:
		fs.block(x)
	case *ast.IIf:
		fs.expr(x.C)
		jf := fs.emit(JF, 0, 0)
		fs.expr(x.T)
		j := fs.emit(JMP, 0, 0)
		fs.patch(jf)
		fs.expr(x.F)
		fs.patch(j)
	case *ast.RefArg:
		fs.pushRef(x.X)
	default:
		fs.c.fail("unsupported expression %T", e)
	}
}

func binOp(op string) Op {
	switch op {
	case "+":
		return ADD
	case "-":
		return SUB
	case "*":
		return MUL
	case "/":
		return DIV
	case "%":
		return MOD
	case "**":
		return POW
	case "=":
		return EQ
	case "==":
		return EXACTEQ
	case "!=":
		return NE
	case "<":
		return LT
	case "<=":
		return LE
	case ">":
		return GT
	case ">=":
		return GE
	case "$":
		return INSTR
	}
	panic("bad op " + op)
}

func (fs *fstate) fieldMacro(x *ast.FieldRef, assignVal func()) {
	// FIELD->&name: use FieldGet/FieldPut by name
	nameExpr := &ast.Call{Name: "__MACROSTR", Args: []ast.Expr{&ast.Lit{V: value.Str(x.Name[1:])}}}
	if assignVal == nil {
		fs.expr(nameExpr)
		fs.emit(CALL, fs.name("__FIELDGETNAME"), 1)
		return
	}
	fs.expr(nameExpr)
	assignVal()
	fs.emit(CALL, fs.name("__FIELDPUTNAME"), 2)
}

func (fs *fstate) block(b *ast.Block) {
	fn := &Func{Name: "(block)", IsBlock: true, Src: b.Src, File: fs.c.file, Line: fs.c.line, Module: fs.c.mod, NParams: len(b.Params)}
	bs := newFstate(fs.c, fn, fs)
	bs.noLocals = fs.noLocals
	for _, p := range b.Params {
		bs.newLocal(p)
	}
	for i, e := range b.Body {
		bs.expr(e)
		if i < len(b.Body)-1 {
			bs.emit(POP, 0, 0)
		}
	}
	bs.emit(RET, 0, 0)
	bs.finish()
	fs.fn.Blocks = append(fs.fn.Blocks, fn)
	fs.emit(MKBLOCK, int32(len(fs.fn.Blocks)-1), 0)
}

// assignTo stores the top of stack into target (consuming it).
func (fs *fstate) assignTo(t ast.Expr) {
	switch x := t.(type) {
	case *ast.Ident:
		fs.popVar(x.Name)
	case *ast.MemvarRef:
		fs.emit(POPMEMVAR, fs.name(x.Name), 0)
	case *ast.FieldRef:
		if strings.HasPrefix(x.Name, "&") {
			tmp := fs.tempSlot()
			fs.emitStoreTemp(tmp)
			fs.fieldMacro(x, func() { fs.emitLoadTemp(tmp) })
			fs.emit(POP, 0, 0)
			return
		}
		if x.AliasExpr != nil {
			fs.expr(x.AliasExpr)
			fs.emit(POPFIELDX, fs.name(x.Name), 0)
		} else {
			fs.emit(POPFIELD, fs.name(x.Name), fs.aliasIdx(x.Alias))
		}
	case *ast.Macro:
		if x.X != nil {
			fs.expr(x.X)
			fs.emit(MACROPOPX, 0, 0)
		} else {
			fs.emit(MACROPOP, fs.konst(value.Str(x.Text)), 0)
		}
	case *ast.Index:
		tmp := fs.tempSlot()
		fs.emitStoreTemp(tmp)
		fs.expr(x.X)
		fs.expr(x.Idx)
		fs.emitLoadTemp(tmp)
		fs.emit(ARRAYSET, 0, 0)
		fs.emit(POP, 0, 0)
	case *ast.Send:
		tmp := fs.tempSlot()
		fs.emitStoreTemp(tmp)
		fs.expr(x.X)
		fs.emitLoadTemp(tmp)
		fs.emit(SENDSET, fs.name(x.Msg), 0)
		fs.emit(POP, 0, 0)
	default:
		fs.c.fail("invalid assignment target")
	}
}

// temp storage: locals normally, a hidden memvar in macro context

func (fs *fstate) tempSlot() int {
	return fs.hiddenLocal()
}
func (fs *fstate) emitStoreTemp(slot int) {
	if slot < 0 {
		fs.emit(POPMEMVAR, fs.name("__TMP"), 0)
		return
	}
	fs.emit(POPLOCAL, int32(slot), 0)
}
func (fs *fstate) emitLoadTemp(slot int) {
	if slot < 0 {
		fs.emit(PUSHMEMVAR, fs.name("__TMP"), 0)
		return
	}
	fs.emit(PUSHLOCAL, int32(slot), 0)
}

func (fs *fstate) assign(a *ast.Assign, keep bool) {
	if a.Op == ":=" {
		switch t := a.Target.(type) {
		case *ast.Index:
			fs.expr(t.X)
			fs.expr(t.Idx)
			fs.expr(a.X)
			fs.emit(ARRAYSET, 0, 0)
			if !keep {
				fs.emit(POP, 0, 0)
			}
			return
		case *ast.Send:
			fs.expr(t.X)
			fs.expr(a.X)
			fs.emit(SENDSET, fs.name(t.Msg), 0)
			if !keep {
				fs.emit(POP, 0, 0)
			}
			return
		}
		fs.expr(a.X)
		if keep {
			fs.emit(DUP, 0, 0)
		}
		fs.assignTo(a.Target)
		return
	}
	op := binOp(strings.TrimSuffix(a.Op, "="))
	if t, ok := a.Target.(*ast.Index); ok {
		fs.expr(t.X)
		fs.expr(t.Idx)
		fs.emit(DUP2, 0, 0)
		fs.emit(ARRAYGET, 0, 0)
		fs.expr(a.X)
		fs.emit(op, 0, 0)
		fs.emit(ARRAYSET, 0, 0)
		if !keep {
			fs.emit(POP, 0, 0)
		}
		return
	}
	fs.expr(a.Target)
	fs.expr(a.X)
	fs.emit(op, 0, 0)
	if keep {
		fs.emit(DUP, 0, 0)
	}
	fs.assignTo(a.Target)
}

func (fs *fstate) incdec(x *ast.IncDec, keep bool) {
	op := "+="
	if x.Op == "--" {
		op = "-="
	}
	fs.assign(&ast.Assign{Op: op, Target: x.Target, X: &ast.Lit{V: value.Int(1)}}, keep)
	if keep && !x.Prefix {
		// postfix: value before the change
		fs.emit(PUSHCONST, fs.konst(value.Int(1)), 0)
		if x.Op == "++" {
			fs.emit(SUB, 0, 0)
		} else {
			fs.emit(ADD, 0, 0)
		}
	}
}
